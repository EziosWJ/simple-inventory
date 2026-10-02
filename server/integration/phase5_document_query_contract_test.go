//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"github.com/EziosWJ/simple-inventory/server/internal/app"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestSQLitePhase5DocumentQueryHTTPContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "query.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	r, e := app.Build(testAPIConfig(), db, sqliteDependencies(t, db, t.TempDir()))
	if e != nil {
		t.Fatal(e)
	}
	assertDocumentQuery(t, r, db)
}
func TestPostgresPhase5DocumentQueryHTTPContract(t *testing.T) {
	pg := startPostgres(t)
	runMigrations(t, projectRoot(t), pg.dsn)
	db := openTemporaryDatabase(t, pg.dsn)
	defer db.Close()
	r, e := app.Build(testAPIConfig(), db, testDependencies(t, db, t.TempDir()))
	if e != nil {
		t.Fatal(e)
	}
	assertDocumentQuery(t, r, db)
}
func assertDocumentQuery(t *testing.T, r http.Handler, db *platformdatabase.Database) {
	token := loginAdmin(t, r)
	call := func(method, path string, body any, code int) json.RawMessage {
		t.Helper()
		raw, _ := json.Marshal(body)
		res := serveJSON(r, method, path, string(raw), token)
		if res.Code != code {
			t.Fatalf("%s %s=%d: %s", method, path, res.Code, res.Body.String())
		}
		var v struct{ Data json.RawMessage }
		if e := json.Unmarshal(res.Body.Bytes(), &v); e != nil {
			t.Fatal(e)
		}
		return v.Data
	}
	id := func(raw []byte) int64 { var d struct{ ID int64 }; _ = json.Unmarshal(raw, &d); return d.ID }
	partner := id(call("POST", "/api/v1/partners", map[string]any{"name": "同名对象", "code": "HISTORY", "type": "COMPANY", "isCustomer": true, "isSupplier": true}, 200))
	other := id(call("POST", "/api/v1/partners", map[string]any{"name": "同名对象", "code": "OTHER", "type": "COMPANY", "isCustomer": true, "isSupplier": true}, 200))
	product := id(call("POST", "/api/v1/products", map[string]any{"name": "同名商品", "type": "GOODS", "unit": "台"}, 200))
	call("POST", "/api/v1/products", map[string]any{"name": "同名商品", "type": "GOODS", "unit": "台"}, 200)
	for _, kind := range []string{"purchases", "sales"} {
		call("PUT", fmt.Sprintf("/api/v1/products/%d", product), map[string]any{"name": "同名商品", "type": "GOODS", "unit": "台", "model": nil}, 200)
		ids := []int64{}
		for n := 0; n < 13; n++ {
			pid := partner
			if n == 12 {
				pid = other
			}
			body := map[string]any{"partnerId": pid, "businessDate": "2026-09-15", "items": []any{map[string]any{"productId": product, "productType": "GOODS", "unit": "台", "quantity": "1", "unitPrice": "0.00"}, map[string]any{"productId": product, "productType": "GOODS", "unit": "台", "quantity": "1", "unitPrice": "2.00"}}}
			ids = append(ids, id(call("POST", "/api/v1/"+kind, body, 200)))
		}
		call("POST", fmt.Sprintf("/api/v1/%s/%d/post", kind, ids[0]), map[string]any{"version": 1}, 200)
		call("POST", fmt.Sprintf("/api/v1/%s/%d/cancel", kind, ids[1]), map[string]any{"version": 1, "reason": "历史取消"}, 200)
		base := fmt.Sprintf("/api/v1/%s?partnerId=%d&productId=%d&businessFrom=2026-09-15&businessTo=2026-09-15&pageSize=10", kind, partner, product)
		page := func(path string) (int64, []struct {
			ID                 int64
			DocumentNo, Status string
			Items              []struct {
				ProductName  string
				ProductModel *string
			}
		}) {
			t.Helper()
			var p struct {
				Total   int64
				Records []struct {
					ID                 int64
					DocumentNo, Status string
					Items              []struct {
						ProductName  string
						ProductModel *string
					}
				}
			}
			_ = json.Unmarshal(call("GET", path, nil, 200), &p)
			return p.Total, p.Records
		}
		total, records := page(base)
		if total != 12 || len(records) != 10 {
			t.Fatalf("split lines counted twice: total=%d rows=%d", total, len(records))
		}
		total, records = page(base + "&page=2")
		if total != 12 || len(records) != 2 || records[1].ID != ids[0] {
			t.Fatal("unstable ID pagination")
		}
		total, _ = page(base + "&status=CANCELLED")
		if total != 1 {
			t.Fatal("cancelled missing")
		}
		total, _ = page(strings.ReplaceAll(base, "2026-09-15", "2026-09-16"))
		if total != 0 {
			t.Fatal("date ignored")
		}
		call("GET", base+"&status=UNKNOWN", nil, 400)
		// Keep snapshot NULL even after the current archive is renamed and gains a model.
		call("PUT", fmt.Sprintf("/api/v1/products/%d", product), map[string]any{"name": "现商品名", "type": "GOODS", "unit": "台", "model": "后来型号"}, 200)
		total, records = page(base + "&status=POSTED")
		if total != 1 || records[0].Items[0].ProductModel != nil {
			t.Fatal("posted NULL snapshot overwritten")
		}
		// Make a no-wildcard clause with special characters without fabricating IDs/lines.
		table := map[string]string{"purchases": "purchase_document", "sales": "sale_document"}[kind]
		if e := db.GORM.Table(table).Where("id=?", ids[2]).Update("document_no", "DOC%_!\\AbCzNeedle").Error; e != nil {
			t.Fatal(e)
		}
		for _, keyword := range []string{"%", "_", "!", "\\", "  abczneedle  "} {
			total, _ = page(base + "&documentNo=" + url.QueryEscape(keyword))
			if total != 1 {
				t.Fatalf("literal keyword %q total=%d", keyword, total)
			}
		}
	}
	call("PUT", fmt.Sprintf("/api/v1/partners/%d", partner), map[string]any{"name": "已换身份", "type": "COMPANY", "isCustomer": false, "isSupplier": false}, 400)
	// At least one identity is mandatory; remove each original identity in turn.
	for _, kind := range []string{"purchases", "sales"} {
		isCustomer := kind == "purchases"
		call("PUT", fmt.Sprintf("/api/v1/partners/%d", partner), map[string]any{"name": "历史身份", "type": "COMPANY", "isCustomer": isCustomer, "isSupplier": !isCustomer}, 200)
		call("PUT", fmt.Sprintf("/api/v1/partners/%d/status", partner), map[string]any{"status": 0}, 200)
		call("PUT", fmt.Sprintf("/api/v1/products/%d/status", product), map[string]any{"status": 0}, 200)
		raw := call("GET", fmt.Sprintf("/api/v1/%s?partnerId=%d&productId=%d", kind, partner, product), nil, 200)
		var p struct{ Total int64 }
		_ = json.Unmarshal(raw, &p)
		if p.Total != 12 {
			t.Fatal("inactive/history identity blocked saved ID search")
		}
	}
}
