//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
	"github.com/EziosWJ/simple-inventory/server/internal/directorder"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"github.com/EziosWJ/simple-inventory/server/internal/purchase"
	"github.com/EziosWJ/simple-inventory/server/internal/receivable"
	"github.com/pressly/goose/v3"
)

func TestSQLiteDirectOrderDraftHTTPContract(t *testing.T)   { assertDirectOrderDraft(t, "sqlite") }
func TestPostgresDirectOrderDraftHTTPContract(t *testing.T) { assertDirectOrderDraft(t, "postgres") }
func directDraftDatabase(t *testing.T, dialect string) (*platformdatabase.Database, string) {
	t.Helper()
	if dialect == "sqlite" {
		path := filepath.Join(t.TempDir(), "direct-draft.db")
		runSQLiteMigrations(t, path)
		return openSQLiteDatabase(t, path), filepath.Join(projectRoot(t), "migrations", "sqlite")
	}
	pg := createTestDatabase(t)
	runMigrations(t, projectRoot(t), pg.dsn)
	return openTemporaryDatabase(t, pg.dsn), filepath.Join(projectRoot(t), "migrations")
}
func directDraftRouter(t *testing.T, db *platformdatabase.Database, dialect string) http.Handler {
	t.Helper()
	deps := sqliteDependencies(t, db, t.TempDir())
	if dialect == "postgres" {
		deps = testDependencies(t, db, t.TempDir())
	}
	deps.Receivable = receivable.NewService(receivable.NewRepository(db.GORM))
	r, e := app.Build(testAPIConfig(), db, deps)
	if e != nil {
		t.Fatal(e)
	}
	return r
}
func assertDirectOrderDraft(t *testing.T, dialect string) {
	db, _ := directDraftDatabase(t, dialect)
	defer db.Close()
	r := directDraftRouter(t, db, dialect)
	token := loginAdmin(t, r)
	path := "/api/v1/direct-orders"
	assertUnauthenticated(t, serveJSON(r, "POST", path, `{}`, ""))
	partner := inventoryData[struct{ ID int64 }](t, serveJSON(r, "POST", "/api/v1/partners", `{"code":"DD-P","name":"直送往来","type":"COMPANY","isCustomer":true,"isSupplier":true,"contact":"默认联系人","phone":"123","address":"默认地址"}`, token), 200).ID
	goods := inventoryData[struct{ ID int64 }](t, serveJSON(r, "POST", "/api/v1/products", `{"code":"DD-G","name":"打印机","type":"GOODS","unit":"台"}`, token), 200).ID
	service := inventoryData[struct{ ID int64 }](t, serveJSON(r, "POST", "/api/v1/products", `{"code":"DD-S","name":"安装","type":"SERVICE","unit":"次"}`, token), 200).ID
	body := fmt.Sprintf(`{"requestKey":"direct-create","supplierId":%d,"customerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"1.25","purchasePrice":"10.02","salePrice":"20.03"},{"productId":%d,"productType":"SERVICE","unit":"次","quantity":"1.5","purchasePrice":"0.00","salePrice":"3.00"}]}`, partner, partner, goods, service)
	d := inventoryData[directorder.Draft](t, serveJSON(r, "POST", path, body, token), 200)
	if d.ID < 1 || d.DocumentNo == "" || d.Status != "DRAFT" || d.Version != 1 || len(d.Items) != 2 || d.PurchaseTotal != "12.53" || d.SaleTotal != "29.54" || d.SupplierName != "直送往来" || d.Items[0].ProductName != "打印机" || d.DeliveryContact == nil || *d.DeliveryContact != "默认联系人" || d.SaveReceipt == nil {
		t.Fatalf("unexpected draft: %+v", d)
	}
	replay := inventoryData[directorder.Draft](t, serveJSON(r, "POST", path, strings.Replace(body, `"1.25"`, `"1.250"`, 1), token), 200)
	if replay.ID != d.ID || replay.DocumentNo != d.DocumentNo {
		t.Fatal("save replay created another draft")
	}
	inventoryData[any](t, serveJSON(r, "POST", path, strings.Replace(body, `"20.03"`, `"20.04"`, 1), token), 409)
	for _, table := range []string{"inventory_balance", "inventory_entry", "partner_balance", "partner_balance_entry", "purchase_document", "sale_document"} {
		var n int64
		if e := db.GORM.Table(table).Count(&n).Error; e != nil || n != 0 {
			t.Fatalf("draft changed %s: count=%d err=%v", table, n, e)
		}
	}
	page := inventoryData[directorder.Page](t, serveJSON(r, "GET", path+fmt.Sprintf("?supplierId=%d&customerId=%d&status=DRAFT&businessFrom=2026-10-01&businessTo=2026-10-01", partner, partner), "", token), 200)
	if page.Total != 1 || len(page.Records) != 1 {
		t.Fatalf("unexpected page: %+v", page)
	}
	for _, query := range []string{"?page=no", "?supplierId=-1", "?status=UNKNOWN", "?businessFrom=2026-02-30", "?businessFrom=2026-10-02&businessTo=2026-10-01"} {
		inventoryData[any](t, serveJSON(r, "GET", path+query, "", token), 400)
	}
	inventoryData[any](t, serveJSON(r, "GET", path+"/999999", "", token), 404)
	bad := strings.Replace(body, `"direct-create"`, `"bad-unit"`, 1)
	bad = strings.Replace(bad, `"台"`, `"块"`, 1)
	inventoryData[any](t, serveJSON(r, "POST", path, bad, token), 400)
	onlyService := fmt.Sprintf(`{"supplierId":%d,"customerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"SERVICE","unit":"次","quantity":"1","salePrice":"1.00"}]}`, partner, partner, service)
	inventoryData[any](t, serveJSON(r, "POST", path, onlyService, token), 400)
	invalidService := strings.Replace(body, `"direct-create"`, `"bad-service"`, 1)
	invalidService = strings.Replace(invalidService, `"purchasePrice":"0.00"`, `"purchasePrice":"1.00"`, 1)
	inventoryData[any](t, serveJSON(r, "POST", path, invalidService, token), 400)
	edit := `{"version":1,"deliveryContact":"",` + strings.TrimPrefix(strings.Replace(body, "direct-create", "direct-edit", 1), "{")
	v := inventoryData[directorder.Draft](t, serveJSON(r, "PUT", fmt.Sprintf("%s/%d", path, d.ID), edit, token), 200)
	if v.Version != 2 || v.DeliveryContact == nil || *v.DeliveryContact != "" || v.DeliveryPhone == nil || *v.DeliveryPhone != "123" {
		t.Fatalf("edit lost delivery distinction: %+v", v)
	}
	editReplay := inventoryData[directorder.Draft](t, serveJSON(r, "PUT", fmt.Sprintf("%s/%d", path, d.ID), edit, token), 200)
	if editReplay.Version != 2 {
		t.Fatal("edit replay changed version")
	}
	inventoryData[any](t, serveJSON(r, "PUT", fmt.Sprintf("%s/%d", path, d.ID), strings.Replace(edit, "direct-edit", "stale-edit", 1), token), 409)
	receipt := inventoryData[directorder.SaveResult](t, serveJSON(r, "GET", path+"/save-requests/CREATE/direct-create", "", token), 200)
	if receipt.State != "COMMITTED" || receipt.Receipt.SavedVersion != 1 || receipt.Document.Version != 2 {
		t.Fatal("receipt did not preserve original save version")
	}
	inventoryData[directorder.SaveResult](t, serveJSON(r, "POST", path+"/save-requests/CREATE/late-key/resolve", `{}`, token), 200)
	inventoryData[any](t, serveJSON(r, "POST", path, strings.Replace(body, "direct-create", "late-key", 1), token), 409)
	// Both competing calls must report the same committed draft, with one audit event.
	type result struct {
		Code int
		Body string
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res := serveJSON(r, "POST", path, strings.Replace(body, "direct-create", "concurrent-key", 1), token)
			results <- result{res.Code, res.Body.String()}
		}()
	}
	wg.Wait()
	close(results)
	var concurrentID int64
	for res := range results {
		if res.Code != 200 {
			t.Fatalf("concurrent save failed: %d %s", res.Code, res.Body)
		}
		var env struct{ Data directorder.Draft }
		if e := json.Unmarshal([]byte(res.Body), &env); e != nil {
			t.Fatal(e)
		}
		if concurrentID != 0 && concurrentID != env.Data.ID {
			t.Fatal("concurrent duplicate")
		}
		concurrentID = env.Data.ID
	}
	var logs int64
	if e := db.GORM.Table("sys_oper_log").Where("operation_type='directorder.draft.create'").Count(&logs).Error; e != nil || logs != 2 {
		t.Fatalf("audit count=%d err=%v", logs, e)
	}
	// Audit failure rolls back both edit content/version and the save receipt.
	statement := `CREATE TRIGGER direct_audit_failure BEFORE INSERT ON sys_oper_log WHEN NEW.operation_type='directorder.draft.edit' BEGIN SELECT RAISE(ABORT,'audit unavailable'); END`
	if dialect == "postgres" {
		statement = `CREATE FUNCTION reject_direct_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.operation_type='directorder.draft.edit' THEN RAISE EXCEPTION 'audit unavailable'; END IF; RETURN NEW; END $$`
	}
	if e := db.GORM.Exec(statement).Error; e != nil {
		t.Fatal(e)
	}
	if dialect == "postgres" {
		if e := db.GORM.Exec(`CREATE TRIGGER direct_audit_failure BEFORE INSERT ON sys_oper_log FOR EACH ROW EXECUTE FUNCTION reject_direct_audit()`).Error; e != nil {
			t.Fatal(e)
		}
	}
	failEdit := strings.Replace(strings.Replace(edit, `"version":1`, `"version":2`, 1), "direct-edit", "audit-fail", 1)
	inventoryData[any](t, serveJSON(r, "PUT", fmt.Sprintf("%s/%d", path, d.ID), failEdit, token), 500)
	unchanged := inventoryData[directorder.Draft](t, serveJSON(r, "GET", fmt.Sprintf("%s/%d", path, d.ID), "", token), 200)
	if unchanged.Version != 2 {
		t.Fatal("audit failure changed version")
	}
	var requests int64
	if e := db.GORM.Table("direct_order_save_request").Where("request_key='audit-fail'").Count(&requests).Error; e != nil || requests != 0 {
		t.Fatal("audit failure committed reservation")
	}
	// Authentication alone cannot access this new business permission.
	if e := db.GORM.Exec("DELETE FROM sys_role_menu WHERE menu_id IN (SELECT id FROM sys_menu WHERE permission_code='business:direct-order')").Error; e != nil {
		t.Fatal(e)
	}
	inventoryData[any](t, serveJSON(r, "GET", path, "", token), 403)
	inventoryData[any](t, serveJSON(r, "POST", path, body, token), 403)
}
func TestSQLiteDirectOrderDraftUpgrade(t *testing.T)   { assertDirectOrderDraftUpgrade(t, "sqlite") }
func TestPostgresDirectOrderDraftUpgrade(t *testing.T) { assertDirectOrderDraftUpgrade(t, "postgres") }
func assertDirectOrderDraftUpgrade(t *testing.T, dialect string) {
	var db *platformdatabase.Database
	root := filepath.Join(projectRoot(t), "migrations")
	gooseDialect := "postgres"
	if dialect == "sqlite" {
		db = openSQLiteDatabase(t, filepath.Join(t.TempDir(), "upgrade.db"))
		root = filepath.Join(root, "sqlite")
		gooseDialect = "sqlite3"
	} else {
		pg := createTestDatabase(t)
		db = openTemporaryDatabase(t, pg.dsn)
	}
	defer db.Close()
	if e := goose.SetDialect(gooseDialect); e != nil {
		t.Fatal(e)
	}
	goose.SetTableName("goose_schema_db_version")
	if e := goose.UpToContext(context.Background(), db.SQL, filepath.Join(root, "schema"), 26); e != nil {
		t.Fatal(e)
	}
	goose.SetTableName("goose_seed_db_version")
	if e := goose.UpToContext(context.Background(), db.SQL, filepath.Join(root, "seed"), 24); e != nil {
		t.Fatal(e)
	}
	r := directDraftRouter(t, db, dialect)
	token := loginAdmin(t, r)
	partner := inventoryData[struct{ ID int64 }](t, serveJSON(r, "POST", "/api/v1/partners", `{"code":"UP-DD-P","name":"历史直送往来","type":"COMPANY","isCustomer":true,"isSupplier":true}`, token), 200).ID
	goods := inventoryData[struct{ ID int64 }](t, serveJSON(r, "POST", "/api/v1/products", `{"code":"UP-DD-G","name":"历史商品","type":"GOODS","unit":"台"}`, token), 200).ID
	pi := inventoryData[purchase.Draft](t, serveJSON(r, "POST", "/api/v1/purchases", fmt.Sprintf(`{"directDelivery":true,"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"2","unitPrice":"10.00"}]}`, partner, goods), token), 200)
	inventoryData[any](t, serveJSON(r, "POST", fmt.Sprintf("/api/v1/purchases/%d/post", pi.ID), `{"version":1}`, token), 200)
	inventoryData[any](t, serveJSON(r, "POST", "/api/v1/sales", fmt.Sprintf(`{"directDelivery":true,"directPurchaseId":%d,"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"2","unitPrice":"15.00"}]}`, pi.ID, partner, goods), token), 200)
	tables := []string{"purchase_document", "purchase_document_item", "sale_document", "sale_document_item", "inventory_balance", "inventory_entry", "partner_balance", "partner_balance_entry"}
	snapshot := func() map[string][]map[string]any {
		out := map[string][]map[string]any{}
		for _, table := range tables {
			var rows []map[string]any
			if e := db.GORM.Table(table).Order("id").Find(&rows).Error; e != nil {
				t.Fatal(e)
			}
			for _, row := range rows {
				delete(row, "source_order_id")
			}
			out[table] = rows
		}
		return out
	}
	before := snapshot()
	goose.SetTableName("goose_schema_db_version")
	if e := goose.UpContext(context.Background(), db.SQL, filepath.Join(root, "schema")); e != nil {
		t.Fatal(e)
	}
	// Migration changes SELECT * result shapes; restart idle connections as the API does after upgrade.
	db.SQL.SetMaxIdleConns(0)
	db.SQL.SetMaxIdleConns(5)
	goose.SetTableName("goose_seed_db_version")
	if e := goose.UpContext(context.Background(), db.SQL, filepath.Join(root, "seed")); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(before, snapshot()) {
		t.Fatal("upgrade changed old documents or ledgers")
	}
	inventoryData[any](t, serveJSON(r, "GET", "/api/v1/direct-orders", "", token), 200)
	old := inventoryData[purchase.Draft](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/purchases/%d", pi.ID), "", token), 200)
	if old.Status != "POSTED" || len(old.DirectDocuments) != 1 || old.DirectDocuments[0].Status != "DRAFT" {
		t.Fatal("upgrade changed old direct state")
	}
}
