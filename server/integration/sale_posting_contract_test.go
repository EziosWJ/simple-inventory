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

func TestSQLiteSalePostingPostgresSQLiteHTTPContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sale-posting.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	deps := sqliteDependencies(t, db, t.TempDir())
	deps.Receivable = receivable.NewService(receivable.NewRepository(db.GORM))
	router, e := app.Build(testAPIConfig(), db, deps)
	if e != nil {
		t.Fatal(e)
	}
	assertSalePostingContract(t, router, db, "sqlite")
}
func TestPostgresSalePostingPostgresSQLiteHTTPContract(t *testing.T) {
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
	assertSalePostingContract(t, router, db, "postgres")
}

func assertSalePostingContract(t *testing.T, router http.Handler, db *platformdatabase.Database, dialect string) {
	t.Helper()
	token := loginAdmin(t, router)
	assertUnauthenticated(t, serveJSON(router, http.MethodPost, "/api/v1/sales/1/post", `{"version":1}`, ""))
	partner := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/partners", `{"code":"SPOST-C1","name":"过账客户","type":"COMPANY","isCustomer":true,"contact":"档案联系人","phone":"10086","address":"档案地址"}`, token), 200).ID
	goods := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/products", `{"code":"SPOST-G1","name":"打印机","type":"GOODS","unit":"台"}`, token), 200).ID
	service := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/products", `{"code":"SPOST-S1","name":"安装","type":"SERVICE","unit":"次"}`, token), 200).ID
	if r := serveJSON(router, http.MethodPut, "/api/v1/print-profile", `{"name":"个体经营者","phone":"13900000000","address":"经营者地址"}`, token); r.Code != 200 {
		t.Fatalf("print profile=%d %s", r.Code, r.Body.String())
	}
	opening := inventoryData[struct {
		ID      int64 `json:"id"`
		Version int64 `json:"version"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/inventory/adjustments", fmt.Sprintf(`{"items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"5","reason":"OPENING"}]}`, goods), token), 200)
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/inventory/adjustments/%d/post", opening.ID), fmt.Sprintf(`{"version":%d}`, opening.Version), token); r.Code != 200 {
		t.Fatalf("opening post=%d %s", r.Code, r.Body.String())
	}
	body := fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-10-01","deliveryContact":"本次联系人","deliveryPhone":"本次电话","deliveryAddress":"本次地址","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"1.5","unitPrice":"10.00"},{"productId":%d,"productType":"GOODS","unit":"台","quantity":"1","unitPrice":"0.00"},{"productId":%d,"productType":"SERVICE","unit":"次","quantity":"2","unitPrice":"10.00"}]}`, partner, goods, goods, service)
	draft := inventoryData[struct {
		ID      int64  `json:"id"`
		Version int64  `json:"version"`
		Status  string `json:"status"`
		Total   string `json:"totalAmount"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/sales", body, token), 200)
	if draft.Status != "DRAFT" || draft.Version != 1 || draft.Total != "35.00" {
		t.Fatalf("draft=%+v", draft)
	}
	installSalePostingAuditFailure(t, db, dialect)
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sales/%d/post", draft.ID), `{"version":1}`, token); r.Code != 500 {
		t.Fatalf("audit failure post=%d %s", r.Code, r.Body.String())
	}
	var status string
	if e := db.GORM.Table("sale_document").Select("status").Where("id=?", draft.ID).Scan(&status).Error; e != nil || status != "DRAFT" {
		t.Fatalf("audit rollback status=%q err=%v", status, e)
	}
	var entries int64
	db.GORM.Table("inventory_entry").Count(&entries)
	if entries != 1 {
		t.Fatalf("audit rollback changed inventory ledger: %d", entries)
	}
	if e := dropSalePostingAuditFailure(db, dialect); e != nil {
		t.Fatal(e)
	}
	posted := inventoryData[struct {
		ID              int64   `json:"id"`
		Version         int64   `json:"version"`
		Status          string  `json:"status"`
		PartnerName     string  `json:"partnerName"`
		OwnerName       string  `json:"ownerName"`
		OwnerPhone      string  `json:"ownerPhone"`
		OwnerAddress    string  `json:"ownerAddress"`
		DeliveryContact string  `json:"deliveryContact"`
		DeliveryPhone   string  `json:"deliveryPhone"`
		DeliveryAddress string  `json:"deliveryAddress"`
		PostedAt        *string `json:"postedAt"`
		Items           []struct {
			ID                   int64   `json:"id"`
			ProductType          string  `json:"productType"`
			ProductCode          string  `json:"productCode"`
			ProductName          string  `json:"productName"`
			ProductModel         *string `json:"productModel"`
			ProductSpecification *string `json:"productSpecification"`
			Unit                 string  `json:"unit"`
			Quantity             string  `json:"quantity"`
		} `json:"items"`
	}](t, serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sales/%d/post", draft.ID), `{"version":1}`, token), 200)
	if posted.Status != "POSTED" || posted.Version != 2 || posted.PartnerName != "过账客户" || posted.PostedAt == nil || len(posted.Items) != 3 || posted.Items[0].ProductModel != nil || posted.Items[1].ProductSpecification != nil || posted.OwnerName != "个体经营者" || posted.OwnerPhone != "13900000000" || posted.OwnerAddress != "经营者地址" || posted.DeliveryContact != "本次联系人" || posted.DeliveryPhone != "本次电话" || posted.DeliveryAddress != "本次地址" {
		t.Fatalf("posted snapshot=%+v", posted)
	}
	// Current profile/catalog edits never rewrite the saved business snapshot.
	if e := db.GORM.Table("partner").Where("id=?", partner).Update("name", "档案改名客户").Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GORM.Table("product").Where("id=?", goods).Updates(map[string]any{"code": "SPOST-G1-NEW", "name": "改名打印机", "model": "新型号", "specification": "新规格"}).Error; e != nil {
		t.Fatal(e)
	}
	if r := serveJSON(router, http.MethodPut, "/api/v1/print-profile", `{"name":"改名经营者","phone":"13800000000","address":"新经营者地址"}`, token); r.Code != 200 {
		t.Fatalf("update print profile=%d %s", r.Code, r.Body.String())
	}
	frozen := inventoryData[struct {
		PartnerName     string `json:"partnerName"`
		OwnerName       string `json:"ownerName"`
		OwnerPhone      string `json:"ownerPhone"`
		OwnerAddress    string `json:"ownerAddress"`
		DeliveryPhone   string `json:"deliveryPhone"`
		DeliveryAddress string `json:"deliveryAddress"`
		Items           []struct {
			ProductName  string  `json:"productName"`
			ProductModel *string `json:"productModel"`
		} `json:"items"`
	}](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/sales/%d", draft.ID), "", token), 200)
	if frozen.PartnerName != "过账客户" || frozen.OwnerName != "个体经营者" || frozen.OwnerPhone != "13900000000" || frozen.OwnerAddress != "经营者地址" || frozen.DeliveryPhone != "本次电话" || frozen.DeliveryAddress != "本次地址" || frozen.Items[0].ProductName != "打印机" || frozen.Items[0].ProductModel != nil {
		t.Fatalf("posted snapshot changed after current data edit: %+v", frozen)
	}
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sales/%d/post", draft.ID), `{"version":1}`, token); r.Code != 409 {
		t.Fatalf("duplicate post=%d", r.Code)
	}
	var stock int64
	if e := db.GORM.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", goods).Scan(&stock).Error; e != nil || stock != 2500 {
		t.Fatalf("stock=%d err=%v", stock, e)
	}
	var receivableCents int64
	if e := db.GORM.Table("partner_balance").Select("amount_cents").Where("partner_id=? AND direction='CUSTOMER'", partner).Scan(&receivableCents).Error; e != nil || receivableCents != 3500 {
		t.Fatalf("receivable=%d err=%v", receivableCents, e)
	}
	var saleEntries, serviceEntries int64
	db.GORM.Table("inventory_entry").Where("sale_id=?", draft.ID).Count(&saleEntries)
	db.GORM.Table("inventory_entry").Where("sale_id=? AND product_id=?", draft.ID, service).Count(&serviceEntries)
	if saleEntries != 2 || serviceEntries != 0 {
		t.Fatalf("sale inventory sources=%d service inventory=%d", saleEntries, serviceEntries)
	}
	financial := inventoryData[struct {
		Records []struct {
			EntryType string `json:"entryType"`
			SaleID    int64  `json:"saleId"`
		} `json:"records"`
	}](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/partner-balances/entries?partnerId=%d&direction=CUSTOMER", partner), "", token), 200)
	financialSource := false
	for _, row := range financial.Records {
		if row.EntryType == "SALE" && row.SaleID == draft.ID {
			financialSource = true
		}
	}
	if !financialSource {
		t.Fatalf("sale source is not traceable from receivable entries: %+v", financial.Records)
	}
	ledger := inventoryData[struct {
		Records []struct {
			SourceType string `json:"sourceType"`
			SaleID     int64  `json:"saleId"`
			DocumentNo string `json:"documentNo"`
		} `json:"records"`
	}](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/inventory/entries?productId=%d", goods), "", token), 200)
	found := false
	for _, row := range ledger.Records {
		if row.SourceType == "SALE" && row.SaleID == draft.ID && row.DocumentNo == postedDocNo(t, db, draft.ID) {
			found = true
		}
	}
	if !found {
		t.Fatalf("sale source is not traceable from inventory entries: %+v", ledger.Records)
	}
	// A real receipt remains posted when the sale is cancelled; reversing its
	// amount leaves the customer in refund-pending negative balance.
	settlement := fmt.Sprintf(`{"requestKey":"sale-receipt-%d","partnerId":%d,"direction":"CUSTOMER","amount":"10.00","businessDate":"2026-10-01","paymentMethod":"CASH"}`, draft.ID, partner)
	if r := serveJSON(router, http.MethodPost, "/api/v1/partner-balances/settlements", settlement, token); r.Code != 200 {
		t.Fatalf("receipt=%d %s", r.Code, r.Body.String())
	}
	installSaleCancelAuditFailure(t, db, dialect)
	if r := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sales/%d/cancel", draft.ID), `{"version":2,"reason":"录入错误"}`, token); r.Code != 500 {
		t.Fatalf("cancel audit failure=%d %s", r.Code, r.Body.String())
	}
	if e := db.GORM.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", goods).Scan(&stock).Error; e != nil || stock != 2500 {
		t.Fatalf("audit rollback cancellation stock=%d err=%v", stock, e)
	}
	if e := db.GORM.Table("partner_balance").Select("amount_cents").Where("partner_id=? AND direction='CUSTOMER'", partner).Scan(&receivableCents).Error; e != nil || receivableCents != 2500 {
		t.Fatalf("audit rollback cancellation receivable=%d err=%v", receivableCents, e)
	}
	if e := dropSaleCancelAuditFailure(db, dialect); e != nil {
		t.Fatal(e)
	}
	cancelled := inventoryData[struct {
		Version int64  `json:"version"`
		Status  string `json:"status"`
	}](t, serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sales/%d/cancel", draft.ID), `{"version":2,"reason":"录入错误"}`, token), 200)
	if cancelled.Status != "CANCELLED" || cancelled.Version != 3 {
		t.Fatalf("cancel=%+v", cancelled)
	}
	if e := db.GORM.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", goods).Scan(&stock).Error; e != nil || stock != 5000 {
		t.Fatalf("reversed stock=%d err=%v", stock, e)
	}
	if e := db.GORM.Table("partner_balance").Select("amount_cents").Where("partner_id=? AND direction='CUSTOMER'", partner).Scan(&receivableCents).Error; e != nil || receivableCents != -1000 {
		t.Fatalf("cancelled receivable with receipt=%d err=%v", receivableCents, e)
	}
	var saleSum int64
	if e := db.GORM.Table("partner_balance_entry").Select("COALESCE(SUM(amount_cents),0)").Where("partner_id=? AND direction='CUSTOMER'", partner).Scan(&saleSum).Error; e != nil || saleSum != -1000 {
		t.Fatalf("customer entry sum=%d err=%v", saleSum, e)
	}
	var originals, reversals, receipts int64
	db.GORM.Table("inventory_entry").Where("sale_id=? AND entry_type='ORIGINAL'", draft.ID).Count(&originals)
	db.GORM.Table("inventory_entry").Where("sale_id=? AND entry_type='REVERSAL'", draft.ID).Count(&reversals)
	db.GORM.Table("partner_balance_entry").Where("partner_id=? AND direction='CUSTOMER' AND entry_type='RECEIPT'", partner).Count(&receipts)
	if originals != 2 || reversals != 2 || receipts != 1 {
		t.Fatalf("ledger rows originals=%d reversals=%d receipts=%d", originals, reversals, receipts)
	}
	// Two independent sales compete for four units from a five-unit balance.
	createCompetitionDraft := func() int64 {
		v := inventoryData[struct {
			ID      int64 `json:"id"`
			Version int64 `json:"version"`
		}](t, serveJSON(router, http.MethodPost, "/api/v1/sales", fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"4","unitPrice":"1.00"}]}`, partner, goods), token), 200)
		return v.ID
	}
	first, second := createCompetitionDraft(), createCompetitionDraft()
	start := make(chan struct{})
	responses := make(chan int, 2)
	for _, id := range []int64{first, second} {
		go func(id int64) {
			<-start
			responses <- serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sales/%d/post", id), `{"version":1}`, token).Code
		}(id)
	}
	close(start)
	resultCounts := map[int]int{}
	for range 2 {
		resultCounts[<-responses]++
	}
	if resultCounts[200] != 1 || resultCounts[409] != 1 {
		t.Fatalf("competing sale post results=%v", resultCounts)
	}
	if e := db.GORM.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", goods).Scan(&stock).Error; e != nil || stock != 1000 {
		t.Fatalf("competing sale stock=%d err=%v", stock, e)
	}
}

func postedDocNo(t *testing.T, db *platformdatabase.Database, id int64) string {
	t.Helper()
	var v string
	if e := db.GORM.Table("sale_document").Select("document_no").Where("id=?", id).Scan(&v).Error; e != nil {
		t.Fatal(e)
	}
	return v
}
func installSalePostingAuditFailure(t *testing.T, db *platformdatabase.Database, dialect string) {
	t.Helper()
	statement := `CREATE TRIGGER sale_post_audit_failure BEFORE INSERT ON sys_oper_log WHEN NEW.operation_type='sale.post' BEGIN SELECT RAISE(ABORT,'audit unavailable'); END`
	if dialect == "postgres" {
		statement = `CREATE FUNCTION reject_sale_post_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.operation_type='sale.post' THEN RAISE EXCEPTION 'audit unavailable'; END IF; RETURN NEW; END $$`
	}
	if e := db.GORM.Exec(statement).Error; e != nil {
		t.Fatalf("install post audit trigger: %v", e)
	}
	if dialect == "postgres" {
		if e := db.GORM.Exec("CREATE TRIGGER sale_post_audit_failure BEFORE INSERT ON sys_oper_log FOR EACH ROW EXECUTE FUNCTION reject_sale_post_audit()").Error; e != nil {
			t.Fatal(e)
		}
	}
}
func dropSalePostingAuditFailure(db *platformdatabase.Database, dialect string) error {
	if dialect == "sqlite" {
		return db.GORM.Exec("DROP TRIGGER sale_post_audit_failure").Error
	}
	if e := db.GORM.Exec("DROP TRIGGER sale_post_audit_failure ON sys_oper_log").Error; e != nil {
		return e
	}
	return db.GORM.Exec("DROP FUNCTION reject_sale_post_audit()").Error
}

func installSaleCancelAuditFailure(t *testing.T, db *platformdatabase.Database, dialect string) {
	t.Helper()
	statement := `CREATE TRIGGER sale_cancel_audit_failure BEFORE INSERT ON sys_oper_log WHEN NEW.operation_type='sale.cancel' BEGIN SELECT RAISE(ABORT,'audit unavailable'); END`
	if dialect == "postgres" {
		statement = `CREATE FUNCTION reject_sale_cancel_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.operation_type='sale.cancel' THEN RAISE EXCEPTION 'audit unavailable'; END IF; RETURN NEW; END $$`
	}
	if e := db.GORM.Exec(statement).Error; e != nil {
		t.Fatalf("install cancel audit failure: %v", e)
	}
	if dialect == "postgres" {
		if e := db.GORM.Exec("CREATE TRIGGER sale_cancel_audit_failure BEFORE INSERT ON sys_oper_log FOR EACH ROW EXECUTE FUNCTION reject_sale_cancel_audit()").Error; e != nil {
			t.Fatal(e)
		}
	}
}

func dropSaleCancelAuditFailure(db *platformdatabase.Database, dialect string) error {
	if dialect == "sqlite" {
		return db.GORM.Exec("DROP TRIGGER sale_cancel_audit_failure").Error
	}
	if e := db.GORM.Exec("DROP TRIGGER sale_cancel_audit_failure ON sys_oper_log").Error; e != nil {
		return e
	}
	return db.GORM.Exec("DROP FUNCTION reject_sale_cancel_audit()").Error
}
