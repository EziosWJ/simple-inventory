//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
	"github.com/EziosWJ/simple-inventory/server/internal/auth"
	"github.com/EziosWJ/simple-inventory/server/internal/config"
	"github.com/EziosWJ/simple-inventory/server/internal/dept"
	"github.com/EziosWJ/simple-inventory/server/internal/dictionary"
	"github.com/EziosWJ/simple-inventory/server/internal/filemgmt"
	"github.com/EziosWJ/simple-inventory/server/internal/inventory"
	"github.com/EziosWJ/simple-inventory/server/internal/logmgmt"
	"github.com/EziosWJ/simple-inventory/server/internal/notification"
	"github.com/EziosWJ/simple-inventory/server/internal/partner"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"github.com/EziosWJ/simple-inventory/server/internal/printprofile"
	"github.com/EziosWJ/simple-inventory/server/internal/product"
	"github.com/EziosWJ/simple-inventory/server/internal/rbac"
	"github.com/EziosWJ/simple-inventory/server/internal/sysconfig"
	"github.com/EziosWJ/simple-inventory/server/internal/usermgmt"
	"github.com/EziosWJ/simple-inventory/server/internal/warehouse"
)

func TestSQLiteMigrationLifecycleAndBackup(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "app.db")
	runSQLiteMigrations(t, path)
	runSQLiteMigrations(t, path)

	database := openSQLiteDatabase(t, path)
	assertSQLitePragmas(t, database)
	var users, menus, configs int64
	if err := database.GORM.Table("sys_user").Count(&users).Error; err != nil {
		t.Fatalf("count SQLite users: %v", err)
	}
	if err := database.GORM.Table("sys_menu").Count(&menus).Error; err != nil {
		t.Fatalf("count SQLite menus: %v", err)
	}
	if err := database.GORM.Table("sys_config").Count(&configs).Error; err != nil {
		t.Fatalf("count SQLite configs: %v", err)
	}
	var warehouses int64
	if err := database.GORM.Table("warehouse").Count(&warehouses).Error; err != nil {
		t.Fatalf("count seeded warehouse: %v", err)
	}
	if users != 1 || menus != 24 || configs != 4 || warehouses != 1 {
		t.Fatalf("seed counts = users %d, menus %d, configs %d, warehouses %d; want 1, 24, 4, 1", users, menus, configs, warehouses)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close SQLite database: %v", err)
	}

	database = openSQLiteDatabase(t, path)
	defer func() { _ = database.Close() }()
	if err := database.Ready(context.Background()); err != nil {
		t.Fatalf("reopened SQLite database is not ready: %v", err)
	}
	var username string
	if err := database.GORM.Table("sys_user").Where("id = 1").Pluck("username", &username).Error; err != nil {
		t.Fatalf("read persisted SQLite user: %v", err)
	}
	if username != "admin" {
		t.Fatalf("persisted username = %q, want admin", username)
	}
	if err := database.GORM.Exec("INSERT INTO warehouse(singleton_id,name) VALUES (2,'second')").Error; err == nil {
		t.Fatal("SQLite accepted a second logical warehouse")
	}

	backupPath := filepath.Join(directory, "backup.db")
	if err := platformdatabase.BackupSQLite(context.Background(), path, backupPath); err != nil {
		t.Fatalf("backup SQLite database: %v", err)
	}
	if err := platformdatabase.VerifySQLiteFile(context.Background(), backupPath); err != nil {
		t.Fatalf("verify SQLite backup: %v", err)
	}
	backupInfo, err := os.Stat(backupPath)
	if err != nil {
		t.Fatalf("stat SQLite backup: %v", err)
	}
	if backupInfo.Mode().Perm() != 0o600 {
		t.Fatalf("SQLite backup permissions = %o, want 600", backupInfo.Mode().Perm())
	}
	runSQLiteMigrations(t, backupPath)
	restored := openSQLiteDatabase(t, backupPath)
	defer func() { _ = restored.Close() }()
	if err := restored.GORM.Table("sys_config").Where("config_key = ?", sysconfig.LogClearEnabledKey).Count(&configs).Error; err != nil {
		t.Fatalf("read restored SQLite config: %v", err)
	}
	if configs != 1 {
		t.Fatalf("restored config count = %d, want 1", configs)
	}
}

func TestSQLitePhase2MigrationUpgradeFromPhase1(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade.db")
	database := openSQLiteDatabase(t, path)
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	goose.SetTableName("goose_schema_db_version")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if err := goose.UpToContext(ctx, database.SQL, filepath.Join(projectRoot(t), "migrations", "sqlite", "schema"), 7); err != nil {
		t.Fatalf("apply Phase 1 SQLite schema: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	runSQLiteMigrations(t, path)

	database = openSQLiteDatabase(t, path)
	defer database.Close()
	for _, table := range []string{"product", "partner", "warehouse"} {
		if !database.GORM.Migrator().HasTable(table) {
			t.Fatalf("Phase 1 SQLite database upgrade did not create %s", table)
		}
	}
	var warehouseCount int64
	if err := database.GORM.Table("warehouse").Count(&warehouseCount).Error; err != nil || warehouseCount != 1 {
		t.Fatalf("upgraded SQLite warehouse seed count=%d err=%v", warehouseCount, err)
	}
}

func TestSQLiteBusinessDictionarySeedDownPreservesUserCodeCollision(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom-dictionary.db")
	command := exec.Command("go", "run", "./cmd/migrate", "up", "--kind", "schema")
	command.Dir = projectRoot(t)
	command.Env = sqliteEnvironment(path, config.EnvironmentTest)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("apply SQLite schema before dictionary seed: %v\n%s", err, output)
	}
	database := openSQLiteDatabase(t, path)
	defer database.Close()
	if err := goose.SetDialect("sqlite3"); err != nil {
		t.Fatal(err)
	}
	goose.SetTableName("goose_seed_db_version")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	seedDir := filepath.Join(projectRoot(t), "migrations", "sqlite", "seed")
	if err := goose.UpToContext(ctx, database.SQL, seedDir, 5); err != nil {
		t.Fatalf("apply existing SQLite seeds: %v", err)
	}
	if err := database.GORM.Exec("INSERT INTO sys_dict_type(dict_name,dict_code,status,is_builtin) VALUES('自建商品类型','PRODUCT_TYPE',1,0)").Error; err != nil {
		t.Fatalf("create user dictionary code collision: %v", err)
	}
	if err := database.GORM.Exec(`INSERT INTO sys_dict_data(dict_type_id,dict_label,dict_value)
SELECT id,'用户实物','GOODS' FROM sys_dict_type WHERE dict_code='PRODUCT_TYPE'`).Error; err != nil {
		t.Fatalf("create user dictionary item collision: %v", err)
	}
	if err := goose.UpToContext(ctx, database.SQL, seedDir, 6); err != nil {
		t.Fatalf("apply business dictionary seed with custom collision: %v", err)
	}
	if err := goose.DownToContext(ctx, database.SQL, seedDir, 5); err != nil {
		t.Fatalf("roll back business dictionary seed: %v", err)
	}
	var customTypeCount, customItemCount int64
	if err := database.GORM.Table("sys_dict_type").Where("dict_code='PRODUCT_TYPE' AND is_builtin=0").Count(&customTypeCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GORM.Table("sys_dict_data AS d").Joins("JOIN sys_dict_type AS t ON t.id=d.dict_type_id").Where("t.dict_code='PRODUCT_TYPE' AND t.is_builtin=0 AND d.dict_value='GOODS' AND d.dict_label='用户实物'").Count(&customItemCount).Error; err != nil {
		t.Fatal(err)
	}
	if customTypeCount != 1 || customItemCount != 1 {
		t.Fatalf("business seed rollback changed a user dictionary collision: types=%d items=%d", customTypeCount, customItemCount)
	}
}

func TestSQLiteLogClearSeedDefaultsByEnvironment(t *testing.T) {
	for _, test := range []struct {
		environment string
		want        string
	}{
		{environment: config.EnvironmentDev, want: "true"},
		{environment: config.EnvironmentTest, want: "false"},
		{environment: config.EnvironmentProd, want: "false"},
	} {
		t.Run(test.environment, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "seed.db")
			runSQLiteMigrationsWithEnvironment(t, path, test.environment)
			database := openSQLiteDatabase(t, path)
			defer func() { _ = database.Close() }()

			var value string
			if err := database.SQL.QueryRow("SELECT config_value FROM sys_config WHERE config_key = ?", sysconfig.LogClearEnabledKey).Scan(&value); err != nil {
				t.Fatalf("read SQLite log-clear seed: %v", err)
			}
			if value != test.want {
				t.Fatalf("SQLite log-clear seed for %s = %q, want %q", test.environment, value, test.want)
			}
		})
	}
}

func TestSQLiteSharedHTTPBusinessContract(t *testing.T) {
	directory := t.TempDir()
	databasePath := filepath.Join(directory, "contract.db")
	runSQLiteMigrations(t, databasePath)
	database := openSQLiteDatabase(t, databasePath)
	defer func() { _ = database.Close() }()

	dependencies := sqliteDependencies(t, database, filepath.Join(directory, "uploads"))
	router, err := app.Build(testAPIConfig(), database, dependencies)
	if err != nil {
		t.Fatalf("build SQLite API: %v", err)
	}

	failedLogin := serveJSON(router, http.MethodPost, "/api/auth/login", `{"username":"admin","password":"wrong"}`, "")
	assertEnvelopeCode(t, failedLogin, http.StatusOK, 400, "用户名或密码错误")
	adminToken := loginAdmin(t, router)
	me := serveJSON(router, http.MethodGet, "/api/auth/me", "", adminToken)
	if me.Code != http.StatusOK || !strings.Contains(me.Body.String(), `"roleCode":"ADMIN"`) {
		t.Fatalf("SQLite current user = %d %s", me.Code, me.Body.String())
	}
	menus := serveJSON(router, http.MethodGet, "/api/auth/menus", "", adminToken)
	if menus.Code != http.StatusOK || !strings.Contains(menus.Body.String(), `"menuName":"系统管理"`) {
		t.Fatalf("SQLite current menus = %d %s", menus.Code, menus.Body.String())
	}
	createdRole := serveJSON(router, http.MethodPost, "/api/system/role", `{"roleName":"SQLite运营","roleCode":"SQLITE-OPS","status":1,"sortOrder":2}`, adminToken)
	assertEnvelopeCode(t, createdRole, http.StatusOK, 200, "success")
	roleMenus := serveJSON(router, http.MethodPut, "/api/system/role/2/menus", `{"menuIds":[1,2]}`, adminToken)
	assertEnvelopeCode(t, roleMenus, http.StatusOK, 200, "success")
	roleDetail := serveJSON(router, http.MethodGet, "/api/system/role/2", "", adminToken)
	if roleDetail.Code != http.StatusOK || !strings.Contains(roleDetail.Body.String(), `"menuIds":[1,2]`) {
		t.Fatalf("SQLite role detail = %d %s", roleDetail.Code, roleDetail.Body.String())
	}

	createdDept := serveJSON(router, http.MethodPost, "/api/system/dept", `{"parentId":1,"deptName":"SQLite研发部","deptCode":"SQLITE-RND","status":1}`, adminToken)
	assertEnvelopeCode(t, createdDept, http.StatusOK, 200, "success")
	createdUser := serveJSON(router, http.MethodPost, "/api/system/user", `{"username":"sqlite-user","nickname":"SQLite用户","deptId":2,"status":1}`, adminToken)
	assertEnvelopeCode(t, createdUser, http.StatusOK, 200, "success")
	assignedRoles := serveJSON(router, http.MethodPut, "/api/system/user/2/roles", `{"roleIds":[2]}`, adminToken)
	assertEnvelopeCode(t, assignedRoles, http.StatusOK, 200, "success")
	userToken := loginUser(t, router, "sqlite-user", "admin123")

	items := serveJSON(router, http.MethodGet, "/api/system/dict/USER_STATUS/items", "", adminToken)
	if items.Code != http.StatusOK || !strings.Contains(items.Body.String(), `"value":"1"`) {
		t.Fatalf("SQLite seed dict items = %d %s", items.Code, items.Body.String())
	}
	createdType := serveJSON(router, http.MethodPost, "/api/system/dict-type", `{"dictName":"SQLite环境","dictCode":"SQLITE_ENV"}`, adminToken)
	assertEnvelopeCode(t, createdType, http.StatusOK, 200, "success")
	createdConfig := serveJSON(router, http.MethodPost, "/api/system/config", `{"configName":"SQLite演示","configKey":"sqlite.demo","configValue":"true"}`, adminToken)
	assertEnvelopeCode(t, createdConfig, http.StatusOK, 200, "success")
	configPage := serveJSON(router, http.MethodGet, "/api/system/config/page?page=1&pageSize=20&configKey=sqlite.demo", "", adminToken)
	if configPage.Code != http.StatusOK || !strings.Contains(configPage.Body.String(), `"configKey":"sqlite.demo"`) {
		t.Fatalf("SQLite config page = %d %s", configPage.Code, configPage.Body.String())
	}
	key := serveJSON(router, http.MethodGet, "/api/system/config/key/system.log-clear-enabled", "", adminToken)
	assertEnvelopeCode(t, key, http.StatusOK, 200, "success")
	disabledConfig := serveJSON(router, http.MethodPatch, "/api/system/config/1/status", `{"status":0}`, adminToken)
	assertEnvelopeCode(t, disabledConfig, http.StatusOK, 200, "success")
	missingKey := serveJSON(router, http.MethodGet, "/api/system/config/key/system.log-clear-enabled", "", adminToken)
	assertEnvelopeCode(t, missingKey, http.StatusOK, 404, "数据不存在")

	loginPage := serveJSON(router, http.MethodGet, "/api/system/login-log/page?page=1&pageSize=20", "", adminToken)
	if loginPage.Code != http.StatusOK || !strings.Contains(loginPage.Body.String(), `"loginStatus":"FAIL"`) {
		t.Fatalf("SQLite login logs = %d %s", loginPage.Code, loginPage.Body.String())
	}
	operPage := serveJSON(router, http.MethodGet, "/api/system/oper-log/page?page=1&pageSize=20&operatorName=admin", "", adminToken)
	if operPage.Code != http.StatusOK || !strings.Contains(operPage.Body.String(), `"operatorName":"admin"`) {
		t.Fatalf("SQLite operation logs = %d %s", operPage.Code, operPage.Body.String())
	}

	upload := serveMultipartFile(router, "/api/system/file/upload", adminToken, "sqlite.txt", "SQLite file content")
	assertEnvelopeCode(t, upload, http.StatusOK, 200, "success")
	var uploadPayload struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(upload.Body.Bytes(), &uploadPayload); err != nil || uploadPayload.Data.ID <= 0 {
		t.Fatalf("decode SQLite upload = %s, error=%v", upload.Body.String(), err)
	}
	download := servePlain(t, router, http.MethodGet, "/api/system/file/"+itoa(uploadPayload.Data.ID)+"/download", adminToken)
	if download.Code != http.StatusOK || download.Body.String() != "SQLite file content" {
		t.Fatalf("SQLite file download = %d %q", download.Code, download.Body.String())
	}

	publish := serveJSON(router, http.MethodPost, "/api/system/notification", `{"title":"SQLite通知","content":"通知内容","userIds":[2]}`, adminToken)
	assertEnvelopeCode(t, publish, http.StatusOK, 200, "success")
	page := serveJSON(router, http.MethodGet, "/api/system/notification/page?page=1&pageSize=20", "", userToken)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), `"title":"SQLite通知"`) || !strings.Contains(page.Body.String(), `"isRead":0`) {
		t.Fatalf("SQLite notification page = %d %s", page.Code, page.Body.String())
	}
	unread := serveJSON(router, http.MethodGet, "/api/system/notification/unread-count", "", userToken)
	if unread.Code != http.StatusOK || !strings.Contains(unread.Body.String(), `"data":2`) {
		t.Fatalf("SQLite unread count = %d %s", unread.Code, unread.Body.String())
	}
	detail := serveJSON(router, http.MethodGet, "/api/system/notification/2", "", userToken)
	assertEnvelopeCode(t, detail, http.StatusOK, 200, "success")
	unreadAfterDetail := serveJSON(router, http.MethodGet, "/api/system/notification/unread-count", "", userToken)
	if unreadAfterDetail.Code != http.StatusOK || !strings.Contains(unreadAfterDetail.Body.String(), `"data":1`) {
		t.Fatalf("SQLite unread count after detail = %d %s", unreadAfterDetail.Code, unreadAfterDetail.Body.String())
	}

	var auditCount int64
	if err := database.GORM.Table("sys_oper_log").Where("operator_id = 1 AND operation_status = 'SUCCESS'").Count(&auditCount).Error; err != nil {
		t.Fatalf("count SQLite operation audits: %v", err)
	}
	if auditCount < 5 {
		t.Fatalf("SQLite successful operation audit count = %d, want at least 5", auditCount)
	}
}

func TestSQLitePhase2ProductPartnerWarehouseContract(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "phase2.db")
	runSQLiteMigrations(t, dbPath)
	db := openSQLiteDatabase(t, dbPath)
	defer db.Close()
	router, err := app.Build(testAPIConfig(), db, sqliteDependencies(t, db, filepath.Join(dir, "uploads")))
	if err != nil {
		t.Fatal(err)
	}
	assertEnvelopeCode(t, serveJSON(router, http.MethodGet, "/api/v1/products", "", ""), http.StatusUnauthorized, 401, "未登录或 token 已失效")
	token := loginAdmin(t, router)
	assertBusinessDictionaryContract(t, router, token)
	created := serveJSON(router, http.MethodPost, "/api/v1/products", `{"name":"同名服务","type":"SERVICE","unit":"次","purchasePrice":"0.00","salePrice":"12.30"}`, token)
	assertEnvelopeCode(t, created, http.StatusOK, 200, "success")
	if !strings.Contains(created.Body.String(), `"code":"SP`) || !strings.Contains(created.Body.String(), `"purchasePrice":"0.00"`) {
		t.Fatalf("product result = %s", created.Body.String())
	}
	badPrice := serveJSON(router, http.MethodPost, "/api/v1/products", `{"name":"坏价格","type":"GOODS","unit":"个","salePrice":"1.001"}`, token)
	assertEnvelopeCode(t, badPrice, http.StatusBadRequest, 400, "参数错误")
	assertEnvelopeCode(t, serveJSON(router, http.MethodPost, "/api/v1/products", `{"name":"`+strings.Repeat("长", 201)+`","type":"GOODS","unit":"个"}`, token), 400, 400, "参数错误")
	servicePage := serveJSON(router, http.MethodGet, "/api/v1/products?type=SERVICE&keyword=同名", "", token)
	if servicePage.Code != 200 || !strings.Contains(servicePage.Body.String(), `"total":1`) {
		t.Fatalf("product search = %s", servicePage.Body.String())
	}
	var productResult struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(created.Body.Bytes(), &productResult)
	productPath := "/api/v1/products/" + itoa(productResult.Data.ID)
	assertEnvelopeCode(t, serveJSON(router, http.MethodPut, productPath+"/status", `{"status":0}`, token), 200, 200, "success")
	assertEnvelopeCode(t, serveJSON(router, http.MethodPut, productPath+"/status", `{"status":0}`, token), 200, 200, "success")
	assertEnvelopeCode(t, serveJSON(router, http.MethodPut, productPath+"/status", `{}`, token), 400, 400, "参数错误")
	assertEnvelopeCode(t, serveJSON(router, http.MethodPut, productPath+"/status", `{"status":null}`, token), 400, 400, "参数错误")
	assertEnvelopeCode(t, serveJSON(router, http.MethodPut, "/api/v1/products/0", `{"name":"误新建","type":"GOODS","unit":"个"}`, token), 400, 400, "参数错误")
	beforeProduct := serveJSON(router, http.MethodGet, productPath, "", token)
	var productDetail struct {
		Data struct {
			CreateTime string `json:"createTime"`
		} `json:"data"`
	}
	_ = json.Unmarshal(beforeProduct.Body.Bytes(), &productDetail)
	updatedProduct := serveJSON(router, http.MethodPut, productPath, `{"code":"EDITED-PRODUCT","name":"编辑后服务","type":"GOODS","unit":"台","purchasePrice":null,"salePrice":"0.01"}`, token)
	assertEnvelopeCode(t, updatedProduct, 200, 200, "success")
	if !strings.Contains(updatedProduct.Body.String(), `"createTime":"`+productDetail.Data.CreateTime+`"`) || !strings.Contains(updatedProduct.Body.String(), `"purchasePrice":null`) || !strings.Contains(updatedProduct.Body.String(), `"salePrice":"0.01"`) {
		t.Fatalf("product update did not preserve timestamps or money values: %s", updatedProduct.Body.String())
	}
	assertEnvelopeCode(t, serveJSON(router, http.MethodPut, productPath+"/status", `{"status":1}`, token), 200, 200, "success")
	assertEnvelopeCode(t, serveJSON(router, http.MethodPut, productPath+"/status", `{"status":1}`, token), 200, 200, "success")
	assertEnvelopeCode(t, serveJSON(router, http.MethodPost, "/api/v1/products", `{"code":"manual-product","name":"手工一","type":"GOODS","unit":"个"}`, token), 200, 200, "success")
	assertEnvelopeCode(t, serveJSON(router, http.MethodPost, "/api/v1/products", `{"code":" MANUAL-PRODUCT ","name":"手工二","type":"GOODS","unit":"个"}`, token), 409, 409, "编码已存在")
	assertEnvelopeCode(t, serveJSON(router, http.MethodPut, productPath, `{"code":"manual-product","name":"编码冲突编辑","type":"GOODS","unit":"个"}`, token), 409, 409, "编码已存在")
	assertConcurrentUniqueCode(t, router, token, "/api/v1/products", `{"code":"CONCURRENT-PRODUCT","name":"并发商品","type":"GOODS","unit":"个"}`)
	assertEnvelopeCode(t, serveJSON(router, http.MethodPost, "/api/v1/products", `{"name":"价格溢出","type":"GOODS","unit":"个","salePrice":"92233720368547758.08"}`, token), 400, 400, "参数错误")
	partnerResult := serveJSON(router, http.MethodPost, "/api/v1/partners", `{"name":"同行","type":"COMPANY","isCustomer":true,"isSupplier":true}`, token)
	assertEnvelopeCode(t, partnerResult, 200, 200, "success")
	if !strings.Contains(partnerResult.Body.String(), `"code":"PT`) {
		t.Fatalf("partner result = %s", partnerResult.Body.String())
	}
	assertEnvelopeCode(t, serveJSON(router, http.MethodPost, "/api/v1/partners", `{"name":"无身份","type":"PERSON"}`, token), 400, 400, "参数错误")
	assertEnvelopeCode(t, serveJSON(router, http.MethodPost, "/api/v1/partners", `{"name":"超长电话","type":"PERSON","isCustomer":true,"phone":"`+strings.Repeat("1", 51)+`"}`, token), 400, 400, "参数错误")
	var partnerID struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	_ = json.Unmarshal(partnerResult.Body.Bytes(), &partnerID)
	partnerPath := "/api/v1/partners/" + itoa(partnerID.Data.ID)
	for _, identity := range []string{"CUSTOMER", "SUPPLIER"} {
		page := serveJSON(router, http.MethodGet, "/api/v1/partners?identity="+identity, "", token)
		if page.Code != 200 || !strings.Contains(page.Body.String(), `"total":1`) {
			t.Fatalf("partner %s filter = %s", identity, page.Body.String())
		}
	}
	updatedPartner := serveJSON(router, http.MethodPut, partnerPath, `{"code":"EDITED-PARTNER","name":"同行已修改","type":"COMPANY","isCustomer":false,"isSupplier":true,"contact":null,"phone":null,"invoiceName":null}`, token)
	assertEnvelopeCode(t, updatedPartner, 200, 200, "success")
	if !strings.Contains(updatedPartner.Body.String(), `"contact":null`) {
		t.Fatalf("partner optional contact was not cleared: %s", updatedPartner.Body.String())
	}
	customerPage := serveJSON(router, http.MethodGet, "/api/v1/partners?identity=CUSTOMER", "", token)
	if !strings.Contains(customerPage.Body.String(), `"total":0`) {
		t.Fatalf("partner identity edit not applied: %s", customerPage.Body.String())
	}
	assertEnvelopeCode(t, serveJSON(router, http.MethodPut, partnerPath+"/status", `{"status":0}`, token), 200, 200, "success")
	assertEnvelopeCode(t, serveJSON(router, http.MethodPut, partnerPath+"/status", `{"status":0}`, token), 200, 200, "success")
	assertEnvelopeCode(t, serveJSON(router, http.MethodPut, partnerPath+"/status", `{"status":null}`, token), 400, 400, "参数错误")
	assertEnvelopeCode(t, serveJSON(router, http.MethodPut, partnerPath+"/status", `{"status":1}`, token), 200, 200, "success")
	assertEnvelopeCode(t, serveJSON(router, http.MethodPost, "/api/v1/partners", `{"code":"partner-duplicate","name":"甲","type":"PERSON","isCustomer":true}`, token), 200, 200, "success")
	assertEnvelopeCode(t, serveJSON(router, http.MethodPost, "/api/v1/partners", `{"code":"PARTNER-DUPLICATE","name":"乙","type":"PERSON","isSupplier":true}`, token), 409, 409, "编码已存在")
	assertEnvelopeCode(t, serveJSON(router, http.MethodPut, partnerPath, `{"code":"PARTNER-DUPLICATE","name":"编码冲突编辑","type":"PERSON","isCustomer":true}`, token), 409, 409, "编码已存在")
	assertConcurrentUniqueCode(t, router, token, "/api/v1/partners", `{"code":"CONCURRENT-PARTNER","name":"并发往来单位","type":"PERSON","isCustomer":true}`)
	warehousePage := serveJSON(router, http.MethodGet, "/api/v1/warehouse", "", token)
	assertEnvelopeCode(t, warehousePage, 200, 200, "success")
	assertEnvelopeCode(t, serveJSON(router, http.MethodPut, "/api/v1/warehouse", `{"name":"主仓","remark":"更新"}`, token), 200, 200, "success")
	assertEnvelopeCode(t, serveJSON(router, http.MethodPut, "/api/v1/warehouse", `{"name":"   "}`, token), 400, 400, "参数错误")
	assertEnvelopeCode(t, serveJSON(router, http.MethodPut, "/api/v1/warehouse", `{"name":"`+strings.Repeat("仓", 101)+`"}`, token), 400, 400, "参数错误")
	warehousePage = serveJSON(router, http.MethodGet, "/api/v1/warehouse", "", token)
	if !strings.Contains(warehousePage.Body.String(), `"name":"主仓"`) {
		t.Fatalf("warehouse persistence = %s", warehousePage.Body.String())
	}
	var auditCount int64
	if err := db.GORM.Table("sys_oper_log").Where("module_name IN ?", []string{"product", "partner", "warehouse"}).Count(&auditCount).Error; err != nil || auditCount < 4 {
		t.Fatalf("phase2 audits count=%d err=%v", auditCount, err)
	}
	var auditMeta struct {
		RequestMethod string
		RequestURL    string
	}
	if err := db.GORM.Table("sys_oper_log").Select("request_method,request_url").Where("module_name='product' AND operation_type='product.save'").Order("id").Take(&auditMeta).Error; err != nil || auditMeta.RequestMethod != http.MethodPost || auditMeta.RequestURL != "/api/v1/products" {
		t.Fatalf("phase2 audit request metadata=%+v err=%v", auditMeta, err)
	}
	for _, resource := range []string{"product", "partner", "warehouse"} {
		trigger := "fail_phase2_" + resource + "_audit"
		if err := db.GORM.Exec("CREATE TRIGGER " + trigger + " BEFORE INSERT ON sys_oper_log WHEN NEW.module_name='" + resource + "' BEGIN SELECT RAISE(ABORT, 'phase2 audit failure'); END").Error; err != nil {
			t.Fatalf("create %s audit trigger: %v", resource, err)
		}
		var response *httptest.ResponseRecorder
		switch resource {
		case "product":
			response = serveJSON(router, http.MethodPost, "/api/v1/products", `{"code":"ROLLBACK-PRODUCT","name":"回滚商品","type":"GOODS","unit":"个"}`, token)
		case "partner":
			response = serveJSON(router, http.MethodPost, "/api/v1/partners", `{"code":"ROLLBACK-PARTNER","name":"回滚单位","type":"PERSON","isCustomer":true}`, token)
		case "warehouse":
			response = serveJSON(router, http.MethodPut, "/api/v1/warehouse", `{"name":"不得保存"}`, token)
		}
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("%s audit failure status=%d body=%s", resource, response.Code, response.Body.String())
		}
		if resource == "product" {
			response = serveJSON(router, http.MethodPut, productPath, `{"code":"ROLLBACK-EDIT","name":"不应保存","type":"GOODS","unit":"个"}`, token)
			if response.Code != http.StatusInternalServerError {
				t.Fatalf("product edit audit failure status=%d body=%s", response.Code, response.Body.String())
			}
			response = serveJSON(router, http.MethodPut, productPath+"/status", `{"status":0}`, token)
			if response.Code != http.StatusInternalServerError {
				t.Fatalf("product status audit failure status=%d body=%s", response.Code, response.Body.String())
			}
		}
		if resource == "partner" {
			response = serveJSON(router, http.MethodPut, partnerPath, `{"code":"ROLLBACK-PARTNER-EDIT","name":"不应保存","type":"COMPANY","isCustomer":true,"isSupplier":false}`, token)
			if response.Code != http.StatusInternalServerError {
				t.Fatalf("partner edit audit failure status=%d body=%s", response.Code, response.Body.String())
			}
			response = serveJSON(router, http.MethodPut, partnerPath+"/status", `{"status":0}`, token)
			if response.Code != http.StatusInternalServerError {
				t.Fatalf("partner status audit failure status=%d body=%s", response.Code, response.Body.String())
			}
		}
		if err := db.GORM.Exec("DROP TRIGGER " + trigger).Error; err != nil {
			t.Fatalf("drop %s audit trigger: %v", resource, err)
		}
		if resource == "product" {
			var count int64
			_ = db.GORM.Table("product").Where("code=?", "ROLLBACK-PRODUCT").Count(&count).Error
			if count != 0 {
				t.Fatalf("product was not rolled back: %d", count)
			}
			var unchanged struct {
				Name   string
				Code   string
				Status int
			}
			if err := db.GORM.Table("product").Select("name,code,status").Where("id=?", productResult.Data.ID).Take(&unchanged).Error; err != nil || unchanged.Name != "编辑后服务" || unchanged.Code != "EDITED-PRODUCT" || unchanged.Status != 1 {
				t.Fatalf("product edit/status survived audit failure: %+v err=%v", unchanged, err)
			}
		}
		if resource == "partner" {
			var count int64
			_ = db.GORM.Table("partner").Where("code=?", "ROLLBACK-PARTNER").Count(&count).Error
			if count != 0 {
				t.Fatalf("partner was not rolled back: %d", count)
			}
			var unchanged struct {
				Name   string
				Code   string
				Status int
			}
			if err := db.GORM.Table("partner").Select("name,code,status").Where("id=?", partnerID.Data.ID).Take(&unchanged).Error; err != nil || unchanged.Name != "同行已修改" || unchanged.Code != "EDITED-PARTNER" || unchanged.Status != 1 {
				t.Fatalf("partner edit/status survived audit failure: %+v err=%v", unchanged, err)
			}
		}
		if resource == "warehouse" {
			warehousePage = serveJSON(router, http.MethodGet, "/api/v1/warehouse", "", token)
			if !strings.Contains(warehousePage.Body.String(), `"name":"主仓"`) {
				t.Fatalf("warehouse update survived audit failure: %s", warehousePage.Body.String())
			}
		}
	}
}

func assertConcurrentUniqueCode(t *testing.T, router http.Handler, token, path, body string) {
	t.Helper()
	const workers = 6
	start := make(chan struct{})
	results := make([]int, workers)
	for i := range results {
		results[i] = -1
	}
	var group sync.WaitGroup
	for i := range workers {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			<-start
			for attempt := 0; attempt < 10; attempt++ {
				response := serveJSON(router, http.MethodPost, path, body, token)
				if response.Code == http.StatusOK || response.Code == http.StatusConflict {
					results[index] = response.Code
					return
				}
				time.Sleep(20 * time.Millisecond)
			}
		}(i)
	}
	close(start)
	group.Wait()
	successes, conflicts := 0, 0
	for _, status := range results {
		if status == http.StatusOK {
			successes++
		} else if status == http.StatusConflict {
			conflicts++
		}
	}
	if successes != 1 || conflicts != workers-1 {
		t.Fatalf("concurrent unique-code result statuses=%v, want one success and %d conflicts", results, workers-1)
	}
}

func TestSQLiteBusinessAndAuditWriteRollbackTogether(t *testing.T) {
	directory := t.TempDir()
	databasePath := filepath.Join(directory, "rollback.db")
	runSQLiteMigrations(t, databasePath)
	database := openSQLiteDatabase(t, databasePath)
	defer func() { _ = database.Close() }()
	router, err := app.Build(testAPIConfig(), database, sqliteDependencies(t, database, filepath.Join(directory, "uploads")))
	if err != nil {
		t.Fatalf("build SQLite rollback-test API: %v", err)
	}
	adminToken := loginAdmin(t, router)

	if err := database.GORM.Exec(`
CREATE TRIGGER fail_success_audit
BEFORE INSERT ON sys_oper_log
WHEN NEW.operation_status = 'SUCCESS'
BEGIN
    SELECT RAISE(ABORT, 'audit failure');
END`).Error; err != nil {
		t.Fatalf("create SQLite audit failure trigger: %v", err)
	}
	defer func() { _ = database.GORM.Exec("DROP TRIGGER IF EXISTS fail_success_audit").Error }()

	attempt := serveJSON(router, http.MethodPost, "/api/system/role", `{"roleName":"回滚角色","roleCode":"ROLLBACK","status":1,"sortOrder":2}`, adminToken)
	if attempt.Code != http.StatusInternalServerError || strings.Contains(attempt.Body.String(), "audit failure") {
		t.Fatalf("SQLite audit failure response = %d %s, want generic 500", attempt.Code, attempt.Body.String())
	}
	var roleCount int64
	if err := database.GORM.Table("sys_role").Where("role_code = ?", "ROLLBACK").Count(&roleCount).Error; err != nil {
		t.Fatalf("count SQLite rolled-back role: %v", err)
	}
	if roleCount != 0 {
		t.Fatalf("SQLite role count after audit failure = %d, want 0", roleCount)
	}
}

func TestSQLiteLockTimeoutUsesTemporaryUnavailableContract(t *testing.T) {
	directory := t.TempDir()
	databasePath := filepath.Join(directory, "lock.db")
	runSQLiteMigrations(t, databasePath)
	lockerDatabase := openSQLiteDatabase(t, databasePath)
	defer func() { _ = lockerDatabase.Close() }()
	requestDatabase := openSQLiteDatabase(t, databasePath)
	defer func() { _ = requestDatabase.Close() }()
	router, err := app.Build(testAPIConfig(), requestDatabase, sqliteDependencies(t, requestDatabase, filepath.Join(directory, "uploads")))
	if err != nil {
		t.Fatalf("build SQLite lock-test API: %v", err)
	}
	adminToken := loginAdmin(t, router)

	transaction, err := lockerDatabase.SQL.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin SQLite lock transaction: %v", err)
	}
	if _, err := transaction.ExecContext(context.Background(), "UPDATE sys_config SET update_time = update_time WHERE id = 1"); err != nil {
		_ = transaction.Rollback()
		t.Fatalf("hold SQLite write lock: %v", err)
	}

	started := time.Now()
	response := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		response <- serveJSON(router, http.MethodPatch, "/api/system/config/1/status", `{"status":1}`, adminToken)
	}()
	var result *httptest.ResponseRecorder
	select {
	case result = <-response:
	case <-time.After(8 * time.Second):
		_ = transaction.Rollback()
		t.Fatal("SQLite lock request exceeded its bounded timeout")
	}
	_ = transaction.Rollback()
	if elapsed := time.Since(started); elapsed > 8*time.Second {
		t.Fatalf("SQLite lock request took %s, want bounded response", elapsed)
	}
	if result.Code != http.StatusServiceUnavailable || !strings.Contains(result.Body.String(), `"code":503`) {
		t.Fatalf("SQLite lock response = %d %s, want 503 contract", result.Code, result.Body.String())
	}
	if strings.Contains(strings.ToLower(result.Body.String()), "database is locked") {
		t.Fatalf("SQLite lock response leaked driver detail: %s", result.Body.String())
	}
}

func TestSQLiteTransientLockWaitsAndCommitsOnce(t *testing.T) {
	directory := t.TempDir()
	databasePath := filepath.Join(directory, "transient-lock.db")
	runSQLiteMigrations(t, databasePath)
	lockerDatabase := openSQLiteDatabase(t, databasePath)
	defer func() { _ = lockerDatabase.Close() }()
	requestDatabase := openSQLiteDatabase(t, databasePath)
	defer func() { _ = requestDatabase.Close() }()
	router, err := app.Build(testAPIConfig(), requestDatabase, sqliteDependencies(t, requestDatabase, filepath.Join(directory, "uploads")))
	if err != nil {
		t.Fatalf("build SQLite transient-lock API: %v", err)
	}
	adminToken := loginAdmin(t, router)

	transaction, err := lockerDatabase.SQL.BeginTx(context.Background(), nil)
	if err != nil {
		t.Fatalf("begin SQLite transient lock transaction: %v", err)
	}
	if _, err := transaction.ExecContext(context.Background(), "UPDATE sys_config SET update_time = update_time WHERE id = 1"); err != nil {
		_ = transaction.Rollback()
		t.Fatalf("hold SQLite transient write lock: %v", err)
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = transaction.Rollback()
		close(released)
	}()

	response := serveJSON(router, http.MethodPatch, "/api/system/config/1/status", `{"status":0}`, adminToken)
	select {
	case <-released:
	case <-time.After(2 * time.Second):
		t.Fatal("SQLite transient lock was not released")
	}
	assertEnvelopeCode(t, response, http.StatusOK, 200, "success")
	var auditCount int64
	if err := requestDatabase.GORM.Table("sys_oper_log").Where("module_name = 'config' AND operation_type = 'config.status' AND operator_id = 1").Count(&auditCount).Error; err != nil {
		t.Fatalf("count SQLite transient-lock audits: %v", err)
	}
	if auditCount != 1 {
		t.Fatalf("SQLite transient-lock audit count = %d, want exactly 1", auditCount)
	}
}

func runSQLiteMigrations(t *testing.T, path string) {
	runSQLiteMigrationsWithEnvironment(t, path, config.EnvironmentTest)
}

func runSQLiteMigrationsWithEnvironment(t *testing.T, path, environmentName string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, "go", "run", "./cmd/migrate", "up", "--kind", "all")
	command.Dir = projectRoot(t)
	command.Env = sqliteEnvironment(path, environmentName)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("run SQLite migrations: %v\n%s", err, output)
	}
}

func sqliteEnvironment(path, environmentName string) []string {
	environment := make([]string, 0, len(os.Environ())+5)
	for _, entry := range os.Environ() {
		if strings.HasPrefix(entry, "APP_DATABASE__") || strings.HasPrefix(entry, "APP_JWT__SECRET=") || strings.HasPrefix(entry, "APP_ENV=") || strings.HasPrefix(entry, "APP_CONFIG_PROFILE=") {
			continue
		}
		environment = append(environment, entry)
	}
	return append(environment,
		"APP_ENV="+environmentName,
		"APP_DATABASE__DRIVER=sqlite",
		"APP_DATABASE__URL="+path,
		"APP_DATABASE__USERNAME=",
		"APP_DATABASE__PASSWORD=",
		"APP_JWT__SECRET=integration-test-secret-that-is-never-deployed",
	)
}

func openSQLiteDatabase(t *testing.T, path string) *platformdatabase.Database {
	t.Helper()
	database, err := platformdatabase.Open(context.Background(), config.DatabaseConfig{
		Driver:       platformdatabase.DriverSQLite,
		URL:          path,
		MaxOpenConns: 1,
		MaxIdleConns: 1,
	})
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	return database
}

func assertSQLitePragmas(t *testing.T, database *platformdatabase.Database) {
	t.Helper()
	checks := []struct {
		name string
		want string
	}{
		{name: "foreign_keys", want: "1"},
		{name: "journal_mode", want: "wal"},
		{name: "busy_timeout", want: "5000"},
		{name: "synchronous", want: "1"},
	}
	for _, check := range checks {
		var got string
		if err := database.SQL.QueryRow("PRAGMA " + check.name).Scan(&got); err != nil {
			t.Fatalf("query SQLite pragma %s: %v", check.name, err)
		}
		if got != check.want {
			t.Fatalf("SQLite pragma %s = %q, want %q", check.name, got, check.want)
		}
	}
}

func sqliteDependencies(t *testing.T, database *platformdatabase.Database, storageRoot string) app.Dependencies {
	t.Helper()
	authService, err := auth.NewService(auth.NewRepository(database.GORM), mustTokenManager(t))
	if err != nil {
		t.Fatalf("create SQLite auth service: %v", err)
	}
	rbacService, err := rbac.NewService(rbac.NewRepository(database.GORM))
	if err != nil {
		t.Fatalf("create SQLite RBAC service: %v", err)
	}
	deptService, err := dept.NewService(dept.NewRepository(database.GORM))
	if err != nil {
		t.Fatalf("create SQLite department service: %v", err)
	}
	notificationRepository := notification.NewRepository(database.GORM)
	userService, err := usermgmt.NewService(usermgmt.NewRepository(database.GORM, notificationRepository), "admin123")
	if err != nil {
		t.Fatalf("create SQLite user service: %v", err)
	}
	dictionaryService, err := dictionary.NewService(dictionary.NewRepository(database.GORM))
	if err != nil {
		t.Fatalf("create SQLite dictionary service: %v", err)
	}
	configService := sysconfig.NewService(sysconfig.NewRepository(database.GORM))
	storage, err := filemgmt.NewLocalStorage(storageRoot)
	if err != nil {
		t.Fatalf("create SQLite file storage: %v", err)
	}
	fileService, err := filemgmt.NewService(filemgmt.NewRepository(database.GORM), storage)
	if err != nil {
		t.Fatalf("create SQLite file service: %v", err)
	}
	logService, err := logmgmt.NewService(logmgmt.NewRepository(database.GORM), configService)
	if err != nil {
		t.Fatalf("create SQLite log service: %v", err)
	}
	notificationService, err := notification.NewService(notificationRepository)
	if err != nil {
		t.Fatalf("create SQLite notification service: %v", err)
	}
	return app.Dependencies{
		Auth: authService, RBAC: rbacService, Department: deptService, User: userService,
		Dictionary: dictionaryService, SysConfig: configService, File: fileService,
		Log: logService, Notification: notificationService,
		Inventory:    inventory.NewService(inventory.NewRepository(database.GORM)),
		PrintProfile: printprofile.NewService(printprofile.NewRepository(database.GORM)),
		Product:      product.NewService(product.NewRepository(database.GORM)), Partner: partner.NewService(partner.NewRepository(database.GORM)), Warehouse: warehouse.NewService(warehouse.NewRepository(database.GORM)),
	}
}

func serveMultipartFile(router http.Handler, path, token, filename, content string) *httptest.ResponseRecorder {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		panic(err)
	}
	_, _ = io.WriteString(part, content)
	_ = writer.WriteField("businessModule", "sqlite")
	if err := writer.Close(); err != nil {
		panic(err)
	}
	request := httptest.NewRequest(http.MethodPost, path, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("Authorization", token)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

func itoa(value int64) string {
	return fmt.Sprintf("%d", value)
}
