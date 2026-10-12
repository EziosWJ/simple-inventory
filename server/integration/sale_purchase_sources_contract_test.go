//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/EziosWJ/simple-inventory/server/internal/purchase"
	"github.com/EziosWJ/simple-inventory/server/internal/sale"
)

func TestSQLiteSalePurchaseSourcesHTTPContract(t *testing.T) { assertSalePurchaseSources(t, "sqlite") }
func TestPostgresSalePurchaseSourcesHTTPContract(t *testing.T) {
	assertSalePurchaseSources(t, "postgres")
}
func assertSalePurchaseSources(t *testing.T, dialect string) {
	db, _ := directDraftDatabase(t, dialect)
	defer db.Close()
	r := directDraftRouter(t, db, dialect)
	token := loginAdmin(t, r)
	partner := inventoryData[struct{ ID int64 }](t, serveJSON(r, "POST", "/api/v1/partners", `{"code":"SOURCE-P","name":"来源交易方","type":"COMPANY","isCustomer":true,"isSupplier":true}`, token), 200).ID
	goods := inventoryData[struct{ ID int64 }](t, serveJSON(r, "POST", "/api/v1/products", `{"code":"SOURCE-G","name":"内存","type":"GOODS","unit":"条"}`, token), 200).ID
	wrong := inventoryData[struct{ ID int64 }](t, serveJSON(r, "POST", "/api/v1/products", `{"code":"SOURCE-G2","name":"硬盘","type":"GOODS","unit":"块"}`, token), 200).ID
	createPurchase := func(product int64, unit, date string) purchase.Draft {
		t.Helper()
		p := inventoryData[purchase.Draft](t, serveJSON(r, "POST", "/api/v1/purchases", fmt.Sprintf(`{"partnerId":%d,"businessDate":%q,"items":[{"productId":%d,"productType":"GOODS","unit":%q,"quantity":"10","unitPrice":"1.00"}]}`, partner, date, product, unit), token), 200)
		return inventoryData[purchase.Draft](t, serveJSON(r, "POST", fmt.Sprintf("/api/v1/purchases/%d/post", p.ID), `{"version":1}`, token), 200)
	}
	p1 := createPurchase(goods, "条", "2026-10-01")
	p2 := createPurchase(goods, "条", "2026-10-05")
	pWrong := createPurchase(wrong, "块", "2026-10-01")
	body := fmt.Sprintf(`{"requestKey":"source-create","partnerId":%d,"businessDate":"2026-10-02","items":[{"productId":%d,"productType":"GOODS","unit":"条","quantity":"4","unitPrice":"5","purchaseSources":[{"purchaseItemId":%d,"quantity":"1","confirmLaterDate":false},{"purchaseItemId":%d,"quantity":"2","confirmLaterDate":true}]}]}`, partner, goods, p1.Items[0].ID, p2.Items[0].ID)
	d := inventoryData[sale.Draft](t, serveJSON(r, "POST", "/api/v1/sales", body, token), 200)
	if len(d.Items[0].PurchaseSources) != 2 || d.Items[0].PurchaseSources[1].BusinessDate != "2026-10-05" {
		t.Fatal("multiple sources not returned")
	}
	replay := inventoryData[sale.Draft](t, serveJSON(r, "POST", "/api/v1/sales", body, token), 200)
	if replay.ID != d.ID {
		t.Fatal("source save replay duplicated")
	}
	var request map[string]any
	if e := json.Unmarshal([]byte(body), &request); e != nil {
		t.Fatal(e)
	}
	request["requestKey"] = "source-conflict"
	line := request["items"].([]any)[0].(map[string]any)
	postBody := func(code int) {
		t.Helper()
		data, e := json.Marshal(request)
		if e != nil {
			t.Fatal(e)
		}
		inventoryData[any](t, serveJSON(r, "POST", "/api/v1/sales", string(data), token), code)
	}
	sourceBody := func(ids []int64, quantities []string, confirm bool) []any {
		out := []any{}
		for i, id := range ids {
			out = append(out, map[string]any{"purchaseItemId": id, "quantity": quantities[i], "confirmLaterDate": confirm})
		}
		return out
	}
	line["purchaseSources"] = sourceBody([]int64{p2.Items[0].ID}, []string{"1"}, false)
	postBody(400)
	line["purchaseSources"] = sourceBody([]int64{p1.Items[0].ID, p1.Items[0].ID}, []string{"1", "1"}, false)
	postBody(400)
	line["purchaseSources"] = sourceBody([]int64{pWrong.Items[0].ID}, []string{"1"}, false)
	postBody(400)
	line["purchaseSources"] = sourceBody([]int64{p1.Items[0].ID}, []string{"4.001"}, false)
	postBody(400)
	// Source rows remain valid on draft edits; a smaller quantity cannot silently retain excess sources.
	edit := fmt.Sprintf(`{"version":1,"partnerId":%d,"businessDate":"2026-10-02","items":[{"id":%d,"productId":%d,"productType":"GOODS","unit":"条","quantity":"2","unitPrice":"5"}]}`, partner, d.Items[0].ID, goods)
	inventoryData[any](t, serveJSON(r, "PUT", fmt.Sprintf("/api/v1/sales/%d", d.ID), edit, token), 400)
	// Trace is optional and never reserves purchase quantity across sales.
	request["requestKey"] = "source-another"
	line["purchaseSources"] = sourceBody([]int64{p1.Items[0].ID}, []string{"4"}, false)
	postBody(200)
	d = inventoryData[sale.Draft](t, serveJSON(r, "POST", fmt.Sprintf("/api/v1/sales/%d/post", d.ID), `{"version":1}`, token), 200)
	snapshot := func() map[string][]map[string]any {
		out := map[string][]map[string]any{}
		for _, table := range []string{"inventory_balance", "inventory_entry", "partner_balance", "partner_balance_entry"} {
			var rows []map[string]any
			if e := db.GORM.Table(table).Order("id").Find(&rows).Error; e != nil {
				t.Fatal(e)
			}
			out[table] = rows
		}
		return out
	}
	before := snapshot()
	update := func(version int64, ids []int64, quantities []string, confirm bool, code int) sale.Draft {
		t.Helper()
		data, e := json.Marshal(map[string]any{"version": version, "items": []any{map[string]any{"saleItemId": d.Items[0].ID, "sources": sourceBody(ids, quantities, confirm)}}})
		if e != nil {
			t.Fatal(e)
		}
		return inventoryData[sale.Draft](t, serveJSON(r, "PUT", fmt.Sprintf("/api/v1/sales/%d/purchase-sources", d.ID), string(data), token), code)
	}
	modified := update(d.Version, []int64{p2.Items[0].ID}, []string{"3"}, true, 200)
	if modified.Version != 3 || len(modified.Items[0].PurchaseSources) != 1 || modified.Items[0].PurchaseSources[0].Quantity != "3" || !reflect.DeepEqual(before, snapshot()) {
		t.Fatal("source update changed business ledger or failed")
	}
	update(d.Version, []int64{p1.Items[0].ID}, []string{"1"}, true, 409)
	history := inventoryData[[]sale.SourceHistory](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/sales/%d/purchase-sources/history", d.ID), "", token), 200)
	if len(history) != 2 || history[0].OperatorID == 0 || history[0].OperatorName == "" || history[0].BeforeData == history[0].AfterData {
		t.Fatal("source audit history missing")
	}
	installRedesignAuditFailure(t, db, dialect, "sale.sources.edit")
	update(modified.Version, []int64{p1.Items[0].ID}, []string{"2"}, false, 500)
	dropRedesignAuditFailure(t, db, dialect)
	afterFailure := inventoryData[sale.Draft](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/sales/%d", d.ID), "", token), 200)
	if afterFailure.Version != modified.Version || afterFailure.Items[0].PurchaseSources[0].PurchaseItemID != p2.Items[0].ID {
		t.Fatal("audit failure did not roll back sources")
	}
	// Cancelled source remains traceable without changing the sale or requiring reallocation.
	inventoryData[any](t, serveJSON(r, "POST", fmt.Sprintf("/api/v1/purchases/%d/cancel", p2.ID), `{"version":2,"reason":"来源单录错"}`, token), 200)
	afterCancel := inventoryData[sale.Draft](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/sales/%d", d.ID), "", token), 200)
	if afterCancel.Items[0].PurchaseSources[0].Status != "CANCELLED" || afterCancel.Status != "POSTED" {
		t.Fatal("cancelled source disappeared")
	}
	update(modified.Version, []int64{p2.Items[0].ID}, []string{"3"}, true, 200)
	update(modified.Version+1, []int64{p2.Items[0].ID}, []string{"2"}, true, 400)
	// A buyer's return does not delete original provenance or retroactively allocate quantities.
	sourcesNow := inventoryData[sale.Draft](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/sales/%d", d.ID), "", token), 200)
	if sourcesNow.Items[0].Quantity != "4" {
		t.Fatal("source editing changed original quantity")
	}
	if e := db.GORM.Exec("DELETE FROM sys_role_menu WHERE menu_id IN (SELECT id FROM sys_menu WHERE permission_code='business:sale')").Error; e != nil {
		t.Fatal(e)
	}
	update(sourcesNow.Version, []int64{}, []string{}, false, 403)
	inventoryData[any](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/sales/%d/purchase-sources/history", d.ID), "", token), 403)
}
