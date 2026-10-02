//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"reflect"
	"sync"
	"testing"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
)

func TestSQLitePhase4FixesHTTPContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fixes.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	r, e := app.Build(testAPIConfig(), db, sqliteDependencies(t, db, t.TempDir()))
	if e != nil {
		t.Fatal(e)
	}
	assertPhase4Fixes(t, r, db)
}
func TestPostgresPhase4FixesHTTPContract(t *testing.T) {
	pg := startPostgres(t)
	runMigrations(t, projectRoot(t), pg.dsn)
	db := openTemporaryDatabase(t, pg.dsn)
	defer db.Close()
	r, e := app.Build(testAPIConfig(), db, testDependencies(t, db, t.TempDir()))
	if e != nil {
		t.Fatal(e)
	}
	assertPhase4Fixes(t, r, db)
}

type fixDocument struct {
	ID, Version, CreatedBy                          int64
	Status, PostedByName                            string
	PostedBy                                        *int64
	PostedAt                                        *string
	DeliveryContact, DeliveryPhone, DeliveryAddress *string
	Items                                           []struct {
		ID                                                    int64
		Quantity, Amount, ReturnedQuantity, RemainingQuantity string
	}
}

func assertPhase4Fixes(t *testing.T, r http.Handler, db *platformdatabase.Database) {
	token := loginAdmin(t, r)
	call := func(method, path string, body any, code int) []byte {
		t.Helper()
		raw, _ := json.Marshal(body)
		res := serveJSON(r, method, path, string(raw), token)
		if res.Code != code {
			t.Fatalf("%s %s status=%d want=%d body=%s", method, path, res.Code, code, res.Body.String())
		}
		var v struct{ Data json.RawMessage }
		if e := json.Unmarshal(res.Body.Bytes(), &v); e != nil {
			t.Fatal(e)
		}
		return v.Data
	}
	doc := func(raw []byte) fixDocument {
		t.Helper()
		var v fixDocument
		if e := json.Unmarshal(raw, &v); e != nil {
			t.Fatal(e)
		}
		return v
	}
	resourceID := func(raw []byte) int64 {
		var v struct{ ID int64 }
		if e := json.Unmarshal(raw, &v); e != nil {
			t.Fatal(e)
		}
		return v.ID
	}
	partner := resourceID(call("POST", "/api/v1/partners", map[string]any{"code": "FIX-P", "name": "修复往来", "type": "COMPANY", "isCustomer": true, "isSupplier": true, "contact": "档案联系人", "phone": "123", "address": "档案地址"}, 200))
	product := func(code, kind string) int64 {
		return resourceID(call("POST", "/api/v1/products", map[string]any{"code": code, "name": code, "type": kind, "unit": "台"}, 200))
	}
	line := func(id int64, kind, qty string) map[string]any {
		return map[string]any{"productId": id, "productType": kind, "unit": "台", "quantity": qty, "unitPrice": "0.00"}
	}
	original := func(kind string, id int64, qty string, direct bool, pi int64) fixDocument {
		body := map[string]any{"partnerId": partner, "businessDate": "2026-10-01", "directDelivery": direct, "items": []any{line(id, "GOODS", qty)}}
		if direct {
			body["items"].([]any)[0].(map[string]any)["unitPrice"] = "1.01"
		}
		if pi > 0 {
			body["directPurchaseId"] = pi
		}
		return doc(call("POST", "/api/v1/"+kind, body, 200))
	}
	post := func(kind string, id, version int64) fixDocument {
		return doc(call("POST", fmt.Sprintf("/api/v1/%s/%d/post", kind, id), map[string]any{"version": version}, 200))
	}
	newReturn := func(kind string, origin, item int64, qty string) int64 {
		return doc(call("POST", "/api/v1/"+kind+"-returns", map[string]any{kind + "Id": origin, "businessDate": "2026-10-01", "items": []any{map[string]any{kind + "ItemId": item, "quantity": qty}}}, 200)).ID
	}
	// delivery-omitted-filled-cleared-snapshots
	{
		svc := product("FIX-SERVICE", "SERVICE")
		for _, mode := range []string{"omitted", "filled", "empty"} {
			fields := map[string]string{"deliveryContact": "档案联系人", "deliveryPhone": "123", "deliveryAddress": "档案地址"}
			body := map[string]any{"partnerId": partner, "businessDate": "2026-10-01", "items": []any{line(svc, "SERVICE", "1")}}
			for k := range fields {
				if mode == "filled" {
					fields[k] = "本次资料"
					body[k] = fields[k]
				}
				if mode == "empty" {
					fields[k] = ""
					body[k] = "  "
				}
			}
			created := call("POST", "/api/v1/sales", body, 200)
			v := doc(created)
			check := func(raw []byte) {
				t.Helper()
				var data map[string]any
				if e := json.Unmarshal(raw, &data); e != nil {
					t.Fatal(e)
				}
				for k, want := range fields {
					if data[k] != want {
						t.Fatalf("%s %s=%#v want=%q", mode, k, data[k], want)
					}
				}
			}
			check(created)
			check(call("GET", fmt.Sprintf("/api/v1/sales/%d", v.ID), nil, 200))
			post("sales", v.ID, 1)
			call("PUT", fmt.Sprintf("/api/v1/partners/%d", partner), map[string]any{"code": "FIX-P", "name": "修复往来", "type": "COMPANY", "isCustomer": true, "isSupplier": true, "contact": "后来联系人", "phone": "456", "address": "后来地址"}, 200)
			check(call("GET", fmt.Sprintf("/api/v1/sales/%d", v.ID), nil, 200))
			check(call("GET", fmt.Sprintf("/api/v1/sales/%d/delivery-note", v.ID), nil, 200))
			call("PUT", fmt.Sprintf("/api/v1/partners/%d", partner), map[string]any{"code": "FIX-P", "name": "修复往来", "type": "COMPANY", "isCustomer": true, "isSupplier": true, "contact": "档案联系人", "phone": "123", "address": "档案地址"}, 200)
		}
	}
	// direct-return-order-atomic-and-repeat
	{
		g := product("FIX-DIRECT", "GOODS")
		pi := original("purchases", g, "3", true, 0)
		pi = post("purchases", pi.ID, 1)
		so := original("sales", g, "3", true, pi.ID)
		so = post("sales", so.ID, 1)
		extra := original("purchases", g, "5", false, 0)
		extra = post("purchases", extra.ID, 1)
		pr := newReturn("purchase", pi.ID, pi.Items[0].ID, "1")
		snapshot := func() []int64 {
			t.Helper()
			values := make([]int64, 5)
			queries := []struct{ table, col, where string }{{"inventory_balance", "quantity_milli", fmt.Sprintf("product_id=%d", g)}, {"partner_balance", "amount_cents", fmt.Sprintf("partner_id=%d AND direction='SUPPLIER'", partner)}}
			for i, q := range queries {
				if e := db.GORM.Table(q.table).Select(q.col).Where(q.where).Scan(&values[i]).Error; e != nil {
					t.Fatal(e)
				}
			}
			for i, table := range []string{"inventory_entry", "partner_balance_entry", "sys_oper_log"} {
				if e := db.GORM.Table(table).Count(&values[i+2]).Error; e != nil {
					t.Fatal(e)
				}
			}
			return values
		}
		before := snapshot()
		call("POST", fmt.Sprintf("/api/v1/purchase-returns/%d/post", pr), map[string]any{"version": 1}, 409)
		if !reflect.DeepEqual(before, snapshot()) {
			t.Fatal("refusal mutated stock, balances, entries or audit")
		}
		refused := doc(call("GET", fmt.Sprintf("/api/v1/purchase-returns/%d", pr), nil, 200))
		if refused.Status != "DRAFT" || refused.Version != 1 || refused.Items[0].ReturnedQuantity != "0" {
			t.Fatalf("refused draft=%+v", refused)
		}
		sr := newReturn("sale", so.ID, so.Items[0].ID, "1")
		call("POST", fmt.Sprintf("/api/v1/purchase-returns/%d/post", pr), map[string]any{"version": 1}, 409)
		post("sale-returns", sr, 1)
		var wg sync.WaitGroup
		codes := make(chan int, 2)
		for n := 0; n < 2; n++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				codes <- serveJSON(r, "POST", fmt.Sprintf("/api/v1/purchase-returns/%d/post", pr), `{"version":1}`, token).Code
			}()
		}
		wg.Wait()
		close(codes)
		success := 0
		for code := range codes {
			if code == 200 {
				success++
			} else if code != 409 && code != 503 {
				t.Fatalf("concurrent post=%d", code)
			}
		}
		if success != 1 {
			t.Fatalf("successful posts=%d", success)
		}
		call("POST", fmt.Sprintf("/api/v1/purchase-returns/%d/post", pr), map[string]any{"version": 1}, 409)
		after := snapshot()
		if after[0] != before[0] || after[1] != before[1]-101 || after[2] != before[2]+2 || after[3] != before[3]+2 || after[4] != before[4]+3 {
			t.Fatalf("successful returns must apply once: before=%v after=%v", before, after)
		}
		// Ordinary procurement has no linked-sales-return requirement.
		ordinaryReturn := newReturn("purchase", extra.ID, extra.Items[0].ID, "1")
		post("purchase-returns", ordinaryReturn, 1)
	}
	// cancelled-large-return-history-and-actual-operator
	{
		g := product("FIX-LARGE", "GOODS")
		const qty = "6000000000000000"
		pi := original("purchases", g, qty, false, 0)
		pi = post("purchases", pi.ID, 1)
		so := original("sales", g, qty, false, 0)
		so = post("sales", so.ID, 1)
		call("POST", "/api/system/user", map[string]any{"username": "return-poster", "nickname": "实际过账人", "deptId": 1, "status": 1}, 200)
		other := loginUser(t, r, "return-poster", "admin123")
		for _, kind := range []string{"sale", "purchase"} {
			origin, item := so.ID, so.Items[0].ID
			if kind == "purchase" {
				origin, item = pi.ID, pi.Items[0].ID
			}
			first := newReturn(kind, origin, item, qty)
			post(kind+"-returns", first, 1)
			call("POST", fmt.Sprintf("/api/v1/%s-returns/%d/cancel", kind, first), map[string]any{"version": 2, "reason": "重新办理"}, 200)
			second := newReturn(kind, origin, item, qty)
			inventoryData[any](t, serveJSON(r, "POST", fmt.Sprintf("/api/v1/%s-returns/%d/post", kind, second), `{"version":1}`, other), 200)
			old := doc(call("GET", fmt.Sprintf("/api/v1/%s-returns/%d", kind, first), nil, 200))
			if old.Status != "CANCELLED" || len(old.Items) != 1 || old.Items[0].Amount != "0.00" || old.Items[0].Quantity != qty || old.Items[0].ReturnedQuantity != qty || old.Items[0].RemainingQuantity != "0" {
				t.Fatalf("cancelled history=%+v", old)
			}
			var paged struct {
				Total   int
				Records []fixDocument
			}
			if e := json.Unmarshal(call("GET", fmt.Sprintf("/api/v1/%s-returns?%sId=%d&page=1&pageSize=1", kind, kind, origin), nil, 200), &paged); e != nil {
				t.Fatal(e)
			}
			if paged.Total != 2 || len(paged.Records) != 1 || paged.Records[0].PostedByName != "实际过账人" {
				t.Fatalf("posted page=%+v", paged)
			}
			if e := json.Unmarshal(call("GET", fmt.Sprintf("/api/v1/%s-returns?%sId=%d&page=2&pageSize=1", kind, kind, origin), nil, 200), &paged); e != nil {
				t.Fatal(e)
			}
			if len(paged.Records) != 1 || paged.Records[0].ID != first {
				t.Fatalf("cancelled page=%+v", paged)
			}
			posted := doc(call("GET", fmt.Sprintf("/api/v1/%s-returns/%d", kind, second), nil, 200))
			if posted.PostedBy == nil || *posted.PostedBy == posted.CreatedBy || posted.PostedByName != "实际过账人" || posted.PostedAt == nil || posted.Items[0].Quantity != old.Items[0].Quantity {
				t.Fatalf("posted actor or history quantity=%+v old=%+v", posted, old)
			}
		}
	}
}
