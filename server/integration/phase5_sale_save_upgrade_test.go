//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"github.com/pressly/goose/v3"
)

func TestSQLitePhase5SaleSaveUpgrade(t *testing.T) {
	db := openSQLiteDatabase(t, filepath.Join(t.TempDir(), "upgrade.db"))
	defer db.Close()
	assertSaleSaveUpgrade(t, db, "sqlite3", filepath.Join(projectRoot(t), "migrations", "sqlite"))
}
func TestPostgresPhase5SaleSaveUpgrade(t *testing.T) {
	pg := startPostgres(t)
	db := openTemporaryDatabase(t, pg.dsn)
	defer db.Close()
	assertSaleSaveUpgrade(t, db, "postgres", filepath.Join(projectRoot(t), "migrations"))
}
func assertSaleSaveUpgrade(t *testing.T, db *platformdatabase.Database, dialect, root string) {
	if e := goose.SetDialect(dialect); e != nil {
		t.Fatal(e)
	}
	goose.SetTableName("goose_schema_db_version")
	if e := goose.UpToContext(context.Background(), db.SQL, filepath.Join(root, "schema"), 24); e != nil {
		t.Fatal(e)
	}
	goose.SetTableName("goose_seed_db_version")
	if e := goose.UpContext(context.Background(), db.SQL, filepath.Join(root, "seed")); e != nil {
		t.Fatal(e)
	}
	deps := testDependencies(t, db, t.TempDir())
	if dialect == "sqlite3" {
		deps = sqliteDependencies(t, db, t.TempDir())
	}
	r, e := app.Build(testAPIConfig(), db, deps)
	if e != nil {
		t.Fatal(e)
	}
	token := loginAdmin(t, r)
	call := func(method, path, body string) json.RawMessage {
		t.Helper()
		res := serveJSON(r, method, path, body, token)
		if res.Code != 200 {
			t.Fatal(res.Body.String())
		}
		var v struct{ Data json.RawMessage }
		if e := json.Unmarshal(res.Body.Bytes(), &v); e != nil {
			t.Fatal(e)
		}
		return v.Data
	}
	id := func(raw []byte) int64 { var v struct{ ID int64 }; _ = json.Unmarshal(raw, &v); return v.ID }
	partner := id(call("POST", "/api/v1/partners", `{"name":"旧客户","type":"COMPANY","isCustomer":true}`))
	product := id(call("POST", "/api/v1/products", `{"name":"旧商品","type":"SERVICE","unit":"台"}`))
	body := fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-09-30","items":[{"productId":%d,"productType":"SERVICE","unit":"台","quantity":"1.234","unitPrice":"12.34"}]}`, partner, product)
	posted := id(call("POST", "/api/v1/sales", body))
	draft := id(call("POST", "/api/v1/sales", body))
	emptyBody := body[:len(body)-1] + `,"deliveryContact":"","deliveryPhone":"","deliveryAddress":""}`
	empty := id(call("POST", "/api/v1/sales", emptyBody))
	call("POST", fmt.Sprintf("/api/v1/sales/%d/post", empty), `{"version":1}`)
	call("POST", fmt.Sprintf("/api/v1/sales/%d/post", posted), `{"version":1}`)
	before := map[int64]json.RawMessage{}
	for _, document := range []int64{posted, draft, empty} {
		before[document] = call("GET", fmt.Sprintf("/api/v1/sales/%d", document), "")
	}
	snapshot := func() map[string][]map[string]any {
		t.Helper()
		out := map[string][]map[string]any{}
		for _, table := range []string{"sale_document", "sale_document_item", "inventory_entry", "partner_balance_entry", "sys_oper_log"} {
			var rows []map[string]any
			if e := db.GORM.Table(table).Order("id").Find(&rows).Error; e != nil {
				t.Fatal(e)
			}
			out[table] = rows
		}
		return out
	}
	stored := snapshot()
	goose.SetTableName("goose_schema_db_version")
	if e := goose.UpContext(context.Background(), db.SQL, filepath.Join(root, "schema")); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(stored, snapshot()) {
		t.Fatal("upgrade changed legacy documents, items, ledger or audit")
	}
	for document, original := range before {
		current := call("GET", fmt.Sprintf("/api/v1/sales/%d", document), "")
		if string(current) != string(original) {
			t.Fatalf("legacy document changed: before=%s after=%s", original, current)
		}
	}
	// Legacy clients continue saving without requestKey after the upgrade.
	call("POST", "/api/v1/sales", body)
}
