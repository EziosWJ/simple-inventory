//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
)

type listOrderRow struct {
	ID           int64  `json:"id"`
	DocumentNo   string `json:"documentNo"`
	BusinessDate string `json:"businessDate"`
	Items        []struct {
		ID int64 `json:"id"`
	} `json:"items"`
}

type listOrderPage struct {
	Total   int64          `json:"total"`
	Records []listOrderRow `json:"records"`
}

func TestSQLiteBusinessListOrderHTTPContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "list-order.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	r, err := app.Build(testAPIConfig(), db, sqliteDependencies(t, db, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	assertBusinessListOrder(t, r)
}

func TestPostgresBusinessListOrderHTTPContract(t *testing.T) {
	pg := startPostgres(t)
	runMigrations(t, projectRoot(t), pg.dsn)
	db := openTemporaryDatabase(t, pg.dsn)
	defer db.Close()
	r, err := app.Build(testAPIConfig(), db, testDependencies(t, db, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	assertBusinessListOrder(t, r)
}

func assertBusinessListOrder(t *testing.T, r http.Handler) {
	t.Helper()
	token := loginAdmin(t, r)
	create := func(path, body string) listOrderRow {
		t.Helper()
		return inventoryData[listOrderRow](t, serveJSON(r, "POST", path, body, token), 200)
	}
	submit := func(path string) {
		t.Helper()
		inventoryData[any](t, serveJSON(r, "POST", path, `{"version":1}`, token), 200)
	}
	page := func(path string) {
		t.Helper()
		out := inventoryData[listOrderPage](t, serveJSON(r, "GET", path, "", token), 200)
		if out.Total < 2 || len(out.Records) < 2 || out.Records[0].BusinessDate != "2026-09-02" {
			t.Fatalf("%s must default to the newest business date first: %+v", path, out)
		}
	}

	partner := inventoryData[struct{ ID int64 }](t, serveJSON(r, "POST", "/api/v1/partners", `{"code":"ORDER-P","name":"排序往来","type":"COMPANY","isCustomer":true,"isSupplier":true}`, token), 200).ID
	goods := inventoryData[struct{ ID int64 }](t, serveJSON(r, "POST", "/api/v1/products", `{"code":"ORDER-G","name":"排序商品","type":"GOODS","unit":"台"}`, token), 200).ID
	purchase := func(quantity, date string) string {
		return fmt.Sprintf(`{"partnerId":%d,"businessDate":%q,"items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":%q,"unitPrice":"1.00"}]}`, partner, date, goods, quantity)
	}
	sale := func(quantity, date string) string {
		return fmt.Sprintf(`{"partnerId":%d,"businessDate":%q,"items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":%q,"unitPrice":"1.00"}]}`, partner, date, goods, quantity)
	}

	// The newer business date is created first, so ID order alone cannot satisfy
	// these assertions.
	create("/api/v1/purchases", purchase("5", "2026-09-02"))
	create("/api/v1/purchases", purchase("5", "2026-09-01"))
	page("/api/v1/purchases?pageSize=50")
	create("/api/v1/sales", sale("5", "2026-09-02"))
	create("/api/v1/sales", sale("5", "2026-09-01"))
	page("/api/v1/sales?pageSize=50")
	direct := func(date string) string {
		return fmt.Sprintf(`{"supplierId":%d,"customerId":%d,"businessDate":%q,"items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"1","purchasePrice":"1.00","salePrice":"2.00"}]}`, partner, partner, date, goods)
	}
	create("/api/v1/direct-orders", direct("2026-09-02"))
	create("/api/v1/direct-orders", direct("2026-09-01"))
	page("/api/v1/direct-orders?pageSize=50")

	sourcePurchase := create("/api/v1/purchases", purchase("20", "2026-08-01"))
	submit(fmt.Sprintf("/api/v1/purchases/%d/post", sourcePurchase.ID))
	purchaseReturn := func(date string) string {
		return fmt.Sprintf(`{"purchaseId":%d,"businessDate":%q,"items":[{"purchaseItemId":%d,"quantity":"1"}]}`, sourcePurchase.ID, date, sourcePurchase.Items[0].ID)
	}
	postPurchaseReturn := func(document listOrderRow) {
		t.Helper()
		submit(fmt.Sprintf("/api/v1/purchase-returns/%d/post", document.ID))
	}
	postPurchaseReturn(create("/api/v1/purchase-returns", purchaseReturn("2026-09-02")))
	postPurchaseReturn(create("/api/v1/purchase-returns", purchaseReturn("2026-09-01")))
	page("/api/v1/purchase-returns?pageSize=50")

	sourceSale := create("/api/v1/sales", sale("5", "2026-08-01"))
	submit(fmt.Sprintf("/api/v1/sales/%d/post", sourceSale.ID))
	saleReturn := func(date string) string {
		return fmt.Sprintf(`{"saleId":%d,"businessDate":%q,"items":[{"saleItemId":%d,"quantity":"1"}]}`, sourceSale.ID, date, sourceSale.Items[0].ID)
	}
	postSaleReturn := func(document listOrderRow) {
		t.Helper()
		submit(fmt.Sprintf("/api/v1/sale-returns/%d/post", document.ID))
	}
	postSaleReturn(create("/api/v1/sale-returns", saleReturn("2026-09-02")))
	postSaleReturn(create("/api/v1/sale-returns", saleReturn("2026-09-01")))
	page("/api/v1/sale-returns?pageSize=50")

	// Adjustments have no business date; their list defaults to the newest
	// creation timestamp, with the document ID as the stable tie-breaker.
	adjustment := func() listOrderRow {
		t.Helper()
		return create("/api/v1/inventory/adjustments", fmt.Sprintf(`{"items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"1","reason":"SURPLUS"}]}`, goods))
	}
	first, second := adjustment(), adjustment()
	out := inventoryData[listOrderPage](t, serveJSON(r, "GET", "/api/v1/inventory/adjustments?pageSize=50", "", token), 200)
	if out.Total < 2 || len(out.Records) < 2 || out.Records[0].ID != second.ID || out.Records[1].ID != first.ID {
		t.Fatalf("inventory adjustments must default to the newest creation time first: %+v", out)
	}
}
