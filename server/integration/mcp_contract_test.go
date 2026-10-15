//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/simple-inventory/server/internal/audit"
	"github.com/EziosWJ/simple-inventory/server/internal/config"
	"github.com/EziosWJ/simple-inventory/server/internal/mcpserver"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"github.com/pressly/goose/v3"
)

// The transport contract covers authentication boundaries, host and origin
// admission, reverse-proxy forwarding, request limits, protocol negotiation
// and the disabled default on both databases.
func TestSQLiteMCPHTTPContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	assertMCPTransport(t, db, true)
}

func TestPostgresMCPHTTPContract(t *testing.T) {
	pg := startPostgres(t)
	runMigrations(t, projectRoot(t), pg.dsn)
	db := openTemporaryDatabase(t, pg.dsn)
	defer db.Close()
	assertMCPTransport(t, db, false)
}

func assertMCPTransport(t *testing.T, db *platformdatabase.Database, sqlite bool) {
	t.Helper()
	f := newMCPFixture(t, db, sqlite)
	_, raw := f.issue(t, "transport", nil)
	check := func(want int, headers map[string][]string, body string) []byte {
		t.Helper()
		response := f.request(t, http.MethodPost, body, headers)
		if response.status != want {
			t.Fatalf("HTTP %d want %d: %s", response.status, want, response.body)
		}
		return response.body
	}
	for _, headers := range []map[string][]string{nil, tokenHeader(""), bearerHeader("bad"), tokenHeader(strings.Repeat("a", 70)), tokenHeader("simcp_" + strings.Repeat("a", 64))} {
		check(401, headers, mcpListBody)
	}
	for _, headers := range []map[string][]string{{"Host": {"evil.test"}}, {"Origin": {"https://evil.test"}}} {
		check(403, mergeHeaders(tokenHeader(raw), headers), mcpListBody)
	}
	// Cloudflare Tunnel and reverse proxies append these on normal requests.
	proxy := map[string][]string{
		"X-Forwarded-For": {"192.0.2.12"}, "X-Forwarded-Host": {"mcp.example.test"},
		"X-Forwarded-Proto": {"https"}, "Forwarded": {"for=192.0.2.12;proto=https"}, "X-Real-IP": {"192.0.2.12"},
	}
	check(200, mergeHeaders(tokenHeader(raw), proxy), mcpListBody)
	check(200, mergeHeaders(tokenHeader(raw), proxy, map[string][]string{"Host": {"mcp.example.test"}}), mcpListBody)
	check(401, mergeHeaders(proxy, map[string][]string{"Host": {"mcp.example.test"}}), mcpListBody)
	check(403, mergeHeaders(tokenHeader(raw), proxy, map[string][]string{"Host": {"evil.test"}}), mcpListBody)
	check(413, tokenHeader(raw), strings.Repeat("x", 65537))
	// The original Bearer form keeps working, both forms are accepted only
	// while they agree, and a repeated or malformed header is refused rather
	// than silently choosing one of them.
	check(200, bearerHeader(raw), mcpListBody)
	check(200, mergeHeaders(tokenHeader(raw), bearerHeader(raw)), mcpListBody)
	check(400, mergeHeaders(tokenHeader(raw), bearerHeader(strings.Repeat("a", 70))), mcpListBody)
	check(400, map[string][]string{"token": {raw, raw}}, mcpListBody)
	check(400, map[string][]string{"Authorization": {"Bearer " + raw, "Bearer " + raw}}, mcpListBody)
	check(400, map[string][]string{"Authorization": {"Basic " + raw}}, mcpListBody)
	check(400, tokenHeader(raw+" trailing"), mcpListBody)
	for _, version := range []string{"2025-03-26", "2025-06-18", "2025-11-25"} {
		body := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":%q,"capabilities":{},"clientInfo":{"name":"contract","version":"test"}}}`, version)
		if b := check(200, tokenHeader(raw), body); !bytes.Contains(b, []byte(version)) {
			t.Fatalf("negotiation: %s", b)
		}
	}
	ctx := context.Background()
	session := f.session(t, ctx, tokenHeader(raw))
	names := mcpToolNames(t, ctx, session)
	if len(names) != 10 {
		t.Fatalf("tools=%v", names)
	}
	for want := range mcpserver.Permissions {
		if !containsString(names, want) {
			t.Fatalf("missing tool %s in %v", want, names)
		}
	}
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range tools.Tools {
		encoded, _ := json.Marshal(tool.InputSchema)
		if !bytes.Contains(encoded, []byte(`"additionalProperties":false`)) {
			t.Fatalf("%s schema not strict", tool.Name)
		}
	}
	assertMCPDisabled(t, db, sqlite)
}

// assertMCPDisabled proves the disabled default never exposes tools and never
// falls back to the embedded frontend, for every method the endpoint accepts.
func assertMCPDisabled(t *testing.T, db *platformdatabase.Database, sqlite bool) {
	t.Helper()
	f := newMCPFixture(t, db, sqlite, func(cfg *config.Config) { cfg.MCP.Enabled = false })
	for _, method := range []string{http.MethodGet, http.MethodPost, http.MethodOptions} {
		request := httptest.NewRequest(method, "/mcp", nil)
		request.Header.Set("Origin", "https://evil.test")
		recorder := httptest.NewRecorder()
		f.handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotFound || strings.Contains(recorder.Body.String(), "<html") {
			t.Fatalf("disabled MCP fell through: %d %s", recorder.Code, recorder.Body.String())
		}
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func mergeHeaders(sets ...map[string][]string) map[string][]string {
	out := map[string][]string{}
	for _, set := range sets {
		for key, values := range set {
			out[key] = append(out[key], values...)
		}
	}
	return out
}

func TestSQLiteMCPMigrationUpgrade(t *testing.T)   { assertMCPUpgrade(t, true) }
func TestPostgresMCPMigrationUpgrade(t *testing.T) { assertMCPUpgrade(t, false) }

// assertMCPUpgrade starts from the version before the token tables existed,
// re-runs every later migration over existing business rows, and checks that
// the new administration columns arrive with the logical version.
func assertMCPUpgrade(t *testing.T, sqlite bool) {
	var db *platformdatabase.Database
	root := filepath.Join(projectRoot(t), "migrations")
	dialect := "postgres"
	if sqlite {
		db = openSQLiteDatabase(t, filepath.Join(t.TempDir(), "upgrade.db"))
		root = filepath.Join(root, "sqlite")
		dialect = "sqlite3"
	} else {
		pg := startPostgres(t)
		db = openTemporaryDatabase(t, pg.dsn)
	}
	defer db.Close()
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	must(goose.SetDialect(dialect))
	goose.SetTableName("goose_schema_db_version")
	must(goose.UpToContext(context.Background(), db.SQL, filepath.Join(root, "schema"), 30))
	must(db.GORM.Exec("INSERT INTO sys_user(username,nickname,password,status) VALUES ('old-mcp-user','旧账号','!',1)").Error)
	must(db.GORM.Exec("INSERT INTO product(code,name,type,unit) VALUES ('OLD-MCP','旧商品','GOODS','支')").Error)
	// A digest-only credential of the previous shape must survive the upgrade.
	legacyDigest := "0f1e2d3c4b5a69788796a5b4c3d2e1f00f1e2d3c4b5a69788796a5b4c3d2e1f0"
	must(db.GORM.Exec("INSERT INTO mcp_token(user_id,client_id,digest,scopes,expires_at,created_at,created_by) SELECT id,'legacy-old',?,'[\"search_products\"]',?,?, 'old-operator' FROM sys_user WHERE username='old-mcp-user'", legacyDigest, time.Now().UTC().Add(time.Hour), time.Now().UTC()).Error)
	must(goose.UpContext(context.Background(), db.SQL, filepath.Join(root, "schema")))
	version, e := goose.GetDBVersion(db.SQL)
	must(e)
	if version != 31 {
		t.Fatalf("migration version %d", version)
	}
	var count int64
	must(db.GORM.Table("product").Where("code='OLD-MCP'").Count(&count).Error)
	if count != 1 {
		t.Fatal("old data changed")
	}
	var legacy mcpserver.Token
	must(db.GORM.Where("client_id='legacy-old'").Take(&legacy).Error)
	if legacy.Digest != legacyDigest || legacy.ExpiresAt == nil || legacy.CreatedBy != "old-operator" || legacy.SecretVersion != nil || legacy.DisabledAt != nil {
		t.Fatalf("legacy credential changed by the upgrade: %+v", legacy)
	}
	var user int64
	must(db.GORM.Table("sys_user").Where("username='old-mcp-user'").Pluck("id", &user).Error)
	box, e := mcpserver.NewSecretBox(mcpTestSecret)
	must(e)
	repo := mcpserver.NewRepository(db.GORM, box)
	fresh, _, e := repo.Create(context.Background(), mcpserver.CreateOptions{UserID: user, Name: "after-upgrade", ExpiresAt: ptrTime(time.Now().UTC().Add(time.Hour))}, audit.Event{Action: "mcp-token.create", Resource: "mcp-token"})
	must(e)
	if fresh.SecretVersion == nil || fresh.SecretCipher == "" {
		t.Fatal("new credential is not recoverable")
	}
	must(goose.DownContext(context.Background(), db.SQL, filepath.Join(root, "schema")))
	must(goose.UpContext(context.Background(), db.SQL, filepath.Join(root, "schema")))
	version, e = goose.GetDBVersion(db.SQL)
	must(e)
	if version != 31 {
		t.Fatalf("round trip version %d", version)
	}
}

func ptrTime(value time.Time) *time.Time { return &value }
