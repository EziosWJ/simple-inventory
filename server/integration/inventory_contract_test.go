//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"github.com/pressly/goose/v3"
)

const adjustmentPath = "/api/v1/inventory/adjustments"

type adjustmentItemInput struct {
	ProductID   int64  `json:"productId"`
	ProductType string `json:"productType"`
	Unit        string `json:"unit"`
	Quantity    string `json:"quantity"`
	Reason      string `json:"reason"`
	Remark      string `json:"remark,omitempty"`
}
type adjustmentItemDTO struct {
	ID                   int64  `json:"id"`
	ProductID            int64  `json:"productId"`
	ProductCode          string `json:"productCode"`
	ProductName          string `json:"productName"`
	ProductModel         string `json:"productModel"`
	ProductSpecification string `json:"productSpecification"`
	ProductType          string `json:"productType"`
	Unit                 string `json:"unit"`
	Quantity             string `json:"quantity"`
	Reason               string `json:"reason"`
	Remark               string `json:"remark"`
}
type adjustmentDTO struct {
	ID              int64               `json:"id"`
	DocumentNo      string              `json:"documentNo"`
	Status          string              `json:"status"`
	Version         int64               `json:"version"`
	CreatedBy       int64               `json:"createdBy"`
	CreatedByName   string              `json:"createdByName"`
	CreateTime      time.Time           `json:"createTime"`
	CancelledBy     *int64              `json:"cancelledBy"`
	CancelledByName string              `json:"cancelledByName"`
	CancelledAt     *time.Time          `json:"cancelledAt"`
	CancelReason    *string             `json:"cancelReason"`
	Items           []adjustmentItemDTO `json:"items"`
}
type adjustmentPageDTO struct {
	Records  []adjustmentDTO `json:"records"`
	Total    int64           `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"pageSize"`
}

func inventoryData[T any](t *testing.T, response *httptest.ResponseRecorder, status int) T {
	t.Helper()
	if response.Code != status {
		t.Fatalf("HTTP status=%d want=%d body=%s", response.Code, status, response.Body.String())
	}
	var envelope struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    T      `json:"data"`
	}
	if e := json.Unmarshal(response.Body.Bytes(), &envelope); e != nil {
		t.Fatalf("decode inventory response: %v body=%s", e, response.Body.String())
	}
	if envelope.Code != status {
		t.Fatalf("envelope code=%d want=%d body=%s", envelope.Code, status, response.Body.String())
	}
	return envelope.Data
}
func inventoryBody(items ...adjustmentItemInput) string {
	v := struct {
		Items []adjustmentItemInput `json:"items"`
	}{items}
	b, e := json.Marshal(v)
	if e != nil {
		panic(e)
	}
	return string(b)
}

func TestSQLiteInventoryDraftContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory.db")
	db := openSQLiteDatabase(t, path)
	prepareInventoryPhase2(t, db, "sqlite3", filepath.Join(projectRoot(t), "migrations", "sqlite"))
	if e := db.Close(); e != nil {
		t.Fatal(e)
	}
	runSQLiteMigrations(t, path)
	runSQLiteMigrations(t, path)
	db = openSQLiteDatabase(t, path)
	defer db.Close()
	assertInventoryUpgrade(t, db)
	router, e := app.Build(testAPIConfig(), db, sqliteDependencies(t, db, t.TempDir()))
	if e != nil {
		t.Fatal(e)
	}
	assertInventoryDraftContract(t, router, db, "sqlite")
}
func TestPostgresInventoryDraftContract(t *testing.T) {
	if _, e := exec.LookPath("docker"); e != nil {
		t.Skip("Docker is required for PostgreSQL integration tests")
	}
	temporary := startPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	prepareInventoryPhase2(t, db, "postgres", filepath.Join(projectRoot(t), "migrations"))
	if e := db.Close(); e != nil {
		t.Fatal(e)
	}
	runMigrations(t, projectRoot(t), temporary.dsn)
	runMigrations(t, projectRoot(t), temporary.dsn)
	db = openTemporaryDatabase(t, temporary.dsn)
	defer db.Close()
	assertInventoryUpgrade(t, db)
	router, e := app.Build(testAPIConfig(), db, testDependencies(t, db, t.TempDir()))
	if e != nil {
		t.Fatal(e)
	}
	assertInventoryDraftContract(t, router, db, "postgres")
}

// Start at the last Phase 2 logical versions with real existing business data.
// The ordinary migration lifecycle tests separately cover empty databases.
func prepareInventoryPhase2(t *testing.T, db *platformdatabase.Database, dialect, root string) {
	t.Helper()
	if e := goose.SetDialect(dialect); e != nil {
		t.Fatal(e)
	}
	goose.SetTableName("goose_schema_db_version")
	if e := goose.UpToContext(context.Background(), db.SQL, filepath.Join(root, "schema"), 8); e != nil {
		t.Fatal(e)
	}
	goose.SetTableName("goose_seed_db_version")
	if e := goose.UpToContext(context.Background(), db.SQL, filepath.Join(root, "seed"), 6); e != nil {
		t.Fatal(e)
	}
	goose.SetTableName("goose_schema_db_version")
	if e := db.GORM.Exec("INSERT INTO product(code,name,type,unit) VALUES ('PHASE2-KEPT','升级保留商品','GOODS','台')").Error; e != nil {
		t.Fatal(e)
	}
}
func assertInventoryUpgrade(t *testing.T, db *platformdatabase.Database) {
	t.Helper()
	var n int64
	for _, test := range []struct{ table, condition string }{{"product", "code='PHASE2-KEPT' AND name='升级保留商品'"}, {"sys_user", "id=1 AND username='admin'"}, {"sys_menu", "path='/business/inventory-adjustments' AND deleted=0"}} {
		if e := db.GORM.Table(test.table).Where(test.condition).Count(&n).Error; e != nil || n != 1 {
			t.Fatalf("upgrade lost %s data count=%d error=%v", test.table, n, e)
		}
	}
	for _, table := range []string{"inventory_adjustment", "inventory_adjustment_item"} {
		if !db.GORM.Migrator().HasTable(table) {
			t.Fatalf("upgrade missing %s", table)
		}
	}
	for _, test := range []struct {
		table string
		want  int64
	}{{"goose_schema_db_version", 10}, {"goose_seed_db_version", 7}} {
		var version int64
		if e := db.GORM.Table(test.table).Select("MAX(version_id)").Scan(&version).Error; e != nil || version != test.want {
			t.Fatalf("upgrade version %s=%d want=%d err=%v", test.table, version, test.want, e)
		}
	}
}
func inventoryCounts(t *testing.T, db *platformdatabase.Database) (int64, int64, int64) {
	t.Helper()
	var docs, items, logs int64
	if e := db.GORM.Table("inventory_adjustment").Count(&docs).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GORM.Table("inventory_adjustment_item").Count(&items).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GORM.Table("sys_oper_log").Where("module_name=? AND operation_type=?", "inventory", "inventory.adjustment.create").Count(&logs).Error; e != nil {
		t.Fatal(e)
	}
	return docs, items, logs
}
func assertInventoryDraftContract(t *testing.T, router http.Handler, db *platformdatabase.Database, dialect string) {
	t.Helper()
	token := loginAdmin(t, router)
	for _, test := range []struct{ method, path string }{{http.MethodGet, adjustmentPath}, {http.MethodPost, adjustmentPath}, {http.MethodGet, adjustmentPath + "/1"}} {
		assertUnauthenticated(t, serveJSON(router, test.method, test.path, `{"items":[]}`, ""))
	}
	// Use only public HTTP APIs to create product references and disable a product.
	productIDs := map[string]int64{}
	for _, p := range []struct{ code, kind, unit string }{{"INV-A", "GOODS", "台"}, {"INV-B", "GOODS", "支"}, {"INV-SERVICE", "SERVICE", "次"}, {"INV-DISABLED", "GOODS", "个"}} {
		response := serveJSON(router, http.MethodPost, "/api/v1/products", fmt.Sprintf(`{"code":%q,"name":%q,"type":%q,"unit":%q,"model":"型号甲","specification":"规格甲"}`, p.code, p.code+"名称", p.kind, p.unit), token)
		result := inventoryData[struct {
			ID int64 `json:"id"`
		}](t, response, 200)
		productIDs[p.code] = result.ID
	}
	inventoryData[any](t, serveJSON(router, http.MethodPut, fmt.Sprintf("/api/v1/products/%d/status", productIDs["INV-DISABLED"]), `{"status":0}`, token), 200)
	a := adjustmentItemInput{ProductID: productIDs["INV-A"], ProductType: "GOODS", Unit: "台", Quantity: "1.230", Reason: "OPENING"}
	b := adjustmentItemInput{ProductID: productIDs["INV-B"], ProductType: "GOODS", Unit: "支", Quantity: "-0.001", Reason: "DAMAGE"}
	first := inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPost, adjustmentPath, inventoryBody(a, b), token), 200)
	if first.ID <= 0 || first.DocumentNo == "" || first.Status != "DRAFT" || first.Version != 1 || first.CreatedBy != 1 || first.CreatedByName == "" || first.CreateTime.IsZero() || len(first.Items) != 2 {
		t.Fatalf("created adjustment=%+v", first)
	}
	if first.Items[0].Quantity != "1.23" || first.Items[1].Quantity != "-0.001" || first.Items[0].Unit != "台" || first.Items[1].Unit != "支" || first.Items[0].ProductType != "GOODS" || first.Items[0].ProductCode != "INV-A" || first.Items[0].ProductName != "INV-A名称" || first.Items[0].ProductModel != "型号甲" || first.Items[0].ProductSpecification != "规格甲" {
		t.Fatalf("created items=%+v", first.Items)
	}
	for _, item := range first.Items {
		if item.ID <= 0 {
			t.Fatalf("item missing ID: %+v", item)
		}
	}
	detailPath := adjustmentPath + "/" + itoa(first.ID)
	detail := inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodGet, detailPath, "", token), 200)
	detailJSON, _ := json.Marshal(detail)
	firstJSON, _ := json.Marshal(first)
	if string(detailJSON) != string(firstJSON) {
		t.Fatalf("detail differs: %s / %s", detailJSON, firstJSON)
	}
	// Quantity precision and signed storage extremes round-trip through both DBs.
	for _, test := range []struct{ input, want, reason string }{{"0.001", "0.001", "SURPLUS"}, {"1.111", "1.111", "OPENING"}, {"-1.999", "-1.999", "SHORTAGE"}, {"9223372036854775.807", "9223372036854775.807", "SURPLUS"}, {"-9223372036854775.808", "-9223372036854775.808", "DAMAGE"}, {"+0001.000", "1", "OTHER"}, {"-2", "-2", "OTHER"}} {
		item := a
		item.Quantity = test.input
		item.Reason = test.reason
		if test.reason == "OTHER" {
			item.Remark = "核对实物"
		}
		created := inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPost, adjustmentPath, inventoryBody(item), token), 200)
		loaded := inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodGet, adjustmentPath+"/"+itoa(created.ID), "", token), 200)
		if len(loaded.Items) != 1 || loaded.Items[0].Quantity != test.want || loaded.Items[0].Reason != test.reason || loaded.Items[0].Remark != item.Remark {
			t.Fatalf("quantity %q round-trip=%+v", test.input, loaded.Items)
		}
	}
	docsBefore, itemsBefore, logsBefore := inventoryCounts(t, db)
	badBodies := []string{`{"items":[]}`, `{}`, inventoryBody(a, a), `{"items":[{"productId":1,"productType":"GOODS","unit":"台","quantity":1,"reason":"OPENING"}]}`}
	for _, q := range []string{"", "0", "0.000", "-0.000", "1.0000", "1.", ".1", "1e2", "NaN", " 1", "1 ", "--1", "9223372036854775.808", "-9223372036854775.809"} {
		item := a
		item.Quantity = q
		badBodies = append(badBodies, inventoryBody(item))
	}
	for _, change := range []func(*adjustmentItemInput){func(x *adjustmentItemInput) {
		x.ProductID = productIDs["INV-SERVICE"]
		x.ProductType = "SERVICE"
		x.Unit = "次"
	}, func(x *adjustmentItemInput) { x.ProductID = productIDs["INV-SERVICE"]; x.Unit = "次" }, func(x *adjustmentItemInput) { x.ProductID = productIDs["INV-DISABLED"]; x.Unit = "个" }, func(x *adjustmentItemInput) { x.ProductID = 999999 }, func(x *adjustmentItemInput) { x.ProductID = 0 }, func(x *adjustmentItemInput) { x.ProductType = "SERVICE" }, func(x *adjustmentItemInput) { x.Unit = "件" }, func(x *adjustmentItemInput) { x.Unit = "" }, func(x *adjustmentItemInput) { x.Reason = "UNKNOWN" }, func(x *adjustmentItemInput) { x.Reason = "OPENING"; x.Quantity = "-1" }, func(x *adjustmentItemInput) { x.Reason = "SURPLUS"; x.Quantity = "-1" }, func(x *adjustmentItemInput) { x.Reason = "SHORTAGE"; x.Quantity = "1" }, func(x *adjustmentItemInput) { x.Reason = "DAMAGE"; x.Quantity = "1" }, func(x *adjustmentItemInput) { x.Reason = "OTHER"; x.Remark = "" }, func(x *adjustmentItemInput) { x.Reason = "OTHER"; x.Remark = "  \n\t " }} {
		item := a
		change(&item)
		badBodies = append(badBodies, inventoryBody(item))
	}
	for i, body := range badBodies {
		response := serveJSON(router, http.MethodPost, adjustmentPath, body, token)
		inventoryData[any](t, response, 400)
		if !strings.Contains(response.Body.String(), `"message":`) {
			t.Fatalf("validation %d missing explanation", i)
		}
	}
	docsAfter, itemsAfter, logsAfter := inventoryCounts(t, db)
	if docsAfter != docsBefore || itemsAfter != itemsBefore || logsAfter != logsBefore {
		t.Fatalf("invalid drafts persisted docs/items/audit: %d/%d/%d -> %d/%d/%d", docsBefore, itemsBefore, logsBefore, docsAfter, itemsAfter, logsAfter)
	}
	for _, id := range []string{"0", "-1", "abc", "9223372036854775808"} {
		inventoryData[any](t, serveJSON(router, http.MethodGet, adjustmentPath+"/"+id, "", token), 400)
	}
	inventoryData[any](t, serveJSON(router, http.MethodGet, adjustmentPath+"/999999", "", token), 404)
	for _, test := range []struct{ method, suffix string }{{http.MethodDelete, ""}, {http.MethodPost, "/post"}} {
		inventoryData[any](t, serveJSON(router, test.method, detailPath+test.suffix, `{"version":1}`, token), 404)
	}
	page := func(query string) adjustmentPageDTO {
		return inventoryData[adjustmentPageDTO](t, serveJSON(router, http.MethodGet, adjustmentPath+query, "", token), 200)
	}
	all := page("")
	if all.Total != docsAfter || all.Page != 1 || all.PageSize != 10 {
		t.Fatalf("default page=%+v", all)
	}
	for _, q := range []string{"?status=POSTED", "?status=CANCELLED", "?productId=999999", "?documentNo=%25", "?documentNo=_"} {
		if got := page(q); got.Total != 0 || len(got.Records) != 0 {
			t.Fatalf("empty filter %s=%+v", q, got)
		}
	}
	if got := page("?status=DRAFT"); got.Total != docsAfter {
		t.Fatalf("DRAFT count=%d want=%d", got.Total, docsAfter)
	}
	if got := page("?productId=" + itoa(a.ProductID)); got.Total != docsAfter {
		t.Fatalf("multi-item document counted twice: %+v", got)
	}
	if got := page("?productId=" + itoa(b.ProductID)); got.Total != 1 {
		t.Fatalf("second product filter=%+v", got)
	}
	if got := page("?documentNo=" + url.QueryEscape(first.DocumentNo[len(first.DocumentNo)-16:])); got.Total != 1 || got.Records[0].ID != first.ID {
		t.Fatalf("document substring filter=%+v", got)
	}
	stamp := url.QueryEscape(first.CreateTime.Format(time.RFC3339Nano))
	if got := page("?createdFrom=" + stamp); got.Total != docsAfter {
		t.Fatalf("inclusive lower bound=%+v", got)
	}
	if got := page("?createdTo=" + stamp); got.Total != 0 {
		t.Fatalf("exclusive upper bound=%+v", got)
	}
	if got := page("?documentNo=" + url.QueryEscape(first.DocumentNo) + "&createdFrom=" + stamp + "&createdTo=" + url.QueryEscape(first.CreateTime.Add(time.Microsecond).Format(time.RFC3339Nano))); got.Total != 1 {
		t.Fatalf("time bounded document=%+v", got)
	}
	firstPage, secondPage := page("?page=1&pageSize=2"), page("?page=2&pageSize=2")
	if firstPage.Total != docsAfter || secondPage.Total != docsAfter || len(firstPage.Records) != 2 || len(secondPage.Records) != 2 || firstPage.Records[0].ID <= firstPage.Records[1].ID || firstPage.Records[1].ID <= secondPage.Records[0].ID || secondPage.Records[0].ID <= secondPage.Records[1].ID {
		t.Fatalf("unstable pagination: %+v / %+v", firstPage, secondPage)
	}
	repeated := page("?page=1&pageSize=2")
	if repeated.Records[0].ID != firstPage.Records[0].ID || repeated.Records[1].ID != firstPage.Records[1].ID {
		t.Fatal("pagination order changed without writes")
	}
	for _, q := range []string{"?status=UNKNOWN", "?productId=0", "?productId=-1", "?productId=abc", "?page=0", "?page=abc", "?pageSize=-1", "?pageSize=abc", "?createdFrom=2026-01-01", "?createdTo=bad", "?createdFrom=" + stamp + "&createdTo=" + stamp} {
		inventoryData[any](t, serveJSON(router, http.MethodGet, adjustmentPath+q, "", token), 400)
	}
	// The application-level SQLite connection serializes low-contention writes;
	// PostgreSQL exercises independent transactions through its normal pool.
	responses := make(chan *httptest.ResponseRecorder, 8)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			responses <- serveJSON(router, http.MethodPost, adjustmentPath, inventoryBody(a, b), token)
		}()
	}
	wg.Wait()
	close(responses)
	unique := map[string]bool{}
	for response := range responses {
		v := inventoryData[adjustmentDTO](t, response, 200)
		if unique[v.DocumentNo] || v.DocumentNo == first.DocumentNo || v.Version != 1 || v.Status != "DRAFT" || len(v.Items) != 2 {
			t.Fatalf("invalid concurrent result=%+v", v)
		}
		unique[v.DocumentNo] = true
		loaded := inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodGet, adjustmentPath+"/"+itoa(v.ID), "", token), 200)
		if len(loaded.Items) != 2 {
			t.Fatalf("partial concurrent draft=%+v", loaded)
		}
	}
	docsConcurrent, itemsConcurrent, logsConcurrent := inventoryCounts(t, db)
	if docsConcurrent != docsAfter+8 || itemsConcurrent != itemsAfter+16 || logsConcurrent != logsAfter+8 {
		t.Fatalf("concurrent persistence mismatch docs/items/audit=%d/%d/%d", docsConcurrent, itemsConcurrent, logsConcurrent)
	}
	installInventoryAuditFailure(t, db, dialect)
	inventoryData[any](t, serveJSON(router, http.MethodPost, adjustmentPath, inventoryBody(a, b), token), 500)
	docsFailed, itemsFailed, logsFailed := inventoryCounts(t, db)
	if docsFailed != docsConcurrent || itemsFailed != itemsConcurrent || logsFailed != logsConcurrent || page("").Total != docsConcurrent {
		t.Fatalf("audit failure left data docs/items/audit=%d/%d/%d", docsFailed, itemsFailed, logsFailed)
	}
	removeInventoryAuditFailure(t, db, dialect)
	inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPost, adjustmentPath, inventoryBody(a, b), token), 200)
	docsFinal, itemsFinal, logsFinal := inventoryCounts(t, db)
	if docsFinal != docsConcurrent+1 || itemsFinal != itemsConcurrent+2 || logsFinal != logsConcurrent+1 {
		t.Fatal("draft creation did not recover after audit trigger removal")
	}
	for _, table := range []string{"inventory_balance", "inventory_entry", "inventory_balances", "inventory_entries"} {
		if db.GORM.Migrator().HasTable(table) {
			t.Fatalf("draft task unexpectedly creates stock table %s", table)
		}
	}
	// An enabled account without ADMIN menu assignments may access all drafts;
	// creating a draft records the authenticated actor, never a supplied owner.
	inventoryData[any](t, serveJSON(router, http.MethodPost, "/api/system/user", `{"username":"inventory-operator","nickname":"库存操作员","deptId":1,"status":1}`, token), 200)
	users := inventoryData[struct {
		Records []struct {
			ID int64 `json:"id"`
		} `json:"records"`
	}](t, serveJSON(router, http.MethodGet, "/api/system/user/page?username=inventory-operator", "", token), 200)
	if len(users.Records) != 1 {
		t.Fatalf("created operator page=%+v", users)
	}
	user := users.Records[0]
	userToken := loginUser(t, router, "inventory-operator", "admin123")
	otherDetail := inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodGet, detailPath, "", userToken), 200)
	if otherDetail.ID != first.ID || otherDetail.CreatedBy != 1 {
		t.Fatalf("other-account detail=%+v", otherDetail)
	}
	otherDraft := inventoryData[adjustmentDTO](t, serveJSON(router, http.MethodPost, adjustmentPath, inventoryBody(a), userToken), 200)
	if otherDraft.CreatedBy != user.ID || otherDraft.CreatedByName != "库存操作员" {
		t.Fatalf("actual draft actor=%+v", otherDraft)
	}
	inventoryData[adjustmentPageDTO](t, serveJSON(router, http.MethodGet, adjustmentPath, "", userToken), 200)
	assertInventoryMenuDictionary(t, router, token, db)
}
func installInventoryAuditFailure(t *testing.T, db *platformdatabase.Database, dialect string) {
	t.Helper()
	if dialect == "sqlite" {
		if e := db.GORM.Exec("CREATE TRIGGER fail_inventory_audit BEFORE INSERT ON sys_oper_log WHEN NEW.module_name='inventory' BEGIN SELECT RAISE(ABORT,'inventory audit failure'); END").Error; e != nil {
			t.Fatal(e)
		}
		return
	}
	if e := db.GORM.Exec(`CREATE FUNCTION fail_inventory_audit() RETURNS trigger AS $$ BEGIN IF NEW.module_name='inventory' THEN RAISE EXCEPTION 'inventory audit failure'; END IF; RETURN NEW; END; $$ LANGUAGE plpgsql`).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.GORM.Exec("CREATE TRIGGER fail_inventory_audit BEFORE INSERT ON sys_oper_log FOR EACH ROW EXECUTE FUNCTION fail_inventory_audit()").Error; e != nil {
		t.Fatal(e)
	}
}
func removeInventoryAuditFailure(t *testing.T, db *platformdatabase.Database, dialect string) {
	t.Helper()
	sql := "DROP TRIGGER fail_inventory_audit"
	if dialect == "postgres" {
		sql += " ON sys_oper_log"
	}
	if e := db.GORM.Exec(sql).Error; e != nil {
		t.Fatal(e)
	}
	if dialect == "postgres" {
		if e := db.GORM.Exec("DROP FUNCTION fail_inventory_audit()").Error; e != nil {
			t.Fatal(e)
		}
	}
}
func assertInventoryMenuDictionary(t *testing.T, router http.Handler, token string, db *platformdatabase.Database) {
	t.Helper()
	var assigned int64
	if e := db.GORM.Table("sys_role_menu rm").Joins("JOIN sys_menu m ON m.id=rm.menu_id").Joins("JOIN sys_role r ON r.id=rm.role_id").Where("r.role_code='ADMIN' AND m.path='/business/inventory-adjustments' AND m.deleted=0").Count(&assigned).Error; e != nil || assigned != 1 {
		t.Fatalf("inventory menu ADMIN assignment=%d err=%v", assigned, e)
	}
	// Dictionary labels remain presentation; the service validates stable codes.
	for _, test := range []struct {
		code   string
		values []string
	}{{"INVENTORY_ADJUSTMENT_STATUS", []string{"DRAFT", "POSTED", "CANCELLED"}}, {"INVENTORY_ADJUSTMENT_REASON", []string{"OPENING", "SURPLUS", "SHORTAGE", "DAMAGE", "OTHER"}}} {
		response := serveJSON(router, http.MethodGet, "/api/system/dict/"+test.code+"/items", "", token)
		if response.Code != 200 {
			t.Fatalf("inventory dictionary %s: %s", test.code, response.Body.String())
		}
		for _, value := range test.values {
			if !strings.Contains(response.Body.String(), `"value":"`+value+`"`) {
				t.Fatalf("dictionary %s missing %s: %s", test.code, value, response.Body.String())
			}
		}
	}
}
