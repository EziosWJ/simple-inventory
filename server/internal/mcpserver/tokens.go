// Package mcpserver adapts the four read-only business services to MCP.
package mcpserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
)

var Permissions = map[string]string{
	"search_products": "business:product", "search_partners": "business:partner",
	"get_inventory_balances": "business:inventory-balance", "get_partner_balances": "business:partner-balance",
}
var ErrUnauthorized = errors.New("unauthenticated")
var ErrUnavailable = errors.New("temporarily_unavailable")

type Token struct {
	ID        int64      `json:"id"`
	UserID    int64      `json:"userId"`
	ClientID  string     `json:"clientId"`
	Digest    string     `json:"-"`
	Scopes    string     `json:"scopes"`
	ExpiresAt time.Time  `json:"expiresAt"`
	RevokedAt *time.Time `json:"revokedAt"`
	CreatedAt time.Time  `json:"createdAt"`
	CreatedBy string     `json:"createdBy"`
	RevokedBy *string    `json:"revokedBy"`
}

func (Token) TableName() string { return "mcp_token" }

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db} }
func digest(raw string) string              { sum := sha256.Sum256([]byte(raw)); return hex.EncodeToString(sum[:]) }
func validLabel(s string) bool {
	return strings.TrimSpace(s) != "" && utf8.RuneCountInString(s) <= 100 && !strings.ContainsAny(s, "\r\n\x00")
}
func (r *Repository) Issue(ctx context.Context, userID int64, client, actor string, scopes []string, ttl time.Duration) (Token, string, error) {
	if userID <= 0 || !validLabel(client) || !validLabel(actor) || len(scopes) == 0 || ttl <= 0 || ttl > 90*24*time.Hour {
		return Token{}, "", errors.New("invalid token options")
	}
	for _, s := range scopes {
		if _, ok := Permissions[s]; !ok {
			return Token{}, "", errors.New("invalid scope")
		}
	}
	var count int64
	if e := r.db.WithContext(ctx).Table("sys_user").Where("id=? AND status=1 AND deleted=0", userID).Count(&count).Error; e != nil {
		return Token{}, "", e
	}
	if count != 1 {
		return Token{}, "", ErrUnauthorized
	}
	raw := make([]byte, 32)
	if _, e := rand.Read(raw); e != nil {
		return Token{}, "", e
	}
	plaintext := "simcp_" + hex.EncodeToString(raw)
	encoded, _ := json.Marshal(scopes)
	t := Token{UserID: userID, ClientID: client, Digest: digest(plaintext), Scopes: string(encoded), ExpiresAt: time.Now().UTC().Add(ttl), CreatedAt: time.Now().UTC(), CreatedBy: actor}
	if e := r.db.WithContext(ctx).Create(&t).Error; e != nil {
		return Token{}, "", e
	}
	return t, plaintext, nil
}
func (r *Repository) List(ctx context.Context, userID int64) ([]Token, error) {
	out := []Token{}
	q := r.db.WithContext(ctx).Omit("digest")
	if userID > 0 {
		q = q.Where("user_id=?", userID)
	}
	e := q.Order("id").Find(&out).Error
	return out, e
}
func (r *Repository) Revoke(ctx context.Context, id int64, actor string) error {
	if id <= 0 || !validLabel(actor) {
		return errors.New("invalid revoke options")
	}
	q := r.db.WithContext(ctx).Model(&Token{}).Where("id=? AND revoked_at IS NULL", id).Updates(map[string]any{"revoked_at": time.Now().UTC(), "revoked_by": actor})
	if q.Error != nil {
		return q.Error
	}
	if q.RowsAffected != 1 {
		return errors.New("token missing or already revoked")
	}
	return nil
}
func (r *Repository) Authenticate(ctx context.Context, raw string) (Token, map[string]bool, error) {
	var t Token
	if len(raw) != 70 || !strings.HasPrefix(raw, "simcp_") {
		return t, nil, ErrUnauthorized
	}
	e := r.db.WithContext(ctx).Where("digest=? AND revoked_at IS NULL AND expires_at>?", digest(raw), time.Now().UTC()).Take(&t).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return t, nil, ErrUnauthorized
	}
	if e != nil {
		return t, nil, ErrUnavailable
	}
	var count int64
	e = r.db.WithContext(ctx).Table("sys_user").Where("id=? AND status=1 AND deleted=0", t.UserID).Count(&count).Error
	if e != nil {
		return t, nil, ErrUnavailable
	}
	if count != 1 {
		return t, nil, ErrUnauthorized
	}
	var codes []string
	e = r.db.WithContext(ctx).Table("sys_menu m").Distinct("m.permission_code").Joins("JOIN sys_role_menu rm ON rm.menu_id=m.id").Joins("JOIN sys_role r ON r.id=rm.role_id").Joins("JOIN sys_user_role ur ON ur.role_id=r.id").Where("ur.user_id=? AND r.status=1 AND r.deleted=0 AND m.status=1 AND m.deleted=0 AND m.permission_code IS NOT NULL", t.UserID).Pluck("m.permission_code", &codes).Error
	if e != nil {
		return t, nil, ErrUnavailable
	}
	have := map[string]bool{}
	for _, c := range codes {
		have[c] = true
	}
	var scopes []string
	if json.Unmarshal([]byte(t.Scopes), &scopes) != nil {
		return t, nil, ErrUnauthorized
	}
	allowed := map[string]bool{}
	for _, s := range scopes {
		if code, ok := Permissions[s]; ok && have[code] {
			allowed[s] = true
		}
	}
	return t, allowed, nil
}
