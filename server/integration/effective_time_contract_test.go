//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"github.com/EziosWJ/simple-inventory/server/internal/receivable"
	"github.com/EziosWJ/simple-inventory/server/internal/sale"
)

func TestSQLiteEffectiveTimePostgresSQLiteHTTPContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "time.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	deps := sqliteDependencies(t, db, t.TempDir())
	deps.Receivable = receivable.NewService(receivable.NewRepository(db.GORM))
	r, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	assertEffectiveTimeContract(t, db, r, false)
}

func TestPostgresEffectiveTimePostgresSQLiteHTTPContract(t *testing.T) {
	pg := startPostgres(t)
	runMigrations(t, projectRoot(t), pg.dsn)
	db := openTemporaryDatabase(t, pg.dsn)
	defer db.Close()
	// The holder, two waiters and lock observer need independent connections.
	db.SQL.SetMaxOpenConns(4)
	deps := testDependencies(t, db, t.TempDir())
	deps.Receivable = receivable.NewService(receivable.NewRepository(db.GORM))
	r, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	assertEffectiveTimeContract(t, db, r, true)
}

func assertEffectiveTimeContract(t *testing.T, db *platformdatabase.Database, r http.Handler, postgres bool) {
	t.Helper()
	token := loginAdmin(t, r)
	p := inventoryData[struct{ ID int64 }](t, serveJSON(r, "POST", "/api/v1/partners", `{"code":"TIME-P","name":"时间链客户","type":"COMPANY","isCustomer":true}`, token), 200).ID
	product := inventoryData[struct{ ID int64 }](t, serveJSON(r, "POST", "/api/v1/products", `{"code":"TIME-S","name":"服务","type":"SERVICE","unit":"次"}`, token), 200).ID
	inventoryData[receivable.Entry](t, serveJSON(r, "POST", "/api/v1/partner-balances/opening", fmt.Sprintf(`{"requestKey":"TIME-OPEN","partnerId":%d,"direction":"CUSTOMER","amount":"100.00","businessDate":"2026-10-01","description":"期初"}`, p), token), 200)
	so := inventoryData[sale.Draft](t, serveJSON(r, "POST", "/api/v1/sales", fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"SERVICE","unit":"次","quantity":"1","unitPrice":"10.00"}]}`, p, product), token), 200)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	holder, err := db.SQL.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Rollback()
	if postgres {
		var id int64
		err = holder.QueryRowContext(ctx, "SELECT id FROM partner_balance WHERE partner_id=$1 AND direction='CUSTOMER' FOR UPDATE", p).Scan(&id)
	} else {
		_, err = holder.ExecContext(ctx, "UPDATE partner_balance SET amount_cents=amount_cents WHERE partner_id=? AND direction='CUSTOMER'", p)
	}
	if err != nil {
		t.Fatal(err)
	}
	baselineWaits := db.SQL.Stats().WaitCount
	waitFor := func(count int) {
		t.Helper()
		for ctx.Err() == nil {
			if postgres {
				var waiting int
				err := db.SQL.QueryRowContext(ctx, "SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%partner_balance%'").Scan(&waiting)
				if err != nil {
					t.Fatal(err)
				}
				if waiting >= count {
					return
				}
			} else if db.SQL.Stats().WaitCount >= baselineWaits+int64(count) {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		t.Fatalf("did not observe %d balance waiters: %v", count, ctx.Err())
	}
	receiptResult := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		receiptResult <- serveJSON(r, "POST", "/api/v1/partner-balances/settlements", fmt.Sprintf(`{"requestKey":"TIME-RECEIPT","partnerId":%d,"direction":"CUSTOMER","amount":"1.00","businessDate":"2026-10-01","paymentMethod":"CASH"}`, p), token)
	}()
	waitFor(1)
	saleResult := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		saleResult <- serveJSON(r, "POST", fmt.Sprintf("/api/v1/sales/%d/post", so.ID), `{"version":1}`, token)
	}()
	waitFor(2)
	// The receipt was queued first. A timestamp captured by the sale before its
	// balance wait would sort it ahead of the receipt and break the public chain.
	if err := holder.Commit(); err != nil {
		t.Fatal(err)
	}
	receipt := inventoryData[receivable.Entry](t, <-receiptResult, 200)
	so = inventoryData[sale.Draft](t, <-saleResult, 200)
	entriesURL := fmt.Sprintf("/api/v1/partner-balances/entries?partnerId=%d&direction=CUSTOMER&page=1&pageSize=100", p)
	checkChain := func(want int, final string) []receivable.Entry {
		t.Helper()
		page := inventoryData[receivable.Page](t, serveJSON(r, "GET", entriesURL, "", token), 200)
		if len(page.Records) != want {
			t.Fatalf("entries=%+v", page)
		}
		previous := "0.00"
		for _, entry := range page.Records {
			if entry.BalanceBefore != previous {
				t.Fatalf("effective-time order breaks balance chain: %+v", page.Records)
			}
			previous = entry.BalanceAfter
		}
		if previous != final {
			t.Fatalf("final balance=%s want %s", previous, final)
		}
		return page.Records
	}
	entries := checkChain(3, "109.00")
	var saleEntry receivable.Entry
	for _, entry := range entries {
		if entry.SaleID != nil && *entry.SaleID == so.ID {
			saleEntry = entry
		}
	}
	if so.PostedAt == nil || !so.PostedAt.Equal(saleEntry.EffectiveAt) {
		t.Fatalf("posting time diverged: sale=%+v entry=%+v", so, saleEntry)
	}
	if postgres && (receipt.ID != entries[1].ID || saleEntry.ID != entries[2].ID) {
		t.Fatalf("balance lock queue order diverged: %+v", entries)
	}
	note := inventoryData[struct{ BusinessDate string }](t, serveJSON(r, "GET", fmt.Sprintf("/api/v1/sales/%d/delivery-note", so.ID), "", token), 200)
	if note.BusinessDate != "2026-10-01" {
		t.Fatalf("delivery business date=%q", note.BusinessDate)
	}

	so = inventoryData[sale.Draft](t, serveJSON(r, "POST", fmt.Sprintf("/api/v1/sales/%d/cancel", so.ID), `{"version":2,"reason":"时间一致性"}`, token), 200)
	entries = checkChain(4, "99.00")
	if so.CancelledAt == nil || !so.CancelledAt.Equal(entries[3].EffectiveAt) {
		t.Fatalf("cancel time diverged: sale=%+v entry=%+v", so, entries[3])
	}
}
