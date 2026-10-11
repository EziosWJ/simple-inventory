//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"net/http"
	"net/url"
	"testing"
)

func TestSQLiteMasterDataSelectionHTTPContract(t *testing.T) {
	withRefundStatementDB(t, "sqlite", func(t *testing.T, r http.Handler, _ *platformdatabase.Database) { assertMasterDataSelection(t, r) })
}
func TestPostgresMasterDataSelectionHTTPContract(t *testing.T) {
	withRefundStatementDB(t, "postgres", func(t *testing.T, r http.Handler, _ *platformdatabase.Database) { assertMasterDataSelection(t, r) })
}
func assertMasterDataSelection(t *testing.T, r http.Handler) {
	token := loginAdmin(t, r)
	call := func(method, path string, body any, status int) json.RawMessage {
		t.Helper()
		raw, _ := json.Marshal(body)
		res := serveJSON(r, method, path, string(raw), token)
		if res.Code != status {
			t.Fatalf("%s %s: %d %s", method, path, res.Code, res.Body.String())
		}
		var v struct{ Data json.RawMessage }
		if err := json.Unmarshal(res.Body.Bytes(), &v); err != nil {
			t.Fatal(err)
		}
		return v.Data
	}
	id := func(raw json.RawMessage) int64 {
		var v struct{ ID int64 }
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatal(err)
		}
		return v.ID
	}
	partnerBody := map[string]any{"code": "PART-Exact%_!", "name": "旧客户甲", "type": "COMPANY", "isCustomer": true, "isSupplier": true, "contact": "陈某", "phone": "13800123456"}
	partner := id(call("POST", "/api/v1/partners", partnerBody, 200))
	productBody := map[string]any{"code": "PROD-Exact%_!", "name": "旧粉盒", "brand": "立思辰", "model": "ABC%_!\\", "type": "GOODS", "unit": "支", "purchasePrice": "2.00", "salePrice": "5.00"}
	product := id(call("POST", "/api/v1/products", productBody, 200))
	other := id(call("POST", "/api/v1/products", map[string]any{"code": "prefix-PROD-Exact%_!-suffix", "name": "别的粉盒", "type": "GOODS", "unit": "支"}, 200))
	blue := id(call("POST", "/api/v1/products", map[string]any{"name": "蓝色墨水", "type": "GOODS", "unit": "支"}, 200))
	type record struct {
		ID             int64
		MatchesHistory bool
	}
	page := func(path string) (int64, []record) {
		t.Helper()
		var p struct {
			Total   int64
			Records []record
		}
		if err := json.Unmarshal(call("GET", path, nil, 200), &p); err != nil {
			t.Fatal(err)
		}
		return p.Total, p.Records
	}
	total, rows := page("/api/v1/products?keyword=" + url.QueryEscape("prod-exact%_!"))
	if total != 2 || rows[0].ID != product {
		t.Fatal("exact code must outrank newer containing code")
	}
	total, _ = page("/api/v1/products?keyword=" + url.QueryEscape("立思辰 ABC%_!\\"))
	if total != 1 {
		t.Fatal("cross-field literal words missing")
	}
	total, _ = page("/api/v1/partners?keyword=" + url.QueryEscape("  陈某   13800 "))
	if total != 1 {
		t.Fatal("partner words should match across fields")
	}
	line := func(pid int64) map[string]any {
		return map[string]any{"productId": pid, "productType": "GOODS", "unit": "支", "quantity": "10", "unitPrice": "2.00"}
	}
	create := func(kind string, items []any) int64 {
		return id(call("POST", "/api/v1/"+kind, map[string]any{"partnerId": partner, "businessDate": "2026-10-11", "items": items}, 200))
	}
	var purchase int64
	for n := 0; n < 12; n++ {
		p := create("purchases", []any{line(product), line(product)})
		if n == 0 {
			purchase = p
		}
	}
	create("purchases", []any{line(other), line(blue)})
	call("POST", fmt.Sprintf("/api/v1/purchases/%d/post", purchase), map[string]any{"version": 1}, 200)
	sale := create("sales", []any{line(product)})
	call("POST", fmt.Sprintf("/api/v1/sales/%d/post", sale), map[string]any{"version": 1}, 200)
	productBody["name"] = "新粉盒"
	productBody["model"] = "NEW-model"
	call("PUT", fmt.Sprintf("/api/v1/products/%d", product), productBody, 200)
	partnerBody["name"] = "新客户甲"
	call("PUT", fmt.Sprintf("/api/v1/partners/%d", partner), partnerBody, 200)
	for _, kind := range []string{"purchases", "sales"} {
		base := "/api/v1/" + kind + "?productKeyword=" + url.QueryEscape("旧粉盒 abc%_!\\") + "&pageSize=10"
		total, rows = page(base)
		expected := int64(12)
		if kind == "sales" {
			expected = 1
		}
		if total != expected || len(rows) == 0 || !rows[0].MatchesHistory {
			t.Fatalf("%s historical words/unique count missing: %d %#v", kind, total, rows)
		}
		if kind == "purchases" {
			total, rows = page(base + "&page=2")
			if total != 12 || len(rows) != 2 {
				t.Fatal("historical pagination truncated")
			}
		}
		call("GET", base+fmt.Sprintf("&productId=%d", product), nil, 400)
		call("GET", "/api/v1/"+kind+fmt.Sprintf("?partnerId=%d&partnerKeyword=陈某", partner), nil, 400)
		total, _ = page("/api/v1/" + kind + "?productKeyword=" + url.QueryEscape("别的 蓝色"))
		if total != 0 {
			t.Fatal("words from different line objects must not combine")
		}
	}
	total, rows = page("/api/v1/sales?partnerKeyword=" + url.QueryEscape("旧客户甲"))
	if total != 1 || !rows[0].MatchesHistory {
		t.Fatal("frozen customer name missing")
	}
	total, _ = page("/api/v1/purchases?partnerKeyword=" + url.QueryEscape("旧客户甲"))
	if total != 0 {
		t.Fatal("supplier history must not be fabricated")
	}
	for _, path := range []string{"/api/v1/partner-balances?category=ALL&partnerKeyword=", "/api/v1/partner-balances/entries?partnerKeyword="} {
		total, _ = page(path + url.QueryEscape("陈某 13800"))
		if total < 1 {
			t.Fatal("partner business words missing")
		}
		call("GET", path+url.QueryEscape("陈某")+fmt.Sprintf("&partnerId=%d", partner), nil, 400)
	}
	call("PUT", fmt.Sprintf("/api/v1/products/%d/status", product), map[string]any{"status": 0}, 200)
	call("PUT", fmt.Sprintf("/api/v1/partners/%d/status", partner), map[string]any{"status": 0}, 200)
	total, _ = page("/api/v1/purchases?productKeyword=" + url.QueryEscape("旧粉盒"))
	if total != 12 {
		t.Fatal("inactive history lost")
	}
	total, _ = page("/api/v1/products?status=1&keyword=" + url.QueryEscape("新粉盒"))
	if total != 0 {
		t.Fatal("inactive product selected for new entry")
	}
}
