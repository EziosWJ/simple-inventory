//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
	"github.com/EziosWJ/simple-inventory/server/internal/auth"
	"github.com/EziosWJ/simple-inventory/server/internal/config"
	"github.com/EziosWJ/simple-inventory/server/internal/inventory"
	"github.com/EziosWJ/simple-inventory/server/internal/mcpserver"
	database "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"github.com/EziosWJ/simple-inventory/server/internal/receivable"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pressly/goose/v3"
)

func TestSQLiteMCPHTTPContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	assertMCP(t, db, true)
}
func TestPostgresMCPHTTPContract(t *testing.T) {
	pg := startPostgres(t)
	runMigrations(t, projectRoot(t), pg.dsn)
	db := openTemporaryDatabase(t, pg.dsn)
	defer db.Close()
	assertMCP(t, db, false)
}

type mcpRoundTrip struct{ raw string }

func (rt mcpRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+rt.raw)
	return http.DefaultTransport.RoundTrip(r)
}
func assertMCP(t *testing.T, db *database.Database, sqlite bool) {
	t.Helper()
	ctx := context.Background()
	repo := mcpserver.NewRepository(db.GORM)
	// Separate test subject, password deliberately unusable; never touch a production account.
	u := auth.User{Username: "father-mcp-test", Nickname: "父亲隔离测试身份", PasswordHash: "!", Status: 1}
	must := func(e error) {
		t.Helper()
		if e != nil {
			t.Fatal(e)
		}
	}
	must(db.GORM.Create(&u).Error)
	must(db.GORM.Exec("INSERT INTO sys_role(role_name,role_code,status) VALUES ('MCP test','mcp-test',1)").Error)
	var role int64
	must(db.GORM.Table("sys_role").Where("role_code='mcp-test'").Pluck("id", &role).Error)
	must(db.GORM.Exec("INSERT INTO sys_user_role(user_id,role_id) VALUES (?,?)", u.ID, role).Error)
	for _, permission := range mcpserver.Permissions {
		must(db.GORM.Exec("INSERT INTO sys_role_menu(role_id,menu_id) SELECT ?,id FROM sys_menu WHERE permission_code=?", role, permission).Error)
	}
	// Menus may legitimately have a NULL permission; they confer no tool access.
	must(db.GORM.Exec("INSERT INTO sys_menu(menu_name,menu_type,status) VALUES ('无权限编码菜单','DIR',1)").Error)
	must(db.GORM.Exec("INSERT INTO sys_role_menu(role_id,menu_id) SELECT ?,id FROM sys_menu WHERE menu_name='无权限编码菜单'", role).Error)
	cfg := testAPIConfig()
	cfg.MCP = config.MCPConfig{Enabled: true, MaxBodyBytes: 65536, MaxOutputBytes: 131072, Timeout: 10 * time.Second, RequestsPerMinute: 600, MaxConcurrent: 4, MaxPage: 1000}
	var deps app.Dependencies
	if sqlite {
		deps = sqliteDependencies(t, db, t.TempDir())
	} else {
		deps = testDependencies(t, db, t.TempDir())
	}
	deps.MCP = repo
	deps.Inventory = inventory.NewService(inventory.NewRepository(db.GORM))
	deps.Receivable = receivable.NewService(receivable.NewRepository(db.GORM))
	// Bind a real loopback socket first so exact authority is known.
	server := httptest.NewUnstartedServer(nil)
	cfg.MCP.AllowedHosts = []string{server.Listener.Addr().String(), "mcp.example.test"}
	router, e := app.Build(cfg, db, deps)
	must(e)
	server.Config.Handler = router
	server.Start()
	defer server.Close()
	scopes := []string{"search_products", "search_partners", "get_inventory_balances", "get_partner_balances"}
	credential, raw, e := repo.Issue(ctx, u.ID, "cherry-test", "contract-admin", scopes, 30*24*time.Hour)
	must(e)
	if !strings.HasPrefix(raw, "simcp_") || credential.Digest == raw {
		t.Fatal("credential not independent")
	}
	listed, e := repo.List(ctx, u.ID)
	must(e)
	j, _ := json.Marshal(listed)
	if bytes.Contains(j, []byte(raw)) || bytes.Contains(j, []byte(credential.Digest)) {
		t.Fatal("list leaked secret")
	}
	admin := loginAdmin(t, router)
	createProduct := func(code, kind string) int64 {
		return inventoryData[struct{ ID int64 }](t, serveJSON(router, "POST", "/api/v1/products", fmt.Sprintf(`{"code":%q,"name":"同名硒鼓","type":%q,"unit":"支","purchasePrice":"10.20","salePrice":"30.00","remark":"private"}`, code, kind), admin), 200).ID
	}
	goods := createProduct("MCP-G1", "GOODS")
	_ = createProduct("MCP-G2", "GOODS")
	service := createProduct("MCP-S", "SERVICE")
	pt := inventoryData[struct{ ID int64 }](t, serveJSON(router, "POST", "/api/v1/partners", `{"code":"MCP-P","name":"隔离客户供应商","type":"COMPANY","isCustomer":true,"isSupplier":true,"contact":"private","phone":"secret","bankAccount":"secret"}`, admin), 200).ID
	request := func(token, body string, headers map[string]string) (int, []byte) {
		t.Helper()
		r, e := http.NewRequest("POST", server.URL+"/mcp", strings.NewReader(body))
		must(e)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Accept", "application/json, text/event-stream")
		r.Header.Set("MCP-Protocol-Version", "2025-06-18")
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		for k, v := range headers {
			if k == "Host" {
				r.Host = v
			} else {
				r.Header.Set(k, v)
			}
		}
		res, e := http.DefaultClient.Do(r)
		must(e)
		defer res.Body.Close()
		b, e := io.ReadAll(res.Body)
		must(e)
		return res.StatusCode, b
	}
	const list = `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	checkHTTP := func(want int, token, body string, headers map[string]string) []byte {
		t.Helper()
		status, b := request(token, body, headers)
		if status != want {
			t.Fatalf("HTTP %d want %d: %s", status, want, b)
		}
		return b
	}
	for _, token := range []string{"", "bad", strings.Repeat("a", 70)} {
		checkHTTP(401, token, list, nil)
	}
	for _, headers := range []map[string]string{{"Host": "evil.test"}, {"Origin": "https://evil.test"}} {
		checkHTTP(403, raw, list, headers)
	}
	// Cloudflare Tunnel / reverse proxies append these headers on normal requests.
	proxyHeaders := map[string]string{"X-Forwarded-For": "192.0.2.12", "X-Forwarded-Host": "mcp.example.test", "X-Forwarded-Proto": "https", "Forwarded": "for=192.0.2.12;proto=https", "X-Real-IP": "192.0.2.12"}
	checkHTTP(200, raw, list, proxyHeaders)
	proxyHeaders["Host"] = "mcp.example.test"
	checkHTTP(200, raw, list, proxyHeaders)
	checkHTTP(401, "", list, proxyHeaders)
	proxyHeaders["Host"] = "evil.test"
	checkHTTP(403, raw, list, proxyHeaders)
	checkHTTP(413, raw, strings.Repeat("x", 65537), nil)
	// Cherry v1.6.0 uses initialize and merges custom headers into fetch requests.
	for _, version := range []string{"2025-03-26", "2025-06-18", "2025-11-25"} {
		body := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":%q,"capabilities":{},"clientInfo":{"name":"cherry-compatible-contract","version":"test"}}}`, version)
		b := checkHTTP(200, raw, body, nil)
		if !bytes.Contains(b, []byte(version)) {
			t.Fatalf("negotiation: %s", b)
		}
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "contract", Version: "1"}, nil)
	session, e := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server.URL + "/mcp", HTTPClient: &http.Client{Transport: mcpRoundTrip{raw}}}, &mcp.ClientSessionOptions{ProtocolVersion: "2025-06-18"})
	must(e)
	defer session.Close()
	tools, e := session.ListTools(ctx, nil)
	must(e)
	if len(tools.Tools) != 4 {
		t.Fatalf("tools=%v", tools.Tools)
	}
	for _, tool := range tools.Tools {
		b, _ := json.Marshal(tool.InputSchema)
		if !bytes.Contains(b, []byte(`"additionalProperties":false`)) {
			t.Fatal("missing strict schema")
		}
	}
	call := func(name string, args any, wantError bool) map[string]any {
		t.Helper()
		res, e := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		must(e)
		if res.IsError != wantError {
			t.Fatalf("%s error=%v contents=%v", name, res.IsError, res.Content)
		}
		b := res.Content[0].(*mcp.TextContent).Text
		for _, secret := range []string{"purchasePrice", "salePrice", "contact", "phone", "address", "bankAccount", "remark", "secret", "private"} {
			if strings.Contains(b, secret) {
				t.Fatalf("field leaked %s: %s", secret, b)
			}
		}
		var value map[string]any
		must(json.Unmarshal([]byte(b), &value))
		return value
	}
	search := call("search_products", map[string]any{"keyword": "同名硒鼓"}, false)
	if search["total"] != float64(3) || search["pageSize"] != float64(20) || search["selectionRequired"] != true {
		t.Fatal(search)
	}
	call("search_partners", map[string]any{"identity": "CUSTOMER", "keyword": "隔离"}, false)
	stock := call("get_inventory_balances", map[string]any{"productId": goods}, false)
	if stock["quantity"] != "0" {
		t.Fatal(stock)
	}
	stock = call("get_inventory_balances", map[string]any{"productId": service}, false)
	if stock["inventoryStatus"] != "NOT_APPLICABLE_SERVICE" || stock["quantity"] != nil {
		t.Fatal(stock)
	}
	if call("get_inventory_balances", map[string]any{"productId": 999999999}, true)["error"] != "not_found" {
		t.Fatal("missing object semantics")
	}
	for _, spec := range []struct {
		name string
		args any
	}{{"get_inventory_balances", map[string]any{"productId": 0}}, {"get_inventory_balances", map[string]any{"productId": 1.2}}, {"get_inventory_balances", map[string]any{"productId": goods, "keyword": "x"}}, {"search_products", map[string]any{"page": 0}}, {"search_products", map[string]any{"page": 1001}}, {"search_products", map[string]any{"pageSize": 51}}, {"search_products", map[string]any{"type": "bad"}}, {"search_partners", map[string]any{"identity": ""}}, {"search_products", map[string]any{"keyword": nil}}, {"get_partner_balances", map[string]any{"partnerId": pt, "direction": "ALL"}}} {
		call(spec.name, spec.args, true)
	}
	balances := call("get_partner_balances", map[string]any{"partnerId": pt}, false)
	rows := balances["items"].([]any)
	if len(rows) != 2 || rows[0].(map[string]any)["hasRecords"] != false {
		t.Fatal(balances)
	}
	// Seed isolated balance states, without changing any business posting rules.
	for _, direction := range []string{"CUSTOMER", "SUPPLIER"} {
		for _, amount := range []int64{12034, -12034, 0} {
			must(db.GORM.Exec("DELETE FROM partner_balance WHERE partner_id=? AND direction=?", pt, direction).Error)
			must(db.GORM.Exec("INSERT INTO partner_balance(partner_id,direction,amount_cents,entry_count) VALUES (?,?,?,1)", pt, direction, amount).Error)
			v := call("get_partner_balances", map[string]any{"partnerId": pt, "direction": direction}, false)
			row := v["items"].([]any)[0].(map[string]any)
			if row["hasRecords"] != true || row["currency"] != "CNY" {
				t.Fatal(v)
			}
			expected := "0.00"
			if amount > 0 {
				expected = "120.34"
			}
			if amount < 0 {
				expected = "-120.34"
			}
			if row["amount"] != expected {
				t.Fatal(v)
			}
		}
	}
	must(db.GORM.Exec("UPDATE partner SET is_supplier=FALSE WHERE id=?", pt).Error)
	must(db.GORM.Exec("DELETE FROM partner_balance WHERE partner_id=? AND direction='SUPPLIER'", pt).Error)
	v := call("get_partner_balances", map[string]any{"partnerId": pt, "direction": "SUPPLIER"}, false)
	if v["items"].([]any)[0].(map[string]any)["amount"] != nil {
		t.Fatal(v)
	}
	// Opposite balances remain separate, with no implicit netting.
	must(db.GORM.Exec("UPDATE partner_balance SET amount_cents=CASE WHEN direction='CUSTOMER' THEN 12345 ELSE -6789 END WHERE partner_id=?", pt).Error)
	both := call("get_partner_balances", map[string]any{"partnerId": pt}, false)
	if len(both["items"].([]any)) != 1 {
		t.Fatal("removed supplier identity should not reappear without history")
	}
	must(db.GORM.Exec("INSERT INTO partner_balance(partner_id,direction,amount_cents,entry_count) VALUES (?,'SUPPLIER',-6789,1)", pt).Error)
	both = call("get_partner_balances", map[string]any{"partnerId": pt}, false)
	if len(both["items"].([]any)) != 2 {
		t.Fatal(both)
	}
	for _, row := range both["items"].([]any) {
		v := row.(map[string]any)
		if v["direction"] == "CUSTOMER" && (v["amount"] != "123.45" || v["meaning"] != "客户欠经营者") {
			t.Fatal(v)
		}
		if v["direction"] == "SUPPLIER" && (v["amount"] != "-67.89" || v["meaning"] != "供应商应退经营者") {
			t.Fatal(v)
		}
	}
	must(db.GORM.Exec("UPDATE sys_role SET status=0 WHERE id=?", role).Error)
	empty, e := session.ListTools(ctx, nil)
	must(e)
	if len(empty.Tools) != 0 {
		t.Fatal("disabled role disclosed tools")
	}
	must(db.GORM.Exec("UPDATE sys_role SET status=1 WHERE id=?", role).Error)
	must(db.GORM.Exec("UPDATE sys_user SET deleted=1 WHERE id=?", u.ID).Error)
	checkHTTP(401, raw, list, nil)
	must(db.GORM.Exec("UPDATE sys_user SET deleted=0 WHERE id=?", u.ID).Error)
	// Subsequent requests on the same SDK client immediately observe revocation.
	must(db.GORM.Exec("DELETE FROM sys_role_menu WHERE role_id=? AND menu_id IN (SELECT id FROM sys_menu WHERE permission_code='business:product')", role).Error)
	tools, e = session.ListTools(ctx, nil)
	must(e)
	if len(tools.Tools) != 3 {
		t.Fatal("stale tools after permission revoke")
	}
	res, e := session.CallTool(ctx, &mcp.CallToolParams{Name: "search_products", Arguments: map[string]any{}})
	if e == nil && !res.IsError {
		t.Fatal("revoked tool callable")
	}
	narrow, narrowRaw, e := repo.Issue(ctx, u.ID, "scope-test", "contract-admin", []string{"search_partners"}, time.Hour)
	must(e)
	_ = narrow
	b := checkHTTP(200, narrowRaw, list, nil)
	if bytes.Contains(b, []byte("get_inventory_balances")) {
		t.Fatal("scope disclosure")
	}
	must(db.GORM.Exec("UPDATE sys_user SET status=0 WHERE id=?", u.ID).Error)
	checkHTTP(401, raw, list, nil)
	must(db.GORM.Exec("UPDATE sys_user SET status=1 WHERE id=?", u.ID).Error)
	must(db.GORM.Model(&mcpserver.Token{}).Where("id=?", credential.ID).Update("expires_at", time.Now().UTC().Add(-time.Hour)).Error)
	checkHTTP(401, raw, list, nil)
	must(db.GORM.Model(&mcpserver.Token{}).Where("id=?", credential.ID).Update("expires_at", time.Now().UTC().Add(time.Hour)).Error)
	must(repo.Revoke(ctx, credential.ID, "contract-admin"))
	checkHTTP(401, raw, list, nil)
	cfg.MCP.Enabled = false
	disabled, e := app.Build(cfg, db, deps)
	must(e)
	for _, method := range []string{"GET", "POST", "OPTIONS"} {
		r := httptest.NewRequest(method, "/mcp", nil)
		r.Header.Set("Origin", "https://evil.test")
		rec := httptest.NewRecorder()
		disabled.ServeHTTP(rec, r)
		if rec.Code != 404 || strings.Contains(rec.Body.String(), "<html") {
			t.Fatal("disabled MCP fell through")
		}
	}
}

func TestSQLiteMCPMigrationUpgrade(t *testing.T)   { assertMCPUpgrade(t, true) }
func TestPostgresMCPMigrationUpgrade(t *testing.T) { assertMCPUpgrade(t, false) }
func assertMCPUpgrade(t *testing.T, sqlite bool) {
	var db *database.Database
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
	must(goose.UpToContext(context.Background(), db.SQL, filepath.Join(root, "schema"), 29))
	must(db.GORM.Exec("INSERT INTO sys_user(username,nickname,password,status) VALUES ('old-mcp-user','旧账号','!',1)").Error)
	must(db.GORM.Exec("INSERT INTO product(code,name,type,unit) VALUES ('OLD-MCP','旧商品','GOODS','支')").Error)
	must(goose.UpContext(context.Background(), db.SQL, filepath.Join(root, "schema")))
	version, e := goose.GetDBVersion(db.SQL)
	must(e)
	if version != 30 {
		t.Fatalf("migration version %d", version)
	}
	var count int64
	must(db.GORM.Table("product").Where("code='OLD-MCP'").Count(&count).Error)
	if count != 1 {
		t.Fatal("old data changed")
	}
	var user int64
	must(db.GORM.Table("sys_user").Where("username='old-mcp-user'").Pluck("id", &user).Error)
	repo := mcpserver.NewRepository(db.GORM)
	_, _, e = repo.Issue(context.Background(), user, "upgrade", "test", []string{"search_products"}, time.Hour)
	must(e)
	must(goose.DownContext(context.Background(), db.SQL, filepath.Join(root, "schema")))
	must(goose.UpContext(context.Background(), db.SQL, filepath.Join(root, "schema")))
}
