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
)

func TestSQLiteSaleDraftPostgresSQLiteHTTPContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sale-draft.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	router, e := app.Build(testAPIConfig(), db, sqliteDependencies(t, db, t.TempDir()))
	if e != nil {
		t.Fatal(e)
	}
	assertSaleDraftContract(t, router, db, "sqlite")
}
func TestPostgresSaleDraftPostgresSQLiteHTTPContract(t *testing.T) {
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
	assertSaleDraftContract(t, router, db, "postgres")
}
func assertSaleDraftContract(t *testing.T, router http.Handler, db *platformdatabase.Database, dialect string) {
	t.Helper()
	var menuCount, adminGrantCount int64
	if e := db.GORM.Table("sys_menu").Where("path=? AND deleted=0", "/business/sales").Count(&menuCount).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GORM.Table("sys_role_menu rm").Joins("JOIN sys_menu m ON m.id=rm.menu_id").Joins("JOIN sys_role r ON r.id=rm.role_id").Where("m.path=? AND r.role_code=?", "/business/sales", "ADMIN").Count(&adminGrantCount).Error; e != nil {
		t.Fatal(e)
	}
	if menuCount != 1 || adminGrantCount != 1 {
		t.Fatalf("sale menu seed: menus=%d admin grants=%d", menuCount, adminGrantCount)
	}
	token := loginAdmin(t, router)
	assertUnauthenticated(t, serveJSON(router, http.MethodPost, "/api/v1/sales", `{}`, ""))
	partner := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/partners", `{"code":"SALE-C1","name":"销售客户","type":"COMPANY","isCustomer":true,"contact":"档案联系人","phone":"10086","address":"档案地址"}`, token), 200).ID
	goods := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/products", `{"code":"SALE-G1","name":"打印机","type":"GOODS","unit":"台"}`, token), 200).ID
	service := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/products", `{"code":"SALE-S1","name":"安装服务","type":"SERVICE","unit":"次"}`, token), 200).ID
	body := fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-09-30","deliveryContact":"现场联系人","deliveryPhone":"12345","deliveryAddress":"本次地址","items":[{"productId":%d,"productType":"GOODS","unit":"台","quantity":"1.25","unitPrice":"800.00"},{"productId":%d,"productType":"GOODS","unit":"台","quantity":"1","unitPrice":"0.00"},{"productId":%d,"productType":"SERVICE","unit":"次","quantity":"2","unitPrice":"30.00"}]}`, partner, goods, goods, service)
	installSaleAuditFailure(t, db, dialect)
	if failed := serveJSON(router, http.MethodPost, "/api/v1/sales", body, token); failed.Code != 500 {
		t.Fatalf("audit failure create status=%d", failed.Code)
	}
	var rolledBack int64
	db.GORM.Table("sale_document").Count(&rolledBack)
	if rolledBack != 0 {
		t.Fatalf("audit failure committed sale draft count=%d", rolledBack)
	}
	if e := dropSaleAuditFailure(db, dialect); e != nil {
		t.Fatal(e)
	}
	type line struct {
		ID                            int64 `json:"id"`
		ProductType, Quantity, Amount string
	}
	type document struct {
		ID                                              int64  `json:"id"`
		DocumentNo                                      string `json:"documentNo"`
		Version                                         int64  `json:"version"`
		Status                                          string `json:"status"`
		DeliveryContact, DeliveryPhone, DeliveryAddress string
		TotalAmount                                     string `json:"totalAmount"`
		Items                                           []line `json:"items"`
	}
	d := inventoryData[document](t, serveJSON(router, http.MethodPost, "/api/v1/sales", body, token), 200)
	if d.DocumentNo == "" || d.Status != "DRAFT" || d.Version != 1 || len(d.Items) != 3 || d.Items[0].ID == d.Items[1].ID || d.TotalAmount != "1060.00" || d.DeliveryContact != "现场联系人" || d.DeliveryAddress != "本次地址" {
		t.Fatalf("bad mixed sale draft (%s): %+v", dialect, d)
	}
	installSaleAuditFailure(t, db, dialect)
	if failed := serveJSON(router, http.MethodPut, fmt.Sprintf("/api/v1/sales/%d", d.ID), fmt.Sprintf(`{"version":1,"partnerId":%d,"businessDate":"2026-09-30","items":[{"id":%d,"productId":%d,"productType":"GOODS","unit":"台","quantity":"1","unitPrice":"0"}]}`, partner, d.Items[0].ID, goods), token); failed.Code != 500 {
		t.Fatalf("audit failure edit status=%d", failed.Code)
	}
	if e := dropSaleAuditFailure(db, dialect); e != nil {
		t.Fatal(e)
	}
	var sameVersion int64
	db.GORM.Table("sale_document").Where("id=?", d.ID).Pluck("version", &sameVersion)
	if sameVersion != 1 {
		t.Fatalf("audit failure committed edit version=%d", sameVersion)
	}
	var stockRows, receivableRows int64
	db.GORM.Table("inventory_balance").Count(&stockRows)
	db.GORM.Table("partner_balance").Where("partner_id=? AND direction='CUSTOMER'", partner).Count(&receivableRows)
	if stockRows != 0 || receivableRows != 0 {
		t.Fatalf("draft changed stock/receivable: %d/%d", stockRows, receivableRows)
	}
	if code := serveJSON(router, http.MethodPut, fmt.Sprintf("/api/v1/sales/%d", d.ID), `{"version":2,"partnerId":0}`, token).Code; code != 400 {
		t.Fatalf("invalid edit=%d", code)
	}
	firstLineID := d.Items[0].ID
	edit := fmt.Sprintf(`{"version":1,"partnerId":%d,"businessDate":"2026-09-30","deliveryContact":"编辑联系人","deliveryPhone":"67890","deliveryAddress":"编辑地址","items":[{"id":%d,"productId":%d,"productType":"SERVICE","unit":"次","quantity":"2.5","unitPrice":"10.00"}]}`, partner, firstLineID, service)
	d = inventoryData[document](t, serveJSON(router, http.MethodPut, fmt.Sprintf("/api/v1/sales/%d", d.ID), edit, token), 200)
	if d.Version != 2 || len(d.Items) != 1 || d.Items[0].ID != firstLineID || d.TotalAmount != "25.00" {
		t.Fatalf("edit: %+v", d)
	}
	if code := serveJSON(router, http.MethodPut, fmt.Sprintf("/api/v1/sales/%d", d.ID), edit, token).Code; code != 409 {
		t.Fatalf("stale edit=%d", code)
	}
	page := inventoryData[struct {
		Total   int64      `json:"total"`
		Records []document `json:"records"`
	}](t, serveJSON(router, http.MethodGet, "/api/v1/sales?productId="+fmt.Sprint(service), "", token), 200)
	if page.Total != 1 || len(page.Records) != 1 {
		t.Fatalf("filtered page: %+v", page)
	}
	d = inventoryData[document](t, serveJSON(router, http.MethodPost, fmt.Sprintf("/api/v1/sales/%d/cancel", d.ID), `{"version":2,"reason":"录入错误"}`, token), 200)
	if d.Status != "CANCELLED" || d.Version != 3 {
		t.Fatalf("cancel: %+v", d)
	}
	db.GORM.Table("inventory_balance").Count(&stockRows)
	db.GORM.Table("partner_balance").Where("partner_id=? AND direction='CUSTOMER'", partner).Count(&receivableRows)
	if stockRows != 0 || receivableRows != 0 {
		t.Fatalf("cancelled draft changed stock/receivable: %d/%d", stockRows, receivableRows)
	}
	pure := fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"SERVICE","unit":"次","quantity":"1","unitPrice":"0"}]}`, partner, service)
	serviceDraft := inventoryData[document](t, serveJSON(router, http.MethodPost, "/api/v1/sales", pure, token), 200)
	if len(serviceDraft.Items) != 1 || serviceDraft.Items[0].ProductType != "SERVICE" || serviceDraft.TotalAmount != "0.00" {
		t.Fatalf("pure service draft: %+v", serviceDraft)
	}
	defaultContact := db.GORM.Table("sale_document").Select("delivery_contact").Where("id=?", serviceDraft.ID)
	var contactValue *string
	if e := defaultContact.Scan(&contactValue).Error; e != nil {
		t.Fatal(e)
	}
	if contactValue == nil || *contactValue != "档案联系人" {
		t.Fatalf("new sale did not default contact snapshot: %v", contactValue)
	}
	disabled := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/partners", `{"code":"SALE-C2","name":"停用客户","type":"COMPANY","isCustomer":true}`, token), 200).ID
	if code := serveJSON(router, http.MethodPut, fmt.Sprintf("/api/v1/partners/%d/status", disabled), `{"status":0}`, token).Code; code != 200 {
		t.Fatalf("disable customer status=%d", code)
	}
	invalid := fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"SERVICE","unit":"次","quantity":"1","unitPrice":"0"}]}`, disabled, service)
	if code := serveJSON(router, http.MethodPost, "/api/v1/sales", invalid, token).Code; code != 400 {
		t.Fatalf("disabled customer accepted status=%d", code)
	}
	noCustomer := inventoryData[struct {
		ID int64 `json:"id"`
	}](t, serveJSON(router, http.MethodPost, "/api/v1/partners", `{"code":"SALE-V1","name":"仅供应商","type":"COMPANY","isSupplier":true}`, token), 200).ID
	invalid = fmt.Sprintf(`{"partnerId":%d,"businessDate":"2026-10-01","items":[{"productId":%d,"productType":"SERVICE","unit":"次","quantity":"1","unitPrice":"0"}]}`, noCustomer, service)
	if code := serveJSON(router, http.MethodPost, "/api/v1/sales", invalid, token).Code; code != 400 {
		t.Fatalf("supplier-only partner accepted status=%d", code)
	}
}

func installSaleAuditFailure(t *testing.T, db *platformdatabase.Database, dialect string) {
	t.Helper()
	var statement string
	if dialect == "sqlite" {
		statement = `CREATE TRIGGER sale_audit_failure BEFORE INSERT ON sys_oper_log WHEN NEW.operation_type IN ('sale.draft.create','sale.draft.edit') BEGIN SELECT RAISE(ABORT,'audit unavailable'); END`
	} else {
		statement = `CREATE FUNCTION reject_sale_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.operation_type IN ('sale.draft.create','sale.draft.edit') THEN RAISE EXCEPTION 'audit unavailable'; END IF; RETURN NEW; END $$`
	}
	if e := db.GORM.Exec(statement).Error; e != nil {
		t.Fatalf("install sale audit trigger: %v", e)
	}
	if dialect == "postgres" {
		if e := db.GORM.Exec("CREATE TRIGGER sale_audit_failure BEFORE INSERT ON sys_oper_log FOR EACH ROW EXECUTE FUNCTION reject_sale_audit()").Error; e != nil {
			t.Fatalf("install postgres sale audit trigger: %v", e)
		}
	}
}
func dropSaleAuditFailure(db *platformdatabase.Database, dialect string) error {
	if dialect == "sqlite" {
		return db.GORM.Exec("DROP TRIGGER sale_audit_failure").Error
	}
	if e := db.GORM.Exec("DROP TRIGGER sale_audit_failure ON sys_oper_log").Error; e != nil {
		return e
	}
	return db.GORM.Exec("DROP FUNCTION reject_sale_audit()").Error
}
