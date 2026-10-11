//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"testing"

	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"github.com/EziosWJ/simple-inventory/server/internal/receivable"
)

func TestSQLitePartnerBalanceOverviewHTTPContract(t *testing.T) {
	withRefundStatementDB(t, "sqlite", assertPartnerBalanceOverviewHTTPContract)
}
func TestPostgresPartnerBalanceOverviewHTTPContract(t *testing.T) {
	withRefundStatementDB(t, "postgres", assertPartnerBalanceOverviewHTTPContract)
}

func assertPartnerBalanceOverviewHTTPContract(t *testing.T, router http.Handler, _ *platformdatabase.Database) {
	f := newFundsFixture(t, router)
	const endpoint = "/api/v1/partner-balances"
	assertUnauthenticated(t, serveJSON(router, http.MethodGet, endpoint+"/overview", "", ""))
	overview := func() receivable.BalanceOverview {
		return inventoryData[receivable.BalanceOverview](t, serveJSON(router, http.MethodGet, endpoint+"/overview", "", f.token), 200)
	}
	page := func(query string) receivable.BalancePage {
		return inventoryData[receivable.BalancePage](t, serveJSON(router, http.MethodGet, endpoint+query, "", f.token), 200)
	}
	assertSummary := func(v receivable.BalanceOverview, category, amount string, count int64) {
		t.Helper()
		for _, s := range v.Summaries {
			if s.Category == category {
				if s.Amount != amount || s.PartnerCount != count {
					t.Fatalf("%s summary=%+v want=%s/%d", category, s, amount, count)
				}
				return
			}
		}
		t.Fatalf("missing %s summary: %+v", category, v)
	}
	empty := overview()
	for _, c := range []string{"RECEIVABLE", "PAYABLE", "CUSTOMER_REFUND", "SUPPLIER_REFUND"} {
		assertSummary(empty, c, "0.00", 0)
	}
	if len(empty.TopReceivables) != 0 || len(empty.TopPayables) != 0 {
		t.Fatalf("empty rankings=%+v", empty)
	}
	newPartner := func(code string) int64 {
		return f.id("/api/v1/partners", fmt.Sprintf(`{"code":%q,"name":%q,"type":"COMPANY","isCustomer":true,"isSupplier":true}`, code, code))
	}
	opening := func(id int64, direction, amount string) {
		f.post(endpoint+"/opening", fmt.Sprintf(`{"requestKey":%q,"partnerId":%d,"direction":%q,"amount":%q,"businessDate":"2026-10-01","description":"余额概览契约"}`, fmt.Sprintf("O-%d-%s", id, direction), id, direction, amount))
	}
	var largest, historical int64
	for i := 1; i <= 25; i++ {
		id := newPartner(fmt.Sprintf("RANK-%02d", i))
		amount := fmt.Sprintf("%d.00", i)
		if i == 25 {
			largest = id
			amount = "1000.00"
		}
		if i == 24 {
			historical = id
		}
		opening(id, "CUSTOMER", amount)
	}
	// Existing debt remains after partner identity changes and deactivation.
	inventoryData[any](t, serveJSON(router, http.MethodPut, fmt.Sprintf("/api/v1/partners/%d", historical), `{"name":"历史客户","type":"COMPANY","isCustomer":false,"isSupplier":true}`, f.token), 200)
	inventoryData[any](t, serveJSON(router, http.MethodPut, fmt.Sprintf("/api/v1/partners/%d/status", historical), `{"status":0}`, f.token), 200)
	dual := newPartner("DUAL")
	opening(dual, "CUSTOMER", "200.00")
	opening(dual, "SUPPLIER", "70.00")
	// Real settled trades followed by returns produce two refund directions.
	purchase := f.trade("purchases", "1", "50.00")
	sale := f.trade("sales", "1", "80.00")
	f.funds("settlements", "CUSTOMER", "80.00")
	f.funds("settlements", "SUPPLIER", "50.00")
	f.returnGoods("sale-returns", sale, "1")
	f.returnGoods("purchase-returns", purchase, "1")
	zero := newPartner("ZERO")
	opening(zero, "CUSTOMER", "1.00")
	original := f.partner
	f.partner = zero
	f.funds("settlements", "CUSTOMER", "1.00")
	f.partner = original
	v := overview()
	assertSummary(v, "RECEIVABLE", "1500.00", 26)
	assertSummary(v, "PAYABLE", "70.00", 1)
	assertSummary(v, "CUSTOMER_REFUND", "80.00", 1)
	assertSummary(v, "SUPPLIER_REFUND", "50.00", 1)
	if len(v.TopReceivables) != 10 || v.TopReceivables[0].PartnerID != largest || v.TopReceivables[1].PartnerID != dual || v.TopReceivables[2].PartnerID != historical {
		t.Fatalf("full-data ranking=%+v", v.TopReceivables)
	}
	if len(v.TopPayables) != 1 || v.TopPayables[0].PartnerID != dual {
		t.Fatalf("supplier ranking=%+v", v.TopPayables)
	}
	first := page("?category=RECEIVABLE&pageSize=20")
	second := page("?category=RECEIVABLE&pageSize=20&page=2")
	if first.Total != 26 || len(first.Records) != 20 || len(second.Records) != 6 || first.Records[0].PartnerID != largest || second.Records[5].Amount != "1.00" {
		t.Fatalf("sorted pages=%+v / %+v", first, second)
	}
	searched := page(fmt.Sprintf("?category=RECEIVABLE&partnerId=%d", dual))
	if searched.Total != 1 || searched.Records[0].Amount != "200.00" {
		t.Fatalf("search=%+v", searched)
	}
	assertSummary(overview(), "RECEIVABLE", "1500.00", 26)
	for _, c := range []string{"CUSTOMER_REFUND", "SUPPLIER_REFUND"} {
		r := page("?category=" + c)
		if r.Total != 1 || r.Records[0].PartnerID != original || r.Records[0].Amount[0] != '-' {
			t.Fatalf("refund contract keeps signed balances: %+v", r)
		}
	}
	legacy := page(fmt.Sprintf("?partnerId=%d", dual))
	if legacy.Total != 2 || legacy.Records[0].Direction != "CUSTOMER" || legacy.Records[1].Direction != "SUPPLIER" {
		t.Fatalf("legacy query changed: %+v", legacy)
	}
	allZero := page(fmt.Sprintf("?category=ALL&partnerId=%d", zero))
	if allZero.Total != 2 || !allZero.Records[0].HasRecords || allZero.Records[1].HasRecords {
		t.Fatalf("zero/no-record history=%+v", allZero)
	}
	for _, q := range []string{"?category=INVALID", "?direction=INVALID", "?pageSize=501", "?partnerId=-1"} {
		inventoryData[any](t, serveJSON(router, http.MethodGet, endpoint+q, "", f.token), 400)
	}
	// A partial payment updates the overview; full settlement removes the task.
	f.partner = dual
	f.funds("settlements", "CUSTOMER", "50.00")
	assertSummary(overview(), "RECEIVABLE", "1450.00", 26)
	f.funds("settlements", "CUSTOMER", "150.00")
	assertSummary(overview(), "RECEIVABLE", "1300.00", 25)
	if r := page(fmt.Sprintf("?category=RECEIVABLE&partnerId=%d", dual)); r.Total != 0 {
		t.Fatalf("settled task still visible: %+v", r)
	}
	// Two individually valid balances can overflow int64 in aggregate.
	for _, code := range []string{"LARGE-A", "LARGE-B"} {
		opening(newPartner(code), "CUSTOMER", "92233720368547758.07")
	}
	assertSummary(overview(), "RECEIVABLE", "184467440737096816.14", 27)
	// Tied amounts retain a stable partner order across pagination.
	for _, code := range []string{"TIE-A", "TIE-B"} {
		opening(newPartner(code), "CUSTOMER", "2.00")
	}
	all := page("?category=RECEIVABLE&pageSize=500")
	lastTie := int64(0)
	tieCount := 0
	for _, row := range all.Records {
		if row.Amount == "2.00" {
			if row.PartnerID <= lastTie {
				t.Fatalf("unstable tied order: %+v", all.Records)
			}
			lastTie = row.PartnerID
			tieCount++
		}
	}
	if tieCount != 3 {
		t.Fatalf("tied fixtures missing: %d", tieCount)
	}
	assertSummary(overview(), "RECEIVABLE", "184467440737096820.14", 29)
	// Refund pagination orders absolute amounts descending, keeping signs.
	f.partner = newPartner("SMALL-REFUND")
	smallPurchase := f.trade("purchases", "1", "20.00")
	smallSale := f.trade("sales", "1", "10.00")
	f.funds("settlements", "CUSTOMER", "10.00")
	f.funds("settlements", "SUPPLIER", "20.00")
	f.returnGoods("sale-returns", smallSale, "1")
	f.returnGoods("purchase-returns", smallPurchase, "1")
	for category, small := range map[string]string{"CUSTOMER_REFUND": "-10.00", "SUPPLIER_REFUND": "-20.00"} {
		first := page("?category=" + category + "&pageSize=1")
		second := page("?category=" + category + "&pageSize=1&page=2")
		if first.Total != 2 || first.Records[0].PartnerID != original || second.Records[0].Amount != small {
			t.Fatalf("refund magnitude ordering: %+v / %+v", first, second)
		}
	}
}
