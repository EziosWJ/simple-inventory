//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
	"github.com/EziosWJ/simple-inventory/server/internal/receivable"
)

func TestSQLiteZeroAmountPostgresSQLiteHTTPContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "zero.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	deps := sqliteDependencies(t, db, t.TempDir())
	deps.Receivable = receivable.NewService(receivable.NewRepository(db.GORM))
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	assertZeroAmountContract(t, router)
}

func TestPostgresZeroAmountPostgresSQLiteHTTPContract(t *testing.T) {
	pg := startPostgres(t)
	runMigrations(t, projectRoot(t), pg.dsn)
	db := openTemporaryDatabase(t, pg.dsn)
	defer db.Close()
	deps := testDependencies(t, db, t.TempDir())
	deps.Receivable = receivable.NewService(receivable.NewRepository(db.GORM))
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	assertZeroAmountContract(t, router)
}

func assertZeroAmountContract(t *testing.T, r http.Handler) {
	t.Helper()
	token := loginAdmin(t, r)
	type item struct {
		ID int64 `json:"id"`
	}
	type document struct {
		ID          int64  `json:"id"`
		Version     int64  `json:"version"`
		Status      string `json:"status"`
		TotalAmount string `json:"totalAmount"`
		Items       []item `json:"items"`
	}
	call := func(method, path, body string) document {
		t.Helper()
		return inventoryData[document](t, serveJSON(r, method, path, body, token), 200)
	}
	createID := func(path, body string) int64 {
		return inventoryData[item](t, serveJSON(r, "POST", path, body, token), 200).ID
	}
	partner := createID("/api/v1/partners", `{"code":"ZERO-P","name":"零价往来","type":"COMPANY","isCustomer":true,"isSupplier":true}`)
	goods := createID("/api/v1/products", `{"code":"ZERO-G","name":"赠品","type":"GOODS","unit":"台"}`)
	service := createID("/api/v1/products", `{"code":"ZERO-S","name":"免费服务","type":"SERVICE","unit":"次"}`)
	create := func(path string, product int64, typ, unit, qty string) document {
		return call("POST", path, fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":%q,"unit":%q,"quantity":%q,"unitPrice":"0.00"}]}`, partner, product, typ, unit, qty))
	}
	post := func(path string, d document) document {
		t.Helper()
		d = call("POST", fmt.Sprintf("%s/%d/post", path, d.ID), fmt.Sprintf(`{"version":%d}`, d.Version))
		if d.Status != "POSTED" || d.TotalAmount != "0.00" {
			t.Fatalf("zero document not posted: %+v", d)
		}
		return d
	}
	cancel := func(path string, d document) {
		t.Helper()
		x := call("POST", fmt.Sprintf("%s/%d/cancel", path, d.ID), fmt.Sprintf(`{"version":%d,"reason":"零价取消"}`, d.Version))
		if x.Status != "CANCELLED" {
			t.Fatalf("zero document not cancelled: %+v", x)
		}
	}
	purchase := post("/api/v1/purchases", create("/api/v1/purchases", goods, "GOODS", "台", "10"))
	sale := post("/api/v1/sales", create("/api/v1/sales", goods, "GOODS", "台", "2"))
	for _, side := range []struct {
		path, source, originalKey, itemKey string
		d                                  document
	}{
		{"/api/v1/sale-returns", "/api/v1/sales", "saleId", "saleItemId", sale},
		{"/api/v1/purchase-returns", "/api/v1/purchases", "purchaseId", "purchaseItemId", purchase},
	} {
		original := call("GET", fmt.Sprintf("%s/%d", side.source, side.d.ID), "")
		ret := call("POST", side.path, fmt.Sprintf(`{%q:%d,"businessDate":"2026-10-01","items":[{%q:%d,"quantity":"1"}]}`, side.originalKey, side.d.ID, side.itemKey, original.Items[0].ID))
		cancel(side.path, post(side.path, ret))
	}
	cancel("/api/v1/sales", sale)
	cancel("/api/v1/purchases", purchase)
	cancel("/api/v1/sales", post("/api/v1/sales", create("/api/v1/sales", service, "SERVICE", "次", "3")))
	stock := inventoryData[struct {
		Records []struct {
			ProductID int64  `json:"productId"`
			Quantity  string `json:"quantity"`
		} `json:"records"`
	}](t, serveJSON(r, "GET", "/api/v1/inventory/balances?page=1&pageSize=500&stock=all", "", token), 200)
	found := false
	for _, row := range stock.Records {
		if row.ProductID == service {
			t.Fatal("service acquired a stock balance")
		}
		if row.ProductID == goods {
			found = true
			if quantityToMilli(t, row.Quantity) != 0 {
				t.Fatalf("zero document cancellation left stock: %+v", row)
			}
		}
	}
	if !found {
		t.Fatal("goods stock missing after zero document lifecycle")
	}
	entries := inventoryData[struct {
		Total int64 `json:"total"`
	}](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/partner-balances/entries?partnerId=%d&page=1&pageSize=500", partner), "", token), 200)
	if entries.Total != 0 {
		t.Fatalf("zero documents created monetary movements: %+v", entries)
	}
	balances := inventoryData[struct {
		Records []struct {
			Amount     string `json:"amount"`
			EntryCount int64  `json:"entryCount"`
		} `json:"records"`
	}](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/partner-balances?partnerId=%d&page=1&pageSize=500", partner), "", token), 200)
	for _, b := range balances.Records {
		if b.Amount != "0.00" || b.EntryCount != 0 {
			t.Fatalf("zero balance changed: %+v", b)
		}
	}
}
