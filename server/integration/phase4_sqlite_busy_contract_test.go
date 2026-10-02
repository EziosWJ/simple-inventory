//go:build integration

package integration

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
	"github.com/EziosWJ/simple-inventory/server/internal/purchase"
	"github.com/EziosWJ/simple-inventory/server/internal/receivable"
	"github.com/EziosWJ/simple-inventory/server/internal/sale"
)

func TestSQLitePhase4WriteLockHTTPContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "phase4-lock.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	locker := openSQLiteDatabase(t, path)
	defer locker.Close()
	deps := sqliteDependencies(t, db, t.TempDir())
	deps.Receivable = receivable.NewService(receivable.NewRepository(db.GORM))
	r, e := app.Build(testAPIConfig(), db, deps)
	if e != nil {
		t.Fatal(e)
	}
	token := loginAdmin(t, r)
	p := inventoryData[struct{ ID int64 }](t, serveJSON(r, "POST", "/api/v1/partners", `{"code":"BUSY-P","name":"锁测试往来","type":"COMPANY","isCustomer":true,"isSupplier":true}`, token), 200).ID
	g := inventoryData[struct{ ID int64 }](t, serveJSON(r, "POST", "/api/v1/products", `{"code":"BUSY-G","name":"锁测试商品","type":"GOODS","unit":"台"}`, token), 200).ID
	draft := fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"2","unitPrice":"1.00"}]}`, p, g)
	pi := inventoryData[purchase.Draft](t, serveJSON(r, "POST", "/api/v1/purchases", draft, token), 200)
	inventoryData[any](t, serveJSON(r, "POST", fmt.Sprintf("/api/v1/purchases/%d/post", pi.ID), `{"version":1}`, token), 200)
	so := inventoryData[sale.Draft](t, serveJSON(r, "POST", "/api/v1/sales", draft, token), 200)
	inventoryData[any](t, serveJSON(r, "POST", fmt.Sprintf("/api/v1/sales/%d/post", so.ID), `{"version":1}`, token), 200)
	funds := fmt.Sprintf(`{"partnerId":%d,"direction":"CUSTOMER","amount":"1.00","requestKey":"busy","businessDate":"2026-10-01","description":"期初","paymentMethod":"CASH"}`, p)
	bodies := []struct{ method, path, body string }{
		{"POST", "/api/v1/purchases", draft}, {"POST", "/api/v1/sales", draft},
		{"POST", "/api/v1/purchase-returns", fmt.Sprintf(`{"purchaseId":%d,"businessDate":"2026-10-01","items":[{"purchaseItemId":%d,"quantity":"1"}]}`, pi.ID, pi.Items[0].ID)},
		{"POST", "/api/v1/sale-returns", fmt.Sprintf(`{"saleId":%d,"businessDate":"2026-10-01","items":[{"saleItemId":%d,"quantity":"1"}]}`, so.ID, so.Items[0].ID)},
		{"POST", "/api/v1/partner-balances/opening", funds}, {"POST", "/api/v1/partner-balances/settlements", funds}, {"POST", "/api/v1/partner-balances/refunds", funds},
		{"PUT", "/api/v1/print-profile", `{"name":"繁忙时不可保存"}`},
	}
	tables := []string{"purchase_document", "sale_document", "purchase_return_document", "sale_return_document", "inventory_entry", "partner_balance_entry", "sys_oper_log"}
	before := map[string]int64{}
	for _, table := range tables {
		var n int64
		if e := db.GORM.Table(table).Count(&n).Error; e != nil {
			t.Fatal(e)
		}
		before[table] = n
	}
	tx, e := locker.SQL.BeginTx(context.Background(), nil)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	if _, e = tx.Exec("UPDATE sys_config SET update_time=update_time WHERE id=1"); e != nil {
		t.Fatal(e)
	}
	// Settlements acquire the writer before reading, so the production wait is exercised.
	bodies[0], bodies[5] = bodies[5], bodies[0]
	for i, req := range bodies {
		// The first request uses the production 5s bound; later requests use a short
		// connection-local bound to verify every business handler without 40s waits.
		if i == 1 {
			if e := db.GORM.Exec("PRAGMA busy_timeout=20").Error; e != nil {
				t.Fatal(e)
			}
		}
		started := time.Now()
		res := serveJSON(r, req.method, req.path, req.body, token)
		if res.Code != 503 || !strings.Contains(res.Body.String(), `"code":503`) || !strings.Contains(res.Body.String(), `"message":"service temporarily unavailable"`) {
			t.Fatalf("%s %s busy response=%d %s", req.method, req.path, res.Code, res.Body.String())
		}
		if i == 0 && (time.Since(started) < 4*time.Second || time.Since(started) > 8*time.Second) {
			t.Fatalf("production busy bound elapsed=%s", time.Since(started))
		}
	}
	if e := tx.Rollback(); e != nil {
		t.Fatal(e)
	}
	for _, table := range tables {
		var n int64
		if e := db.GORM.Table(table).Count(&n).Error; e != nil || n != before[table] {
			t.Fatalf("failed operations changed %s: %d -> %d err=%v", table, before[table], n, e)
		}
	}
	// A retry after the lock releases is normal, rather than a partial success.
	inventoryData[any](t, serveJSON(r, "POST", "/api/v1/purchases", draft, token), 200)
}
