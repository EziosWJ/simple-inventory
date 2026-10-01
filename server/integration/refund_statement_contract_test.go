//go:build integration

package integration

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"github.com/EziosWJ/simple-inventory/server/internal/receivable"
)

func TestSQLiteRefundPostgresSQLiteHTTPContract(t *testing.T) {
	withRefundStatementDB(t, "sqlite", assertRefundHTTPContract)
}
func TestPostgresRefundPostgresSQLiteHTTPContract(t *testing.T) {
	withRefundStatementDB(t, "postgres", assertRefundHTTPContract)
}
func TestSQLitePartnerStatementPostgresSQLiteHTTPContract(t *testing.T) {
	withRefundStatementDB(t, "sqlite", assertPartnerStatementHTTPContract)
}
func TestPostgresPartnerStatementPostgresSQLiteHTTPContract(t *testing.T) {
	withRefundStatementDB(t, "postgres", assertPartnerStatementHTTPContract)
}

func withRefundStatementDB(t *testing.T, dialect string, check func(*testing.T, http.Handler, *platformdatabase.Database)) {
	t.Helper()
	var db *platformdatabase.Database
	if dialect == "sqlite" {
		path := filepath.Join(t.TempDir(), "funds-statement.db")
		runSQLiteMigrations(t, path)
		db = openSQLiteDatabase(t, path)
	} else {
		if _, err := exec.LookPath("docker"); err != nil {
			t.Skip("Docker is required for PostgreSQL integration tests")
		}
		pg := startPostgres(t)
		runMigrations(t, projectRoot(t), pg.dsn)
		db = openTemporaryDatabase(t, pg.dsn)
	}
	defer db.Close()
	deps := sqliteDependencies(t, db, t.TempDir())
	if dialect == "postgres" {
		deps = testDependencies(t, db, t.TempDir())
	}
	deps.Receivable = receivable.NewService(receivable.NewRepository(db.GORM))
	router, err := app.Build(testAPIConfig(), db, deps)
	if err != nil {
		t.Fatal(err)
	}
	check(t, router, db)
}

type fundsFixture struct {
	t                *testing.T
	router           http.Handler
	token            string
	partner, product int64
	counter          int
}

func newFundsFixture(t *testing.T, router http.Handler) *fundsFixture {
	t.Helper()
	f := &fundsFixture{t: t, router: router, token: loginAdmin(t, router)}
	f.partner = f.id("/api/v1/partners", `{"code":"FUNDS-1","name":"退款与对账单位","type":"COMPANY","isCustomer":true,"isSupplier":true}`)
	f.product = f.id("/api/v1/products", `{"code":"FUNDS-GOODS","name":"退款测试商品","type":"GOODS","unit":"台"}`)
	return f
}
func (f *fundsFixture) post(path, body string) map[string]any {
	f.t.Helper()
	return inventoryData[map[string]any](f.t, serveJSON(f.router, http.MethodPost, path, body, f.token), 200)
}
func (f *fundsFixture) id(path, body string) int64 { return int64(f.post(path, body)["id"].(float64)) }
func (f *fundsFixture) trade(kind string, quantity, price string) int64 {
	f.t.Helper()
	id := f.id("/api/v1/"+kind, fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-01-01","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":%q,"unitPrice":%q}]}`, f.partner, f.product, quantity, price))
	f.post(fmt.Sprintf("/api/v1/%s/%d/post", kind, id), `{"version":1}`)
	return id
}
func (f *fundsFixture) returnGoods(kind string, sourceID int64, qty string) int64 {
	f.t.Helper()
	sourceKind, itemKey := "purchase", "purchaseItemId"
	if kind == "sale-returns" {
		sourceKind, itemKey = "sale", "saleItemId"
	}
	source := inventoryData[struct {
		Items []map[string]any `json:"items"`
	}](f.t, serveJSON(f.router, http.MethodGet, fmt.Sprintf("/api/v1/%s/source/%d", kind, sourceID), "", f.token), 200)
	id := f.id("/api/v1/"+kind, fmt.Sprintf(`{"%sId":%d,"businessDate":"2026-01-01","items":[{"%s":%v,"quantity":%q}]}`, sourceKind, sourceID, itemKey, source.Items[0][itemKey], qty))
	f.post(fmt.Sprintf("/api/v1/%s/%d/post", kind, id), `{"version":1}`)
	return id
}
func (f *fundsFixture) cancel(kind string, id int64) {
	f.t.Helper()
	f.post(fmt.Sprintf("/api/v1/%s/%d/cancel", kind, id), `{"version":2,"reason":"纠正交易"}`)
}
func (f *fundsFixture) fundsBody(direction, amount, key string) string {
	return fmt.Sprintf(`{"partnerId":%d,"direction":%q,"amount":%q,"requestKey":%q,"businessDate":"2026-01-01","paymentMethod":"CASH","transactionNo":"BANK-1","remark":"真实资金"}`, f.partner, direction, amount, key)
}
func (f *fundsFixture) funds(path, direction, amount string) receivable.Entry {
	f.t.Helper()
	f.counter++
	return inventoryData[receivable.Entry](f.t, serveJSON(f.router, http.MethodPost, "/api/v1/partner-balances/"+path, f.fundsBody(direction, amount, fmt.Sprintf("funds-%d", f.counter)), f.token), 200)
}
func (f *fundsFixture) balance(direction, want string) {
	f.t.Helper()
	page := inventoryData[receivable.BalancePage](f.t, serveJSON(f.router, http.MethodGet, fmt.Sprintf("/api/v1/partner-balances?partnerId=%d&direction=%s", f.partner, direction), "", f.token), 200)
	if len(page.Records) != 1 || page.Records[0].Amount != want {
		f.t.Fatalf("%s balance=%+v want=%s", direction, page, want)
	}
}
func assertRefundHTTPContract(t *testing.T, router http.Handler, db *platformdatabase.Database) {
	f := newFundsFixture(t, router)
	refundPath := "/api/v1/partner-balances/refunds"
	assertUnauthenticated(t, serveJSON(router, http.MethodPost, refundPath, `{}`, ""))
	purchase := f.trade("purchases", "2", "100.00")
	sale := f.trade("sales", "1", "100.00")
	// A partial unpaid return first reduces debt and does not enable refunds.
	firstReturn := f.returnGoods("sale-returns", sale, "0.2")
	f.balance("CUSTOMER", "80.00")
	inventoryData[any](t, serveJSON(router, http.MethodPost, refundPath, f.fundsBody("CUSTOMER", "1.00", "positive-balance"), f.token), 409)
	f.funds("settlements", "CUSTOMER", "80.00")
	secondReturn := f.returnGoods("sale-returns", sale, "0.8")
	f.balance("CUSTOMER", "-80.00")
	// Positive refund amounts increase the negative balance; complete retries
	// compare all semantic payload fields and return the original immutable entry.
	body := f.fundsBody("CUSTOMER", "30.00", "refund-once")
	first := inventoryData[receivable.Entry](t, serveJSON(router, http.MethodPost, refundPath, body, f.token), 200)
	retry := inventoryData[receivable.Entry](t, serveJSON(router, http.MethodPost, refundPath, body, f.token), 200)
	if first.ID != retry.ID || first.Amount != "30.00" || first.BalanceBefore != "-80.00" || first.BalanceAfter != "-50.00" || first.EntryType != "CUSTOMER_REFUND" || first.OperatorID <= 0 || first.EffectiveAt.IsZero() {
		t.Fatalf("refund data=%+v retry=%+v", first, retry)
	}
	for _, changed := range []string{strings.Replace(body, "2026-01-01", "2026-01-02", 1), strings.Replace(body, "真实资金", "其他说明", 1), strings.Replace(body, "BANK-1", "BANK-2", 1), strings.Replace(body, "CASH", "WECHAT", 1), strings.Replace(body, "30.00", "30.01", 1)} {
		inventoryData[any](t, serveJSON(router, http.MethodPost, refundPath, changed, f.token), 409)
	}
	for _, amount := range []string{"0.00", "-1.00", "1.-1", "1.001", "92233720368547758.08"} {
		inventoryData[any](t, serveJSON(router, http.MethodPost, refundPath, f.fundsBody("CUSTOMER", amount, "invalid-"+amount), f.token), 400)
	}
	inventoryData[any](t, serveJSON(router, http.MethodPost, refundPath, f.fundsBody("CUSTOMER", "50.01", "over-refund"), f.token), 409)
	// Two simultaneous 40 refunds cannot consume the remaining 50 twice.
	var wg sync.WaitGroup
	results := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results <- serveJSON(router, http.MethodPost, refundPath, f.fundsBody("CUSTOMER", "40.00", fmt.Sprintf("refund-race-%d", i)), f.token).Code
		}(i)
	}
	wg.Wait()
	close(results)
	codes := map[int]int{}
	for code := range results {
		codes[code]++
	}
	if codes[200] != 1 || codes[409] != 1 {
		t.Fatalf("concurrent refund statuses=%v", codes)
	}
	f.balance("CUSTOMER", "-10.00")
	// Audit storage failure rolls back the entry and projected balance atomically.
	var countBefore int64
	db.GORM.Table("partner_balance_entry").Count(&countBefore)
	if err := db.GORM.Exec("ALTER TABLE sys_oper_log RENAME TO sys_oper_log_unavailable").Error; err != nil {
		t.Fatal(err)
	}
	response := serveJSON(router, http.MethodPost, refundPath, f.fundsBody("CUSTOMER", "10.00", "refund-audit-failure"), f.token)
	if err := db.GORM.Exec("ALTER TABLE sys_oper_log_unavailable RENAME TO sys_oper_log").Error; err != nil {
		t.Fatal(err)
	}
	inventoryData[any](t, response, 500)
	var countAfter int64
	db.GORM.Table("partner_balance_entry").Count(&countAfter)
	if countBefore != countAfter {
		t.Fatal("audit failure committed a funds entry")
	}
	f.balance("CUSTOMER", "-10.00")
	f.funds("refunds", "CUSTOMER", "10.00")
	f.balance("CUSTOMER", "0.00")
	inventoryData[any](t, serveJSON(router, http.MethodPost, refundPath, f.fundsBody("CUSTOMER", "1.00", "zero-balance"), f.token), 409)
	// Cancelling the return preserves actual funds, and the new debt can be paid.
	f.cancel("sale-returns", secondReturn)
	f.balance("CUSTOMER", "80.00")
	f.funds("settlements", "CUSTOMER", "80.00")
	reversePath := fmt.Sprintf("/api/v1/partner-balances/entries/%d/reverse", first.ID)
	inventoryData[any](t, serveJSON(router, http.MethodPost, reversePath, `{"reason":""}`, f.token), 400)
	type reverseResult struct {
		code int
		body []byte
	}
	reverseResults := make(chan reverseResult, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			response := serveJSON(router, http.MethodPost, reversePath, `{"reason":"退款金额录错"}`, f.token)
			reverseResults <- reverseResult{response.Code, response.Body.Bytes()}
		}()
	}
	wg.Wait()
	close(reverseResults)
	reverseCodes := map[int]int{}
	var reversed receivable.Entry
	for result := range reverseResults {
		reverseCodes[result.code]++
		if result.code == 200 {
			var envelope struct {
				Data receivable.Entry `json:"data"`
			}
			if err := json.Unmarshal(result.body, &envelope); err != nil {
				t.Fatal(err)
			}
			reversed = envelope.Data
		}
	}
	if reverseCodes[200] != 1 || reverseCodes[409] != 1 {
		t.Fatalf("concurrent reversal statuses=%v", reverseCodes)
	}
	if reversed.Amount != "-30.00" || reversed.BalanceAfter != "-30.00" || reversed.ReversesID == nil || *reversed.ReversesID != first.ID {
		t.Fatalf("refund reversal=%+v", reversed)
	}
	inventoryData[any](t, serveJSON(router, http.MethodPost, reversePath, `{"reason":"重复冲销"}`, f.token), 409)
	detail := inventoryData[receivable.Entry](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/partner-balances/entries/%d", first.ID), "", f.token), 200)
	if detail.Amount != "30.00" || detail.ReversedByID == nil || *detail.ReversedByID != reversed.ID {
		t.Fatalf("original funds modified=%+v", detail)
	}
	f.funds("refunds", "CUSTOMER", "30.00")
	f.cancel("sale-returns", firstReturn)
	f.balance("CUSTOMER", "20.00")
	f.funds("settlements", "CUSTOMER", "20.00")
	// Cancelling a fully collected original transaction also creates legal pending refunds.
	f.cancel("sales", sale)
	f.balance("CUSTOMER", "-100.00")
	f.funds("refunds", "CUSTOMER", "100.00")
	// Supplier side is independent, and real purchase returns symmetrically form pending refunds.
	f.balance("SUPPLIER", "200.00")
	f.funds("settlements", "SUPPLIER", "200.00")
	purchaseReturn := f.returnGoods("purchase-returns", purchase, "1")
	f.balance("SUPPLIER", "-100.00")
	supplierRefund := f.funds("refunds", "SUPPLIER", "100.00")
	if supplierRefund.EntryType != "SUPPLIER_REFUND" {
		t.Fatal(supplierRefund)
	}
	f.cancel("purchase-returns", purchaseReturn)
	f.balance("SUPPLIER", "100.00")
	f.funds("settlements", "SUPPLIER", "100.00")
	f.cancel("purchases", purchase)
	f.balance("SUPPLIER", "-200.00")
	// A new correct purchase offsets pending refunds before another refund.
	f.trade("purchases", "1", "50.00")
	f.balance("SUPPLIER", "-150.00")
	if err := db.GORM.Exec("UPDATE partner SET status=0,is_supplier=FALSE WHERE id=?", f.partner).Error; err != nil {
		t.Fatal(err)
	}
	f.funds("refunds", "SUPPLIER", "150.00")
	f.balance("CUSTOMER", "0.00")
	f.balance("SUPPLIER", "0.00")
	// Legacy storage is represented by public creations, followed by the one
	// historical fixture alteration needed to emulate the old empty description.
	legacyPartner := f.id("/api/v1/partners", `{"code":"LEGACY-1","name":"旧收款幂等","type":"COMPANY","isCustomer":true}`)
	f.post(openingPath, fmt.Sprintf(`{"requestKey":"legacy-opening","partnerId":%d,"direction":"CUSTOMER","amount":"10.00","businessDate":"2026-01-01","description":"旧往来"}`, legacyPartner))
	for i, description := range []string{"", "往来结算"} {
		key := fmt.Sprintf("legacy-empty-%d", i)
		legacyBody := fmt.Sprintf(`{"requestKey":%q,"partnerId":%d,"direction":"CUSTOMER","amount":"1.00","businessDate":"2026-01-01","paymentMethod":"CASH"}`, key, legacyPartner)
		original := inventoryData[receivable.Entry](t, serveJSON(router, http.MethodPost, "/api/v1/partner-balances/settlements", legacyBody, f.token), 200)
		if err := db.GORM.Exec("UPDATE partner_balance_entry SET description=? WHERE id=?", description, original.ID).Error; err != nil {
			t.Fatal(err)
		}
		result := inventoryData[receivable.Entry](t, serveJSON(router, http.MethodPost, "/api/v1/partner-balances/settlements", legacyBody, f.token), 200)
		if result.ID != original.ID || result.Description != description {
			t.Fatal("legacy retry rewrote the entry")
		}
		inventoryData[any](t, serveJSON(router, http.MethodPost, "/api/v1/partner-balances/settlements", strings.TrimSuffix(legacyBody, "}")+`,"remark":"changed"}`, f.token), 409)
	}

}

func assertPartnerStatementHTTPContract(t *testing.T, router http.Handler, db *platformdatabase.Database) {
	f := newFundsFixture(t, router)
	const entriesPath = "/api/v1/partner-balances/entries"
	const statementPath = "/api/v1/partner-balances/statement"
	assertUnauthenticated(t, serveJSON(router, http.MethodGet, entriesPath, "", ""))
	assertUnauthenticated(t, serveJSON(router, http.MethodGet, statementPath, "", ""))
	// Real HTTP effects exercise every source type, including transaction and
	// return cancellations and the two actual-funds directions.
	purchase := f.trade("purchases", "3", "100.00")
	sale := f.trade("sales", "1", "100.00")
	f.funds("settlements", "CUSTOMER", "100.00")
	saleReturn := f.returnGoods("sale-returns", sale, "1")
	refund := f.funds("refunds", "CUSTOMER", "100.00")
	f.cancel("sale-returns", saleReturn)
	f.post(fmt.Sprintf("%s/%d/reverse", entriesPath, refund.ID), `{"reason":"纠正退款"}`)
	f.cancel("sales", sale)
	f.funds("refunds", "CUSTOMER", "100.00")
	f.funds("settlements", "SUPPLIER", "300.00")
	purchaseReturn := f.returnGoods("purchase-returns", purchase, "1")
	f.funds("refunds", "SUPPLIER", "100.00")
	f.cancel("purchase-returns", purchaseReturn)
	f.cancel("purchases", purchase)
	all := inventoryData[receivable.Page](t, serveJSON(router, http.MethodGet, fmt.Sprintf("%s?partnerId=%d&page=1&pageSize=500", entriesPath, f.partner), "", f.token), 200)
	sources := map[string]bool{}
	for _, entry := range all.Records {
		sources[entry.EntryType] = true
		if entry.OperatorID <= 0 || entry.OperatorName == "" || entry.PartnerName == "" || len(entry.BusinessDate) != 10 {
			t.Fatalf("missing source/operator identity=%+v", entry)
		}
		if entry.EntryType == "SALE_RETURN" && (entry.SaleReturnID == nil || *entry.SaleReturnID != saleReturn) {
			t.Fatal("sale return source missing")
		}
		if entry.EntryType == "PURCHASE_RETURN" && (entry.PurchaseReturnID == nil || *entry.PurchaseReturnID != purchaseReturn) {
			t.Fatal("purchase return source missing")
		}
		if entry.ReversesID != nil {
			original := inventoryData[receivable.Entry](t, serveJSON(router, http.MethodGet, fmt.Sprintf("%s/%d", entriesPath, *entry.ReversesID), "", f.token), 200)
			if entry.ReversedDocumentNo != original.DocumentNo || !sameOptionalID(entry.PurchaseID, original.PurchaseID) || !sameOptionalID(entry.SaleID, original.SaleID) || !sameOptionalID(entry.PurchaseReturnID, original.PurchaseReturnID) || !sameOptionalID(entry.SaleReturnID, original.SaleReturnID) {
				t.Fatalf("reverse source not inherited=%+v original=%+v", entry, original)
			}
		}
	}
	for _, kind := range []string{"PURCHASE", "SALE", "RECEIPT", "PAYMENT", "PURCHASE_RETURN", "SALE_RETURN", "CUSTOMER_REFUND", "SUPPLIER_REFUND", "REVERSAL"} {
		if !sources[kind] {
			t.Fatalf("missing real source %s", kind)
		}
	}
	negativeStatement := inventoryData[receivable.Statement](t, serveJSON(router, http.MethodGet, fmt.Sprintf("%s?partnerId=%d&direction=SUPPLIER&from=2010-01-01T00:00:00Z&to=2040-01-01T00:00:00Z", statementPath, f.partner), "", f.token), 200)
	if negativeStatement.OpeningAmount != "0.00" || negativeStatement.IncreaseAmount != "500.00" || negativeStatement.DecreaseAmount != "700.00" || negativeStatement.ClosingAmount != "-200.00" {
		t.Fatalf("negative supplier statement=%+v", negativeStatement)
	}
	// Use a separate unit to pin timestamps at exact boundaries and the same
	// second. The records themselves are created only through public HTTP APIs.
	history := f.id("/api/v1/partners", `{"code":"HISTORY-1","name":"历史区间单位","type":"COMPANY","isCustomer":true,"isSupplier":true}`)
	var created []receivable.Entry
	for i := 0; i < 42; i++ {
		created = append(created, inventoryData[receivable.Entry](t, serveJSON(router, http.MethodPost, openingPath, fmt.Sprintf(`{"requestKey":"historical-%d","partnerId":%d,"direction":"CUSTOMER","amount":"1.00","businessDate":"2020-01-01","description":"跨期补录"}`, i, history), f.token), 200))
	}
	from := time.Date(2026, 9, 29, 16, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	for i, entry := range created {
		at := from
		switch {
		case i == 0:
			at = from.Add(-time.Microsecond)
		case i == 41:
			at = to
		}
		if err := db.GORM.Exec("UPDATE partner_balance_entry SET effective_at=? WHERE id=?", at, entry.ID).Error; err != nil {
			t.Fatal(err)
		}
	}
	query := url.Values{"partnerId": {fmt.Sprint(history)}, "direction": {"CUSTOMER"}, "from": {from.In(time.FixedZone("local", 8*3600)).Format(time.RFC3339Nano)}, "to": {to.Format(time.RFC3339Nano)}}
	statement := inventoryData[receivable.Statement](t, serveJSON(router, http.MethodGet, statementPath+"?"+query.Encode(), "", f.token), 200)
	if statement.OpeningAmount != "1.00" || statement.IncreaseAmount != "40.00" || statement.DecreaseAmount != "0.00" || statement.NetChange != "40.00" || statement.ClosingAmount != "41.00" || len(statement.Records) != 40 {
		t.Fatalf("half-open historical totals=%+v", statement)
	}
	if statement.From.Location() != time.UTC || !statement.From.Equal(from) {
		t.Fatalf("RFC3339 offset did not normalize=%v", statement.From)
	}
	var ids []int64
	for page := 1; page <= 6; page++ {
		q := url.Values{}
		for k, v := range query {
			q[k] = v
		}
		q.Set("page", fmt.Sprint(page))
		q.Set("pageSize", "7")
		entries := inventoryData[receivable.Page](t, serveJSON(router, http.MethodGet, entriesPath+"?"+q.Encode(), "", f.token), 200)
		if entries.Total != 40 {
			t.Fatal(entries)
		}
		for _, entry := range entries.Records {
			ids = append(ids, entry.ID)
		}
	}
	if len(ids) != 40 {
		t.Fatalf("pagination truncated=%d", len(ids))
	}
	for i, id := range ids {
		if id != created[i+1].ID || id != statement.Records[i].ID {
			t.Fatalf("same-second stable order mismatch index=%d id=%d", i, id)
		}
	}
	huge := query.Encode() + "&page=9223372036854775807&pageSize=500"
	empty := inventoryData[receivable.Page](t, serveJSON(router, http.MethodGet, entriesPath+"?"+huge, "", f.token), 200)
	if len(empty.Records) != 0 || empty.Total != 40 {
		t.Fatal(empty)
	}
	// Later reversal changes today's balance, while the old statement's financial
	// data and original sources remain the same. Business dates never shift it.
	f.post(fmt.Sprintf("%s/%d/reverse", entriesPath, created[1].ID), `{"reason":"后续冲销"}`)
	f.post(openingPath, fmt.Sprintf(`{"requestKey":"future-backdated-opening","partnerId":%d,"direction":"CUSTOMER","amount":"100.00","businessDate":"2020-01-01","description":"事后补录不倒签"}`, history))
	old := inventoryData[receivable.Statement](t, serveJSON(router, http.MethodGet, statementPath+"?"+query.Encode(), "", f.token), 200)
	if old.OpeningAmount != statement.OpeningAmount || old.ClosingAmount != statement.ClosingAmount || old.NetChange != statement.NetChange || len(old.Records) != 40 {
		t.Fatalf("future reversal rewrote past financial period=%+v", old)
	}
	if err := db.GORM.Exec("UPDATE partner SET name='当前档案改名',status=0,is_customer=FALSE WHERE id=?", history).Error; err != nil {
		t.Fatal(err)
	}
	historical := inventoryData[receivable.Statement](t, serveJSON(router, http.MethodGet, statementPath+"?"+query.Encode(), "", f.token), 200)
	if historical.PartnerName != "当前档案改名" || historical.ClosingAmount != "41.00" {
		t.Fatal(historical)
	}
	balances := inventoryData[receivable.BalancePage](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/partner-balances?partnerId=%d&direction=CUSTOMER", history), "", f.token), 200)
	if len(balances.Records) != 1 || balances.Records[0].Amount != "141.00" {
		t.Fatal(balances)
	}
	// Direction separation and genuinely empty historical statements are both
	// available without deriving an opening amount from today's balance.
	q := url.Values{}
	for k, v := range query {
		q[k] = v
	}
	q.Set("direction", "SUPPLIER")
	supplier := inventoryData[receivable.Statement](t, serveJSON(router, http.MethodGet, statementPath+"?"+q.Encode(), "", f.token), 200)
	if supplier.OpeningAmount != "0.00" || supplier.ClosingAmount != "0.00" || len(supplier.Records) != 0 {
		t.Fatal(supplier)
	}
	q.Set("direction", "CUSTOMER")
	q.Set("from", "2010-01-01T00:00:00Z")
	q.Set("to", "2011-01-01T00:00:00Z")
	past := inventoryData[receivable.Statement](t, serveJSON(router, http.MethodGet, statementPath+"?"+q.Encode(), "", f.token), 200)
	if past.OpeningAmount != "0.00" || past.ClosingAmount != "0.00" {
		t.Fatal(past)
	}
	for _, bad := range []string{"?partnerId=1&direction=CUSTOMER&from=2026-01-01&to=2026-02-01", "?partnerId=1&direction=CUSTOMER&from=2026-02-01T00:00:00Z&to=2026-01-01T00:00:00Z", "?partnerId=1&direction=OTHER&from=2026-01-01T00:00:00Z&to=2026-02-01T00:00:00Z"} {
		inventoryData[any](t, serveJSON(router, http.MethodGet, statementPath+bad, "", f.token), 400)
	}
	// Assert responses remain ordinary JSON envelopes, with an explicit empty
	// array (rather than null) for a period with no movements.
	raw := serveJSON(router, http.MethodGet, statementPath+"?"+q.Encode(), "", f.token)
	var envelope map[string]any
	if err := json.Unmarshal(raw.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if _, ok := envelope["data"].(map[string]any)["records"].([]any); !ok {
		t.Fatal("empty statement records was not an array")
	}
}
func sameOptionalID(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
