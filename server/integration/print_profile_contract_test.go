//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
)

const printProfilePath = "/api/v1/print-profile"

func TestSQLitePrintProfileContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "print-profile.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	router, err := app.Build(testAPIConfig(), db, sqliteDependencies(t, db, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	assertPrintProfileContract(t, router, db, "sqlite")
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = openSQLiteDatabase(t, path)
	defer db.Close()
	router, err = app.Build(testAPIConfig(), db, sqliteDependencies(t, db, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	got := inventoryData[map[string]string](t, serveJSON(router, http.MethodGet, printProfilePath, "", loginAdmin(t, router)), http.StatusOK)
	if got["name"] != "云杉商贸" || got["phone"] != "13800001234" || got["address"] != "上海市徐汇区" {
		t.Fatalf("reopened profile=%v", got)
	}
}

func TestPostgresPrintProfileContract(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker is required for PostgreSQL integration tests")
	}
	temporary := startPostgres(t)
	db := openTemporaryDatabase(t, temporary.dsn)
	defer db.Close()
	runMigrations(t, projectRoot(t), temporary.dsn)
	router, err := app.Build(testAPIConfig(), db, testDependencies(t, db, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	assertPrintProfileContract(t, router, db, "postgres")
}

func assertPrintProfileContract(t *testing.T, router http.Handler, db *platformdatabase.Database, dialect string) {
	t.Helper()
	var menuCount, adminGrantCount int64
	if err := db.GORM.Table("sys_menu").Where("path=? AND deleted=0", "/business/print-profile").Count(&menuCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Table("sys_role_menu rm").Joins("JOIN sys_role r ON r.id=rm.role_id").Joins("JOIN sys_menu m ON m.id=rm.menu_id").Where("r.role_code='ADMIN' AND m.path=?", "/business/print-profile").Count(&adminGrantCount).Error; err != nil {
		t.Fatal(err)
	}
	if menuCount != 1 || adminGrantCount != 1 {
		t.Fatalf("print profile menu=%d ADMIN grants=%d", menuCount, adminGrantCount)
	}
	token := loginAdmin(t, router)
	assertUnauthenticated(t, serveJSON(router, http.MethodGet, printProfilePath, "", ""))
	assertUnauthenticated(t, serveJSON(router, http.MethodPut, printProfilePath, `{"name":"blocked"}`, ""))
	initial := inventoryData[map[string]string](t, serveJSON(router, http.MethodGet, printProfilePath, "", token), http.StatusOK)
	if initial["name"] != "" {
		t.Fatalf("seeded name=%q; want blank ready-to-configure profile", initial["name"])
	}
	invalid := serveJSON(router, http.MethodPut, printProfilePath, `{"name":"   ","phone":"","address":""}`, token)
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("blank name status=%d body=%s", invalid.Code, invalid.Body.String())
	}
	tooLong := serveJSON(router, http.MethodPut, printProfilePath, fmt.Sprintf(`{"name":"%s"}`, strings.Repeat("名", 201)), token)
	if tooLong.Code != http.StatusBadRequest {
		t.Fatalf("long name status=%d", tooLong.Code)
	}
	updated := inventoryData[map[string]string](t, serveJSON(router, http.MethodPut, printProfilePath, `{"name":"  云杉商贸  ","phone":" 13800001234 ","address":" 上海市徐汇区 "}`, token), http.StatusOK)
	if updated["name"] != "云杉商贸" || updated["phone"] != "13800001234" || updated["address"] != "上海市徐汇区" {
		t.Fatalf("updated profile=%v", updated)
	}
	readBack := inventoryData[map[string]string](t, serveJSON(router, http.MethodGet, printProfilePath, "", token), http.StatusOK)
	if readBack["name"] != updated["name"] || readBack["address"] != updated["address"] {
		t.Fatalf("readback=%v updated=%v", readBack, updated)
	}
	var actor, audits int64
	var actors []int64
	if err := db.GORM.Table("sys_oper_log").Where("module_name=? AND operation_type=?", "print-profile", "print-profile.update").Count(&audits).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Table("sys_oper_log").Where("module_name=? AND operation_type=?", "print-profile", "print-profile.update").Pluck("operator_id", &actors).Error; err != nil {
		t.Fatal(err)
	}
	if len(actors) > 0 {
		actor = actors[0]
	}
	if audits != 1 || actor != 1 {
		t.Fatalf("audit count=%d actor=%d; want one operation by admin", audits, actor)
	}

	if dialect == "sqlite" {
		if err := db.GORM.Exec(`CREATE TRIGGER fail_print_profile_audit BEFORE INSERT ON sys_oper_log WHEN NEW.operation_type='print-profile.update' BEGIN SELECT RAISE(ABORT,'forced audit failure'); END`).Error; err != nil {
			t.Fatal(err)
		}
	} else {
		if err := db.GORM.Exec(`CREATE FUNCTION fail_print_profile_audit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.operation_type='print-profile.update' THEN RAISE EXCEPTION 'forced audit failure'; END IF; RETURN NEW; END $$`).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.GORM.Exec(`CREATE TRIGGER fail_print_profile_audit BEFORE INSERT ON sys_oper_log FOR EACH ROW EXECUTE FUNCTION fail_print_profile_audit()`).Error; err != nil {
			t.Fatal(err)
		}
	}
	failed := serveJSON(router, http.MethodPut, printProfilePath, `{"name":"必须回滚","phone":"","address":""}`, token)
	if failed.Code != http.StatusInternalServerError {
		t.Fatalf("audit failure status=%d body=%s", failed.Code, failed.Body.String())
	}
	var name string
	if err := db.GORM.Table("sys_config").Select("config_value").Where("config_key='business.print-profile.name'").Scan(&name).Error; err != nil {
		t.Fatal(err)
	}
	if name != "云杉商贸" {
		t.Fatalf("profile changed despite audit failure: %q", name)
	}
	if dialect == "sqlite" {
		if err := db.GORM.Exec(`DROP TRIGGER fail_print_profile_audit`).Error; err != nil {
			t.Fatal(err)
		}
	} else {
		_ = db.GORM.Exec(`DROP TRIGGER fail_print_profile_audit ON sys_oper_log`).Error
		_ = db.GORM.Exec(`DROP FUNCTION fail_print_profile_audit()`).Error
	}
}
