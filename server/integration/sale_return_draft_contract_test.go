//go:build integration

package integration

import (
	"fmt"
	"github.com/EziosWJ/simple-inventory/server/internal/app"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"github.com/EziosWJ/simple-inventory/server/internal/salereturn"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSQLiteSaleReturnDraftPostgresSQLiteHTTPContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sale-return.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	deps := sqliteDependencies(t, db, t.TempDir())
	deps.SaleReturn = salereturn.NewService(salereturn.NewRepository(db.GORM))
	router, e := app.Build(testAPIConfig(), db, deps)
	if e != nil {
		t.Fatal(e)
	}
	assertSaleReturnDraftContract(t, router, db, "sqlite")
}
func TestPostgresSaleReturnDraftPostgresSQLiteHTTPContract(t *testing.T) {
	if _, e := exec.LookPath("docker"); e != nil {
		t.Skip("Docker is required for PostgreSQL integration tests")
	}
	pg := startPostgres(t)
	runMigrations(t, projectRoot(t), pg.dsn)
	db := openTemporaryDatabase(t, pg.dsn)
	defer db.Close()
	deps := testDependencies(t, db, t.TempDir())
	deps.SaleReturn = salereturn.NewService(salereturn.NewRepository(db.GORM))
	router, e := app.Build(testAPIConfig(), db, deps)
	if e != nil {
		t.Fatal(e)
	}
	assertSaleReturnDraftContract(t, router, db, "postgres")
}
func assertSaleReturnDraftContract(t *testing.T, router http.Handler, db *platformdatabase.Database, dialect string) {
	t.Helper()
	token := loginAdmin(t, router)
	if r := serveJSON(router, http.MethodGet, "/api/v1/sale-returns", "", ""); r.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list status=%d body=%s", r.Code, r.Body.String())
	}
	menus := serveJSON(router, http.MethodGet, "/api/auth/menus", "", token)
	if menus.Code != 200 || !strings.Contains(menus.Body.String(), "/business/sale-returns") {
		t.Fatalf("ADMIN sale-return menu not granted: %d %s", menus.Code, menus.Body.String())
	}
	partner := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/partners", `{"code":"RET-`+dialect+`","name":"历史停用客户","type":"COMPANY","isCustomer":true}`, token), 200).ID
	product := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/products", `{"code":"RET-G-`+dialect+`","name":"历史商品","type":"GOODS","model":"M1","specification":"S1","unit":"台"}`, token), 200).ID
	service := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/products", `{"code":"RET-S-`+dialect+`","name":"历史服务","type":"SERVICE","unit":"次"}`, token), 200).ID
	adjustment := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/inventory/adjustments", fmt.Sprintf(`{"items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"5","reason":"OPENING"}]}`, product), token), 200)
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/inventory/adjustments/%d/post", adjustment.ID), `{"version":1}`, token); r.Code != 200 {
		t.Fatalf("post opening adjustment=%d %s", r.Code, r.Body.String())
	}
	created := serveJSON(router, http.MethodPost, "/api/v1/sales", fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"2","unitPrice":"1.01"},{"productId":%d,"productType":"GOODS","unit":"台","quantity":"3","unitPrice":"2.03"},{"productId":%d,"productType":"SERVICE","unit":"次","quantity":"1","unitPrice":"3.00"}]}`, partner, product, product, service), token)
	saleID := inventoryData[struct {
		ID      int64 `json:"id"`
		Version int64 `json:"version"`
	}](t, created, 200).ID
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sales/%d/post", saleID), `{"version":1}`, token); r.Code != 200 {
		t.Fatalf("post original sale=%d %s", r.Code, r.Body.String())
	}
	otherDraft := inventoryData[struct {
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/sales", fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"1","unitPrice":"1.00"}]}`, partner, product), token), 200)
	foreignItemID := otherDraft.Items[0].ID
	var stockBefore, balanceBefore int64
	if e := db.GORM.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", product).Scan(&stockBefore).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GORM.Table("partner_balance").Select("amount_cents").Where("partner_id=? AND direction='CUSTOMER'", partner).Scan(&balanceBefore).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GORM.Table("product").Where("id=?", product).Update("status", 0).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GORM.Table("partner").Where("id=?", partner).Update("status", 0).Error; e != nil {
		t.Fatal(e)
	}
	source := inventoryData[struct {
		Items []struct {
			SaleItemID       int64   `json:"saleItemId"`
			OriginalQuantity string  `json:"originalQuantity"`
			UnitPrice        string  `json:"unitPrice"`
			ProductModel     *string `json:"productModel"`
		} `json:"items"`
	}](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/sale-returns/source/%d", saleID), "", token), 200)
	if len(source.Items) != 2 || source.Items[0].SaleItemID == source.Items[1].SaleItemID || source.Items[0].UnitPrice == source.Items[1].UnitPrice || source.Items[0].ProductModel == nil || *source.Items[0].ProductModel != "M1" {
		t.Fatalf("origin rows lost independent IDs/prices/snapshot: %+v", source.Items)
	}
	var serviceSaleItemID int64
	if e := db.GORM.Table("sale_document_item").Select("id").Where("document_id=? AND product_type='SERVICE'", saleID).Scan(&serviceSaleItemID).Error; e != nil {
		t.Fatal(e)
	}
	if r := serveJSON(router, http.MethodPost, "/api/v1/sale-returns", fmt.Sprintf(`{"saleId":%d,"businessDate":"2026-10-01","items":[{"saleItemId":%d,"quantity":"1"}]}`, saleID, serviceSaleItemID), token); r.Code != 400 {
		t.Fatalf("service return status=%d %s", r.Code, r.Body.String())
	}
	if r := serveJSON(router, http.MethodPost, "/api/v1/sale-returns", fmt.Sprintf(`{"saleId":%d,"businessDate":"2026-10-01","items":[{"saleItemId":%d,"quantity":"1"}]}`, saleID, foreignItemID), token); r.Code != 400 {
		t.Fatalf("cross-sale item status=%d %s", r.Code, r.Body.String())
	}
	id := source.Items[0].SaleItemID
	installSaleReturnAuditFailure(t, db, dialect)
	failed := serveJSON(router, http.MethodPost, "/api/v1/sale-returns", fmt.Sprintf(`{"saleId":%d,"businessDate":"2026-10-01","items":[{"saleItemId":%d,"quantity":"1"}]}`, saleID, id), token)
	if failed.Code != 500 {
		t.Fatalf("forced audit failure status=%d %s", failed.Code, failed.Body.String())
	}
	var failedRows int64
	if e := db.GORM.Table("sale_return_document").Where("sale_id=?", saleID).Count(&failedRows).Error; e != nil || failedRows != 0 {
		t.Fatalf("audit failure left return rows=%d err=%v", failedRows, e)
	}
	if e := dropSaleReturnAuditFailure(db, dialect); e != nil {
		t.Fatal(e)
	}
	draft := serveJSON(router, http.MethodPost, "/api/v1/sale-returns", fmt.Sprintf(`{"saleId":%d,"businessDate":"2026-10-01","items":[{"saleItemId":%d,"quantity":"1"}]}`, saleID, id), token)
	v := inventoryData[struct {
		ID      int64  `json:"id"`
		Version int64  `json:"version"`
		Status  string `json:"status"`
		Items   []struct {
			SaleItemID int64  `json:"saleItemId"`
			Quantity   string `json:"quantity"`
			UnitPrice  string `json:"unitPrice"`
			Amount     string `json:"amount"`
		} `json:"items"`
	}](t, draft, 200)
	if v.Status != "DRAFT" || v.Version != 1 || v.Items[0].SaleItemID != id || v.Items[0].UnitPrice != "1.01" || v.Items[0].Amount != "1.01" {
		t.Fatalf("draft preview = %+v", v)
	}
	// Draft creation cannot reserve the original line's quota or post stock/payables.
	var stockAfter, balanceAfter int64
	db.GORM.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", product).Scan(&stockAfter)
	db.GORM.Table("partner_balance").Select("amount_cents").Where("partner_id=? AND direction='CUSTOMER'", partner).Scan(&balanceAfter)
	if stockAfter != stockBefore || balanceAfter != balanceBefore {
		t.Fatalf("draft mutated inventory/payable: stock %d/%d balance %d/%d", stockBefore, stockAfter, balanceBefore, balanceAfter)
	}
	duplicate := serveJSON(router, http.MethodPost, "/api/v1/sale-returns", fmt.Sprintf(`{"saleId":%d,"businessDate":"2026-10-01","items":[{"saleItemId":%d,"quantity":"1"},{"saleItemId":%d,"quantity":"1"}]}`, saleID, id, id), token)
	if duplicate.Code != 400 {
		t.Fatalf("duplicate source item status=%d %s", duplicate.Code, duplicate.Body.String())
	}
	edited := serveJSON(router, http.MethodPut, fmt.Sprintf("/api/v1/sale-returns/%d", v.ID), fmt.Sprintf(`{"version":1,"saleId":%d,"businessDate":"2026-10-01","items":[{"saleItemId":%d,"quantity":"1.25"}]}`, saleID, id), token)
	updated := inventoryData[struct {
		Version int64 `json:"version"`
	}](t, edited, 200)
	if updated.Version != 2 {
		t.Fatalf("edited version=%d", updated.Version)
	}
	if r := serveJSON(router, http.MethodPut, fmt.Sprintf("/api/v1/sale-returns/%d", v.ID), fmt.Sprintf(`{"version":1,"saleId":%d,"businessDate":"2026-10-01","items":[{"saleItemId":%d,"quantity":"1"}]}`, saleID, id), token); r.Code != 409 {
		t.Fatalf("stale edit status=%d %s", r.Code, r.Body.String())
	}
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sale-returns/%d/cancel", v.ID), `{"version":2,"reason":"重复录入"}`, token); r.Code != 200 {
		t.Fatalf("cancel draft=%d %s", r.Code, r.Body.String())
	}
	var status string
	if e := db.GORM.Table("sale_return_document").Select("status").Where("id=?", v.ID).Scan(&status).Error; e != nil || status != "CANCELLED" {
		t.Fatalf("cancel state=%q err=%v", status, e)
	}
	guardDraft := serveJSON(router, http.MethodPost, "/api/v1/sale-returns", fmt.Sprintf(`{"saleId":%d,"businessDate":"2026-10-01","items":[{"saleItemId":%d,"quantity":"0.5"}]}`, saleID, source.Items[1].SaleItemID), token)
	guardReturnID := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, guardDraft, 200).ID
	postedAt := time.Now().UTC()
	if e := db.GORM.Table("sale_return_document").Where("id=?", guardReturnID).Updates(map[string]any{"status": "POSTED", "posted_by": 1, "posted_at": postedAt}).Error; e != nil {
		t.Fatal(e)
	}
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sales/%d/cancel", saleID), `{"version":2,"reason":"原销售录错"}`, token); r.Code != 409 {
		t.Fatalf("posted sale return did not guard origin cancellation: %d %s", r.Code, r.Body.String())
	}
	if e := db.GORM.Table("sale_return_document").Where("id=?", guardReturnID).Updates(map[string]any{"status": "DRAFT", "posted_by": nil, "posted_at": nil}).Error; e != nil {
		t.Fatal(e)
	}
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sale-returns/%d/cancel", guardReturnID), `{"version":1,"reason":"仅用于取消guard测试"}`, token); r.Code != 200 {
		t.Fatalf("cancel guard fixture draft=%d %s", r.Code, r.Body.String())
	}
	var auditCount int64
	if e := db.GORM.Table("sys_oper_log").Where("module_name='sale_return' AND operation_type LIKE 'sale_return.%'").Count(&auditCount).Error; e != nil {
		t.Fatal(e)
	}
	if auditCount < 3 {
		t.Fatalf("audit events=%d", auditCount)
	}
	invalid := serveJSON(router, http.MethodPost, "/api/v1/sale-returns", fmt.Sprintf(`{"saleId":%d,"businessDate":"2026-10-01","items":[{"saleItemId":%d,"quantity":"0.0001"}]}`, saleID, id), token)
	if invalid.Code != 400 {
		t.Fatalf("invalid precision status=%d %s", invalid.Code, invalid.Body.String())
	}
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sales/%d/cancel", saleID), `{"version":2,"reason":"原销售录错"}`, token); r.Code != 200 {
		t.Fatalf("cancel original sale=%d %s", r.Code, r.Body.String())
	}
	if r := serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/sale-returns/source/%d", saleID), "", token); r.Code != 400 {
		t.Fatalf("cancelled origin source status=%d %s", r.Code, r.Body.String())
	}
	if r := serveJSON(router, http.MethodPost, "/api/v1/sale-returns", fmt.Sprintf(`{"saleId":%d,"businessDate":"2026-10-01","items":[{"saleItemId":%d,"quantity":"1"}]}`, saleID, id), token); r.Code != 400 {
		t.Fatalf("cancelled sale return create status=%d %s", r.Code, r.Body.String())
	}
}
func installSaleReturnAuditFailure(t *testing.T, db *platformdatabase.Database, dialect string) {
	t.Helper()
	if dialect == "sqlite" {
		e := db.GORM.Exec(`CREATE TRIGGER sale_return_audit_failure BEFORE INSERT ON sys_oper_log WHEN NEW.operation_type='sale_return.draft.create' BEGIN SELECT RAISE(ABORT,'audit unavailable'); END`).Error
		if e != nil {
			t.Fatal(e)
		}
		return
	}
	if e := db.GORM.Exec(`CREATE FUNCTION reject_sale_return_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.operation_type='sale_return.draft.create' THEN RAISE EXCEPTION 'audit unavailable'; END IF; RETURN NEW; END $$`).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GORM.Exec(`CREATE TRIGGER sale_return_audit_failure BEFORE INSERT ON sys_oper_log FOR EACH ROW EXECUTE FUNCTION reject_sale_return_audit()`).Error; e != nil {
		t.Fatal(e)
	}
}
func dropSaleReturnAuditFailure(db *platformdatabase.Database, dialect string) error {
	if dialect == "sqlite" {
		return db.GORM.Exec(`DROP TRIGGER sale_return_audit_failure`).Error
	}
	if e := db.GORM.Exec(`DROP TRIGGER sale_return_audit_failure ON sys_oper_log`).Error; e != nil {
		return e
	}
	return db.GORM.Exec(`DROP FUNCTION reject_sale_return_audit()`).Error
}
