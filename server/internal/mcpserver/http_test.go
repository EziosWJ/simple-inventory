package mcpserver

import (
	"bytes"
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
	return Token{ID: 1, UserID: 2, Name: "test"}, f.allowed, f.fail
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

// request sends one MCP request with exactly the given headers, so a test can
// control which credential headers are present.
func request(t *testing.T, h http.Handler, method string, headers map[string][]string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, "http://localhost:18300/mcp", strings.NewReader(listBody))
	r.RemoteAddr = "127.0.0.1:54321"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Accept", "application/json, text/event-stream")
	for key, values := range headers {
		if key == "Host" {
			r.Host = values[0]
			continue
		}
		// The caller's headers replace the defaults instead of adding to them,
		// so a test can send exactly one Accept or credential header.
		r.Header.Del(key)
		for _, value := range values {
			r.Header.Add(key, value)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

// A rejected request must be diagnosable from the server log alone, because a
// mobile client only reports "it failed". The log names the failing rule and
// the offending origin or host, and never the credential itself.
func TestRejectionsAreLoggedWithoutTheCredential(t *testing.T) {
	cfg := testConfig()
	cfg.AllowedOrigins = []string{"http://allowed.test"}
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	h := NewHandler(cfg, fakeAuth{allowed: map[string]bool{}}, Services{}, logger)
	const marker = "simcp_secret_marker_value"
	for _, tc := range []struct {
		name    string
		headers map[string][]string
		reason  string
		status  int
	}{
		{"missing credential", nil, "credential_missing", 401},
		{"malformed credential", map[string][]string{"token": {"Bearer " + marker}}, "credential_malformed", 400},
		{"conflicting credentials", map[string][]string{"token": {marker}, "Authorization": {"Bearer other"}}, "credential_conflict", 400},
		{"duplicated credential", map[string][]string{"token": {marker, marker}}, "credential_duplicated", 400},
		{"rejected origin", map[string][]string{"Origin": {"http://blocked.test"}}, "origin_not_allowed", 403},
		{"rejected host", map[string][]string{"Host": {"evil.test"}}, "host_not_allowed", 403},
	} {
		logs.Reset()
		w := request(t, h, "POST", tc.headers)
		if w.Code != tc.status {
			t.Fatalf("%s: status=%d want=%d", tc.name, w.Code, tc.status)
		}
		if !strings.Contains(logs.String(), "reason="+tc.reason) {
			t.Fatalf("%s: log does not name the rule: %s", tc.name, logs.String())
		}
		if strings.Contains(logs.String(), marker) || strings.Contains(logs.String(), "simcp_secret") {
			t.Fatalf("%s: log leaked the credential: %s", tc.name, logs.String())
		}
		if w.Header().Get("X-Request-ID") == "" {
			t.Fatalf("%s: rejected request must still carry a request id", tc.name)
		}
	}
	// A protocol-level rejection (here an Accept header the endpoint cannot
	// answer) is logged as well, because the client only sees status text.
	logs.Reset()
	credential := "Bearer " + strings.Repeat("a", 70)
	if w := request(t, h, "POST", map[string][]string{"Authorization": {credential}, "Accept": {"application/json"}}); w.Code != 400 {
		t.Fatalf("unacceptable Accept returned %d", w.Code)
	}
	if !strings.Contains(logs.String(), "reason=protocol_rejected") || !strings.Contains(logs.String(), "text/event-stream") {
		t.Fatalf("protocol rejection must be diagnosable: %s", logs.String())
	}
	if strings.Contains(logs.String(), strings.Repeat("a", 70)) {
		t.Fatalf("protocol rejection leaked the credential: %s", logs.String())
	}

	// The value the operator has to fix is logged; that is the whole point.
	logs.Reset()
	request(t, h, "POST", map[string][]string{"Origin": {"http://blocked.test"}})
	if !strings.Contains(logs.String(), "origin=http://blocked.test") || !strings.Contains(logs.String(), "allowed_origins=http://allowed.test") {
		t.Fatalf("origin rejection must be actionable: %s", logs.String())
	}
}

// The development entry point opts into any Origin explicitly; the response
// still echoes the caller's own origin rather than a blanket wildcard.
func TestDevelopmentOriginWildcardAcceptsAnyOrigin(t *testing.T) {
	cfg := testConfig()
	cfg.AllowedOrigins = []string{"*"}
	h := NewHandler(cfg, fakeAuth{allowed: map[string]bool{}}, Services{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	credential := "Bearer " + strings.Repeat("a", 70)
	for _, origin := range []string{"http://192.168.1.32:8300", "null", "capacitor://localhost"} {
		w := request(t, h, "POST", map[string][]string{"Origin": {origin}, "Authorization": {credential}})
		if w.Code != 200 {
			t.Fatalf("origin %s rejected with %d", origin, w.Code)
		}
		if got := w.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Fatalf("echoed origin %q want %q", got, origin)
		}
		if w.Header().Get("Vary") != "Origin" {
			t.Fatal("wildcard origin must still vary on Origin")
		}
	}
	w := request(t, h, "OPTIONS", map[string][]string{"Origin": {"capacitor://localhost"}, "Authorization": {credential}})
	if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != "capacitor://localhost" {
		t.Fatalf("preflight: %d %v", w.Code, w.Header())
	}
}

// flushRecorder signals the first flush so a streaming response can be
// inspected without waiting for the handler to return.
type flushRecorder struct {
	*httptest.ResponseRecorder
	flushed chan struct{}
	once    sync.Once
}

func (f *flushRecorder) Flush() {
	f.ResponseRecorder.Flush()
	f.once.Do(func() { close(f.flushed) })
}

func TestGetStreamDoesNotBlockTools(t *testing.T) {
	cfg := testConfig()
	cfg.MaxConcurrent = 1
	h := NewHandler(cfg, fakeAuth{allowed: map[string]bool{}}, Services{}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	ctx, cancel := context.WithCancel(context.Background())
	stream := httptest.NewRequest(http.MethodGet, "http://localhost:18300/mcp", nil).WithContext(ctx)
	stream.RemoteAddr = "127.0.0.1:54321"
	stream.Header.Set("token", strings.Repeat("a", 70))
	recorder := &flushRecorder{ResponseRecorder: httptest.NewRecorder(), flushed: make(chan struct{})}
	done := make(chan struct{})
	go func() {
		h.ServeHTTP(recorder, stream)
		close(done)
	}()

	select {
	case <-recorder.flushed:
	case <-time.After(time.Second):
		t.Fatal("GET stream did not start")
	}
	if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("stream response: %d %q", recorder.Code, recorder.Header().Get("Content-Type"))
	}
	// The idle stream must not consume the only tool slot.
	if w := req(h, "POST", listBody, nil); w.Code != http.StatusOK {
		t.Fatalf("tool call blocked by stream: %d %s", w.Code, w.Body.String())
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("GET stream did not end with the client")
	}
	if !strings.Contains(recorder.Body.String(), ": connected") {
		t.Fatalf("stream did not prime the connection: %q", recorder.Body.String())
	}
}

func TestGetStreamRequiresCredential(t *testing.T) {
	h := NewHandler(testConfig(), fakeAuth{}, Services{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r := httptest.NewRequest(http.MethodGet, "http://localhost:18300/mcp", nil)
	r.RemoteAddr = "127.0.0.1:54321"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated stream: %d %s", w.Code, w.Body.String())
	}
}
