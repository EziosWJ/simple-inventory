//go:build integration

package integration

import (
	"fmt"
	"github.com/EziosWJ/simple-inventory/server/internal/app"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSQLitePurchaseReturnDraftPostgresSQLiteHTTPContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "purchase-return.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	deps := sqliteDependencies(t, db, t.TempDir())
	router, e := app.Build(testAPIConfig(), db, deps)
	if e != nil {
		t.Fatal(e)
	}
	assertPurchaseReturnDraftContract(t, router, db, "sqlite")
}
func TestPostgresPurchaseReturnDraftPostgresSQLiteHTTPContract(t *testing.T) {
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
	assertPurchaseReturnDraftContract(t, router, db, "postgres")
}
func assertPurchaseReturnDraftContract(t *testing.T, router http.Handler, db *platformdatabase.Database, dialect string) {
	t.Helper()
	token := loginAdmin(t, router)
	if r := serveJSON(router, http.MethodGet, "/api/v1/purchase-returns", "", ""); r.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list status=%d body=%s", r.Code, r.Body.String())
	}
	menus := serveJSON(router, http.MethodGet, "/api/auth/menus", "", token)
	if menus.Code != 200 || !strings.Contains(menus.Body.String(), "/business/purchase-returns") {
		t.Fatalf("ADMIN purchase-return menu not granted: %d %s", menus.Code, menus.Body.String())
	}
	partner := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/partners", `{"code":"RET-`+dialect+`","name":"历史停用供应商","type":"COMPANY","isSupplier":true}`, token), 200).ID
	product := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/products", `{"code":"RET-G-`+dialect+`","name":"历史商品","type":"GOODS","model":"M1","specification":"S1","unit":"台"}`, token), 200).ID
	created := serveJSON(router, http.MethodPost, "/api/v1/purchases", fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"2","unitPrice":"1.01"},{"productId":%d,"productType":"GOODS","unit":"台","quantity":"3","unitPrice":"2.03"}]}`, partner, product, product), token)
	purchaseID := inventoryData[struct {
		ID      int64 `json:"id"`
		Version int64 `json:"version"`
	}](t, created, 200).ID
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/purchases/%d/post", purchaseID), `{"version":1}`, token); r.Code != 200 {
		t.Fatalf("post original purchase=%d %s", r.Code, r.Body.String())
	}
	var stockBefore, balanceBefore int64
	if e := db.GORM.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", product).Scan(&stockBefore).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GORM.Table("partner_balance").Select("amount_cents").Where("partner_id=? AND direction='SUPPLIER'", partner).Scan(&balanceBefore).Error; e != nil {
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
			PurchaseItemID   int64   `json:"purchaseItemId"`
			OriginalQuantity string  `json:"originalQuantity"`
			UnitPrice        string  `json:"unitPrice"`
			ProductModel     *string `json:"productModel"`
		} `json:"items"`
	}](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/purchase-returns/source/%d", purchaseID), "", token), 200)
	if len(source.Items) != 2 || source.Items[0].PurchaseItemID == source.Items[1].PurchaseItemID || source.Items[0].UnitPrice == source.Items[1].UnitPrice || source.Items[0].ProductModel == nil || *source.Items[0].ProductModel != "M1" {
		t.Fatalf("origin rows lost independent IDs/prices/snapshot: %+v", source.Items)
	}
	id := source.Items[0].PurchaseItemID
	installPurchaseReturnAuditFailure(t, db, dialect)
	failed := serveJSON(router, http.MethodPost, "/api/v1/purchase-returns", fmt.Sprintf(`{"purchaseId":%d,"businessDate":"2026-10-01","items":[{"purchaseItemId":%d,"quantity":"1"}]}`, purchaseID, id), token)
	if failed.Code != 500 {
		t.Fatalf("forced audit failure status=%d %s", failed.Code, failed.Body.String())
	}
	var failedRows int64
	if e := db.GORM.Table("purchase_return_document").Where("purchase_id=?", purchaseID).Count(&failedRows).Error; e != nil || failedRows != 0 {
		t.Fatalf("audit failure left return rows=%d err=%v", failedRows, e)
	}
	if e := dropPurchaseReturnAuditFailure(db, dialect); e != nil {
		t.Fatal(e)
	}
	draft := serveJSON(router, http.MethodPost, "/api/v1/purchase-returns", fmt.Sprintf(`{"purchaseId":%d,"businessDate":"2026-10-01","items":[{"purchaseItemId":%d,"quantity":"1"}]}`, purchaseID, id), token)
	v := inventoryData[struct {
		ID      int64  `json:"id"`
		Version int64  `json:"version"`
		Status  string `json:"status"`
		Items   []struct {
			PurchaseItemID int64  `json:"purchaseItemId"`
			Quantity       string `json:"quantity"`
			UnitPrice      string `json:"unitPrice"`
			Amount         string `json:"amount"`
		} `json:"items"`
	}](t, draft, 200)
	if v.Status != "DRAFT" || v.Version != 1 || v.Items[0].PurchaseItemID != id || v.Items[0].UnitPrice != "1.01" || v.Items[0].Amount != "1.01" {
		t.Fatalf("draft preview = %+v", v)
	}
	// Draft creation cannot reserve the original line's quota or post stock/payables.
	var stockAfter, balanceAfter int64
	db.GORM.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", product).Scan(&stockAfter)
	db.GORM.Table("partner_balance").Select("amount_cents").Where("partner_id=? AND direction='SUPPLIER'", partner).Scan(&balanceAfter)
	if stockAfter != stockBefore || balanceAfter != balanceBefore {
		t.Fatalf("draft mutated inventory/payable: stock %d/%d balance %d/%d", stockBefore, stockAfter, balanceBefore, balanceAfter)
	}
	duplicate := serveJSON(router, http.MethodPost, "/api/v1/purchase-returns", fmt.Sprintf(`{"purchaseId":%d,"businessDate":"2026-10-01","items":[{"purchaseItemId":%d,"quantity":"1"},{"purchaseItemId":%d,"quantity":"1"}]}`, purchaseID, id, id), token)
	if duplicate.Code != 400 {
		t.Fatalf("duplicate source item status=%d %s", duplicate.Code, duplicate.Body.String())
	}
	edited := serveJSON(router, http.MethodPut, fmt.Sprintf("/api/v1/purchase-returns/%d", v.ID), fmt.Sprintf(`{"version":1,"purchaseId":%d,"businessDate":"2026-10-01","items":[{"purchaseItemId":%d,"quantity":"1.25"}]}`, purchaseID, id), token)
	updated := inventoryData[struct {
		Version int64 `json:"version"`
	}](t, edited, 200)
	if updated.Version != 2 {
		t.Fatalf("edited version=%d", updated.Version)
	}
	if r := serveJSON(router, http.MethodPut, fmt.Sprintf("/api/v1/purchase-returns/%d", v.ID), fmt.Sprintf(`{"version":1,"purchaseId":%d,"businessDate":"2026-10-01","items":[{"purchaseItemId":%d,"quantity":"1"}]}`, purchaseID, id), token); r.Code != 409 {
		t.Fatalf("stale edit status=%d %s", r.Code, r.Body.String())
	}
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/purchase-returns/%d/cancel", v.ID), `{"version":2,"reason":"重复录入"}`, token); r.Code != 200 {
		t.Fatalf("cancel draft=%d %s", r.Code, r.Body.String())
	}
	var status string
	if e := db.GORM.Table("purchase_return_document").Select("status").Where("id=?", v.ID).Scan(&status).Error; e != nil || status != "CANCELLED" {
		t.Fatalf("cancel state=%q err=%v", status, e)
	}
	var auditCount int64
	if e := db.GORM.Table("sys_oper_log").Where("module_name='purchase_return' AND operation_type LIKE 'purchase_return.%'").Count(&auditCount).Error; e != nil {
		t.Fatal(e)
	}
	if auditCount < 3 {
		t.Fatalf("audit events=%d", auditCount)
	}
	invalid := serveJSON(router, http.MethodPost, "/api/v1/purchase-returns", fmt.Sprintf(`{"purchaseId":%d,"businessDate":"2026-10-01","items":[{"purchaseItemId":%d,"quantity":"0.0001"}]}`, purchaseID, id), token)
	if invalid.Code != 400 {
		t.Fatalf("invalid precision status=%d %s", invalid.Code, invalid.Body.String())
	}
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/purchases/%d/cancel", purchaseID), `{"version":2,"reason":"原采购录错"}`, token); r.Code != 200 {
		t.Fatalf("cancel original purchase=%d %s", r.Code, r.Body.String())
	}
	if r := serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/purchase-returns/source/%d", purchaseID), "", token); r.Code != 400 {
		t.Fatalf("cancelled origin source status=%d %s", r.Code, r.Body.String())
	}
}
func installPurchaseReturnAuditFailure(t *testing.T, db *platformdatabase.Database, dialect string) {
	t.Helper()
	if dialect == "sqlite" {
		e := db.GORM.Exec(`CREATE TRIGGER purchase_return_audit_failure BEFORE INSERT ON sys_oper_log WHEN NEW.operation_type='purchase_return.draft.create' BEGIN SELECT RAISE(ABORT,'audit unavailable'); END`).Error
		if e != nil {
			t.Fatal(e)
		}
		return
	}
	if e := db.GORM.Exec(`CREATE FUNCTION reject_purchase_return_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.operation_type='purchase_return.draft.create' THEN RAISE EXCEPTION 'audit unavailable'; END IF; RETURN NEW; END $$`).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GORM.Exec(`CREATE TRIGGER purchase_return_audit_failure BEFORE INSERT ON sys_oper_log FOR EACH ROW EXECUTE FUNCTION reject_purchase_return_audit()`).Error; e != nil {
		t.Fatal(e)
	}
}
func dropPurchaseReturnAuditFailure(db *platformdatabase.Database, dialect string) error {
	if dialect == "sqlite" {
		return db.GORM.Exec(`DROP TRIGGER purchase_return_audit_failure`).Error
	}
	if e := db.GORM.Exec(`DROP TRIGGER purchase_return_audit_failure ON sys_oper_log`).Error; e != nil {
		return e
	}
	return db.GORM.Exec(`DROP FUNCTION reject_purchase_return_audit()`).Error
}
