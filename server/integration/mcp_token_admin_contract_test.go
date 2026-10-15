//go:build integration

package integration

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/EziosWJ/simple-inventory/server/internal/auth"
	"github.com/EziosWJ/simple-inventory/server/internal/mcpserver"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type tokenView struct {
	ExpiresAt      *time.Time `json:"expiresAt"`
	CreatedAt      time.Time  `json:"createdAt"`
	ID             int64      `json:"id"`
	UserID         int64      `json:"userId"`
	Name           string     `json:"name"`
	UserName       string     `json:"userName"`
	CreatedBy      string     `json:"createdBy"`
	Enabled        bool       `json:"enabled"`
	Permanent      bool       `json:"permanent"`
	Expired        bool       `json:"expired"`
	Revoked        bool       `json:"revoked"`
	Usable         bool       `json:"usable"`
	SecretViewable bool       `json:"secretViewable"`
	Legacy         bool       `json:"legacy"`
}

type tokenSecret struct {
	ExpiresAt *time.Time `json:"expiresAt"`
	Token     string     `json:"token"`
	Permanent bool       `json:"permanent"`
	Record    tokenView  `json:"record"`
}

type tokenPage struct {
	Records  []tokenView `json:"records"`
	Total    int64       `json:"total"`
	Page     int         `json:"page"`
	PageSize int         `json:"pageSize"`
}

var shanghaiZone = time.FixedZone("Asia/Shanghai", 8*60*60)

func TestSQLiteMCPTokenAdminContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp-token-admin.db")
	runSQLiteMigrations(t, path)
	db := openSQLiteDatabase(t, path)
	defer db.Close()
	assertMCPTokenAdminContract(t, db, true)
}

func TestPostgresMCPTokenAdminContract(t *testing.T) {
	pg := startPostgres(t)
	runMigrations(t, projectRoot(t), pg.dsn)
	db := openTemporaryDatabase(t, pg.dsn)
	defer db.Close()
	assertMCPTokenAdminContract(t, db, false)
}

func assertMCPTokenAdminContract(t *testing.T, db *platformdatabase.Database, sqlite bool) {
	t.Helper()
	f := newMCPFixture(t, db, sqlite)
	ctx := context.Background()

	// The administration menu is seeded and granted to the built-in
	// administrator, so the API is reachable without hiding a browser button.
	var menuCount, grantCount int64
	if err := db.GORM.Table("sys_menu").Where("path='/system/mcp-token' AND deleted=0").Count(&menuCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.GORM.Table("sys_role_menu rm").Joins("JOIN sys_menu m ON m.id=rm.menu_id").Joins("JOIN sys_role r ON r.id=rm.role_id").Where("m.path='/system/mcp-token' AND r.role_code='ADMIN'").Count(&grantCount).Error; err != nil {
		t.Fatal(err)
	}
	if menuCount != 1 || grantCount != 1 {
		t.Fatalf("menu=%d admin grants=%d", menuCount, grantCount)
	}

	admin := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		return serveJSON(f.handler, method, path, body, f.adminToken)
	}
	ok := func(method, path, body string) *httptest.ResponseRecorder {
		t.Helper()
		r := admin(method, path, body)
		if r.Code != http.StatusOK {
			t.Fatalf("%s %s -> %d %s", method, path, r.Code, r.Body.String())
		}
		return r
	}
	page := func(query string) tokenPage {
		t.Helper()
		path := "/api/system/mcp/tokens/page" + query
		return decodeTokenPage(t, ok(http.MethodGet, path, ""), http.MethodGet, path)
	}

	view := func(method, path, body string) tokenView {
		t.Helper()
		return decodeTokenView(t, ok(method, path, body), method, path)
	}
	secret := func(method, path, body string) tokenSecret {
		t.Helper()
		return decodeTokenSecret(t, ok(method, path, body), method, path)
	}

	empty := page("")
	if empty.Total != 0 || len(empty.Records) != 0 || empty.Page != 1 || empty.PageSize != 10 {
		t.Fatalf("initial page: %+v", empty)
	}

	// Permanent is the administration default and is expressed as no expiry.
	permanent := secret(http.MethodPost, "/api/system/mcp/tokens", fmt.Sprintf(`{"name":"永久手机","userId":%d,"permanent":true}`, f.subjectID))
	if !strings.HasPrefix(permanent.Token, "simcp_") || len(permanent.Token) != 70 {
		t.Fatalf("created value: %q", permanent.Token)
	}
	if !permanent.Record.Permanent || permanent.Record.ExpiresAt != nil || !permanent.Record.SecretViewable || permanent.Record.Legacy || !permanent.Record.Usable {
		t.Fatalf("permanent record: %+v", permanent.Record)
	}
	dated := secret(http.MethodPost, "/api/system/mcp/tokens", fmt.Sprintf(`{"name":"电脑","userId":%d,"expiresInDays":30}`, f.subjectID))
	if dated.Record.Permanent || dated.Record.ExpiresAt == nil {
		t.Fatalf("dated record: %+v", dated.Record)
	}
	if delta := dated.Record.ExpiresAt.Sub(time.Now().UTC()); delta < 29*24*time.Hour || delta > 31*24*time.Hour {
		t.Fatalf("30 day expiry: %v", delta)
	}
	// A custom date is read as a Shanghai calendar day valid through its end.
	tomorrow := time.Now().In(shanghaiZone).AddDate(0, 0, 1)
	custom := secret(http.MethodPost, "/api/system/mcp/tokens", fmt.Sprintf(`{"name":"备用","userId":%d,"expiresOn":%q}`, f.subjectID, tomorrow.Format("2006-01-02")))
	wantCustom := time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, shanghaiZone).AddDate(0, 0, 1).UTC()
	if custom.Record.ExpiresAt == nil || !custom.Record.ExpiresAt.Equal(wantCustom) {
		t.Fatalf("custom expiry %v want %v", custom.Record.ExpiresAt, wantCustom)
	}
	// Mutually exclusive validity choices and an unknown user are refused.
	for _, body := range []string{
		fmt.Sprintf(`{"name":"x","userId":%d,"permanent":true,"expiresInDays":7}`, f.subjectID),
		fmt.Sprintf(`{"name":"x","userId":%d,"expiresInDays":0}`, f.subjectID),
		fmt.Sprintf(`{"name":"x","userId":%d,"expiresOn":"2020-01-01"}`, f.subjectID),
		`{"name":"x","userId":999999,"permanent":true}`,
		`{"name":"","userId":1,"permanent":true}`,
	} {
		if r := admin(http.MethodPost, "/api/system/mcp/tokens", body); r.Code != http.StatusOK || envelopeCode(t, r) == 200 {
			t.Fatalf("accepted %s -> %d %s", body, r.Code, r.Body.String())
		}
	}

	listed := page("?pageSize=50")
	if listed.Total != 3 || len(listed.Records) != 3 {
		t.Fatalf("listed %d of %d", len(listed.Records), listed.Total)
	}
	if listed.Records[0].ID < listed.Records[2].ID {
		t.Fatal("administration list must show the newest credential first")
	}
	body := ok(http.MethodGet, "/api/system/mcp/tokens/page?pageSize=50", "").Body.String()
	for _, secret := range []string{custom.Token, dated.Token, permanent.Token, "digest", "secretCipher", "secretVersion"} {
		if strings.Contains(body, secret) {
			t.Fatalf("list disclosed %q", secret)
		}
	}
	filtered := page("?state=ENABLED")
	if filtered.Total != 3 {
		t.Fatalf("enabled filter: %+v", filtered)
	}
	if got := page(fmt.Sprintf("?userId=%d", f.subjectID)); got.Total != 3 {
		t.Fatalf("user filter: %+v", got)
	}
	if got := page("?state=DISABLED"); got.Total != 0 {
		t.Fatalf("disabled filter: %+v", got)
	}
	if r := admin(http.MethodGet, "/api/system/mcp/tokens/page?state=BOGUS", ""); envelopeCode(t, r) != 400 {
		t.Fatalf("invalid state accepted: %s", r.Body.String())
	}
	if r := admin(http.MethodGet, "/api/system/mcp/tokens/page?page=0", ""); envelopeCode(t, r) != 400 {
		t.Fatalf("invalid page accepted: %s", r.Body.String())
	}

	// The reveal endpoint is the only place the original value comes back, and
	// it must forbid caching.
	secretResponse := ok(http.MethodGet, fmt.Sprintf("/api/system/mcp/tokens/%d/secret", permanent.Record.ID), "")
	if secretResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("reveal cache header: %q", secretResponse.Header().Get("Cache-Control"))
	}
	revealed := inventoryData[tokenSecret](t, secretResponse, 200)
	if revealed.Token != permanent.Token {
		t.Fatal("revealed value differs from the issued one")
	}
	if r := admin(http.MethodGet, "/api/system/mcp/tokens/999999/secret", ""); envelopeCode(t, r) != 404 {
		t.Fatalf("missing credential: %s", r.Body.String())
	}

	// A changed display key keeps digest authentication working but must say
	// the value needs the original configuration instead of pretending the
	// credential is gone.
	otherBox, err := mcpserver.NewSecretBox("another-integration-secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mcpserver.NewRepository(db.GORM, otherBox).Reveal(ctx, permanent.Record.ID); err == nil {
		t.Fatal("a different key decrypted the stored value")
	} else if err.Error() != "token secret needs the original configuration or a reset" {
		t.Fatalf("unexpected reveal error: %v", err)
	}

	// Extension keeps the value and the disabled state; an unexpired
	// credential grows from its expiry and an expired one counts from now.
	before := *dated.Record.ExpiresAt
	extended := view(http.MethodPost, fmt.Sprintf("/api/system/mcp/tokens/%d/extend", dated.Record.ID), `{"days":30}`)
	if extended.ExpiresAt == nil || !extended.ExpiresAt.Equal(before.Add(30*24*time.Hour)) {
		t.Fatalf("extension from the original expiry: %v want %v", extended.ExpiresAt, before.Add(30*24*time.Hour))
	}
	mustExec(t, db, "UPDATE mcp_token SET expires_at=? WHERE id=?", time.Now().UTC().Add(-time.Hour), dated.Record.ID)
	revived := view(http.MethodPost, fmt.Sprintf("/api/system/mcp/tokens/%d/extend", dated.Record.ID), `{"days":7}`)
	if revived.ExpiresAt == nil {
		t.Fatal("expired credential did not regain an expiry")
	}
	if delta := revived.ExpiresAt.Sub(time.Now().UTC()); delta < 6*24*time.Hour || delta > 8*24*time.Hour {
		t.Fatalf("expired extension counts from now: %v", delta)
	}
	asPermanent := view(http.MethodPost, fmt.Sprintf("/api/system/mcp/tokens/%d/extend", dated.Record.ID), `{"permanent":true}`)
	if !asPermanent.Permanent || asPermanent.ExpiresAt != nil {
		t.Fatalf("permanent extension: %+v", asPermanent)
	}
	if r := admin(http.MethodPost, fmt.Sprintf("/api/system/mcp/tokens/%d/extend", dated.Record.ID), `{"days":0}`); envelopeCode(t, r) == 200 {
		t.Fatalf("zero day extension accepted: %s", r.Body.String())
	}
	if r := admin(http.MethodPost, fmt.Sprintf("/api/system/mcp/tokens/%d/extend", dated.Record.ID), `{"days":5,"permanent":true}`); envelopeCode(t, r) == 200 {
		t.Fatalf("contradictory extension accepted: %s", r.Body.String())
	}

	// Disabling takes effect on the next request, and extension never
	// re-enables a disabled credential.
	disabled := view(http.MethodPut, fmt.Sprintf("/api/system/mcp/tokens/%d/status", custom.Record.ID), `{"enabled":false}`)
	if disabled.Enabled || disabled.Usable || !disabled.Expired && disabled.Permanent {
		t.Fatalf("disabled record: %+v", disabled)
	}
	if got := f.request(t, http.MethodPost, mcpListBody, tokenHeader(custom.Token)); got.status != 401 {
		t.Fatalf("disabled credential status %d", got.status)
	}
	stillDisabled := view(http.MethodPost, fmt.Sprintf("/api/system/mcp/tokens/%d/extend", custom.Record.ID), `{"days":30}`)
	if stillDisabled.Enabled {
		t.Fatal("extension re-enabled a disabled credential")
	}
	if got := f.request(t, http.MethodPost, mcpListBody, tokenHeader(custom.Token)); got.status != 401 {
		t.Fatalf("extended disabled credential status %d", got.status)
	}
	reEnabled := view(http.MethodPut, fmt.Sprintf("/api/system/mcp/tokens/%d/status", custom.Record.ID), `{"enabled":true}`)
	if !reEnabled.Enabled || !reEnabled.Usable {
		t.Fatalf("re-enabled record: %+v", reEnabled)
	}
	if got := f.request(t, http.MethodPost, mcpListBody, tokenHeader(custom.Token)); got.status != 200 {
		t.Fatalf("re-enabled credential status %d", got.status)
	}
	if r := admin(http.MethodPut, fmt.Sprintf("/api/system/mcp/tokens/%d/status", custom.Record.ID), `{}`); envelopeCode(t, r) == 200 {
		t.Fatalf("status without a value accepted: %s", r.Body.String())
	}

	// Reset replaces the value immediately and keeps name, owner, expiry and
	// enabled state.
	oldValue := custom.Token
	reset := secret(http.MethodPost, fmt.Sprintf("/api/system/mcp/tokens/%d/reset", custom.Record.ID), "")
	if reset.Token == oldValue || len(reset.Token) != 70 {
		t.Fatalf("reset value: %q", reset.Token)
	}
	if reset.Record.Name != "备用" || reset.Record.UserID != f.subjectID || !reset.Record.Enabled {
		t.Fatalf("reset record: %+v", reset.Record)
	}
	if got := f.request(t, http.MethodPost, mcpListBody, tokenHeader(oldValue)); got.status != 401 {
		t.Fatalf("reset left the previous value usable: %d", got.status)
	}
	if got := f.request(t, http.MethodPost, mcpListBody, tokenHeader(reset.Token)); got.status != 200 {
		t.Fatalf("reset value rejected: %d", got.status)
	}
	// Resetting an administratively disabled credential must not quietly
	// restore it; the checker restores it afterwards because the permission
	// inheritance checks below connect with it.
	mustExec(t, db, "UPDATE mcp_token SET disabled_at=? WHERE id=?", time.Now().UTC(), permanent.Record.ID)
	resetDisabled := secret(http.MethodPost, fmt.Sprintf("/api/system/mcp/tokens/%d/reset", permanent.Record.ID), "")
	if resetDisabled.Record.Enabled || resetDisabled.Record.Usable {
		t.Fatalf("reset re-enabled a disabled credential: %+v", resetDisabled.Record)
	}
	mustExec(t, db, "UPDATE mcp_token SET disabled_at=NULL WHERE id=?", permanent.Record.ID)

	// A credential stored by the previous release keeps authenticating, is
	// marked as not viewable, and becomes viewable after a reset.
	legacyRaw := "simcp_" + strings.Repeat("b", 64)
	sum := sha256.Sum256([]byte(legacyRaw))
	mustExec(t, db, "INSERT INTO mcp_token(user_id,client_id,digest,scopes,expires_at,created_at,created_by) VALUES (?,?,?,'[\"search_products\"]',?,?,'legacy-operator')",
		f.subjectID, "旧版电脑", hex.EncodeToString(sum[:]), time.Now().UTC().Add(time.Hour), time.Now().UTC())
	if got := f.request(t, http.MethodPost, mcpListBody, tokenHeader(legacyRaw)); got.status != 200 {
		t.Fatalf("legacy credential rejected: %d %s", got.status, got.body)
	}
	var legacyID int64
	if err := db.GORM.Table("mcp_token").Where("digest=?", hex.EncodeToString(sum[:])).Pluck("id", &legacyID).Error; err != nil {
		t.Fatal(err)
	}
	if r := admin(http.MethodGet, fmt.Sprintf("/api/system/mcp/tokens/%d/secret", legacyID), ""); !strings.Contains(r.Body.String(), "恢复原配置或重置") {
		t.Fatalf("legacy guidance: %s", r.Body.String())
	}
	legacyInList := page("?pageSize=50")
	found := false
	for _, record := range legacyInList.Records {
		if record.ID == legacyID {
			found = true
			if !record.Legacy || record.SecretViewable || !record.Usable {
				t.Fatalf("legacy list record: %+v", record)
			}
		}
	}
	if !found {
		t.Fatal("legacy credential missing from the list")
	}
	// The credential is not force-invalidated, but the legacy scopes must no
	// longer restrict the tools its owner is entitled to.
	session := f.session(t, ctx, tokenHeader(legacyRaw))
	if names := mcpToolNames(t, ctx, session); len(names) != 10 {
		t.Fatalf("legacy scopes still restricted tools: %v", names)
	}
	legacyReset := secret(http.MethodPost, fmt.Sprintf("/api/system/mcp/tokens/%d/reset", legacyID), "")
	if !legacyReset.Record.SecretViewable || legacyReset.Record.Legacy {
		t.Fatalf("reset did not make the legacy credential viewable: %+v", legacyReset.Record)
	}
	if got := f.request(t, http.MethodPost, mcpListBody, tokenHeader(legacyRaw)); got.status != 401 {
		t.Fatalf("legacy value survived a reset: %d", got.status)
	}
	if got := f.request(t, http.MethodPost, mcpListBody, tokenHeader(legacyReset.Token)); got.status != 200 {
		t.Fatalf("reset legacy value rejected: %d", got.status)
	}

	// Tool access follows the bound user's current business permissions.
	subjectToken := resetDisabled.Token
	subjectSession := f.session(t, ctx, tokenHeader(subjectToken))
	if names := mcpToolNames(t, ctx, subjectSession); len(names) != 10 {
		t.Fatalf("subject tools: %v", names)
	}
	f.revokeTool(t, "business:product")
	if names := mcpToolNames(t, ctx, subjectSession); len(names) != 9 {
		t.Fatalf("revocation did not take effect: %v", names)
	}
	// A revoked tool is no longer disclosed, so the call fails before the
	// handler can answer with a business error.
	if result, err := subjectSession.CallTool(ctx, &mcp.CallToolParams{Name: "search_products", Arguments: map[string]any{"keyword": "x"}}); err == nil && !result.IsError {
		t.Fatalf("revoked tool callable: %v", result)
	}
	mustExec(t, db, "UPDATE sys_user SET status=0 WHERE id=?", f.subjectID)
	if got := f.request(t, http.MethodPost, mcpListBody, tokenHeader(subjectToken)); got.status != 401 {
		t.Fatalf("disabled owner status %d", got.status)
	}
	mustExec(t, db, "UPDATE sys_user SET status=1 WHERE id=?", f.subjectID)
	if got := f.request(t, http.MethodPost, mcpListBody, tokenHeader(subjectToken)); got.status != 200 {
		t.Fatalf("re-enabled owner status %d", got.status)
	}

	// Administration is refused without the menu permission, whatever the
	// browser shows.
	limitedID := createLimitedUser(t, db)
	limitedToken := loginUser(t, f.handler, "mcp-limited-user", "admin123")
	for _, call := range []struct {
		method, path, body string
	}{
		{http.MethodGet, "/api/system/mcp/tokens/page", ""},
		{http.MethodPost, "/api/system/mcp/tokens", fmt.Sprintf(`{"name":"x","userId":%d,"permanent":true}`, f.subjectID)},
		{http.MethodGet, fmt.Sprintf("/api/system/mcp/tokens/%d/secret", permanent.Record.ID), ""},
		{http.MethodPost, fmt.Sprintf("/api/system/mcp/tokens/%d/reset", permanent.Record.ID), ""},
		{http.MethodPut, fmt.Sprintf("/api/system/mcp/tokens/%d/status", permanent.Record.ID), `{"enabled":false}`},
		{http.MethodPost, fmt.Sprintf("/api/system/mcp/tokens/%d/extend", permanent.Record.ID), `{"days":1}`},
	} {
		response := serveJSON(f.handler, call.method, call.path, call.body, limitedToken)
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s %s without the administration grant -> %d %s", call.method, call.path, response.Code, response.Body.String())
		}
	}
	// The grant is exactly what a second administrator role would add.
	grantAdministration(t, db, limitedID)
	limitedPage := inventoryData[tokenPage](t, serveJSON(f.handler, http.MethodGet, "/api/system/mcp/tokens/page", "", limitedToken), 200)
	if limitedPage.Total == 0 {
		t.Fatal("the administration grant did not enable the API")
	}

	// Every management action is recorded, and never with the value or the
	// ciphertext.
	var audited int64
	if err := db.GORM.Table("sys_oper_log").Where("module_name='mcp-token'").Count(&audited).Error; err != nil {
		t.Fatal(err)
	}
	if audited < 4 {
		t.Fatalf("audit rows for the token lifecycle: %d", audited)
	}
	for _, action := range []string{"mcp-token.create", "mcp-token.extend", "mcp-token.status", "mcp-token.reset", "mcp-token.reveal"} {
		var count int64
		if err := db.GORM.Table("sys_oper_log").Where("module_name='mcp-token' AND operation_type=?", action).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count == 0 {
			t.Fatalf("missing audit action %s", action)
		}
	}
	logs := []struct {
		Summary string `gorm:"column:summary"`
		URL     string `gorm:"column:request_url"`
	}{}
	if err := db.GORM.Table("sys_oper_log").Select("'' AS summary,request_url AS url").Where("module_name='mcp-token'").Scan(&logs).Error; err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(logs)
	for _, secret := range []string{permanent.Token, reset.Token, legacyReset.Token, "simcp_"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("audit recorded a credential value: %s", encoded)
		}
	}
}

// decodeTokenView and decodeTokenSecret report the request path because a
// business failure answers with HTTP 200 and a non-200 envelope code.
func decodeTokenView(t *testing.T, response *httptest.ResponseRecorder, method, path string) tokenView {
	t.Helper()
	if envelopeCode(t, response) != http.StatusOK {
		t.Fatalf("%s %s -> %s", method, path, response.Body.String())
	}
	return inventoryData[tokenView](t, response, 200)
}

func decodeTokenSecret(t *testing.T, response *httptest.ResponseRecorder, method, path string) tokenSecret {
	t.Helper()
	if envelopeCode(t, response) != http.StatusOK {
		t.Fatalf("%s %s -> %s", method, path, response.Body.String())
	}
	return inventoryData[tokenSecret](t, response, 200)
}

func decodeTokenPage(t *testing.T, response *httptest.ResponseRecorder, method, path string) tokenPage {
	t.Helper()
	if envelopeCode(t, response) != http.StatusOK {
		t.Fatalf("%s %s -> %s", method, path, response.Body.String())
	}
	return inventoryData[tokenPage](t, response, 200)
}

func envelopeCode(t *testing.T, response *httptest.ResponseRecorder) int {
	t.Helper()
	var payload struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode envelope: %v (%s)", err, response.Body.String())
	}
	return payload.Code
}

// createLimitedUser makes a loginable account that carries no administration
// grant, so the REST layer, not the browser, has to refuse it.
func createLimitedUser(t *testing.T, db *platformdatabase.Database) int64 {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	user := auth.User{Username: "mcp-limited-user", Nickname: "受限管理测试", PasswordHash: string(hash), Status: 1}
	if err := db.GORM.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	mustExec(t, db, "INSERT INTO sys_role(role_name,role_code,status) VALUES ('MCP limited','mcp-limited',1)")
	mustExec(t, db, "INSERT INTO sys_user_role(user_id,role_id) SELECT ?,id FROM sys_role WHERE role_code='mcp-limited'", user.ID)
	return user.ID
}

func grantAdministration(t *testing.T, db *platformdatabase.Database, userID int64) {
	t.Helper()
	mustExec(t, db, "INSERT INTO sys_role_menu(role_id,menu_id) SELECT r.id,m.id FROM sys_role r CROSS JOIN sys_menu m WHERE r.role_code='mcp-limited' AND m.path='/system/mcp-token'")
	_ = userID
}
