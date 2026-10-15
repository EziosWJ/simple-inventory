package mcpserver

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/EziosWJ/simple-inventory/server/internal/audit"
	"github.com/EziosWJ/simple-inventory/server/internal/config"
	"github.com/EziosWJ/simple-inventory/server/internal/product"
)

type fakeAuth struct {
	fail    error
	allowed map[string]bool
}

func (f fakeAuth) Authenticate(context.Context, string) (Token, map[string]bool, error) {
	return Token{ID: 1, UserID: 2, ClientID: "test"}, f.allowed, f.fail
}
func testConfig() config.MCPConfig {
	return config.MCPConfig{Enabled: true, AllowedHosts: []string{"localhost:18300"}, AllowedOrigins: []string{"app://cherry"}, MaxBodyBytes: 65536, MaxOutputBytes: 131072, Timeout: time.Second, RequestsPerMinute: 60, MaxConcurrent: 1, MaxPage: 1000}
}
func req(h http.Handler, method, body string, headers map[string]string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost:18300/mcp", strings.NewReader(body))
	r.RemoteAddr = "127.0.0.1:54321"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	r.Header.Set("MCP-Protocol-Version", "2025-06-18")
	r.Header.Set("Authorization", "Bearer "+strings.Repeat("a", 70))
	for k, v := range headers {
		if k == "Host" {
			r.Host = v
		} else if k == "RemoteAddr" {
			r.RemoteAddr = v
		} else {
			r.Header.Set(k, v)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

const listBody = `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`

func TestBoundaries(t *testing.T) {
	cfg := testConfig()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(cfg, fakeAuth{allowed: map[string]bool{}}, Services{}, logger)
	for _, tc := range []struct {
		headers map[string]string
		status  int
	}{{map[string]string{"RemoteAddr": "192.168.1.20:54321"}, 200}, {map[string]string{"RemoteAddr": "bad"}, 403}, {map[string]string{"Host": "localhost.evil:18300"}, 403}, {map[string]string{"Origin": "app://evil"}, 403}, {map[string]string{"Origin": "null"}, 403}, {map[string]string{"X-Forwarded-Host": "localhost:18300"}, 200}, {map[string]string{"X-Real-IP": "127.0.0.1"}, 200}, {map[string]string{"Origin": "app://cherry"}, 200}} {
		w := req(h, "POST", listBody, tc.headers)
		if w.Code != tc.status {
			t.Fatalf("%v: %d %s", tc.headers, w.Code, w.Body.String())
		}
	}
	if w := req(h, "OPTIONS", "", map[string]string{"Origin": "app://evil"}); w.Code != 403 {
		t.Fatal("preflight bypass")
	}
	if w := req(h, "OPTIONS", "", map[string]string{"Origin": "app://cherry"}); w.Code != 204 {
		t.Fatal(w.Code)
	}
	cfg.Enabled = false
	h = NewHandler(cfg, nil, Services{}, logger)
	if req(h, "POST", listBody, nil).Code != 404 {
		t.Fatal("not default disabled")
	}
	cfg.Enabled = true
	h = NewHandler(cfg, fakeAuth{fail: ErrUnavailable}, Services{}, logger)
	if req(h, "POST", listBody, nil).Code != 503 {
		t.Fatal("read failure confused with auth")
	}
	cfg.RequestsPerMinute = 1
	h = NewHandler(cfg, fakeAuth{}, Services{}, logger)
	if req(h, "POST", listBody, nil).Code != 200 || req(h, "POST", listBody, nil).Code != 429 {
		t.Fatal("rate limit")
	}
}

type blockedProduct struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
	failure error
	name    string
}

func (s *blockedProduct) Page(ctx context.Context, q product.Query) (product.Page, error) {
	s.once.Do(func() { close(s.started) })
	if s.release != nil {
		<-s.release
	}
	return product.Page{Records: []product.Product{{ID: 1, Name: s.name}}, Page: q.Page, PageSize: q.PageSize, Total: 1}, s.failure
}
func (s *blockedProduct) Find(context.Context, int64) (*product.Product, error) {
	return nil, errors.New("SQL secret")
}
func (s *blockedProduct) Save(context.Context, product.Product, bool, audit.Event) (product.Product, error) {
	panic("write")
}
func (s *blockedProduct) SetStatus(context.Context, int64, int, audit.Event) error { panic("write") }

const searchBody = `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search_products","arguments":{}}}`

func TestTimeoutConcurrencyAndSanitization(t *testing.T) {
	cfg := testConfig()
	cfg.Timeout = 30 * time.Millisecond
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	store := &blockedProduct{started: make(chan struct{}), release: make(chan struct{})}
	h := NewHandler(cfg, fakeAuth{allowed: map[string]bool{"search_products": true}}, Services{Product: product.NewService(store)}, logger)
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- req(h, "POST", searchBody, nil) }()
	<-store.started
	if w := req(h, "POST", searchBody, nil); w.Code != 429 {
		t.Fatal("concurrency", w.Code)
	}
	w := <-done
	if w.Code != 503 || !strings.Contains(w.Body.String(), "timeout") {
		t.Fatal(w.Code, w.Body.String())
	}
	if req(h, "POST", searchBody, nil).Code != 429 {
		t.Fatal("timeout released active slot")
	}
	close(store.release)
	store = &blockedProduct{started: make(chan struct{}), failure: errors.New("SELECT secret FROM password")}
	cfg.Timeout = time.Second
	h = NewHandler(cfg, fakeAuth{allowed: map[string]bool{"search_products": true}}, Services{Product: product.NewService(store)}, logger)
	w = req(h, "POST", searchBody, nil)
	if strings.Contains(w.Body.String(), "secret") || !strings.Contains(w.Body.String(), "temporarily_unavailable") {
		t.Fatal(w.Body.String())
	}
	store = &blockedProduct{started: make(chan struct{}), name: strings.Repeat("汉", 200)}
	cfg.MaxOutputBytes = 300
	h = NewHandler(cfg, fakeAuth{allowed: map[string]bool{"search_products": true}}, Services{Product: product.NewService(store)}, logger)
	w = req(h, "POST", searchBody, nil)
	if !strings.Contains(w.Body.String(), "output_too_large") {
		t.Fatal(w.Body.String())
	}
}

func TestSearchWithoutArguments(t *testing.T) {
	cfg := testConfig()
	store := &blockedProduct{started: make(chan struct{})}
	h := NewHandler(cfg, fakeAuth{allowed: map[string]bool{"search_products": true}}, Services{Product: product.NewService(store)}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	w := req(h, "POST", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search_products"}}`, nil)
	if w.Code != 200 || strings.Contains(w.Body.String(), `"isError":true`) {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestLegacyBatchCannotBypassAdmission(t *testing.T) {
	cfg := testConfig()
	h := NewHandler(cfg, fakeAuth{}, Services{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	w := req(h, "POST", "["+listBody+","+listBody+"]", map[string]string{"MCP-Protocol-Version": "2025-03-26"})
	if w.Code != 400 || !strings.Contains(w.Body.String(), "batch_requests_disabled") {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestLANHostStillRequiresCredentials(t *testing.T) {
	cfg := testConfig()
	cfg.AllowedHosts = []string{"192.168.1.10:18300"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	headers := map[string]string{"Host": "192.168.1.10:18300", "RemoteAddr": "192.168.1.20:54321"}
	h := NewHandler(cfg, fakeAuth{}, Services{}, logger)
	if w := req(h, "POST", listBody, headers); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	h = NewHandler(cfg, fakeAuth{fail: ErrUnauthorized}, Services{}, logger)
	if w := req(h, "POST", listBody, headers); w.Code != 401 {
		t.Fatal("LAN auth bypass", w.Code)
	}
	headers["X-Forwarded-For"] = "127.0.0.1"
	if w := req(h, "POST", listBody, headers); w.Code != 401 {
		t.Fatal("forwarding header changed authentication result", w.Code)
	}
}

func TestDevelopmentWildcardHost(t *testing.T) {
	cfg := testConfig()
	cfg.AllowedHosts = []string{"*"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	headers := map[string]string{"Host": "192.168.1.50:8300", "RemoteAddr": "192.168.1.20:1234"}
	h := NewHandler(cfg, fakeAuth{}, Services{}, logger)
	if w := req(h, "POST", listBody, headers); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	h = NewHandler(cfg, fakeAuth{fail: ErrUnauthorized}, Services{}, logger)
	if w := req(h, "POST", listBody, headers); w.Code != 401 {
		t.Fatal(w.Code)
	}
}

func TestReverseProxyForwardingHeaders(t *testing.T) {
	cfg := testConfig()
	cfg.AllowedHosts = []string{"*"}
	headers := map[string]string{"Host": "mcp.example.test", "RemoteAddr": "127.0.0.1:34567", "Forwarded": "for=192.0.2.12;proto=https;host=mcp.example.test", "X-Forwarded-For": "192.0.2.12", "X-Forwarded-Host": "mcp.example.test", "X-Forwarded-Proto": "https", "X-Real-IP": "192.0.2.12"}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewHandler(cfg, fakeAuth{}, Services{}, logger)
	if w := req(h, "POST", listBody, headers); w.Code != 200 {
		t.Fatalf("proxy request rejected: HTTP %d %s", w.Code, w.Body.String())
	}
	h = NewHandler(cfg, fakeAuth{fail: ErrUnauthorized}, Services{}, logger)
	if w := req(h, "POST", listBody, headers); w.Code != 401 {
		t.Fatalf("proxy must still authenticate: HTTP %d", w.Code)
	}
	cfg.AllowedHosts = []string{"localhost:18300"}
	h = NewHandler(cfg, fakeAuth{}, Services{}, logger)
	if w := req(h, "POST", listBody, headers); w.Code != 403 {
		t.Fatal("forwarded authority bypassed actual Host")
	}
}

func TestTunnelHostThroughLoopbackOrigin(t *testing.T) {
	cfg := testConfig()
	cfg.AllowedHosts = []string{"*"}
	handler := NewHandler(cfg, fakeAuth{}, Services{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	server := httptest.NewServer(handler)
	defer server.Close()
	r, e := http.NewRequest("POST", server.URL+"/mcp", strings.NewReader(listBody))
	if e != nil {
		t.Fatal(e)
	}
	r.Host = "mcp.example.test"
	r.Header.Set("Authorization", "Bearer test")
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	r.Header.Set("MCP-Protocol-Version", "2025-06-18")
	r.Header.Set("X-Forwarded-For", "192.0.2.12")
	res, e := server.Client().Do(r)
	if e != nil {
		t.Fatal(e)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		t.Fatalf("loopback tunnel origin rejected external Host: HTTP %d %s", res.StatusCode, body)
	}
}
