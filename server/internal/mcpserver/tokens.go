// Package mcpserver adapts read-only business services to MCP and manages the
// independent credentials that grant access to them.
package mcpserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/EziosWJ/simple-inventory/server/internal/audit"
	"github.com/EziosWJ/simple-inventory/server/internal/auth"
	"gorm.io/gorm"
)

// Permissions maps every read-only tool to the business permission code that
// currently grants it. A credential no longer selects its own tools, so a tool
// is available exactly while the bound user still holds the code; the legacy
// scopes column is retained for migration compatibility but never filters.
var Permissions = map[string]string{
	"search_products":        "business:product",
	"search_partners":        "business:partner",
	"get_inventory_balances": "business:inventory-balance",
	"get_partner_balances":   "business:partner-balance",
	"query_purchase_items":   "business:purchase",
	"get_purchase_document":  "business:purchase",
	"query_sale_items":       "business:sale",
	"get_sale_document":      "business:sale",
	"query_partner_entries":  "business:partner-ledger",
	"get_partner_statement":  "business:partner-ledger",
}

var (
	ErrUnauthorized = errors.New("unauthenticated")
	ErrUnavailable  = errors.New("temporarily_unavailable")
	ErrInvalid      = errors.New("invalid token options")
	ErrNotFound     = errors.New("token does not exist")
	ErrConflict     = errors.New("token state conflict")
)

// shanghai is a fixed offset zone: mainland China has no daylight saving, so
// this needs no host tzdata and matches exportfile.BusinessTime.
var shanghai = time.FixedZone("Asia/Shanghai", 8*60*60)

// Token is one stored credential. The display name reuses the original
// client_id column; the recoverable secret lives beside the authentication
// digest and is never used to authenticate.
type Token struct {
	CreatedAt     time.Time  `json:"createdAt"`
	ExpiresAt     *time.Time `json:"expiresAt"`
	DisabledAt    *time.Time `json:"disabledAt"`
	RevokedAt     *time.Time `json:"revokedAt"`
	SecretVersion *int       `json:"-"`
	RevokedBy     *string    `json:"-"`
	ID            int64      `json:"id"`
	UserID        int64      `json:"userId"`
	Name          string     `json:"name" gorm:"column:client_id"`
	Digest        string     `json:"-"`
	Scopes        string     `json:"-"`
	SecretCipher  string     `json:"-" gorm:"column:secret_cipher"`
	CreatedBy     string     `json:"createdBy"`
}

func (Token) TableName() string { return "mcp_token" }

// TokenView is the administration projection: it never carries the digest,
// the ciphertext or the original value.
type TokenView struct {
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

// Expiry and disabled state deliberately stay independent: expiry follows the
// clock, while enable/disable is an explicit administrator decision.
func (t Token) view(now time.Time, user string) TokenView {
	permanent := t.ExpiresAt == nil
	expired := !permanent && !t.ExpiresAt.After(now)
	enabled := t.DisabledAt == nil && t.RevokedAt == nil
	viewable := t.SecretVersion != nil && t.SecretCipher != ""
	return TokenView{
		ID: t.ID, Name: t.Name, UserID: t.UserID, UserName: user,
		Enabled: enabled, Permanent: permanent, ExpiresAt: t.ExpiresAt, Expired: expired,
		Revoked: t.RevokedAt != nil, Usable: enabled && !expired,
		SecretViewable: viewable, Legacy: t.SecretVersion == nil,
		CreatedAt: t.CreatedAt, CreatedBy: t.CreatedBy,
	}
}

// Page is the module-local pagination envelope used by the system REST API.
type Page[T any] struct {
	Records  []T   `json:"records"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"pageSize"`
}

// CreateOptions describes one credential. A nil ExpiresAt means permanent.
type CreateOptions struct {
	ExpiresAt *time.Time
	UserID    int64
	ActorID   int64
	Label     string
	Name      string
}

func digest(raw string) string { sum := sha256.Sum256([]byte(raw)); return hex.EncodeToString(sum[:]) }

func validLabel(s string) bool {
	return strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= 100 && !strings.ContainsAny(s, "\r\n\x00")
}

func validCredential(raw string) bool {
	return len(raw) == 70 && strings.HasPrefix(raw, "simcp_")
}

type Repository struct {
	db  *gorm.DB
	box *SecretBox
}

// NewRepository needs the display key box only to store and reveal
// recoverable secrets; authentication works through the digest alone.
func NewRepository(db *gorm.DB, box *SecretBox) *Repository { return &Repository{db: db, box: box} }

func (r *Repository) actorName(ctx context.Context, e audit.Event, label string) string {
	if validLabel(label) {
		return strings.TrimSpace(label)
	}
	if e.Metadata.ActorID > 0 {
		var name string
		if r.db.WithContext(ctx).Table("sys_user").Where("id=?", e.Metadata.ActorID).Pluck("username", &name).Error == nil && name != "" {
			return name
		}
		return fmt.Sprintf("user:%d", e.Metadata.ActorID)
	}
	return "unknown"
}

func (r *Repository) activeUser(ctx context.Context, userID int64) (bool, error) {
	var count int64
	if err := r.db.WithContext(ctx).Table("sys_user").Where("id=? AND status=1 AND deleted=0", userID).Count(&count).Error; err != nil {
		return false, err
	}
	return count == 1, nil
}

// Create issues a credential, storing both the authentication digest and a
// recoverable copy for later viewing.
func (r *Repository) Create(ctx context.Context, opts CreateOptions, e audit.Event) (Token, string, error) {
	if opts.UserID <= 0 || !validLabel(opts.Name) {
		return Token{}, "", ErrInvalid
	}
	active, err := r.activeUser(ctx, opts.UserID)
	if err != nil {
		return Token{}, "", err
	}
	if !active {
		return Token{}, "", ErrUnauthorized
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return Token{}, "", err
	}
	plaintext := "simcp_" + hex.EncodeToString(raw)
	sum := digest(plaintext)
	version, cipher, err := r.box.Seal(sum, plaintext)
	if err != nil {
		return Token{}, "", err
	}
	now := time.Now().UTC()
	t := Token{
		UserID: opts.UserID, Name: strings.TrimSpace(opts.Name), Digest: sum, Scopes: "[]",
		ExpiresAt: opts.ExpiresAt, SecretVersion: &version, SecretCipher: cipher,
		CreatedAt: now, CreatedBy: r.actorName(ctx, e, opts.Label),
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&t).Error; err != nil {
			return err
		}
		e.ResourceID = t.ID
		return audit.RecordOn(ctx, tx, e)
	})
	if err != nil {
		return Token{}, "", err
	}
	return t, plaintext, nil
}

// Issue keeps the original command-line behaviour with the administration
// default: a non-positive lifetime means a permanent credential.
func (r *Repository) Issue(ctx context.Context, userID int64, name, actor string, ttl time.Duration) (Token, string, error) {
	opts := CreateOptions{UserID: userID, Name: name, Label: actor}
	if ttl > 0 {
		expires := time.Now().UTC().Add(ttl)
		opts.ExpiresAt = &expires
	}
	return r.Create(ctx, opts, audit.Event{Action: "mcp-token.issue", Resource: "mcp-token", Summary: "签发 MCP 令牌"})
}

type TokenQuery struct {
	UserID   int64
	Keyword  string
	State    string
	Page     int
	PageSize int
}

func (r *Repository) tokenScope(ctx context.Context, q TokenQuery) *gorm.DB {
	query := r.db.WithContext(ctx).Table("mcp_token t")
	if q.UserID > 0 {
		query = query.Where("t.user_id=?", q.UserID)
	}
	if keyword := strings.TrimSpace(q.Keyword); keyword != "" {
		query = query.Where("t.client_id LIKE ?", "%"+strings.NewReplacer("%", "!%", "_", "!_").Replace(keyword)+"%")
	}
	now := time.Now().UTC()
	switch q.State {
	case "ENABLED":
		query = query.Where("t.disabled_at IS NULL AND t.revoked_at IS NULL AND (t.expires_at IS NULL OR t.expires_at>?)", now)
	case "DISABLED":
		query = query.Where("t.disabled_at IS NOT NULL OR t.revoked_at IS NOT NULL")
	case "EXPIRED":
		query = query.Where("t.disabled_at IS NULL AND t.revoked_at IS NULL AND t.expires_at IS NOT NULL AND t.expires_at<=?", now)
	}
	return query
}

type tokenRow struct {
	Token
	UserName string `gorm:"column:user_name"`
}

// List returns the administration page. It selects explicit columns so the
// digest and ciphertext can never reach an API response.
func (r *Repository) List(ctx context.Context, q TokenQuery) (Page[TokenView], error) {
	out := Page[TokenView]{Records: []TokenView{}, Page: q.Page, PageSize: q.PageSize}
	if err := r.tokenScope(ctx, q).Count(&out.Total).Error; err != nil {
		return out, err
	}
	rows := []tokenRow{}
	err := r.tokenScope(ctx, q).
		Select("t.id,t.user_id,t.client_id,t.expires_at,t.disabled_at,t.secret_version,t.secret_cipher,t.revoked_at,t.created_at,t.created_by,COALESCE(u.nickname,u.username,'') AS user_name").
		Joins("LEFT JOIN sys_user u ON u.id=t.user_id").
		Order("t.id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Scan(&rows).Error
	if err != nil {
		return out, err
	}
	now := time.Now().UTC()
	for _, row := range rows {
		out.Records = append(out.Records, row.Token.view(now, row.UserName))
	}
	return out, nil
}

func (r *Repository) find(ctx context.Context, tx *gorm.DB, id int64) (Token, error) {
	var t Token
	if id <= 0 {
		return t, ErrNotFound
	}
	err := tx.WithContext(ctx).Where("id=?", id).Take(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return t, ErrNotFound
	}
	return t, err
}

// View reads one administration row including its bound user's display name.
func (r *Repository) View(ctx context.Context, id int64) (TokenView, error) {
	rows := []tokenRow{}
	err := r.db.WithContext(ctx).Table("mcp_token t").
		Select("t.id,t.user_id,t.client_id,t.expires_at,t.disabled_at,t.secret_version,t.secret_cipher,t.revoked_at,t.created_at,t.created_by,COALESCE(u.nickname,u.username,'') AS user_name").
		Joins("LEFT JOIN sys_user u ON u.id=t.user_id").Where("t.id=?", id).Scan(&rows).Error
	if err != nil {
		return TokenView{}, err
	}
	if len(rows) == 0 {
		return TokenView{}, ErrNotFound
	}
	return rows[0].Token.view(time.Now().UTC(), rows[0].UserName), nil
}

// Reveal decrypts the stored original value. A digest-only legacy credential
// and a credential whose display key changed both report the same explicit
// recovery guidance instead of pretending the token is missing.
func (r *Repository) Reveal(ctx context.Context, id int64) (string, error) {
	t, err := r.find(ctx, r.db, id)
	if err != nil {
		return "", err
	}
	return r.box.Open(t.SecretVersion, t.Digest, t.SecretCipher)
}

// Reset replaces the value, the digest and the ciphertext while keeping the
// name, bound user, expiry and enabled state; the previous value stops working
// immediately and an expired or disabled credential stays that way.
func (r *Repository) Reset(ctx context.Context, id int64, e audit.Event) (Token, string, error) {
	var t Token
	var plaintext string
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existing, err := r.find(ctx, tx, id)
		if err != nil {
			return err
		}
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return err
		}
		plaintext = "simcp_" + hex.EncodeToString(raw)
		sum := digest(plaintext)
		version, cipher, err := r.box.Seal(sum, plaintext)
		if err != nil {
			return err
		}
		if err := tx.Model(&Token{}).Where("id=?", id).Updates(map[string]any{
			"digest": sum, "secret_version": version, "secret_cipher": cipher,
		}).Error; err != nil {
			return err
		}
		existing.Digest, existing.SecretVersion, existing.SecretCipher = sum, &version, cipher
		t = existing
		e.ResourceID = id
		return audit.RecordOn(ctx, tx, e)
	})
	if err != nil {
		return Token{}, "", err
	}
	return t, plaintext, nil
}

// Extend moves the expiry and never re-enables a disabled credential. An
// unexpired credential grows from its existing expiry; an expired one counts
// from now. Permanent is expressed as no expiry at all.
func (r *Repository) Extend(ctx context.Context, id int64, days int, permanent bool, e audit.Event) (Token, error) {
	var t Token
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existing, err := r.find(ctx, tx, id)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		var expires *time.Time
		if !permanent {
			base := now
			if existing.ExpiresAt != nil && existing.ExpiresAt.After(now) {
				base = existing.ExpiresAt.UTC()
			}
			next := base.Add(time.Duration(days) * 24 * time.Hour)
			expires = &next
		}
		if err := tx.Model(&Token{}).Where("id=?", id).Update("expires_at", expires).Error; err != nil {
			return err
		}
		existing.ExpiresAt = expires
		t = existing
		e.ResourceID = id
		return audit.RecordOn(ctx, tx, e)
	})
	if err != nil {
		return Token{}, err
	}
	return t, nil
}

// SetEnabled toggles the independent disabled state. Enabling does not extend
// an expired credential or restore a revoked one.
func (r *Repository) SetEnabled(ctx context.Context, id int64, enabled bool, e audit.Event) (Token, error) {
	var t Token
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existing, err := r.find(ctx, tx, id)
		if err != nil {
			return err
		}
		if existing.RevokedAt != nil {
			return ErrConflict
		}
		var disabled *time.Time
		if !enabled {
			now := time.Now().UTC()
			disabled = &now
		}
		if err := tx.Model(&Token{}).Where("id=?", id).Update("disabled_at", disabled).Error; err != nil {
			return err
		}
		existing.DisabledAt = disabled
		t = existing
		e.ResourceID = id
		return audit.RecordOn(ctx, tx, e)
	})
	if err != nil {
		return Token{}, err
	}
	return t, nil
}

// Revoke keeps the original revocation path for credentials issued before
// enable/disable existed, and remains available through the command line.
func (r *Repository) Revoke(ctx context.Context, id int64, actor string) error {
	if id <= 0 || !validLabel(actor) {
		return ErrInvalid
	}
	q := r.db.WithContext(ctx).Model(&Token{}).Where("id=? AND revoked_at IS NULL", id).Updates(map[string]any{"revoked_at": time.Now().UTC(), "revoked_by": actor})
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected != 1 {
		return ErrNotFound
	}
	return nil
}

// Authenticate keeps using the digest only, so a changed display key cannot
// affect access. The returned set is the tools the bound user currently holds.
func (r *Repository) Authenticate(ctx context.Context, raw string) (Token, map[string]bool, error) {
	var t Token
	if !validCredential(raw) {
		return t, nil, ErrUnauthorized
	}
	err := r.db.WithContext(ctx).Where("digest=? AND revoked_at IS NULL AND disabled_at IS NULL AND (expires_at IS NULL OR expires_at>?)", digest(raw), time.Now().UTC()).Take(&t).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return t, nil, ErrUnauthorized
	}
	if err != nil {
		return t, nil, ErrUnavailable
	}
	active, err := r.activeUser(ctx, t.UserID)
	if err != nil {
		return t, nil, ErrUnavailable
	}
	if !active {
		return t, nil, ErrUnauthorized
	}
	have, err := r.permissionCodes(ctx, t.UserID)
	if err != nil {
		return t, nil, ErrUnavailable
	}
	allowed := map[string]bool{}
	for tool, code := range Permissions {
		if have[code] {
			allowed[tool] = true
		}
	}
	return t, allowed, nil
}

// permissionCodes resolves the user's current menu grants on every request so
// revocation, account disablement and role changes take effect immediately.
func (r *Repository) permissionCodes(ctx context.Context, userID int64) (map[string]bool, error) {
	var codes []string
	err := r.db.WithContext(ctx).Table("sys_menu m").Distinct("m.permission_code").Joins("JOIN sys_role_menu rm ON rm.menu_id=m.id").Joins("JOIN sys_role r ON r.id=rm.role_id").Joins("JOIN sys_user_role ur ON ur.role_id=r.id").Where("ur.user_id=? AND r.status=1 AND r.deleted=0 AND m.status=1 AND m.deleted=0 AND m.permission_code IS NOT NULL", userID).Pluck("m.permission_code", &codes).Error
	if err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, code := range codes {
		have[code] = true
	}
	return have, nil
}

// Record writes one administration audit event for actions that do not change
// stored state, such as revealing an original value.
func (r *Repository) Record(ctx context.Context, e audit.Event) error {
	return audit.RecordOn(ctx, r.db.WithContext(ctx), e)
}

// CanManage proves the caller holds the administration menu permission; the
// REST layer never relies on the menu being hidden in the browser.
func (r *Repository) CanManage(ctx context.Context) (bool, error) {
	return auth.HasPermission(ctx, r.db, managePermission)
}

const managePermission = "system:mcp-token"
