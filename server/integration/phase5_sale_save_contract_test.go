//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"github.com/EziosWJ/simple-inventory/server/internal/sale"
)

func TestSQLitePhase5SaleSaveHTTPContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	r, e := app.Build(testAPIConfig(), db, sqliteDependencies(t, db, t.TempDir()))
	if e != nil {
		t.Fatal(e)
	}
	assertSaleSave(t, r, db, "sqlite", func() *platformdatabase.Database { return openSQLiteDatabase(t, path) })
}
func TestPostgresPhase5SaleSaveHTTPContract(t *testing.T) {
	pg := startPostgres(t)
	runMigrations(t, projectRoot(t), pg.dsn)
	db := openTemporaryDatabase(t, pg.dsn)
	defer db.Close()
	r, e := app.Build(testAPIConfig(), db, testDependencies(t, db, t.TempDir()))
	if e != nil {
		t.Fatal(e)
	}
	assertSaleSave(t, r, db, "postgres", func() *platformdatabase.Database { return openTemporaryDatabase(t, pg.dsn) })
}
func assertSaleSave(t *testing.T, r http.Handler, db *platformdatabase.Database, dialect string, reopen func() *platformdatabase.Database) {
	token := loginAdmin(t, r)
	call := func(method, path string, body any, code int) json.RawMessage {
		t.Helper()
		raw, _ := json.Marshal(body)
		res := serveJSON(r, method, path, string(raw), token)
		if res.Code != code {
			t.Fatalf("%s %s=%d want%d: %s", method, path, res.Code, code, res.Body.String())
		}
		var v struct{ Data json.RawMessage }
		if e := json.Unmarshal(res.Body.Bytes(), &v); e != nil {
			t.Fatal(e)
		}
		return v.Data
	}
	id := func(raw []byte) int64 { var v struct{ ID int64 }; _ = json.Unmarshal(raw, &v); return v.ID }
	partner := id(call("POST", "/api/v1/partners", map[string]any{"name": "保存客户", "type": "COMPANY", "isCustomer": true, "isSupplier": true}, 200))
	product := id(call("POST", "/api/v1/products", map[string]any{"name": "保存商品", "type": "SERVICE", "unit": "台"}, 200))
	body := func(key, qty string, version int64) map[string]any {
		v := map[string]any{"partnerId": partner, "businessDate": "2026-10-02", "items": []any{map[string]any{"productId": product, "productType": "SERVICE", "unit": "台", "quantity": qty, "unitPrice": "1.01"}}}
		if key != "" {
			v["requestKey"] = key
		}
		if version > 0 {
			v["version"] = version
		}
		return v
	}
	parse := func(raw []byte) sale.Draft {
		t.Helper()
		var v sale.Draft
		if e := json.Unmarshal(raw, &v); e != nil {
			t.Fatal(e)
		}
		return v
	}
	result := func(operation, key string) sale.SaveResult {
		t.Helper()
		var v sale.SaveResult
		raw := call("GET", "/api/v1/sales/save-requests/"+operation+"/"+key, nil, 200)
		if e := json.Unmarshal(raw, &v); e != nil {
			t.Fatal(e)
		}
		return v
	}
	if v := result("CREATE", "pending"); v.State != "UNCONFIRMED" {
		t.Fatalf("missing receipt=%+v", v)
	}
	// Concurrent identical requests must produce one draft, one set of lines and one audit.
	const n = 8
	out := make(chan []byte, n)
	codes := make(chan int, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			raw, _ := json.Marshal(body("create-key", "1.000", 0))
			res := serveJSON(r, "POST", "/api/v1/sales", string(raw), token)
			codes <- res.Code
			out <- res.Body.Bytes()
		}()
	}
	close(start)
	wg.Wait()
	close(codes)
	close(out)
	for code := range codes {
		if code != 200 {
			t.Fatalf("concurrent save=%d", code)
		}
	}
	var created sale.Draft
	for raw := range out {
		var v struct{ Data sale.Draft }
		_ = json.Unmarshal(raw, &v)
		if created.ID == 0 {
			created = v.Data
		}
		if v.Data.ID != created.ID || v.Data.SaveReceipt == nil || v.Data.SaveReceipt.SavedVersion != 1 {
			t.Fatalf("duplicate result: %+v", v.Data)
		}
	}
	count := func(table, where string, want int64) {
		t.Helper()
		var n int64
		if e := db.GORM.Table(table).Where(where).Count(&n).Error; e != nil || n != want {
			t.Fatalf("%s count=%d want%d err=%v", table, n, want, e)
		}
	}
	count("sale_document", "1=1", 1)
	count("sale_document_item", "1=1", 1)
	count("sale_save_request", "1=1", 1)
	count("sys_oper_log", "operation_type='sale.draft.create'", 1)
	count("inventory_entry", "1=1", 0)
	count("partner_balance_entry", "1=1", 0)
	if replay := parse(call("POST", "/api/v1/sales", body(" create-key ", "1", 0), 200)); replay.ID != created.ID {
		t.Fatal("normalized retry duplicated")
	}
	call("POST", "/api/v1/sales", body("create-key", "2", 0), 409)
	invalid := body("invalid", "0", 0)
	call("POST", "/api/v1/sales", invalid, 400)
	if v := result("CREATE", "invalid"); v.State != "UNCONFIRMED" {
		t.Fatal("failed validation left receipt")
	}
	for _, key := range []string{" ", strings.Repeat("x", 101)} {
		call("POST", "/api/v1/sales", body(key, "1", 0), 400)
	}
	// Explicit empty delivery values differ from omission even if current archive is empty.
	different := body("create-key", "1", 0)
	different["deliveryContact"] = ""
	call("POST", "/api/v1/sales", different, 409)
	path := fmt.Sprintf("/api/v1/sales/%d", created.ID)
	editBody := func(key, qty string, version int64) map[string]any {
		b := body(key, qty, version)
		b["items"].([]any)[0].(map[string]any)["id"] = created.Items[0].ID
		return b
	}
	editRaw, _ := json.Marshal(editBody("edit-key", "2", 1))
	editCodes := make(chan int, n)
	editStart := make(chan struct{})
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-editStart
			editCodes <- serveJSON(r, "PUT", path, string(editRaw), token).Code
		}()
	}
	close(editStart)
	wg.Wait()
	close(editCodes)
	for code := range editCodes {
		if code != 200 {
			t.Fatalf("concurrent edit=%d", code)
		}
	}
	edited := parse(call("PUT", path, editBody("edit-key", "2", 1), 200))
	count("sys_oper_log", "operation_type='sale.draft.edit'", 1)
	if edited.Items[0].ID != created.Items[0].ID || edited.Version != 2 || edited.SaveReceipt == nil || edited.SaveReceipt.SavedVersion != 2 {
		t.Fatalf("edit=%+v", edited)
	}
	call("PUT", path, editBody("edit-key", "3", 1), 409)
	changedLine := body("edit-key", "2", 1)
	call("PUT", path, changedLine, 409)
	// Close and reopen the real database and rebuild the app to verify persistence.
	if e := db.Close(); e != nil {
		t.Fatal(e)
	}
	db = reopen()
	defer db.Close()
	deps := testDependencies(t, db, t.TempDir())
	if dialect == "sqlite" {
		deps = sqliteDependencies(t, db, t.TempDir())
	}
	restarted, e := app.Build(testAPIConfig(), db, deps)
	if e != nil {
		t.Fatal(e)
	}
	r = restarted
	raw, _ := json.Marshal(map[string]any{"username": "save-other", "nickname": "另一个操作者", "deptId": 1, "status": 1})
	if res := serveJSON(restarted, "POST", "/api/system/user", string(raw), token); res.Code != 200 {
		t.Fatal(res.Body.String())
	}
	other := loginUser(t, restarted, "save-other", "admin123")
	raw, _ = json.Marshal(editBody("other-edit", "3", 2))
	otherEdit := serveJSON(restarted, "PUT", path, string(raw), other)
	if otherEdit.Code != 200 {
		t.Fatal(otherEdit.Body.String())
	}
	if res := serveJSON(restarted, "GET", "/api/v1/sales/save-requests/EDIT/edit-key", "", other); res.Code != 200 || !strings.Contains(res.Body.String(), "UNCONFIRMED") {
		t.Fatal("receipt leaked across actors")
	}
	old := result("EDIT", "edit-key")
	if old.State != "COMMITTED" || old.Receipt.SavedVersion != 2 || old.Document.Version != 3 {
		t.Fatalf("original receipt vs current=%+v", old)
	}
	replay := parse(call("PUT", path, editBody("edit-key", "2", 1), 200))
	if replay.Version != 3 || replay.SaveReceipt.SavedVersion != 2 {
		t.Fatal("edit replay lost original success")
	}
	call("PUT", path, editBody("stale-key", "4", 1), 409)
	if v := result("EDIT", "stale-key"); v.State != "UNCONFIRMED" {
		t.Fatal("conflict left receipt")
	}
	installSaleAuditFailure(t, db, dialect)
	call("PUT", path, editBody("edit-audit-failure", "4", 3), 500)
	if v := result("EDIT", "edit-audit-failure"); v.State != "UNCONFIRMED" {
		t.Fatal("failed edit left receipt")
	}
	if v := parse(call("GET", path, nil, 200)); v.Version != 3 {
		t.Fatal("failed edit changed document")
	}
	if e := dropSaleAuditFailure(db, dialect); e != nil {
		t.Fatal(e)
	}
	call("POST", path+"/post", map[string]any{"version": 3}, 200)
	old = result("EDIT", "edit-key")
	if old.Receipt.SavedVersion != 2 || old.Document.Status != "POSTED" {
		t.Fatal("receipt lost after post")
	}
	replay = parse(call("POST", "/api/v1/sales", body("create-key", "1", 0), 200))
	if replay.ID != created.ID || replay.Status != "POSTED" || replay.SaveReceipt.SavedVersion != 1 {
		t.Fatal("create replay lost after post")
	}
	count("sys_oper_log", "operation_type='sale.draft.create'", 1)
	installSaleAuditFailure(t, db, dialect)
	call("POST", "/api/v1/sales", body("audit-failure", "1", 0), 500)
	if v := result("CREATE", "audit-failure"); v.State != "UNCONFIRMED" {
		t.Fatal("audit failure left receipt")
	}
	count("sale_document", "1=1", 1)
	count("sale_document_item", "1=1", 1)
	if e := dropSaleAuditFailure(db, dialect); e != nil {
		t.Fatal(e)
	}
	call("POST", "/api/v1/sales/save-requests/CREATE/never-arrived/resolve", nil, 200)
	if v := result("CREATE", "never-arrived"); v.State != "NOT_COMMITTED" {
		t.Fatalf("resolve=%+v", v)
	}
	call("POST", "/api/v1/sales", body("never-arrived", "1", 0), 409)
	call("POST", "/api/v1/sales/save-requests/CREATE/create-key/resolve", nil, 200)
	if v := result("CREATE", "create-key"); v.State != "COMMITTED" {
		t.Fatal("resolve changed successful save")
	}

	goods := id(call("POST", "/api/v1/products", map[string]any{"name": "混合实物", "type": "GOODS", "unit": "台"}, 200))
	mixed := body("mixed", "1", 0)
	mixed["deliveryContact"] = ""
	mixed["deliveryPhone"] = ""
	mixed["deliveryAddress"] = ""
	mixed["items"] = []any{
		map[string]any{"productId": goods, "productType": "GOODS", "unit": "台", "quantity": "1.001", "unitPrice": "1.25"},
		map[string]any{"productId": goods, "productType": "GOODS", "unit": "台", "quantity": "2", "unitPrice": "2.00"},
		map[string]any{"productId": product, "productType": "SERVICE", "unit": "台", "quantity": "1", "unitPrice": "3.00"},
	}
	mixedSaved := parse(call("POST", "/api/v1/sales", mixed, 200))
	if len(mixedSaved.Items) != 3 || mixedSaved.TotalAmount != "8.25" || mixedSaved.DeliveryContact == nil || *mixedSaved.DeliveryContact != "" {
		t.Fatalf("mixed sale=%+v", mixedSaved)
	}
	inventoryBefore := int64(0)
	_ = db.GORM.Table("inventory_entry").Count(&inventoryBefore).Error
	if inventoryBefore != 0 {
		t.Fatal("service post or mixed draft changed stock")
	}
	purchaseBody := map[string]any{"requestKey": "create-key", "partnerId": partner, "businessDate": "2026-10-02", "directDelivery": true, "items": []any{map[string]any{"productId": goods, "productType": "GOODS", "unit": "台", "quantity": "3.001", "unitPrice": "2.00"}}}
	source := id(call("POST", "/api/v1/purchases", purchaseBody, 200))
	linked := mixed
	linked["requestKey"] = "direct"
	linked["directDelivery"] = true
	linked["directPurchaseId"] = source
	directSaved := parse(call("POST", "/api/v1/sales", linked, 200))
	if directSaved.DirectPurchaseID == nil || *directSaved.DirectPurchaseID != source {
		t.Fatal("direct source lost")
	}
	if replay := parse(call("POST", "/api/v1/sales", linked, 200)); replay.ID != directSaved.ID {
		t.Fatal("direct retry duplicated")
	}
	linked["directPurchaseId"] = source + 1
	call("POST", "/api/v1/sales", linked, 409)
	cancel := parse(call("POST", path+"/cancel", map[string]any{"version": 4, "reason": "测试取消"}, 200))
	if cancel.Status != "CANCELLED" || result("CREATE", "create-key").Document.Status != "CANCELLED" {
		t.Fatal("receipt lost after cancellation")
	}

}
