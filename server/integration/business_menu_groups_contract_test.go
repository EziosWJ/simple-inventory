//go:build integration

package integration

import (
	"context"
	"database/sql"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
	"gorm.io/gorm"
)

func TestSQLiteBusinessMenuGroupsContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "business-menu-groups.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	assertBusinessMenuGroupsMigrationContract(t, db.SQL, db.GORM, filepath.Join(projectRoot(t), "migrations", "sqlite", "seed"), "sqlite3")
}

func TestPostgresBusinessMenuGroupsContract(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker is required for PostgreSQL integration tests")
	}
	pg := startPostgres(t)
	runMigrations(t, projectRoot(t), pg.dsn)
	db := openTemporaryDatabase(t, pg.dsn)
	defer db.Close()
	assertBusinessMenuGroupsMigrationContract(t, db.SQL, db.GORM, filepath.Join(projectRoot(t), "migrations", "seed"), "postgres")
}

func assertBusinessMenuGroupsMigrationContract(t *testing.T, sqlDB *sql.DB, db *gorm.DB, seedDir, dialect string) {
	t.Helper()
	if err := goose.SetDialect(dialect); err != nil {
		t.Fatal(err)
	}
	goose.SetTableName("goose_seed_db_version")
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	type leaf struct {
		ID             int64
		Path           string
		PermissionCode *string
		ParentID       int64 `gorm:"column:parent_id"`
	}
	var leaves []leaf
	if err := db.Table("sys_menu").Select("id, path, permission_code, parent_id").Where("path LIKE '/business/%' AND menu_type='MENU'").Where("path NOT IN ?", []string{"/business/settlements", "/business/opening-balances"}).Order("path").Find(&leaves).Error; err != nil {
		t.Fatal(err)
	}
	leafIDs := make(map[string]int64, len(leaves))
	permissions := make(map[int64][]int64)
	for _, item := range leaves {
		leafIDs[item.Path] = item.ID
		var roles []int64
		if err := db.Table("sys_role_menu").Where("menu_id = ?", item.ID).Order("role_id").Pluck("role_id", &roles).Error; err != nil {
			t.Fatal(err)
		}
		permissions[item.ID] = roles
	}
	if len(leafIDs) != 14 {
		t.Fatalf("captured %d business leaves, want 14: %v", len(leafIDs), leafIDs)
	}

	// Add a restricted role with one pre-existing leaf grant. Re-applying the seed
	// should add only that leaf's navigation ancestor, never another business page.
	if err := db.Exec("INSERT INTO sys_role(role_name,role_code,status) VALUES(?,?,1)", "Sales-only", "TEST_SALES_ONLY").Error; err != nil {
		t.Fatal(err)
	}
	var roleID int64
	if err := db.Table("sys_role").Where("role_code = ?", "TEST_SALES_ONLY").Pluck("id", &roleID).Error; err != nil {
		t.Fatal(err)
	}
	salesID := leafIDs["/business/sales"]
	if err := db.Exec("INSERT INTO sys_role_menu(role_id,menu_id) VALUES(?,?)", roleID, salesID).Error; err != nil {
		t.Fatal(err)
	}
	if err := goose.DownContext(ctx, sqlDB, seedDir); err != nil {
		t.Fatalf("roll back partner page split: %v", err)
	}
	if err := goose.DownContext(ctx, sqlDB, seedDir); err != nil {
		t.Fatalf("roll back business menu groups: %v", err)
	}
	var directoryCount int64
	if err := db.Table("sys_menu").Where("path LIKE '/business/group/%'").Count(&directoryCount).Error; err != nil || directoryCount != 0 {
		t.Fatalf("directory count after rollback=%d err=%v, want 0", directoryCount, err)
	}
	for _, item := range leaves {
		var current leaf
		if err := db.Table("sys_menu").Select("id,path,permission_code,parent_id").Where("id = ?", item.ID).Take(&current).Error; err != nil {
			t.Fatalf("leaf %s missing after rollback: %v", item.Path, err)
		}
		if current.ID != item.ID || current.ParentID != 0 || current.PermissionCode == nil != (item.PermissionCode == nil) || (current.PermissionCode != nil && *current.PermissionCode != *item.PermissionCode) {
			t.Fatalf("leaf changed after rollback: before=%+v after=%+v", item, current)
		}
	}
	if err := goose.UpContext(ctx, sqlDB, seedDir); err != nil {
		t.Fatalf("re-apply business menu groups: %v", err)
	}
	if err := goose.UpContext(ctx, sqlDB, seedDir); err != nil {
		t.Fatalf("repeat business menu groups seed: %v", err)
	}

	wantGroups := map[string][]string{
		"/business/group/basic":     {"/business/products", "/business/partners", "/business/warehouse", "/business/print-profile"},
		"/business/group/purchases": {"/business/purchases", "/business/purchase-returns"},
		"/business/group/sales":     {"/business/sales", "/business/sale-returns"},
		"/business/group/inventory": {"/business/inventory-balances", "/business/inventory-entries", "/business/inventory-adjustments"},
		"/business/group/partners":  {"/business/partner-balances", "/business/partner-ledger", "/business/settlements", "/business/refunds", "/business/opening-balances"},
	}
	groupIDs := map[string]int64{}
	for path, expectedLeaves := range wantGroups {
		var group struct {
			ID             int64
			ParentID       int64 `gorm:"column:parent_id"`
			MenuName       string
			MenuType       string
			PermissionCode *string
		}
		if err := db.Table("sys_menu").Select("id,parent_id,menu_name,menu_type,permission_code").Where("path = ? AND deleted=0", path).Take(&group).Error; err != nil {
			t.Fatalf("load group %s: %v", path, err)
		}
		if group.ParentID != 0 || group.MenuType != "DIR" || group.PermissionCode != nil {
			t.Fatalf("invalid directory %s: %+v", path, group)
		}
		groupIDs[path] = group.ID
		var childPaths []string
		if err := db.Table("sys_menu").Where("parent_id = ? AND menu_type='MENU' AND deleted=0", group.ID).Order("path").Pluck("path", &childPaths).Error; err != nil {
			t.Fatal(err)
		}
		if !sameStrings(childPaths, expectedLeaves) {
			t.Fatalf("children of %s = %v, want %v", path, childPaths, expectedLeaves)
		}
		var duplicates int64
		if err := db.Table("sys_menu").Where("path = ? AND deleted=0", path).Count(&duplicates).Error; err != nil || duplicates != 1 {
			t.Fatalf("group %s count=%d err=%v, want exactly one", path, duplicates, err)
		}
	}
	for _, item := range leaves {
		var current leaf
		if err := db.Table("sys_menu").Select("id,path,permission_code,parent_id").Where("id = ?", item.ID).Take(&current).Error; err != nil {
			t.Fatal(err)
		}
		if current.ID != item.ID || current.ParentID == 0 || current.PermissionCode == nil != (item.PermissionCode == nil) || (current.PermissionCode != nil && *current.PermissionCode != *item.PermissionCode) {
			t.Fatalf("leaf identity/path permission changed: before=%+v after=%+v", item, current)
		}
	}
	var roleMenus []int64
	if err := db.Table("sys_role_menu").Where("role_id = ?", roleID).Order("menu_id").Pluck("menu_id", &roleMenus).Error; err != nil {
		t.Fatal(err)
	}
	if !sameInt64s(roleMenus, []int64{salesID, groupIDs["/business/group/sales"]}) {
		t.Fatalf("restricted role menu grants=%v; want only sales leaf %d and its directory %d", roleMenus, salesID, groupIDs["/business/group/sales"])
	}
	for id, rolesBefore := range permissions {
		var rolesAfter []int64
		if err := db.Table("sys_role_menu").Where("menu_id = ?", id).Order("role_id").Pluck("role_id", &rolesAfter).Error; err != nil {
			t.Fatal(err)
		}
		// The fixture deliberately added the restricted role to one leaf after capture.
		if id == salesID {
			rolesBefore = append(rolesBefore, roleID)
			rolesBefore = sortedInt64s(rolesBefore)
		}
		if !sameInt64s(rolesBefore, rolesAfter) {
			t.Fatalf("leaf role grants changed for %d: before=%v after=%v", id, rolesBefore, rolesAfter)
		}
	}
	for groupPath, groupID := range groupIDs {
		var groupRoles []int64
		if err := db.Table("sys_role_menu").Where("menu_id = ?", groupID).Order("role_id").Pluck("role_id", &groupRoles).Error; err != nil {
			t.Fatal(err)
		}
		wantRoles := []int64{}
		for leafPath, leafID := range leafIDs {
			if expectedGroupForBusinessPath(leafPath) == groupPath {
				for _, role := range permissions[leafID] {
					wantRoles = append(wantRoles, role)
				}
			}
		}
		if groupPath == "/business/group/sales" {
			wantRoles = append(wantRoles, roleID)
		}
		wantRoles = uniqueSortedInt64s(wantRoles)
		if !sameInt64s(groupRoles, wantRoles) {
			t.Fatalf("directory %s roles=%v, want navigation ancestors %v", groupPath, groupRoles, wantRoles)
		}
	}
}

func expectedGroupForBusinessPath(path string) string {
	switch path {
	case "/business/products", "/business/partners", "/business/warehouse", "/business/print-profile":
		return "/business/group/basic"
	case "/business/purchases", "/business/purchase-returns":
		return "/business/group/purchases"
	case "/business/sales", "/business/sale-returns":
		return "/business/group/sales"
	case "/business/inventory-balances", "/business/inventory-entries", "/business/inventory-adjustments":
		return "/business/group/inventory"
	default:
		return "/business/group/partners"
	}
}

func sameStrings(a, b []string) bool {
	a, b = sortedStrings(a), sortedStrings(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sortedStrings(values []string) []string {
	copyValues := append([]string(nil), values...)
	sort.Strings(copyValues)
	return copyValues
}

func sameInt64s(a, b []int64) bool {
	a, b = sortedInt64s(a), sortedInt64s(b)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sortedInt64s(values []int64) []int64 {
	copyValues := append([]int64(nil), values...)
	sort.Slice(copyValues, func(i, j int) bool { return copyValues[i] < copyValues[j] })
	return copyValues
}

func uniqueSortedInt64s(values []int64) []int64 {
	values = sortedInt64s(values)
	unique := values[:0]
	for _, value := range values {
		if len(unique) == 0 || unique[len(unique)-1] != value {
			unique = append(unique, value)
		}
	}
	return unique
}
