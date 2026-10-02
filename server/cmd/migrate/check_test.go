package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestCheckRejectsUnmigratedMissingAndNewerStateWithoutWrites(t *testing.T) {
	t.Chdir(t.TempDir())
	dir := filepath.Join("migrations", "sqlite", "schema")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range []struct{ name, body string }{{"00001_one.sql", "CREATE TABLE one(id INTEGER PRIMARY KEY);"}, {"00002_two.sql", "CREATE TABLE two(id INTEGER PRIMARY KEY);"}} {
		if err := os.WriteFile(filepath.Join(dir, fixture.name), []byte("-- +goose Up\n"+fixture.body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "guard.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	check := func(wantSuccess bool) {
		t.Helper()
		var before, after int64
		if err := db.QueryRow("SELECT total_changes()").Scan(&before); err != nil {
			t.Fatal(err)
		}
		err := checkMigrationsForDriver(context.Background(), db, "sqlite", "schema")
		if (err == nil) != wantSuccess {
			t.Fatalf("check success=%v err=%v", wantSuccess, err)
		}
		if err := db.QueryRow("SELECT total_changes()").Scan(&after); err != nil {
			t.Fatal(err)
		}
		if before != after {
			t.Fatal("readonly check wrote database")
		}
	}
	check(false)
	var tables int
	if err := db.QueryRow("SELECT count(*) FROM sqlite_master WHERE type='table'").Scan(&tables); err != nil || tables != 0 {
		t.Fatalf("check created tables: %d err=%v", tables, err)
	}
	if _, _, err := applyMigrationsForDriver(context.Background(), db, "sqlite", "schema"); err != nil {
		t.Fatal(err)
	}
	check(true)
	if _, err := db.Exec("INSERT INTO goose_schema_db_version(version_id,is_applied) VALUES(1,0)"); err != nil {
		t.Fatal(err)
	}
	check(false)
	if _, err := db.Exec("INSERT INTO goose_schema_db_version(version_id,is_applied) VALUES(1,1),(99,1)"); err != nil {
		t.Fatal(err)
	}
	check(false)
}
