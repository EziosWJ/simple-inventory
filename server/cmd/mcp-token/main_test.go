package main

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/EziosWJ/simple-inventory/server/internal/config"
	"github.com/EziosWJ/simple-inventory/server/internal/mcpserver"
	database "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"github.com/pressly/goose/v3"
)

func TestTokenCLI(t *testing.T) {
	t.Chdir("../..")
	t.Setenv("APP_ENV", "test")
	t.Setenv("APP_CONFIG_PROFILE", "")
	t.Setenv("APP_DATABASE__DRIVER", "sqlite")
	t.Setenv("APP_DATABASE__URL", filepath.Join(t.TempDir(), "cli.db"))
	t.Setenv("APP_DATABASE__USERNAME", "")
	t.Setenv("APP_DATABASE__PASSWORD", "")
	t.Setenv("APP_JWT__SECRET", "isolated-cli-only-test-secret")
	cfg, e := config.Load()
	if e != nil {
		t.Fatal(e)
	}
	db, e := database.Open(context.Background(), cfg.Database)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	if e := goose.SetDialect("sqlite3"); e != nil {
		t.Fatal(e)
	}
	goose.SetTableName("goose_schema_db_version")
	if e := goose.UpContext(context.Background(), db.SQL, "migrations/sqlite/schema"); e != nil {
		t.Fatal(e)
	}
	if e := db.GORM.Exec("INSERT INTO sys_user(id,username,nickname,password,status) VALUES (1,'father-test','测试身份','!',1)").Error; e != nil {
		t.Fatal(e)
	}
	var out bytes.Buffer
	args := []string{"issue", "--user-id", "1", "--client", "cherry-test", "--actor", "operator-test", "--scopes", "search_products,search_partners"}
	if e := run(context.Background(), args, &out); e != nil {
		t.Fatal(e)
	}
	var issued struct {
		Token    string          `json:"token"`
		Metadata mcpserver.Token `json:"metadata"`
	}
	if e := json.Unmarshal(out.Bytes(), &issued); e != nil {
		t.Fatal(e)
	}
	if len(issued.Token) != 70 || issued.Metadata.ID != 1 || issued.Metadata.ExpiresAt.Sub(issued.Metadata.CreatedAt).Hours() < 719 {
		t.Fatal("issuance metadata")
	}
	out.Reset()
	if e := run(context.Background(), []string{"list", "--user-id", "1"}, &out); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(out.String(), issued.Token) || strings.Contains(out.String(), "digest") {
		t.Fatal("list secret disclosure")
	}
	if e := run(context.Background(), []string{"revoke", "--id", "1", "--actor", "operator-revoke"}, &out); e != nil {
		t.Fatal(e)
	}
	var stored mcpserver.Token
	if e := db.GORM.First(&stored, 1).Error; e != nil {
		t.Fatal(e)
	}
	if stored.RevokedAt == nil || stored.RevokedBy == nil || *stored.RevokedBy != "operator-revoke" || stored.CreatedBy != "operator-test" {
		t.Fatal("missing lifecycle actor")
	}
	if _, _, e := mcpserver.NewRepository(db.GORM).Authenticate(context.Background(), issued.Token); e != mcpserver.ErrUnauthorized {
		t.Fatal("revocation")
	}
	for _, args := range [][]string{{"issue", "--user-id", "1", "--client", "test", "--actor", "op", "--scopes", "write"}, {"issue", "--user-id", "999", "--client", "test", "--actor", "op", "--scopes", "search_products"}, {"revoke", "--id", "0"}, {"issue", "--user-id", "1", "--client", "test", "--actor", "op", "--scopes", "search_products", "--ttl", "2200h"}, {"unknown"}, {"list", "unexpected"}} {
		if run(context.Background(), args, &out) == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
