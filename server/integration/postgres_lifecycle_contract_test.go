//go:build integration

package integration

import (
	"os/exec"
	"testing"
)

// TestPostgresTestDatabasesAreIsolatedAndDropped guards the two promises the
// shared container makes: every test gets a database of its own, and that
// database is gone once the test ends.
func TestPostgresTestDatabasesAreIsolatedAndDropped(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("Docker is required for PostgreSQL integration tests")
	}

	var first, second testDatabase
	t.Run("each test owns its own database", func(t *testing.T) {
		first = createTestDatabase(t)
		second = createTestDatabase(t)
		if first.name == second.name {
			t.Fatalf("both tests received the database name %s", first.name)
		}

		// Probes that could only ever answer "no" would make the checks below
		// vacuous, so pin the state each one is supposed to see.
		for _, database := range []testDatabase{first, second} {
			if !testDatabaseExists(t, database) {
				t.Fatalf("test database %s is missing while the test owning it is running", database.name)
			}
		}

		owner := openTemporaryDatabase(t, first.dsn)
		defer func() { _ = owner.Close() }()
		if err := owner.GORM.Exec("CREATE TABLE lifecycle_probe (id bigint)").Error; err != nil {
			t.Fatalf("create probe table in %s: %v", first.name, err)
		}
		if !owner.GORM.Migrator().HasTable("lifecycle_probe") {
			t.Fatalf("%s cannot see the table it just created", first.name)
		}

		neighbour := openTemporaryDatabase(t, second.dsn)
		defer func() { _ = neighbour.Close() }()
		if neighbour.GORM.Migrator().HasTable("lifecycle_probe") {
			t.Fatalf("%s sees a table created in %s", second.name, first.name)
		}
	})

	// The subtest has returned, so both databases must already be dropped.
	for _, database := range []testDatabase{first, second} {
		if testDatabaseExists(t, database) {
			t.Fatalf("test database %s outlived the test that owned it", database.name)
		}
	}
}

func testDatabaseExists(t *testing.T, database testDatabase) bool {
	t.Helper()
	admin := openSharedDatabase(t, database.shared.adminConfig(), "connect to the shared PostgreSQL admin database")
	defer func() { _ = admin.Close() }()

	var count int64
	if err := admin.GORM.Raw("SELECT count(*) FROM pg_database WHERE datname = ?", database.name).Scan(&count).Error; err != nil {
		t.Fatalf("look up test database %s: %v", database.name, err)
	}
	return count > 0
}
