//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"github.com/EziosWJ/simple-inventory/server/internal/receivable"
)

const openingPath = "/api/v1/partner-balances/opening"

func TestSQLiteOpeningBalanceContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opening.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	deps := sqliteDependencies(t, db, t.TempDir())
	deps.Receivable = receivable.NewService(receivable.NewRepository(db.GORM))
	router, e := app.Build(testAPIConfig(), db, deps)
	if e != nil {
		t.Fatal(e)
	}
	assertOpeningBalanceContract(t, router, db)
}
func TestPostgresOpeningBalanceContract(t *testing.T) {
	if _, e := exec.LookPath("docker"); e != nil {
		t.Skip("Docker is required for PostgreSQL integration tests")
	}
	pg := startPostgres(t)
	runMigrations(t, projectRoot(t), pg.dsn)
	db := openTemporaryDatabase(t, pg.dsn)
	defer db.Close()
	deps := testDependencies(t, db, t.TempDir())
	deps.Receivable = receivable.NewService(receivable.NewRepository(db.GORM))
	router, e := app.Build(testAPIConfig(), db, deps)
	if e != nil {
		t.Fatal(e)
	}
	assertOpeningBalanceContract(t, router, db)
}
func assertOpeningBalanceContract(t *testing.T, router http.Handler, db *platformdatabase.Database) {
	t.Helper()
	token := loginAdmin(t, router)
	assertUnauthenticated(t, serveJSON(router, http.MethodPost, openingPath, `{}`, ""))
	var partnerID int64
	// A single record can hold both identities; directions remain independent.
	if err := db.GORM.Exec("INSERT INTO partner(code,name,type,is_customer,is_supplier,status) VALUES ('OPEN-1','期初往来单位','COMPANY',TRUE,TRUE,1)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Table("partner").Select("id").Where("code=?", "OPEN-1").Scan(&partnerID).Error; err != nil {
		t.Fatal(err)
	}
	keyCounter := 0
	createBody := func(direction, amount, key string) string {
		return fmt.Sprintf(`{"requestKey":%q,"partnerId":%d,"direction":%q,"amount":%q,"businessDate":"2026-09-30","description":"接入前余额"}`, key, partnerID, direction, amount)
	}
	create := func(direction, amount string) map[string]any {
		keyCounter++
		body := createBody(direction, amount, fmt.Sprintf("opening-%d", keyCounter))
		return inventoryData[map[string]any](t, serveJSON(router, http.MethodPost, openingPath, body, token), 200)
	}
	firstBody := createBody("CUSTOMER", "1200.05", "opening-retry")
	first := inventoryData[map[string]any](t, serveJSON(router, http.MethodPost, openingPath, firstBody, token), 200)
	retried := inventoryData[map[string]any](t, serveJSON(router, http.MethodPost, openingPath, firstBody, token), 200)
	if retried["id"] != first["id"] {
		t.Fatalf("idempotent retry created a second record: %#v %#v", first, retried)
	}
	changedPayload := createBody("CUSTOMER", "1200.06", "opening-retry")
	inventoryData[any](t, serveJSON(router, http.MethodPost, openingPath, changedPayload, token), 409)
	if first["amount"] != "1200.05" || first["balanceAfter"] != "1200.05" {
		t.Fatalf("opening amount lost precision: %#v", first)
	}
	detail := inventoryData[map[string]any](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/partner-balances/entries/%v", first["id"]), "", token), 200)
	if detail["documentNo"] != first["documentNo"] {
		t.Fatalf("entry detail mismatch %#v", detail)
	}
	second := create("CUSTOMER", "10.25")
	if second["balanceAfter"] != "1210.30" {
		t.Fatalf("cumulative balance=%v", second["balanceAfter"])
	}
	supplier := create("SUPPLIER", "100.00")
	if supplier["balanceAfter"] != "100.00" {
		t.Fatal(supplier)
	}
	for _, pageNo := range []int{1, 2} {
		path := fmt.Sprintf("/api/v1/partner-balances/entries?partnerId=%d&direction=CUSTOMER&category=OPENING&page=%d&pageSize=1", partnerID, pageNo)
		page := inventoryData[receivable.Page](t, serveJSON(router, http.MethodGet, path, "", token), 200)
		if page.Total != 2 || len(page.Records) != 1 || page.Records[0].EntryType != "OPENING" {
			t.Fatalf("opening page %d does not preserve filtered total: %+v", pageNo, page)
		}
	}
	var wg sync.WaitGroup
	errors := make(chan string, 2)
	for _, amount := range []string{"1.00", "2.00"} {
		wg.Add(1)
		go func(amount string) {
			defer wg.Done()
			body := createBody("SUPPLIER", amount, "concurrent-"+amount)
			response := serveJSON(router, http.MethodPost, openingPath, body, token)
			if response.Code != 200 {
				errors <- response.Body.String()
			}
		}(amount)
	}
	wg.Wait()
	close(errors)
	for e := range errors {
		t.Errorf("concurrent opening failed: %s", e)
	}
	var ledgerCents, balanceCents int64
	if err := db.GORM.Table("partner_balance_entry").Select("COALESCE(SUM(amount_cents),0)").Where("partner_id=? AND direction='SUPPLIER'", partnerID).Scan(&ledgerCents).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Table("partner_balance").Select("amount_cents").Where("partner_id=? AND direction='SUPPLIER'", partnerID).Scan(&balanceCents).Error; err != nil {
		t.Fatal(err)
	}
	if ledgerCents != balanceCents || balanceCents != 10300 {
		t.Fatalf("concurrent ledger/balance mismatch: ledger=%d balance=%d", ledgerCents, balanceCents)
	}
	settlePath := "/api/v1/partner-balances/settlements"
	settleBody := fmt.Sprintf(`{"requestKey":"settlement-once","partnerId":%d,"direction":"SUPPLIER","amount":"60.00","businessDate":"2026-09-30","paymentMethod":"WECHAT","transactionNo":"WX-123","remark":"分次付款"}`, partnerID)
	settled := inventoryData[map[string]any](t, serveJSON(router, http.MethodPost, settlePath, settleBody, token), 200)
	if settled["entryType"] != "PAYMENT" || settled["amount"] != "-60.00" || settled["balanceBefore"] != "103.00" || settled["balanceAfter"] != "43.00" || settled["paymentMethod"] != "WECHAT" || settled["transactionNo"] != "WX-123" {
		t.Fatalf("unexpected settlement: %#v", settled)
	}
	retriedSettlement := inventoryData[map[string]any](t, serveJSON(router, http.MethodPost, settlePath, settleBody, token), 200)
	if retriedSettlement["id"] != settled["id"] {
		t.Fatalf("settlement retry duplicated record: %#v %#v", settled, retriedSettlement)
	}
	overpay := fmt.Sprintf(`{"requestKey":"settlement-over","partnerId":%d,"direction":"SUPPLIER","amount":"43.01","businessDate":"2026-09-30","paymentMethod":"CASH"}`, partnerID)
	inventoryData[any](t, serveJSON(router, http.MethodPost, settlePath, overpay, token), 409)
	settlementReversePath := fmt.Sprintf("/api/v1/partner-balances/entries/%v/reverse", settled["id"])
	reversal := inventoryData[map[string]any](t, serveJSON(router, http.MethodPost, settlementReversePath, `{"reason":"付款金额录错"}`, token), 200)
	if reversal["amount"] != "60.00" || reversal["balanceAfter"] != "103.00" {
		t.Fatalf("settlement reversal did not restore debt: %#v", reversal)
	}
	inventoryData[any](t, serveJSON(router, http.MethodPost, settlementReversePath, `{"reason":"再次冲销"}`, token), 409)
	var settleWG sync.WaitGroup
	settleResults := make(chan int, 2)
	for i := 0; i < 2; i++ {
		settleWG.Add(1)
		go func(i int) {
			defer settleWG.Done()
			body := fmt.Sprintf(`{"requestKey":"settlement-race-%d","partnerId":%d,"direction":"SUPPLIER","amount":"60.00","businessDate":"2026-09-30","paymentMethod":"CASH"}`, i, partnerID)
			settleResults <- serveJSON(router, http.MethodPost, settlePath, body, token).Code
		}(i)
	}
	settleWG.Wait()
	close(settleResults)
	twins := map[int]int{}
	for code := range settleResults {
		twins[code]++
	}
	if twins[200] != 1 || twins[409] != 1 {
		t.Fatalf("concurrent settlements did not enforce total balance: %#v", twins)
	}
	for _, amount := range []string{"0.00", "-1.00", "1.001", "92233720368547758.08"} {
		body := fmt.Sprintf(`{"partnerId":%d,"direction":"CUSTOMER","amount":%q,"businessDate":"2026-09-30","description":"bad"}`, partnerID, amount)
		inventoryData[any](t, serveJSON(router, http.MethodPost, openingPath, body, token), 400)
	}
	reverse := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/partner-balances/entries/%v/reverse", first["id"]), `{"reason":"金额录错"}`, token)
	inventoryData[map[string]any](t, reverse, 200)
	inventoryData[any](t, serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/partner-balances/entries/%v/reverse", first["id"]), `{"reason":"重复冲销"}`, token), 409)
	page := inventoryData[map[string]any](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/partner-balances/entries?partnerId=%d&direction=CUSTOMER&page=1&pageSize=20", partnerID), "", token), 200)
	if page["total"] != float64(3) {
		t.Fatalf("entry count after reversal=%v", page["total"])
	}
	if err := db.GORM.Exec("INSERT INTO partner(code,name,type,is_customer,is_supplier,status) VALUES ('OPEN-ZERO','已清零往来','COMPANY',TRUE,FALSE,1),('OPEN-EMPTY','暂无往来','COMPANY',TRUE,FALSE,1)").Error; err != nil {
		t.Fatal(err)
	}
	var zeroID, emptyID int64
	if err := db.GORM.Table("partner").Select("id").Where("code='OPEN-ZERO'").Scan(&zeroID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Table("partner").Select("id").Where("code='OPEN-EMPTY'").Scan(&emptyID).Error; err != nil {
		t.Fatal(err)
	}
	zeroBody := fmt.Sprintf(`{"requestKey":"opening-zero","partnerId":%d,"direction":"CUSTOMER","amount":"9.00","businessDate":"2026-09-30","description":"清零测试"}`, zeroID)
	zero := inventoryData[map[string]any](t, serveJSON(router, http.MethodPost, openingPath, zeroBody, token), 200)
	inventoryData[map[string]any](t, serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/partner-balances/entries/%v/reverse", zero["id"]), `{"reason":"清零测试冲销"}`, token), 200)
	if err := db.GORM.Exec("UPDATE partner SET is_customer=FALSE WHERE id=?", partnerID).Error; err != nil {
		t.Fatal(err)
	}
	oldDirection := inventoryData[map[string]any](t, serveJSON(router, http.MethodGet, "/api/v1/partner-balances?page=1&pageSize=20&direction=CUSTOMER", "", token), 200)
	records := oldDirection["records"].([]any)
	if len(records) != 3 {
		t.Fatalf("removed customer role hid historical balance: %#v", oldDirection)
	}
	states := map[int64]map[string]any{}
	for _, raw := range records {
		record := raw.(map[string]any)
		states[int64(record["partnerId"].(float64))] = record
	}
	if states[partnerID]["hasRecords"] != true || states[zeroID]["amount"] != "0.00" || states[zeroID]["hasRecords"] != true || states[emptyID]["amount"] != "0.00" || states[emptyID]["hasRecords"] != false {
		t.Fatalf("zero balance and no history were conflated: %#v", states)
	}
	var beforeAuditFailure int64
	if err := db.GORM.Table("partner_balance_entry").Count(&beforeAuditFailure).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Exec("ALTER TABLE sys_oper_log RENAME TO sys_oper_log_unavailable").Error; err != nil {
		t.Fatal(err)
	}
	failedBody := createBody("SUPPLIER", "9.00", "audit-failure")
	response := serveJSON(router, http.MethodPost, openingPath, failedBody, token)
	if response.Code == 200 {
		t.Fatal("opening succeeded after audit storage was removed")
	}
	if err := db.GORM.Exec("ALTER TABLE sys_oper_log_unavailable RENAME TO sys_oper_log").Error; err != nil {
		t.Fatal(err)
	}
	var afterAuditFailure int64
	if err := db.GORM.Table("partner_balance_entry").Count(&afterAuditFailure).Error; err != nil {
		t.Fatal(err)
	}
	if afterAuditFailure != beforeAuditFailure {
		t.Fatalf("audit failure committed balance entry: before=%d after=%d", beforeAuditFailure, afterAuditFailure)
	}
	if err := db.GORM.Table("partner_balance_entry").Select("COALESCE(SUM(amount_cents),0)").Where("partner_id=? AND direction='SUPPLIER'", partnerID).Scan(&ledgerCents).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Table("partner_balance").Select("amount_cents").Where("partner_id=? AND direction='SUPPLIER'", partnerID).Scan(&balanceCents).Error; err != nil {
		t.Fatal(err)
	}
	if ledgerCents != balanceCents {
		t.Fatalf("audit failure left projected balance mismatch: ledger=%d balance=%d", ledgerCents, balanceCents)
	}
}
