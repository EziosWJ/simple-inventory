//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
)

func TestSQLiteSaleReturnPostingPostgresSQLiteHTTPContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sale-return-posting.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	deps := sqliteDependencies(t, db, t.TempDir())
	router, e := app.Build(testAPIConfig(), db, deps)
	if e != nil {
		t.Fatal(e)
	}
	assertSaleReturnPostingContract(t, router, db, "sqlite")
}
func TestPostgresSaleReturnPostingPostgresSQLiteHTTPContract(t *testing.T) {
	if _, e := exec.LookPath("docker"); e != nil {
		t.Skip("Docker is required for PostgreSQL integration tests")
	}
	pg := startPostgres(t)
	runMigrations(t, projectRoot(t), pg.dsn)
	db := openTemporaryDatabase(t, pg.dsn)
	defer db.Close()
	deps := testDependencies(t, db, t.TempDir())
	router, e := app.Build(testAPIConfig(), db, deps)
	if e != nil {
		t.Fatal(e)
	}
	assertSaleReturnPostingContract(t, router, db, "postgres")
}

func assertSaleReturnPostingContract(t *testing.T, router http.Handler, db *platformdatabase.Database, dialect string) {
	t.Helper()
	token := loginAdmin(t, router)
	partner := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/partners", fmt.Sprintf(`{"code":"RS-%s","name":"销售退货客户","type":"COMPANY","isCustomer":true}`, dialect), token), 200).ID
	product := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/products", fmt.Sprintf(`{"code":"RSG-%s","name":"尾差退货商品","type":"GOODS","unit":"台"}`, dialect), token), 200).ID
	// Stock the goods so the sale can leave, then post one sale with two
	// differently priced lines of the same product.
	opening := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/inventory/adjustments", fmt.Sprintf(`{"items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"10","reason":"OPENING"}]}`, product), token), 200)
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/inventory/adjustments/%d/post", opening.ID), `{"version":1}`, token); r.Code != 200 {
		t.Fatalf("post opening adjustment: %d %s", r.Code, r.Body.String())
	}
	sale := inventoryData[struct {
		ID      int64 `json:"id"`
		Version int64 `json:"version"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/sales", fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"1","unitPrice":"0.01"},{"productId":%d,"productType":"GOODS","unit":"台","quantity":"1","unitPrice":"0.02"}]}`, partner, product, product), token), 200)
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sales/%d/post", sale.ID), `{"version":1}`, token); r.Code != 200 {
		t.Fatalf("post sale: %d %s", r.Code, r.Body.String())
	}
	var stockAfterSale, receivableAfterSale int64
	if e := db.GORM.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", product).Scan(&stockAfterSale).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GORM.Table("partner_balance").Select("amount_cents").Where("partner_id=? AND direction='CUSTOMER'", partner).Scan(&receivableAfterSale).Error; e != nil {
		t.Fatal(e)
	}
	source := inventoryData[struct {
		Items []struct {
			SaleItemID int64  `json:"saleItemId"`
			UnitPrice  string `json:"unitPrice"`
		} `json:"items"`
	}](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/sale-returns/source/%d", sale.ID), "", token), 200)
	if len(source.Items) != 2 || source.Items[0].UnitPrice == source.Items[1].UnitPrice {
		t.Fatalf("different prices not preserved: %+v", source.Items)
	}
	lineID, otherLineID := source.Items[0].SaleItemID, source.Items[1].SaleItemID
	newReturnFor := func(saleID, itemID int64, qty string) (int64, int64) {
		t.Helper()
		v := inventoryData[struct {
			ID      int64 `json:"id"`
			Version int64 `json:"version"`
		}](t, serveJSON(router, http.MethodPost, "/api/v1/sale-returns", fmt.Sprintf(`{"saleId":%d,"businessDate":"2026-10-01","items":[{"saleItemId":%d,"quantity":%q}]}`, saleID, itemID, qty), token), 200)
		return v.ID, v.Version
	}
	newReturn := func(itemID int64, qty string) (int64, int64) {
		t.Helper()
		return newReturnFor(sale.ID, itemID, qty)
	}
	first, _ := newReturn(lineID, "0.333")
	second, _ := newReturn(lineID, "0.333")
	third, _ := newReturn(lineID, "0.334")
	different, _ := newReturn(otherLineID, "0.333")
	// Audit failure rolls back stock, receivable, ledger rows, amount and status.
	installSaleReturnPostAuditFailure(t, db, dialect)
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sale-returns/%d/post", first), `{"version":1}`, token); r.Code != 500 {
		t.Fatalf("forced post audit failure=%d %s", r.Code, r.Body.String())
	}
	var state string
	db.GORM.Table("sale_return_document").Select("status").Where("id=?", first).Scan(&state)
	if state != "DRAFT" {
		t.Fatalf("audit rollback status=%s", state)
	}
	var rows int64
	db.GORM.Table("inventory_entry").Where("sale_return_id=?", first).Count(&rows)
	if rows != 0 {
		t.Fatalf("audit rollback inventory rows=%d", rows)
	}
	dropSaleReturnPostAuditFailure(t, db, dialect)
	post := func(id int64, wantAmount string) {
		t.Helper()
		r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sale-returns/%d/post", id), `{"version":1}`, token)
		v := inventoryData[struct {
			Status string `json:"status"`
			Items  []struct {
				Amount string `json:"amount"`
			} `json:"items"`
		}](t, r, 200)
		if v.Status != "POSTED" || v.Items[0].Amount != wantAmount {
			t.Fatalf("post id=%d amount/status=%+v want=%s", id, v, wantAmount)
		}
	}
	// Posting the return restores stock and reduces the customer receivable.
	post(first, "0.00")
	var stockAfterFirst, receivableAfterFirst int64
	if e := db.GORM.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", product).Scan(&stockAfterFirst).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GORM.Table("partner_balance").Select("amount_cents").Where("partner_id=? AND direction='CUSTOMER'", partner).Scan(&receivableAfterFirst).Error; e != nil {
		t.Fatal(e)
	}
	if stockAfterFirst != stockAfterSale+333 {
		t.Fatalf("sale return did not add stock back: %d want %d", stockAfterFirst, stockAfterSale+333)
	}
	ledger := inventoryData[struct {
		Records []struct {
			SaleReturnID     *int64 `json:"saleReturnId"`
			SaleReturnItemID *int64 `json:"saleReturnItemId"`
			SourceType       string `json:"sourceType"`
			Reason           string `json:"reason"`
			Quantity         string `json:"quantity"`
		} `json:"records"`
	}](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/inventory/entries?page=1&pageSize=100&productId=%d", product), "", token), 200)
	foundReturnSource := false
	for _, entry := range ledger.Records {
		if entry.SaleReturnID != nil && *entry.SaleReturnID == first {
			foundReturnSource = entry.SourceType == "SALE_RETURN" && entry.Reason == "SALE_RETURN" && entry.SaleReturnItemID != nil && entry.Quantity == "0.333"
		}
	}
	if !foundReturnSource {
		t.Fatalf("inventory ledger did not expose sale-return source: %+v", ledger.Records)
	}
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sales/%d/cancel", sale.ID), `{"version":2,"reason":"原销售录错"}`, token); r.Code != 409 {
		t.Fatalf("posted return must block origin cancel: %d %s", r.Code, r.Body.String())
	}
	// The rounded remainder lands once the line is fully returned.
	post(second, "0.01")
	post(third, "0.00")
	post(different, "0.01")
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sale-returns/%d/cancel", first), `{"version":2,"reason":"逆序验证"}`, token); r.Code != 409 {
		t.Fatalf("older return cancel status=%d %s", r.Code, r.Body.String())
	}
	installSaleReturnCancelAuditFailure(t, db, dialect)
	var beforeStock, beforeReceivable int64
	_ = db.GORM.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", product).Scan(&beforeStock).Error
	_ = db.GORM.Table("partner_balance").Select("amount_cents").Where("partner_id=? AND direction='CUSTOMER'", partner).Scan(&beforeReceivable).Error
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sale-returns/%d/cancel", third), `{"version":2,"reason":"审计失败回滚"}`, token); r.Code != 500 {
		t.Fatalf("forced cancel audit failure=%d %s", r.Code, r.Body.String())
	}
	var rolledState string
	_ = db.GORM.Table("sale_return_document").Select("status").Where("id=?", third).Scan(&rolledState).Error
	var afterStock, afterReceivable int64
	_ = db.GORM.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", product).Scan(&afterStock).Error
	_ = db.GORM.Table("partner_balance").Select("amount_cents").Where("partner_id=? AND direction='CUSTOMER'", partner).Scan(&afterReceivable).Error
	if rolledState != "POSTED" || beforeStock != afterStock || beforeReceivable != afterReceivable {
		t.Fatalf("cancel audit rollback status=%s stock=%d/%d receivable=%d/%d", rolledState, beforeStock, afterStock, beforeReceivable, afterReceivable)
	}
	dropSaleReturnCancelAuditFailure(t, db, dialect)
	// Cancellation only accepts the newest still effective return per origin line.
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sale-returns/%d/cancel", third), `{"version":2,"reason":"冲销末笔"}`, token); r.Code != 200 {
		t.Fatalf("cancel latest return=%d %s", r.Code, r.Body.String())
	}
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sale-returns/%d/cancel", second), `{"version":2,"reason":"冲销次笔"}`, token); r.Code != 200 {
		t.Fatalf("cancel second return=%d %s", r.Code, r.Body.String())
	}
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sale-returns/%d/cancel", different), `{"version":2,"reason":"冲销异价行"}`, token); r.Code != 200 {
		t.Fatalf("cancel other price line=%d %s", r.Code, r.Body.String())
	}
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sale-returns/%d/cancel", first), `{"version":2,"reason":"冲销首笔"}`, token); r.Code != 200 {
		t.Fatalf("cancel first after reverse order=%d %s", r.Code, r.Body.String())
	}
	var receivedStock, receivedReceivable int64
	_ = db.GORM.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", product).Scan(&receivedStock).Error
	_ = db.GORM.Table("partner_balance").Select("amount_cents").Where("partner_id=? AND direction='CUSTOMER'", partner).Scan(&receivedReceivable).Error
	if receivedStock != stockAfterSale || receivedReceivable != receivableAfterSale {
		t.Fatalf("cancelling all returns must restore the posted sale: stock=%d/%d receivable=%d/%d", receivedStock, stockAfterSale, receivedReceivable, receivableAfterSale)
	}
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sales/%d/cancel", sale.ID), `{"version":2,"reason":"原销售录错"}`, token); r.Code != 200 {
		t.Fatalf("origin cancel after returns cleared=%d %s", r.Code, r.Body.String())
	}
	// A return without enough stock to reverse must be rejected whole.
	product2 := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/products", fmt.Sprintf(`{"code":"RSC-%s","name":"取消负库存商品","type":"GOODS","unit":"个"}`, dialect), token), 200).ID
	opening2 := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/inventory/adjustments", fmt.Sprintf(`{"items":[{"productId":%d,"productType":"GOODS","unit":"个","quantity":"2","reason":"OPENING"}]}`, product2), token), 200)
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/inventory/adjustments/%d/post", opening2.ID), `{"version":1}`, token); r.Code != 200 {
		t.Fatalf("post second opening: %d %s", r.Code, r.Body.String())
	}
	sale2 := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/sales", fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"个","quantity":"1","unitPrice":"1.00"}]}`, partner, product2), token), 200).ID
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sales/%d/post", sale2), `{"version":1}`, token); r.Code != 200 {
		t.Fatalf("post second sale: %d %s", r.Code, r.Body.String())
	}
	src2 := inventoryData[struct {
		Items []struct {
			SaleItemID int64 `json:"saleItemId"`
		} `json:"items"`
	}](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/sale-returns/source/%d", sale2), "", token), 200)
	single, _ := newReturnFor(sale2, src2.Items[0].SaleItemID, "1")
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sale-returns/%d/post", single), `{"version":1}`, token); r.Code != 200 {
		t.Fatalf("post single return=%d %s", r.Code, r.Body.String())
	}
	if e := db.GORM.Table("inventory_balance").Where("product_id=?", product2).Update("quantity_milli", int64(0)).Error; e != nil {
		t.Fatal(e)
	}
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sale-returns/%d/cancel", single), `{"version":2,"reason":"库存不足"}`, token); r.Code != 409 {
		t.Fatalf("cancel with insufficient stock status=%d %s", r.Code, r.Body.String())
	}
	var stillPosted string
	_ = db.GORM.Table("sale_return_document").Select("status").Where("id=?", single).Scan(&stillPosted).Error
	if stillPosted != "POSTED" {
		t.Fatalf("rejected cancel must keep POSTED, got %s", stillPosted)
	}
	// Two drafts cannot concurrently consume the same origin-line quota.
	restock := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/inventory/adjustments", fmt.Sprintf(`{"items":[{"productId":%d,"productType":"GOODS","unit":"个","quantity":"3","reason":"SURPLUS"}]}`, product2), token), 200)
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/inventory/adjustments/%d/post", restock.ID), `{"version":1}`, token); r.Code != 200 {
		t.Fatalf("restock for concurrency: %d %s", r.Code, r.Body.String())
	}
	sale3 := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/sales", fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"个","quantity":"1","unitPrice":"1.00"}]}`, partner, product2), token), 200).ID
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sales/%d/post", sale3), `{"version":1}`, token); r.Code != 200 {
		t.Fatalf("post concurrency source: %d %s", r.Code, r.Body.String())
	}
	src3 := inventoryData[struct {
		Items []struct {
			SaleItemID int64 `json:"saleItemId"`
		} `json:"items"`
	}](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/sale-returns/source/%d", sale3), "", token), 200)
	ids := make([]int64, 2)
	for i := range ids {
		ids[i] = newConcurrentSaleReturnDraft(t, router, token, sale3, src3.Items[0].SaleItemID)
	}
	statuses := make(chan int, 2)
	start := make(chan struct{})
	for _, rid := range ids {
		go func(id int64) {
			<-start
			r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sale-returns/%d/post", id), `{"version":1}`, token)
			statuses <- r.Code
		}(rid)
	}
	close(start)
	s1, s2 := <-statuses, <-statuses
	if !((s1 == 200 && s2 == 409) || (s1 == 409 && s2 == 200)) {
		t.Fatalf("concurrent quota posts statuses=%d,%d", s1, s2)
	}
	var returnedQty int64
	if e := db.GORM.Table("sale_return_document_item i").Select("COALESCE(SUM(i.quantity_milli),0)").Joins("JOIN sale_return_document d ON d.id=i.document_id").Where("i.sale_item_id=? AND d.status='POSTED'", src3.Items[0].SaleItemID).Scan(&returnedQty).Error; e != nil || returnedQty > 1000 {
		t.Fatalf("concurrent returned quantity=%d err=%v", returnedQty, e)
	}
	// Stock and the customer balance must each equal the immutable ledger sum.
	var currentStock int64
	if e := db.GORM.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", product).Scan(&currentStock).Error; e != nil {
		t.Fatal(e)
	}
	var stockSum int64
	if e := db.GORM.Table("inventory_entry").Select("COALESCE(SUM(quantity_milli),0)").Where("product_id=?", product).Scan(&stockSum).Error; e != nil {
		t.Fatal(e)
	}
	if stockSum != currentStock {
		t.Fatalf("stock does not equal immutable ledger: ledger=%d stock=%d", stockSum, currentStock)
	}
	var ledgerTotal, balanceTotal int64
	if e := db.GORM.Table("partner_balance_entry").Select("COALESCE(SUM(amount_cents),0)").Where("partner_id=? AND direction='CUSTOMER'", partner).Scan(&ledgerTotal).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GORM.Table("partner_balance").Select("amount_cents").Where("partner_id=? AND direction='CUSTOMER'", partner).Scan(&balanceTotal).Error; e != nil {
		t.Fatal(e)
	}
	if ledgerTotal != balanceTotal {
		t.Fatalf("customer balance does not equal immutable ledger: ledger=%d balance=%d", ledgerTotal, balanceTotal)
	}
}

func newConcurrentSaleReturnDraft(t *testing.T, router http.Handler, token string, saleID, itemID int64) int64 {
	t.Helper()
	return inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/sale-returns", fmt.Sprintf(`{"saleId":%d,"businessDate":"2026-10-01","items":[{"saleItemId":%d,"quantity":"0.6"}]}`, saleID, itemID), token), 200).ID
}

func installSaleReturnPostAuditFailure(t *testing.T, db *platformdatabase.Database, dialect string) {
	t.Helper()
	if dialect == "sqlite" {
		if e := db.GORM.Exec(`CREATE TRIGGER sale_return_post_audit_failure BEFORE INSERT ON sys_oper_log WHEN NEW.operation_type='sale_return.post' BEGIN SELECT RAISE(ABORT,'audit unavailable'); END`).Error; e != nil {
			t.Fatal(e)
		}
		return
	}
	if e := db.GORM.Exec(`CREATE FUNCTION reject_sale_return_post_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.operation_type='sale_return.post' THEN RAISE EXCEPTION 'audit unavailable'; END IF; RETURN NEW; END $$`).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GORM.Exec(`CREATE TRIGGER sale_return_post_audit_failure BEFORE INSERT ON sys_oper_log FOR EACH ROW EXECUTE FUNCTION reject_sale_return_post_audit()`).Error; e != nil {
		t.Fatal(e)
	}
}
func dropSaleReturnPostAuditFailure(t *testing.T, db *platformdatabase.Database, dialect string) {
	t.Helper()
	if dialect == "sqlite" {
		if e := db.GORM.Exec(`DROP TRIGGER sale_return_post_audit_failure`).Error; e != nil {
			t.Fatal(e)
		}
		return
	}
	if e := db.GORM.Exec(`DROP TRIGGER sale_return_post_audit_failure ON sys_oper_log`).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GORM.Exec(`DROP FUNCTION reject_sale_return_post_audit()`).Error; e != nil {
		t.Fatal(e)
	}
}

func installSaleReturnCancelAuditFailure(t *testing.T, db *platformdatabase.Database, dialect string) {
	t.Helper()
	if dialect == "sqlite" {
		if e := db.GORM.Exec(`CREATE TRIGGER sale_return_cancel_audit_failure BEFORE INSERT ON sys_oper_log WHEN NEW.operation_type='sale_return.cancel' BEGIN SELECT RAISE(ABORT,'audit unavailable'); END`).Error; e != nil {
			t.Fatal(e)
		}
		return
	}
	if e := db.GORM.Exec(`CREATE FUNCTION reject_sale_return_cancel_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.operation_type='sale_return.cancel' THEN RAISE EXCEPTION 'audit unavailable'; END IF; RETURN NEW; END $$`).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GORM.Exec(`CREATE TRIGGER sale_return_cancel_audit_failure BEFORE INSERT ON sys_oper_log FOR EACH ROW EXECUTE FUNCTION reject_sale_return_cancel_audit()`).Error; e != nil {
		t.Fatal(e)
	}
}
func dropSaleReturnCancelAuditFailure(t *testing.T, db *platformdatabase.Database, dialect string) {
	t.Helper()
	if dialect == "sqlite" {
		if e := db.GORM.Exec(`DROP TRIGGER sale_return_cancel_audit_failure`).Error; e != nil {
			t.Fatal(e)
		}
		return
	}
	if e := db.GORM.Exec(`DROP TRIGGER sale_return_cancel_audit_failure ON sys_oper_log`).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GORM.Exec(`DROP FUNCTION reject_sale_return_cancel_audit()`).Error; e != nil {
		t.Fatal(e)
	}
}
