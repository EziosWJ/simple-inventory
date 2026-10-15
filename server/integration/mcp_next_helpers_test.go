//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/EziosWJ/simple-inventory/server/internal/app"
	"github.com/EziosWJ/simple-inventory/server/internal/audit"
	"github.com/EziosWJ/simple-inventory/server/internal/auth"
	"github.com/EziosWJ/simple-inventory/server/internal/config"
	"github.com/EziosWJ/simple-inventory/server/internal/inventory"
	"github.com/EziosWJ/simple-inventory/server/internal/mcpserver"
	"github.com/EziosWJ/simple-inventory/server/internal/partner"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"github.com/EziosWJ/simple-inventory/server/internal/product"
	"github.com/EziosWJ/simple-inventory/server/internal/purchase"
	"github.com/EziosWJ/simple-inventory/server/internal/receivable"
	"github.com/EziosWJ/simple-inventory/server/internal/sale"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The next-stage MCP contracts share one fixture: a real router with MCP and
// the token administration service, an administrator session, and a separate
// credential subject whose business permissions are granted and revoked live.
const mcpTestSecret = "integration-mcp-display-secret"

// mcpSubjectSequence keeps each fixture's subject and role names unique so one
// contract can build more than one router over the same database.
var mcpSubjectSequence atomic.Int64

type mcpFixture struct {
	db         *platformdatabase.Database
	handler    http.Handler
	server     *httptest.Server
	repo       *mcpserver.Repository
	service    *mcpserver.TokenService
	adminToken string
	subjectID  int64
	roleID     int64
}

func mustExec(t *testing.T, db *platformdatabase.Database, sql string, args ...any) {
	t.Helper()
	if err := db.GORM.Exec(sql, args...).Error; err != nil {
		t.Fatalf("exec %s: %v", sql, err)
	}
}

func newMCPFixture(t *testing.T, db *platformdatabase.Database, sqlite bool, mutators ...func(*config.Config)) *mcpFixture {
	t.Helper()
	box, err := mcpserver.NewSecretBox(mcpTestSecret)
	if err != nil {
		t.Fatal(err)
	}
	repo := mcpserver.NewRepository(db.GORM, box)
	service, err := mcpserver.NewTokenService(repo)
	if err != nil {
		t.Fatal(err)
	}
	cfg := testAPIConfig()
	cfg.MCP = config.MCPConfig{Enabled: true, MaxBodyBytes: 65536, MaxOutputBytes: 512 * 1024, Timeout: 10 * time.Second, RequestsPerMinute: 600, MaxConcurrent: 4, MaxPage: 1000}
	var deps app.Dependencies
	if sqlite {
		deps = sqliteDependencies(t, db, t.TempDir())
	} else {
		deps = testDependencies(t, db, t.TempDir())
	}
	deps.MCP = repo
	deps.MCPTokens = service
	deps.Product = product.NewService(product.NewRepository(db.GORM))
	deps.Partner = partner.NewService(partner.NewRepository(db.GORM))
	deps.Inventory = inventory.NewService(inventory.NewRepository(db.GORM))
	deps.Receivable = receivable.NewService(receivable.NewRepository(db.GORM))
	deps.Purchase = purchase.NewService(purchase.NewRepository(db.GORM))
	deps.Sale = sale.NewService(sale.NewRepository(db.GORM))
	// Bind a real loopback socket first so the exact authority is known.
	server := httptest.NewUnstartedServer(nil)
	cfg.MCP.AllowedHosts = []string{server.Listener.Addr().String(), "mcp.example.test"}
	for _, mutate := range mutators {
		mutate(&cfg)
	}
	router, err := app.Build(cfg, db, deps)
	if err != nil {
		t.Fatal(err)
	}
	server.Config.Handler = router
	server.Start()
	t.Cleanup(server.Close)
	f := &mcpFixture{db: db, handler: router, server: server, repo: repo, service: service}
	f.adminToken = loginAdmin(t, router)
	f.subjectID, f.roleID = f.createSubject(t)
	return f
}

// createSubject isolates the credential owner from the administrator account:
// it deliberately carries an unusable password and only the business grants.
func (f *mcpFixture) createSubject(t *testing.T) (int64, int64) {
	t.Helper()
	suffix := mcpSubjectSequence.Add(1)
	user := auth.User{Username: fmt.Sprintf("mcp-next-subject-%d", suffix), Nickname: "下一阶段测试身份", PasswordHash: "!", Status: 1}
	if err := f.db.GORM.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	roleCode := fmt.Sprintf("mcp-next-%d", suffix)
	mustExec(t, f.db, "INSERT INTO sys_role(role_name,role_code,status) VALUES ('MCP next subject',?,1)", roleCode)
	var role int64
	if err := f.db.GORM.Table("sys_role").Where("role_code=?", roleCode).Pluck("id", &role).Error; err != nil {
		t.Fatal(err)
	}
	mustExec(t, f.db, "INSERT INTO sys_user_role(user_id,role_id) VALUES (?,?)", user.ID, role)
	f.grantAllTools(t, role)
	return user.ID, role
}

// grantAllTools grants every distinct business permission the read-only tools
// map to, exactly as a role administrator would.
func (f *mcpFixture) grantAllTools(t *testing.T, role int64) {
	t.Helper()
	codes := map[string]bool{}
	for _, code := range mcpserver.Permissions {
		codes[code] = true
	}
	for code := range codes {
		mustExec(t, f.db, "INSERT INTO sys_role_menu(role_id,menu_id) SELECT ?,id FROM sys_menu WHERE permission_code=?", role, code)
	}
}

func (f *mcpFixture) revokeTool(t *testing.T, code string) {
	t.Helper()
	mustExec(t, f.db, "DELETE FROM sys_role_menu WHERE role_id=? AND menu_id IN (SELECT id FROM sys_menu WHERE permission_code=?)", f.roleID, code)
}

func (f *mcpFixture) issue(t *testing.T, name string, expires *time.Time) (mcpserver.Token, string) {
	t.Helper()
	token, raw, err := f.repo.Create(context.Background(), mcpserver.CreateOptions{UserID: f.subjectID, Name: name, Label: "contract", ExpiresAt: expires}, audit.Event{Action: "mcp-token.create", Resource: "mcp-token"})
	if err != nil {
		t.Fatal(err)
	}
	return token, raw
}

// mcpResponse is one raw HTTP exchange with the MCP endpoint.
type mcpResponse struct {
	status int
	body   []byte
}

func (f *mcpFixture) request(t *testing.T, method, body string, headers map[string][]string) mcpResponse {
	t.Helper()
	request, err := http.NewRequest(method, f.server.URL+"/mcp", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("MCP-Protocol-Version", "2025-06-18")
	for key, values := range headers {
		if key == "Host" {
			request.Host = values[0]
			continue
		}
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	return mcpResponse{status: response.StatusCode, body: payload}
}

func tokenHeader(raw string) map[string][]string { return map[string][]string{"token": {raw}} }
func bearerHeader(raw string) map[string][]string {
	return map[string][]string{"Authorization": {"Bearer " + raw}}
}

const mcpListBody = `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`

// session opens the official SDK client so tool discovery and calls run over a
// real HTTP exchange rather than an in-process shortcut.
func (f *mcpFixture) session(t *testing.T, ctx context.Context, headers map[string][]string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "next-contract", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: f.server.URL + "/mcp", HTTPClient: &http.Client{Transport: &staticHeaderClient{headers: headers}}}, &mcp.ClientSessionOptions{ProtocolVersion: "2025-06-18"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

type staticHeaderClient struct{ headers map[string][]string }

func (c *staticHeaderClient) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	for key, values := range c.headers {
		for _, value := range values {
			clone.Header.Add(key, value)
		}
	}
	return http.DefaultTransport.RoundTrip(clone)
}

func mcpToolNames(t *testing.T, ctx context.Context, session *mcp.ClientSession) []string {
	t.Helper()
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(tools.Tools))
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
	}
	return names
}

func callTool(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args any, wantError bool) map[string]any {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s transport: %v", name, err)
	}
	if result.IsError != wantError {
		t.Fatalf("%s error=%v content=%v", name, result.IsError, result.Content)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("%s content type %T", name, result.Content[0])
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(text.Text), &value); err != nil {
		t.Fatalf("%s decode: %v (%s)", name, err, text.Text)
	}
	return value
}

func toolText(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args any) string {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("%s content type %T", name, result.Content[0])
	}
	return text.Text
}
