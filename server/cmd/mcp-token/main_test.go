package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	box, e := mcpserver.NewSecretBox(cfg.JWT.Secret)
	if e != nil {
		t.Fatal(e)
	}
	repo := mcpserver.NewRepository(db.GORM, box)
	var out bytes.Buffer
	// The administration default is a permanent credential: no expiry column.
	if e := run(context.Background(), []string{"issue", "--user-id", "1", "--name", "cherry-test", "--actor", "operator-test"}, &out); e != nil {
		t.Fatal(e)
	}
	var issued struct {
		Token    string          `json:"token"`
		Metadata mcpserver.Token `json:"metadata"`
	}
	if e := json.Unmarshal(out.Bytes(), &issued); e != nil {
		t.Fatal(e)
	}
	if len(issued.Token) != 70 || issued.Metadata.ID != 1 || issued.Metadata.ExpiresAt != nil {
		t.Fatalf("issuance metadata: %+v", issued.Metadata)
	}
	// The deprecated scope flag is accepted but no longer restricts tools.
	out.Reset()
	if e := run(context.Background(), []string{"issue", "--user-id", "1", "--client", "phone", "--actor", "operator-test", "--scopes", "search_products", "--days", "30"}, &out); e != nil {
		t.Fatal(e)
	}
	var dated struct {
		Metadata mcpserver.Token `json:"metadata"`
	}
	if e := json.Unmarshal(out.Bytes(), &dated); e != nil {
		t.Fatal(e)
	}
	if dated.Metadata.ExpiresAt == nil || dated.Metadata.ExpiresAt.Sub(time.Now().UTC()).Hours() < 719 {
		t.Fatalf("dated expiry: %+v", dated.Metadata)
	}
	out.Reset()
	if e := run(context.Background(), []string{"list", "--user-id", "1"}, &out); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(out.String(), issued.Token) || strings.Contains(out.String(), "digest") || strings.Contains(out.String(), "secretCipher") {
		t.Fatalf("list secret disclosure: %s", out.String())
	}
	if !strings.Contains(out.String(), "cherry-test") || !strings.Contains(out.String(), `"permanent":true`) {
		t.Fatalf("list content: %s", out.String())
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
	if _, _, e := repo.Authenticate(context.Background(), issued.Token); !errors.Is(e, mcpserver.ErrUnauthorized) {
		t.Fatalf("revocation: %v", e)
	}
	// A wrong display key leaves digest authentication working while the
	// recoverable copy stays unreadable.
	other, e := mcpserver.NewSecretBox("a-different-jwt-secret")
	if e != nil {
		t.Fatal(e)
	}
	if _, e := mcpserver.NewRepository(db.GORM, other).Reveal(context.Background(), 1); !errors.Is(e, mcpserver.ErrSecretUnavailable) {
		t.Fatalf("changed key: %v", e)
	}
	for _, args := range [][]string{
		{"issue", "--user-id", "1", "--name", "", "--actor", "op"},
		{"issue", "--user-id", "999", "--name", "test", "--actor", "op"},
		{"issue", "--user-id", "1", "--name", "test", "--actor", "op", "--days", "4000"},
		{"issue", "--user-id", "1", "--name", "test", "--actor", "op", "--days", "7", "--expires-on", "2030-01-01"},
		{"issue", "--user-id", "1", "--name", "test", "--actor", "op", "--ttl", "25h"},
		{"issue", "--user-id", "1", "--name", "test", "--actor", "op", "--expires-on", "2020-01-01"},
		{"revoke", "--id", "0"},
		{"unknown"},
		{"list", "unexpected"},
	} {
		if run(context.Background(), args, &out) == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
