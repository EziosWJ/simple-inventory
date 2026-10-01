//go:build integration

package integration

import (
	"fmt"
	"github.com/EziosWJ/simple-inventory/server/internal/app"
	"github.com/EziosWJ/simple-inventory/server/internal/purchase"
	"github.com/EziosWJ/simple-inventory/server/internal/sale"
	"net/http"
	"path/filepath"
	"testing"
)

func TestSQLiteDirectDeliveryPostgresSQLiteHTTPContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "direct.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	router, e := app.Build(testAPIConfig(), db, sqliteDependencies(t, db, t.TempDir()))
	if e != nil {
		t.Fatal(e)
	}
	assertDirectDeliveryContract(t, router)
}
func TestPostgresDirectDeliveryPostgresSQLiteHTTPContract(t *testing.T) {
	pg := startPostgres(t)
	runMigrations(t, projectRoot(t), pg.dsn)
	db := openTemporaryDatabase(t, pg.dsn)
	defer db.Close()
	router, e := app.Build(testAPIConfig(), db, testDependencies(t, db, t.TempDir()))
	if e != nil {
		t.Fatal(e)
	}
	assertDirectDeliveryContract(t, router)
}
func assertDirectDeliveryContract(t *testing.T, r http.Handler) {
	token := loginAdmin(t, r)
	assertUnauthenticated(t, serveJSON(r, "POST", "/api/v1/sales", `{}`, ""))
	p := inventoryData[struct{ ID int64 }](t, serveJSON(r, "POST", "/api/v1/partners", `{"code":"DIRECT-P","name":"直送往来","type":"COMPANY","isCustomer":true,"isSupplier":true}`, token), 200).ID
	g := inventoryData[struct{ ID int64 }](t, serveJSON(r, "POST", "/api/v1/products", `{"code":"DIRECT-G","name":"直送商品","type":"GOODS","unit":"台"}`, token), 200).ID
	svc := inventoryData[struct{ ID int64 }](t, serveJSON(r, "POST", "/api/v1/products", `{"code":"DIRECT-S","name":"安装","type":"SERVICE","unit":"次"}`, token), 200).ID
	pi := inventoryData[purchase.Draft](t, serveJSON(r, "POST", "/api/v1/purchases", fmt.Sprintf(`{"directDelivery":true,"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"1","unitPrice":"10.00"},{"productId":%d,"productType":"GOODS","unit":"台","quantity":"2","unitPrice":"20.00"}]}`, p, g, g), token), 200)
	saleBody := fmt.Sprintf(`{"directDelivery":true,"directPurchaseId":%d,"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"3","unitPrice":"30.00"},{"productId":%d,"productType":"SERVICE","unit":"次","quantity":"1","unitPrice":"5.00"}]}`, pi.ID, p, g, svc)
	so := inventoryData[sale.Draft](t, serveJSON(r, "POST", "/api/v1/sales", saleBody, token), 200)
	expect := func(method, url, body string, code int) {
		t.Helper()
		x := serveJSON(r, method, url, body, token)
		if x.Code != code {
			t.Fatalf("%s %s=%d expected %d: %s", method, url, x.Code, code, x.Body.String())
		}
	}
	expect("POST", fmt.Sprintf("/api/v1/sales/%d/post", so.ID), `{"version":1}`, 409)
	expect("POST", "/api/v1/sales", saleBody, 409)
	expect("POST", fmt.Sprintf("/api/v1/purchases/%d/cancel", pi.ID), `{"version":1,"reason":"不能先取消"}`, 409)
	expect("POST", fmt.Sprintf("/api/v1/purchases/%d/post", pi.ID), `{"version":1}`, 200)
	expect("POST", fmt.Sprintf("/api/v1/sales/%d/post", so.ID), `{"version":1}`, 200)
	expect("POST", fmt.Sprintf("/api/v1/sales/%d/post", so.ID), `{"version":1}`, 409)
	expect("POST", fmt.Sprintf("/api/v1/purchases/%d/cancel", pi.ID), `{"version":2,"reason":"不能先取消"}`, 409)
	expect("POST", fmt.Sprintf("/api/v1/sales/%d/cancel", so.ID), `{"version":2,"reason":"录错"}`, 200)
	replacement := inventoryData[sale.Draft](t, serveJSON(r, "POST", "/api/v1/sales", saleBody, token), 200)
	got := inventoryData[purchase.Draft](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/purchases/%d", pi.ID), "", token), 200)
	if len(got.DirectDocuments) != 2 || got.DirectDocuments[0].Status != "CANCELLED" {
		t.Fatalf("history lost: %+v", got.DirectDocuments)
	}
	expect("POST", fmt.Sprintf("/api/v1/sales/%d/cancel", replacement.ID), `{"version":1,"reason":"取消草稿"}`, 200)
	expect("POST", fmt.Sprintf("/api/v1/purchases/%d/cancel", pi.ID), `{"version":2,"reason":"录错"}`, 200)
}
