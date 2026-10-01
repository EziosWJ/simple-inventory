//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"github.com/EziosWJ/simple-inventory/server/internal/receivable"
)

func TestSQLitePurchasePostingPostgresSQLiteHTTPContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "purchase-posting.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	deps := sqliteDependencies(t, db, t.TempDir())
	deps.Receivable = receivable.NewService(receivable.NewRepository(db.GORM))
	router, e := app.Build(testAPIConfig(), db, deps)
	if e != nil {
		t.Fatal(e)
	}
	assertPurchasePostingContract(t, router, db, "sqlite")
}
func TestPostgresPurchasePostingPostgresSQLiteHTTPContract(t *testing.T) {
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
	assertPurchasePostingContract(t, router, db, "postgres")
}
func assertPurchasePostingContract(t *testing.T, router http.Handler, db *platformdatabase.Database, dialect string) {
	t.Helper()
	token := loginAdmin(t, router)
	partner := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/partners", `{"code":"POST-V1","name":"过账供应商","type":"COMPANY","isSupplier":true}`, token), 200).ID
	product := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/products", `{"code":"POST-G1","name":"空快照商品","type":"GOODS","unit":"台"}`, token), 200).ID
	create := func(qty, price string) int64 {
		body := fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":%q,"unitPrice":%q},{"productId":%d,"productType":"GOODS","unit":"台","quantity":"1","unitPrice":"0.01"}]}`, partner, product, qty, price, product)
		return inventoryData[struct {
			ID      int64 `json:"id"`
			Version int64 `json:"version"`
		}](t, serveJSON(router, http.MethodPost, "/api/v1/purchases", body, token), 200).ID
	}
	first := create("2.125", "10.00")
	second := create("3", "2.00")
	// A failed audit must roll back document state, stock, payable and both ledgers.
	installPurchasePostingAuditFailure(t, db, dialect)
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/purchases/%d/post", first), `{"version":1}`, token); r.Code != 500 {
		t.Fatalf("post audit failure status=%d body=%s", r.Code, r.Body.String())
	}
	assertCount := func(table string, want int64) {
		t.Helper()
		var got int64
		if e := db.GORM.Table(table).Count(&got).Error; e != nil || got != want {
			t.Fatalf("%s count=%d want=%d err=%v", table, got, want, e)
		}
	}
	assertCount("inventory_entry", 0)
	assertCount("partner_balance_entry", 0)
	var balances int64
	if e := db.GORM.Table("inventory_balance").Count(&balances).Error; e != nil || balances != 0 {
		t.Fatalf("audit rollback stock rows=%d err=%v", balances, e)
	}
	if e := dropPurchasePostingAuditFailure(db, dialect); e != nil {
		t.Fatal(e)
	}
	// Concurrent posts serialize on the first supplier balance and product ID.
	responses := make(chan int, 2)
	start := make(chan struct{})
	for _, id := range []int64{first, second} {
		go func(id int64) {
			<-start
			responses <- serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/purchases/%d/post", id), `{"version":1}`, token).Code
		}(id)
	}
	close(start)
	counts := map[int]int{}
	for range 2 {
		counts[<-responses]++
	}
	if counts[200] != 2 {
		t.Fatalf("concurrent post results=%v", counts)
	}
	var stock int64
	if e := db.GORM.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", product).Scan(&stock).Error; e != nil || stock != 7125 {
		t.Fatalf("stock=%d want=7125 err=%v", stock, e)
	}
	var payable int64
	if e := db.GORM.Table("partner_balance").Select("amount_cents").Where("partner_id=? AND direction='SUPPLIER'", partner).Scan(&payable).Error; e != nil || payable != 2727 {
		t.Fatalf("payable=%d want=2727 err=%v", payable, e)
	}
	var entrySum int64
	if e := db.GORM.Table("inventory_entry").Select("COALESCE(SUM(quantity_milli),0)").Where("product_id=?", product).Scan(&entrySum).Error; e != nil || entrySum != stock {
		t.Fatalf("inventory ledger sum=%d stock=%d err=%v", entrySum, stock, e)
	}
	var payableSum int64
	if e := db.GORM.Table("partner_balance_entry").Select("COALESCE(SUM(amount_cents),0)").Where("partner_id=? AND direction='SUPPLIER'", partner).Scan(&payableSum).Error; e != nil || payableSum != payable {
		t.Fatalf("payable ledger sum=%d balance=%d err=%v", payableSum, payable, e)
	}
	entries := inventoryData[struct {
		Records []struct {
			SourceType   string  `json:"sourceType"`
			PurchaseID   int64   `json:"purchaseId"`
			ProductModel *string `json:"productModel"`
		} `json:"records"`
	}](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/inventory/entries?productId=%d", product), "", token), 200)
	if len(entries.Records) != 4 || entries.Records[0].SourceType != "PURCHASE" || entries.Records[0].PurchaseID == 0 || entries.Records[0].ProductModel != nil {
		t.Fatalf("purchase source/snapshot missing: %+v", entries.Records)
	}
	// Cancel one purchase after removing its stock: the whole reversal must refuse.
	adjustment := serveJSON(router, http.MethodPost, "/api/v1/inventory/adjustments", fmt.Sprintf(`{"items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"-6","reason":"DAMAGE"}]}`, product), token)
	if adjustment.Code != 200 {
		t.Fatalf("create shortage adjustment=%d %s", adjustment.Code, adjustment.Body.String())
	}
	var ad struct {
		ID      int64 `json:"id"`
		Version int64 `json:"version"`
	}
	ad = inventoryData[struct {
		ID      int64 `json:"id"`
		Version int64 `json:"version"`
	}](t, adjustment, 200)
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/inventory/adjustments/%d/post", ad.ID), fmt.Sprintf(`{"version":%d}`, ad.Version), token); r.Code != 200 {
		t.Fatalf("post shortage adjustment=%d %s", r.Code, r.Body.String())
	}
	var firstDetail struct {
		Version int64  `json:"version"`
		Status  string `json:"status"`
	}
	firstDetail = inventoryData[struct {
		Version int64  `json:"version"`
		Status  string `json:"status"`
	}](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/purchases/%d", first), "", token), 200)
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/purchases/%d/cancel", first), fmt.Sprintf(`{"version":%d,"reason":"误录"}`, firstDetail.Version), token); r.Code != 409 {
		t.Fatalf("insufficient-stock cancel=%d %s", r.Code, r.Body.String())
	}
	var stillPosted string
	if e := db.GORM.Table("purchase_document").Select("status").Where("id=?", first).Scan(&stillPosted).Error; e != nil || stillPosted != "POSTED" {
		t.Fatalf("failed cancellation changed state=%q err=%v", stillPosted, e)
	}
	// Description changes do not rewrite either the purchase snapshot or its
	// stock entries, and inventory history permanently locks type and unit.
	productPath := fmt.Sprintf("/api/v1/products/%d", product)
	renamed := serveJSON(router, http.MethodPut, productPath, `{"name":"改名后的商品","type":"GOODS","unit":"台"}`, token)
	if renamed.Code != 200 {
		t.Fatalf("rename posted product=%d %s", renamed.Code, renamed.Body.String())
	}
	if r := serveJSON(router, http.MethodPut, productPath, `{"name":"改类型","type":"SERVICE","unit":"台"}`, token); r.Code != 409 {
		t.Fatalf("posted product type edit=%d %s", r.Code, r.Body.String())
	}
	if r := serveJSON(router, http.MethodPut, productPath, `{"name":"改单位","type":"GOODS","unit":"件"}`, token); r.Code != 409 {
		t.Fatalf("posted product unit edit=%d %s", r.Code, r.Body.String())
	}
	var frozen struct {
		Items []struct {
			ProductName  string  `json:"productName"`
			ProductModel *string `json:"productModel"`
		} `json:"items"`
	}
	frozen = inventoryData[struct {
		Items []struct {
			ProductName  string  `json:"productName"`
			ProductModel *string `json:"productModel"`
		} `json:"items"`
	}](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/purchases/%d", first), "", token), 200)
	if frozen.Items[0].ProductName != "空快照商品" || frozen.Items[0].ProductModel != nil {
		t.Fatalf("purchase snapshot changed: %+v", frozen)
	}
	stockAdjustment := inventoryData[struct {
		ID      int64 `json:"id"`
		Version int64 `json:"version"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/inventory/adjustments", fmt.Sprintf(`{"items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"3.125","reason":"SURPLUS"}]}`, product), token), 200)
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/inventory/adjustments/%d/post", stockAdjustment.ID), fmt.Sprintf(`{"version":%d}`, stockAdjustment.Version), token); r.Code != 200 {
		t.Fatalf("replenish stock=%d %s", r.Code, r.Body.String())
	}
	settlement := serveJSON(router, http.MethodPost, "/api/v1/partner-balances/settlements", fmt.Sprintf(`{"requestKey":"purchase-cancel-%s","partnerId":%d,"direction":"SUPPLIER","amount":"27.27","businessDate":"2026-10-01","paymentMethod":"BANK_TRANSFER","transactionNo":"BANK-1","remark":"实际付款"}`, dialect, partner), token)
	if settlement.Code != 200 {
		t.Fatalf("retain supplier payment status=%d body=%s", settlement.Code, settlement.Body.String())
	}
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/purchases/%d/cancel", first), `{"version":2,"reason":"误录"}`, token); r.Code != 200 {
		t.Fatalf("posted purchase cancellation=%d %s", r.Code, r.Body.String())
	}
	var finalPayable, finalEntries int64
	if e := db.GORM.Table("partner_balance").Select("amount_cents").Where("partner_id=? AND direction='SUPPLIER'", partner).Scan(&finalPayable).Error; e != nil || finalPayable != -2126 {
		t.Fatalf("cancel payable=%d want=-2126 err=%v", finalPayable, e)
	}
	if e := db.GORM.Table("partner_balance_entry").Select("COALESCE(SUM(amount_cents),0)").Where("partner_id=? AND direction='SUPPLIER'", partner).Scan(&finalEntries).Error; e != nil || finalEntries != finalPayable {
		t.Fatalf("payable entries=%d balance=%d err=%v", finalEntries, finalPayable, e)
	}
	var payments, reversals int64
	if e := db.GORM.Table("partner_balance_entry").Where("partner_id=? AND entry_type='PAYMENT'", partner).Count(&payments).Error; e != nil || payments != 1 {
		t.Fatalf("retained payments=%d err=%v", payments, e)
	}
	if e := db.GORM.Table("partner_balance_entry").Where("partner_id=? AND entry_type='REVERSAL'", partner).Count(&reversals).Error; e != nil || reversals != 1 {
		t.Fatalf("purchase reversals=%d err=%v", reversals, e)
	}
	if e := db.GORM.Table("inventory_entry").Select("COALESCE(SUM(quantity_milli),0)").Where("product_id=?", product).Scan(&entrySum).Error; e != nil || entrySum != 1125 {
		t.Fatalf("cancel stock ledger=%d want=1125 err=%v", entrySum, e)
	}
}

func installPurchasePostingAuditFailure(t *testing.T, db *platformdatabase.Database, dialect string) {
	t.Helper()
	var s string
	if dialect == "sqlite" {
		s = `CREATE TRIGGER purchase_posting_audit_failure BEFORE INSERT ON sys_oper_log WHEN NEW.operation_type='purchase.post' BEGIN SELECT RAISE(ABORT,'audit unavailable'); END`
	} else {
		s = `CREATE FUNCTION reject_purchase_posting_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.operation_type='purchase.post' THEN RAISE EXCEPTION 'audit unavailable'; END IF; RETURN NEW; END $$`
		if e := db.GORM.Exec(s).Error; e != nil {
			t.Fatal(e)
		}
		s = `CREATE TRIGGER purchase_posting_audit_failure BEFORE INSERT ON sys_oper_log FOR EACH ROW EXECUTE FUNCTION reject_purchase_posting_audit()`
	}
	if e := db.GORM.Exec(s).Error; e != nil {
		t.Fatal(e)
	}
}
func dropPurchasePostingAuditFailure(db *platformdatabase.Database, dialect string) error {
	if dialect == "sqlite" {
		return db.GORM.Exec("DROP TRIGGER purchase_posting_audit_failure").Error
	}
	if e := db.GORM.Exec("DROP TRIGGER purchase_posting_audit_failure ON sys_oper_log").Error; e != nil {
		return e
	}
	return db.GORM.Exec("DROP FUNCTION reject_purchase_posting_audit()").Error
}
