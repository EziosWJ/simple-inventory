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
)

func TestSQLitePurchaseDraftPostgresSQLiteHTTPContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "purchase-draft.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	router, e := app.Build(testAPIConfig(), db, sqliteDependencies(t, db, t.TempDir()))
	if e != nil {
		t.Fatal(e)
	}
	assertPurchaseDraftContract(t, router, db, "sqlite")
}
func TestPostgresPurchaseDraftPostgresSQLiteHTTPContract(t *testing.T) {
	if _, e := exec.LookPath("docker"); e != nil {
		t.Skip("Docker is required for PostgreSQL integration tests")
	}
	temporary := startPostgres(t)
	runMigrations(t, projectRoot(t), temporary.dsn)
	db := openTemporaryDatabase(t, temporary.dsn)
	defer db.Close()
	router, e := app.Build(testAPIConfig(), db, testDependencies(t, db, t.TempDir()))
	if e != nil {
		t.Fatal(e)
	}
	assertPurchaseDraftContract(t, router, db, "postgres")
}
func assertPurchaseDraftContract(t *testing.T, router http.Handler, db *platformdatabase.Database, dialect string) {
	t.Helper()
	var menuCount, adminGrantCount int64
	if err := db.GORM.Table("sys_menu").Where("path = ? AND deleted = 0", "/business/purchases").Count(&menuCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Table("sys_role_menu rm").Joins("JOIN sys_menu m ON m.id=rm.menu_id").Joins("JOIN sys_role r ON r.id=rm.role_id").Where("m.path=? AND r.role_code=?", "/business/purchases", "ADMIN").Count(&adminGrantCount).Error; err != nil {
		t.Fatal(err)
	}
	if menuCount != 1 || adminGrantCount != 1 {
		t.Fatalf("purchase menu seed: menus=%d admin grants=%d", menuCount, adminGrantCount)
	}
	token := loginAdmin(t, router)
	assertUnauthenticated(t, serveJSON(router, http.MethodPost, "/api/v1/purchases", `{}`, ""))
	partner := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/partners", `{"code":"PUR-V1","name":"采购供应商","type":"COMPANY","isSupplier":true}`, token), 200).ID
	product := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/products", `{"code":"PUR-G1","name":"采购商品","type":"GOODS","unit":"台","purchasePrice":"12.34"}`, token), 200).ID
	// Server-side role, status, product type, quantity precision, and price rules are enforced even if the UI is bypassed.
	customerOnly := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/partners", `{"code":"PUR-C1","name":"仅客户","type":"COMPANY","isCustomer":true}`, token), 200).ID
	serviceProduct := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/products", `{"code":"PUR-S1","name":"服务项目","type":"SERVICE","unit":"次"}`, token), 200).ID
	disabledSupplier := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/partners", `{"code":"PUR-V2","name":"停用供应商","type":"COMPANY","isSupplier":true}`, token), 200).ID
	if response := serveJSON(router, http.MethodPut, fmt.Sprintf("/api/v1/partners/%d/status", disabledSupplier), `{"status":0}`, token); response.Code != 200 {
		t.Fatalf("disable supplier status=%d", response.Code)
	}
	disabledProduct := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/products", `{"code":"PUR-G2","name":"停用商品","type":"GOODS","unit":"台"}`, token), 200).ID
	if response := serveJSON(router, http.MethodPut, fmt.Sprintf("/api/v1/products/%d/status", disabledProduct), `{"status":0}`, token); response.Code != 200 {
		t.Fatalf("disable product status=%d", response.Code)
	}
	invalidItem := func(pid int64, kind, quantity, price string) string {
		return fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":%q,"unit":"台","quantity":%q,"unitPrice":%q}]}`, partner, pid, kind, quantity, price)
	}
	for name, invalidBody := range map[string]string{
		"customer-only supplier":  fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"1","unitPrice":"1.00"}]}`, customerOnly, product),
		"disabled supplier":       fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"1","unitPrice":"1.00"}]}`, disabledSupplier, product),
		"service":                 invalidItem(serviceProduct, "GOODS", "1", "1.00"),
		"disabled product":        invalidItem(disabledProduct, "GOODS", "1", "1.00"),
		"confirmed service type":  invalidItem(product, "SERVICE", "1", "1.00"),
		"fraction precision":      invalidItem(product, "GOODS", "1.0001", "1.00"),
		"negative price":          invalidItem(product, "GOODS", "1", "-0.01"),
		"zero quantity":           invalidItem(product, "GOODS", "0", "1.00"),
		"multiplication overflow": invalidItem(product, "GOODS", "9223372036854775.807", "92233720368547758.07"),
		"document sum overflow":   fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"1","unitPrice":"92233720368547758.07"},{"productId":%d,"productType":"GOODS","unit":"台","quantity":"1","unitPrice":"0.01"}]}`, partner, product, product),
	} {
		if response := serveJSON(router, http.MethodPost, "/api/v1/purchases", invalidBody, token); response.Code != 400 {
			t.Errorf("%s accepted: status=%d body=%s", name, response.Code, response.Body.String())
		}
	}
	body := fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-10-01","remark":"草稿验收","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"1.125","unitPrice":"12.36"},{"productId":%d,"productType":"GOODS","unit":"台","quantity":"2","unitPrice":"0"}]}`, partner, product, product)
	type line struct {
		ID                          int64 `json:"id"`
		Quantity, UnitPrice, Amount string
	}
	type draft struct {
		ID              int64   `json:"id"`
		CreatedBy       int64   `json:"createdBy"`
		CreatedByName   string  `json:"createdByName"`
		CancelledBy     *int64  `json:"cancelledBy"`
		CancelledByName string  `json:"cancelledByName"`
		CancelledAt     *string `json:"cancelledAt"`
		DocumentNo      string  `json:"documentNo"`
		Status          string  `json:"status"`
		Version         int64   `json:"version"`
		PartnerID       int64   `json:"partnerId"`
		Total           string  `json:"totalAmount"`
		Items           []line  `json:"items"`
	}
	created := inventoryData[draft](t, serveJSON(router, http.MethodPost, "/api/v1/purchases", body, token), 200)
	if created.DocumentNo == "" || created.Status != "DRAFT" || created.Version != 1 || created.CreatedBy < 1 || created.CreatedByName == "" || len(created.Items) != 2 || created.Items[0].ID == created.Items[1].ID || created.Items[0].Amount != "13.91" || created.Total != "13.91" {
		t.Fatalf("unexpected draft response: %+v", created)
	}
	readBack := inventoryData[draft](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/purchases/%d", created.ID), "", token), 200)
	if readBack.ID != created.ID || readBack.DocumentNo != created.DocumentNo || len(readBack.Items) != 2 || readBack.Items[0].ID != created.Items[0].ID {
		t.Fatalf("purchase detail did not reproduce saved draft: %+v", readBack)
	}
	installPurchaseAuditFailure(t, db, dialect)
	failedCreate := serveJSON(router, http.MethodPost, "/api/v1/purchases", body, token)
	if failedCreate.Code != 500 {
		t.Fatalf("create audit failure status=%d body=%s", failedCreate.Code, failedCreate.Body.String())
	}
	var savedDocuments int64
	if err := db.GORM.Table("purchase_document").Count(&savedDocuments).Error; err != nil || savedDocuments != 1 {
		t.Fatalf("audit failure committed new purchase: count=%d err=%v", savedDocuments, err)
	}
	if err := db.GORM.Table("inventory_balance").Count(new(int64)).Error; err != nil {
		t.Fatal(err)
	}
	var balances int64
	if e := db.GORM.Table("inventory_balance").Count(&balances).Error; e != nil || balances != 0 {
		t.Fatalf("draft changed stock (%s), count=%d err=%v", dialect, balances, e)
	}
	var logCount int64
	if e := db.GORM.Table("sys_oper_log").Where("operation_type='purchase.draft.create'").Count(&logCount).Error; e != nil || logCount != 1 {
		t.Fatalf("audit count=%d err=%v", logCount, e)
	}
	edit := fmt.Sprintf(`{"version":1,"partnerId":%d,"businessDate":"2026-09-30","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"1","unitPrice":"1.00"}]}`, partner, product)
	failedEdit := serveJSON(router, http.MethodPut, fmt.Sprintf("/api/v1/purchases/%d", created.ID), edit, token)
	if failedEdit.Code != 500 {
		t.Fatalf("audit failure status=%d body=%s", failedEdit.Code, failedEdit.Body.String())
	}
	var unchangedVersion int64
	if err := db.GORM.Table("purchase_document").Where("id=?", created.ID).Pluck("version", &unchangedVersion).Error; err != nil || unchangedVersion != 1 {
		t.Fatalf("audit failure committed purchase edit: version=%d err=%v", unchangedVersion, err)
	}
	var unchangedLines int64
	if err := db.GORM.Table("purchase_document_item").Where("document_id=?", created.ID).Count(&unchangedLines).Error; err != nil || unchangedLines != 2 {
		t.Fatalf("audit failure changed lines: count=%d err=%v", unchangedLines, err)
	}
	if err := dropPurchaseAuditFailure(db, dialect); err != nil {
		t.Fatal(err)
	}
	updated := inventoryData[draft](t, serveJSON(router, http.MethodPut, fmt.Sprintf("/api/v1/purchases/%d", created.ID), edit, token), 200)
	if updated.Version != 2 || updated.Items[0].ID == created.Items[0].ID {
		t.Fatalf("edit did not replace stable line: before=%+v after=%+v", created, updated)
	}
	stale := serveJSON(router, http.MethodPut, fmt.Sprintf("/api/v1/purchases/%d", created.ID), edit, token)
	if stale.Code != 409 {
		t.Fatalf("stale version status=%d", stale.Code)
	}
	editV2 := func(date string) string {
		return fmt.Sprintf(`{"version":2,"partnerId":%d,"businessDate":%q,"items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"1","unitPrice":"1.00"}]}`, partner, date, product)
	}
	start := make(chan struct{})
	codes := make(chan int, 2)
	var racers sync.WaitGroup
	for _, date := range []string{"2026-09-29", "2026-09-28"} {
		racers.Add(1)
		go func(body string) {
			defer racers.Done()
			<-start
			codes <- serveJSON(router, http.MethodPut, fmt.Sprintf("/api/v1/purchases/%d", created.ID), body, token).Code
		}(editV2(date))
	}
	close(start)
	racers.Wait()
	close(codes)
	counts := map[int]int{}
	for code := range codes {
		counts[code]++
	}
	if counts[200] != 1 || counts[409] != 1 {
		t.Fatalf("concurrent version guard statuses=%v", counts)
	}
	page := inventoryData[struct {
		Total   int64   `json:"total"`
		Records []draft `json:"records"`
	}](t, serveJSON(router, http.MethodGet, fmt.Sprintf("/api/v1/purchases?productId=%d&page=1&pageSize=10", product), "", token), 200)
	if page.Total != 1 || len(page.Records) != 1 {
		t.Fatalf("product filter duplicated a multi-line purchase: %+v", page)
	}
	cancel := serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/purchases/%d/cancel", created.ID), `{"version":3,"reason":"录入错误"}`, token)
	cancelled := inventoryData[draft](t, cancel, 200)
	if cancelled.Status != "CANCELLED" || cancelled.Version != 4 || cancelled.CancelledBy == nil || *cancelled.CancelledBy != created.CreatedBy || cancelled.CancelledByName == "" || cancelled.CancelledAt == nil {
		t.Fatalf("unexpected cancellation: %+v", cancelled)
	}
	if e := db.GORM.Table("inventory_balance").Count(&balances).Error; e != nil || balances != 0 {
		t.Fatalf("cancel changed stock, count=%d err=%v", balances, e)
	}
	var entries int64
	if e := db.GORM.Table("inventory_entry").Count(&entries).Error; e != nil || entries != 0 {
		t.Fatalf("purchase draft created inventory history, count=%d err=%v", entries, e)
	}
}

func installPurchaseAuditFailure(t *testing.T, db *platformdatabase.Database, dialect string) {
	t.Helper()
	var statement string
	if dialect == "sqlite" {
		statement = `CREATE TRIGGER purchase_audit_failure BEFORE INSERT ON sys_oper_log WHEN NEW.operation_type IN ('purchase.draft.create','purchase.draft.edit') BEGIN SELECT RAISE(ABORT,'audit unavailable'); END`
	} else {
		statement = `CREATE FUNCTION reject_purchase_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.operation_type IN ('purchase.draft.create','purchase.draft.edit') THEN RAISE EXCEPTION 'audit unavailable'; END IF; RETURN NEW; END $$`
	}
	if err := db.GORM.Exec(statement).Error; err != nil {
		t.Fatalf("install audit failure trigger: %v", err)
	}
	if dialect == "postgres" {
		if err := db.GORM.Exec("CREATE TRIGGER purchase_audit_failure BEFORE INSERT ON sys_oper_log FOR EACH ROW EXECUTE FUNCTION reject_purchase_audit()").Error; err != nil {
			t.Fatalf("install postgres audit failure trigger: %v", err)
		}
	}
}

func dropPurchaseAuditFailure(db *platformdatabase.Database, dialect string) error {
	if dialect == "sqlite" {
		return db.GORM.Exec("DROP TRIGGER purchase_audit_failure").Error
	}
	if err := db.GORM.Exec("DROP TRIGGER purchase_audit_failure ON sys_oper_log").Error; err != nil {
		return err
	}
	return db.GORM.Exec("DROP FUNCTION reject_purchase_audit()").Error
}
