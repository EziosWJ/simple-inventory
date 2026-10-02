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
	"github.com/EziosWJ/simple-inventory/server/internal/purchase"
)

func TestSQLitePhase5PurchaseSaveHTTPContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "save.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	r, e := app.Build(testAPIConfig(), db, sqliteDependencies(t, db, t.TempDir()))
	if e != nil {
		t.Fatal(e)
	}
	assertPurchaseSave(t, r, db, "sqlite", func() *platformdatabase.Database { return openSQLiteDatabase(t, path) })
}
func TestPostgresPhase5PurchaseSaveHTTPContract(t *testing.T) {
	pg := startPostgres(t)
	runMigrations(t, projectRoot(t), pg.dsn)
	db := openTemporaryDatabase(t, pg.dsn)
	defer db.Close()
	r, e := app.Build(testAPIConfig(), db, testDependencies(t, db, t.TempDir()))
	if e != nil {
		t.Fatal(e)
	}
	assertPurchaseSave(t, r, db, "postgres", func() *platformdatabase.Database { return openTemporaryDatabase(t, pg.dsn) })
}
func assertPurchaseSave(t *testing.T, r http.Handler, db *platformdatabase.Database, dialect string, reopen func() *platformdatabase.Database) {
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
	partner := id(call("POST", "/api/v1/partners", map[string]any{"name": "保存供应商", "type": "COMPANY", "isSupplier": true}, 200))
	product := id(call("POST", "/api/v1/products", map[string]any{"name": "保存商品", "type": "GOODS", "unit": "台"}, 200))
	body := func(key, qty string, version int64) map[string]any {
		v := map[string]any{"partnerId": partner, "businessDate": "2026-10-02", "items": []any{map[string]any{"productId": product, "productType": "GOODS", "unit": "台", "quantity": qty, "unitPrice": "1.01"}}}
		if key != "" {
			v["requestKey"] = key
		}
		if version > 0 {
			v["version"] = version
		}
		return v
	}
	parse := func(raw []byte) purchase.Draft {
		t.Helper()
		var v purchase.Draft
		if e := json.Unmarshal(raw, &v); e != nil {
			t.Fatal(e)
		}
		return v
	}
	result := func(operation, key string) purchase.SaveResult {
		t.Helper()
		var v purchase.SaveResult
		raw := call("GET", "/api/v1/purchases/save-requests/"+operation+"/"+key, nil, 200)
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
			res := serveJSON(r, "POST", "/api/v1/purchases", string(raw), token)
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
	var created purchase.Draft
	for raw := range out {
		var v struct{ Data purchase.Draft }
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
	count("purchase_document", "1=1", 1)
	count("purchase_document_item", "1=1", 1)
	count("purchase_save_request", "1=1", 1)
	count("sys_oper_log", "operation_type='purchase.draft.create'", 1)
	count("inventory_entry", "1=1", 0)
	count("partner_balance_entry", "1=1", 0)
	if replay := parse(call("POST", "/api/v1/purchases", body(" create-key ", "1", 0), 200)); replay.ID != created.ID {
		t.Fatal("normalized retry duplicated")
	}
	call("POST", "/api/v1/purchases", body("create-key", "2", 0), 409)
	invalid := body("invalid", "0", 0)
	call("POST", "/api/v1/purchases", invalid, 400)
	if v := result("CREATE", "invalid"); v.State != "UNCONFIRMED" {
		t.Fatal("failed validation left receipt")
	}
	for _, key := range []string{" ", strings.Repeat("x", 101)} {
		call("POST", "/api/v1/purchases", body(key, "1", 0), 400)
	}
	path := fmt.Sprintf("/api/v1/purchases/%d", created.ID)
	editRaw, _ := json.Marshal(body("edit-key", "2", 1))
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
	edited := parse(call("PUT", path, body("edit-key", "2", 1), 200))
	count("sys_oper_log", "operation_type='purchase.draft.edit'", 1)
	if edited.Version != 2 || edited.SaveReceipt == nil || edited.SaveReceipt.SavedVersion != 2 {
		t.Fatalf("edit=%+v", edited)
	}
	call("PUT", path, body("edit-key", "3", 1), 409)
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
	raw, _ = json.Marshal(body("other-edit", "3", 2))
	otherEdit := serveJSON(restarted, "PUT", path, string(raw), other)
	if otherEdit.Code != 200 {
		t.Fatal(otherEdit.Body.String())
	}
	if res := serveJSON(restarted, "GET", "/api/v1/purchases/save-requests/EDIT/edit-key", "", other); res.Code != 200 || !strings.Contains(res.Body.String(), "UNCONFIRMED") {
		t.Fatal("receipt leaked across actors")
	}
	old := result("EDIT", "edit-key")
	if old.State != "COMMITTED" || old.Receipt.SavedVersion != 2 || old.Document.Version != 3 {
		t.Fatalf("original receipt vs current=%+v", old)
	}
	replay := parse(call("PUT", path, body("edit-key", "2", 1), 200))
	if replay.Version != 3 || replay.SaveReceipt.SavedVersion != 2 {
		t.Fatal("edit replay lost original success")
	}
	call("PUT", path, body("stale-key", "4", 1), 409)
	if v := result("EDIT", "stale-key"); v.State != "UNCONFIRMED" {
		t.Fatal("conflict left receipt")
	}
	installPurchaseAuditFailure(t, db, dialect)
	call("PUT", path, body("edit-audit-failure", "4", 3), 500)
	if v := result("EDIT", "edit-audit-failure"); v.State != "UNCONFIRMED" {
		t.Fatal("failed edit left receipt")
	}
	if v := parse(call("GET", path, nil, 200)); v.Version != 3 {
		t.Fatal("failed edit changed document")
	}
	if e := dropPurchaseAuditFailure(db, dialect); e != nil {
		t.Fatal(e)
	}
	call("POST", path+"/post", map[string]any{"version": 3}, 200)
	old = result("EDIT", "edit-key")
	if old.Receipt.SavedVersion != 2 || old.Document.Status != "POSTED" {
		t.Fatal("receipt lost after post")
	}
	replay = parse(call("POST", "/api/v1/purchases", body("create-key", "1", 0), 200))
	if replay.ID != created.ID || replay.Status != "POSTED" || replay.SaveReceipt.SavedVersion != 1 {
		t.Fatal("create replay lost after post")
	}
	count("sys_oper_log", "operation_type='purchase.draft.create'", 1)
	installPurchaseAuditFailure(t, db, dialect)
	call("POST", "/api/v1/purchases", body("audit-failure", "1", 0), 500)
	if v := result("CREATE", "audit-failure"); v.State != "UNCONFIRMED" {
		t.Fatal("audit failure left receipt")
	}
	count("purchase_document", "1=1", 1)
	count("purchase_document_item", "1=1", 1)
	if e := dropPurchaseAuditFailure(db, dialect); e != nil {
		t.Fatal(e)
	}
	call("POST", "/api/v1/purchases/save-requests/CREATE/never-arrived/resolve", nil, 200)
	if v := result("CREATE", "never-arrived"); v.State != "NOT_COMMITTED" {
		t.Fatalf("resolve=%+v", v)
	}
	call("POST", "/api/v1/purchases", body("never-arrived", "1", 0), 409)
	call("POST", "/api/v1/purchases/save-requests/CREATE/create-key/resolve", nil, 200)
	if v := result("CREATE", "create-key"); v.State != "COMMITTED" {
		t.Fatal("resolve changed successful save")
	}

}
