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

func TestSQLitePurchaseReturnPostingPostgresSQLiteHTTPContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "purchase-return-posting.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	deps := sqliteDependencies(t, db, t.TempDir())
	router, e := app.Build(testAPIConfig(), db, deps)
	if e != nil {
		t.Fatal(e)
	}
	assertPurchaseReturnPostingContract(t, router, db, "sqlite")
}
func TestPostgresPurchaseReturnPostingPostgresSQLiteHTTPContract(t *testing.T) {
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
	assertPurchaseReturnPostingContract(t, router, db, "postgres")
}

func assertPurchaseReturnPostingContract(t *testing.T, router http.Handler, db *platformdatabase.Database, dialect string) {
	t.Helper()
	token := loginAdmin(t, router)
	partner := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/partners", fmt.Sprintf(`{"code":"RP-%s","name":"采购退货供应商","type":"COMPANY","isSupplier":true}`, dialect), token), 200).ID
	product := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/products", fmt.Sprintf(`{"code":"RPG-%s","name":"尾差商品","type":"GOODS","unit":"台"}`, dialect), token), 200).ID
	purchase := inventoryData[struct {
		ID      int64 `json:"id"`
		Version int64 `json:"version"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/purchases", fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"1","unitPrice":"0.01"},{"productId":%d,"productType":"GOODS","unit":"台","quantity":"1","unitPrice":"0.02"}]}`, partner, product, product), token), 200)
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/purchases/%d/post", purchase.ID), `{"version":1}`, token); r.Code != 200 {
		t.Fatalf("post purchase: %d %s", r.Code, r.Body.String())
	}
	type sourceResponse struct {
		Items []struct {
			PurchaseItemID int64  `json:"purchaseItemId"`
			UnitPrice      string `json:"unitPrice"`
		} `json:"items"`
	}
	source := inventoryData[sourceResponse](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/purchase-returns/source/%d", purchase.ID), "", token), 200)
	if len(source.Items) != 2 || source.Items[0].UnitPrice == source.Items[1].UnitPrice {
		t.Fatalf("different prices not preserved: %+v", source.Items)
	}
	lineID, otherLineID := source.Items[0].PurchaseItemID, source.Items[1].PurchaseItemID
	newReturn := func(itemID int64, qty string) (int64, int64) {
		t.Helper()
		v := inventoryData[struct {
			ID      int64 `json:"id"`
			Version int64 `json:"version"`
		}](t, serveJSON(router, http.MethodPost, "/api/v1/purchase-returns", fmt.Sprintf(`{"purchaseId":%d,"businessDate":"2026-10-01","items":[{"purchaseItemId":%d,"quantity":%q}]}`, purchase.ID, itemID, qty), token), 200)
		return v.ID, v.Version
	}
	first, _ := newReturn(lineID, "0.333")
	second, _ := newReturn(lineID, "0.333")
	third, _ := newReturn(lineID, "0.334")
	different, _ := newReturn(otherLineID, "0.333")
	// A draft from another purchase cannot claim a stable item ID from this source.
	other := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/purchases", fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"1","unitPrice":"1.00"}]}`, partner, product), token), 200).ID
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/purchases/%d/post", other), `{"version":1}`, token); r.Code != 200 {
		t.Fatalf("post other purchase: %d %s", r.Code, r.Body.String())
	}
	if r := serveJSON(router, http.MethodPost, "/api/v1/purchase-returns", fmt.Sprintf(`{"purchaseId":%d,"businessDate":"2026-10-01","items":[{"purchaseItemId":%d,"quantity":"0.1"}]}`, other, lineID), token); r.Code != 400 {
		t.Fatalf("cross-origin item status=%d body=%s", r.Code, r.Body.String())
	}
	// Audit failure rolls back stock, payables, ledger rows, amount, and status.
	installPurchaseReturnPostAuditFailure(t, db, dialect)
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/purchase-returns/%d/post", first), `{"version":1}`, token); r.Code != 500 {
		t.Fatalf("forced post audit failure=%d %s", r.Code, r.Body.String())
	}
	var state string
	db.GORM.Table("purchase_return_document").Select("status").Where("id=?", first).Scan(&state)
	if state != "DRAFT" {
		t.Fatalf("audit rollback status=%s", state)
	}
	var rows int64
	db.GORM.Table("inventory_entry").Where("purchase_return_id=?", first).Count(&rows)
	if rows != 0 {
		t.Fatalf("audit rollback inventory rows=%d", rows)
	}
	dropPurchaseReturnPostAuditFailure(t, db, dialect)
	post := func(id int64, wantAmount string) {
		t.Helper()
		r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/purchase-returns/%d/post", id), `{"version":1}`, token)
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
	post(first, "0.00")
	ledger:=inventoryData[struct{Records []struct{PurchaseReturnID *int64 `json:"purchaseReturnId"`;PurchaseReturnItemID *int64 `json:"purchaseReturnItemId"`;SourceType string `json:"sourceType"`;Reason string `json:"reason"`} `json:"records"`}](t,serveJSON(router,http.MethodGet,fmt.Sprintf("/api/v1/inventory/entries?page=1&pageSize=100&productId=%d",product),"",token),200)
	foundReturnSource:=false;for _,entry:=range ledger.Records{if entry.PurchaseReturnID!=nil&&*entry.PurchaseReturnID==first{foundReturnSource=entry.SourceType=="PURCHASE_RETURN"&&entry.Reason=="PURCHASE_RETURN"&&entry.PurchaseReturnItemID!=nil}}
	if !foundReturnSource{t.Fatalf("inventory ledger did not expose purchase-return source: %+v",ledger.Records)}
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/purchases/%d/cancel", purchase.ID), `{"version":2,"reason":"原采购录错"}`, token); r.Code != 409 {
		t.Fatalf("posted return must block origin cancel: %d %s", r.Code, r.Body.String())
	}
	post(second, "0.01")
	post(third, "0.00")
	post(different, "0.01")
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/purchase-returns/%d/cancel", first), `{"version":2,"reason":"逆序验证"}`, token); r.Code != 409 {
		t.Fatalf("older return cancel status=%d %s", r.Code, r.Body.String())
	}
	installPurchaseReturnCancelAuditFailure(t, db, dialect)
	var beforeStock, beforePayable int64
	_ = db.GORM.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", product).Scan(&beforeStock).Error
	_ = db.GORM.Table("partner_balance").Select("amount_cents").Where("partner_id=? AND direction='SUPPLIER'", partner).Scan(&beforePayable).Error
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/purchase-returns/%d/cancel", third), `{"version":2,"reason":"审计失败回滚"}`, token); r.Code != 500 {
		t.Fatalf("forced cancel audit failure=%d %s", r.Code, r.Body.String())
	}
	var rolledState string
	_ = db.GORM.Table("purchase_return_document").Select("status").Where("id=?", third).Scan(&rolledState).Error
	var afterStock, afterPayable int64
	_ = db.GORM.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", product).Scan(&afterStock).Error
	_ = db.GORM.Table("partner_balance").Select("amount_cents").Where("partner_id=? AND direction='SUPPLIER'", partner).Scan(&afterPayable).Error
	if rolledState != "POSTED" || beforeStock != afterStock || beforePayable != afterPayable {
		t.Fatalf("cancel audit rollback status=%s stock=%d/%d payable=%d/%d", rolledState, beforeStock, afterStock, beforePayable, afterPayable)
	}
	dropPurchaseReturnCancelAuditFailure(t, db, dialect)
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/purchase-returns/%d/cancel", third), `{"version":2,"reason":"冲销末笔"}`, token); r.Code != 200 {
		t.Fatalf("cancel latest return=%d %s", r.Code, r.Body.String())
	}
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/purchase-returns/%d/cancel", second), `{"version":2,"reason":"冲销次笔"}`, token); r.Code != 200 {
		t.Fatalf("cancel second return=%d %s", r.Code, r.Body.String())
	}
	// The independent higher-priced line is not interchangeable with lineID.
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/purchase-returns/%d/cancel", different), `{"version":2,"reason":"冲销异价行"}`, token); r.Code != 200 {
		t.Fatalf("cancel other price line=%d %s", r.Code, r.Body.String())
	}
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/purchase-returns/%d/cancel", first), `{"version":2,"reason":"冲销首笔"}`, token); r.Code != 200 {
		t.Fatalf("cancel first after reverse order=%d %s", r.Code, r.Body.String())
	}
	// Availability must be rechecked at posting even though draft previews were valid.
	short, _ := newReturn(lineID, "0.5")
	if e := db.GORM.Table("inventory_balance").Where("product_id=?", product).Update("quantity_milli", int64(0)).Error; e != nil {
		t.Fatal(e)
	}
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/purchase-returns/%d/post", short), `{"version":1}`, token); r.Code != 409 {
		t.Fatalf("stock-short posting status=%d %s", r.Code, r.Body.String())
	}
	// Two different drafts cannot concurrently consume the same original-line quota.
	product2 := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/products", fmt.Sprintf(`{"code":"RPC-%s","name":"并发退货商品","type":"GOODS","unit":"个"}`, dialect), token), 200).ID
	purchase2 := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/purchases", fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"个","quantity":"1","unitPrice":"1.00"}]}`, partner, product2), token), 200).ID
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/purchases/%d/post", purchase2), `{"version":1}`, token); r.Code != 200 {
		t.Fatalf("post concurrency source: %d %s", r.Code, r.Body.String())
	}
	src2 := inventoryData[struct {
		Items []struct {
			PurchaseItemID int64 `json:"purchaseItemId"`
		} `json:"items"`
	}](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/purchase-returns/source/%d", purchase2), "", token), 200)
	ids := make([]int64, 2)
	for i := range ids {
		ids[i] = newConcurrentReturnDraft(t, router, token, purchase2, src2.Items[0].PurchaseItemID)
	}
	statuses := make(chan int, 2)
	start := make(chan struct{})
	for _, rid := range ids {
		go func(id int64) {
			<-start
			r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/purchase-returns/%d/post", id), `{"version":1}`, token)
			statuses <- r.Code
		}(rid)
	}
	close(start)
	s1, s2 := <-statuses, <-statuses
	if !((s1 == 200 && s2 == 409) || (s1 == 409 && s2 == 200)) {
		t.Fatalf("concurrent quota posts statuses=%d,%d", s1, s2)
	}
	var returnedQty int64
	if e := db.GORM.Table("purchase_return_document_item i").Select("COALESCE(SUM(i.quantity_milli),0)").Joins("JOIN purchase_return_document d ON d.id=i.document_id").Where("i.purchase_item_id=? AND d.status='POSTED'", src2.Items[0].PurchaseItemID).Scan(&returnedQty).Error; e != nil || returnedQty > 1000 {
		t.Fatalf("concurrent returned quantity=%d err=%v", returnedQty, e)
	}
	var ledgerTotal, balanceTotal int64
	if e := db.GORM.Table("partner_balance_entry").Select("COALESCE(SUM(amount_cents),0)").Where("partner_id=? AND direction='SUPPLIER'", partner).Scan(&ledgerTotal).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GORM.Table("partner_balance").Select("amount_cents").Where("partner_id=? AND direction='SUPPLIER'", partner).Scan(&balanceTotal).Error; e != nil {
		t.Fatal(e)
	}
	if ledgerTotal != balanceTotal {
		t.Fatalf("supplier balance does not equal immutable ledger: ledger=%d balance=%d", ledgerTotal, balanceTotal)
	}
}

func newConcurrentReturnDraft(t *testing.T, router http.Handler, token string, purchaseID, itemID int64) int64 {
	t.Helper()
	return inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/purchase-returns", fmt.Sprintf(`{"purchaseId":%d,"businessDate":"2026-10-01","items":[{"purchaseItemId":%d,"quantity":"0.6"}]}`, purchaseID, itemID), token), 200).ID
}

func installPurchaseReturnPostAuditFailure(t *testing.T, db *platformdatabase.Database, dialect string) {
	t.Helper()
	if dialect == "sqlite" {
		if e := db.GORM.Exec(`CREATE TRIGGER purchase_return_post_audit_failure BEFORE INSERT ON sys_oper_log WHEN NEW.operation_type='purchase_return.post' BEGIN SELECT RAISE(ABORT,'audit unavailable'); END`).Error; e != nil {
			t.Fatal(e)
		}
		return
	}
	if e := db.GORM.Exec(`CREATE FUNCTION reject_purchase_return_post_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.operation_type='purchase_return.post' THEN RAISE EXCEPTION 'audit unavailable'; END IF; RETURN NEW; END $$`).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GORM.Exec(`CREATE TRIGGER purchase_return_post_audit_failure BEFORE INSERT ON sys_oper_log FOR EACH ROW EXECUTE FUNCTION reject_purchase_return_post_audit()`).Error; e != nil {
		t.Fatal(e)
	}
}
func dropPurchaseReturnPostAuditFailure(t *testing.T, db *platformdatabase.Database, dialect string) {
	t.Helper()
	if dialect == "sqlite" {
		if e := db.GORM.Exec(`DROP TRIGGER purchase_return_post_audit_failure`).Error; e != nil {
			t.Fatal(e)
		}
		return
	}
	if e := db.GORM.Exec(`DROP TRIGGER purchase_return_post_audit_failure ON sys_oper_log`).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GORM.Exec(`DROP FUNCTION reject_purchase_return_post_audit()`).Error; e != nil {
		t.Fatal(e)
	}
}

func installPurchaseReturnCancelAuditFailure(t *testing.T, db *platformdatabase.Database, dialect string) {
	t.Helper()
	if dialect == "sqlite" {
		if e := db.GORM.Exec(`CREATE TRIGGER purchase_return_cancel_audit_failure BEFORE INSERT ON sys_oper_log WHEN NEW.operation_type='purchase_return.cancel' BEGIN SELECT RAISE(ABORT,'audit unavailable'); END`).Error; e != nil {
			t.Fatal(e)
		}
		return
	}
	if e := db.GORM.Exec(`CREATE FUNCTION reject_purchase_return_cancel_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.operation_type='purchase_return.cancel' THEN RAISE EXCEPTION 'audit unavailable'; END IF; RETURN NEW; END $$`).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GORM.Exec(`CREATE TRIGGER purchase_return_cancel_audit_failure BEFORE INSERT ON sys_oper_log FOR EACH ROW EXECUTE FUNCTION reject_purchase_return_cancel_audit()`).Error; e != nil {
		t.Fatal(e)
	}
}
func dropPurchaseReturnCancelAuditFailure(t *testing.T, db *platformdatabase.Database, dialect string) {
	t.Helper()
	if dialect == "sqlite" {
		if e := db.GORM.Exec(`DROP TRIGGER purchase_return_cancel_audit_failure`).Error; e != nil {
			t.Fatal(e)
		}
		return
	}
	if e := db.GORM.Exec(`DROP TRIGGER purchase_return_cancel_audit_failure ON sys_oper_log`).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GORM.Exec(`DROP FUNCTION reject_purchase_return_cancel_audit()`).Error; e != nil {
		t.Fatal(e)
	}
}
