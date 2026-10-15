package mcpserver

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/EziosWJ/simple-inventory/server/internal/audit"
)

// ErrForbidden reports a caller without the token-administration permission.
var ErrForbidden = errors.New("forbidden")

// TokenStore is the persistence boundary consumed by the administration
// service.
type TokenStore interface {
	Create(context.Context, CreateOptions, audit.Event) (Token, string, error)
	List(context.Context, TokenQuery) (Page[TokenView], error)
	View(context.Context, int64) (TokenView, error)
	Reveal(context.Context, int64) (string, error)
	Reset(context.Context, int64, audit.Event) (Token, string, error)
	Extend(context.Context, int64, int, bool, audit.Event) (Token, error)
	SetEnabled(context.Context, int64, bool, audit.Event) (Token, error)
	CanManage(context.Context) (bool, error)
	Record(context.Context, audit.Event) error
}

var _ TokenStore = (*Repository)(nil)

// TokenService owns the credential lifecycle rules. It never sees the
// plaintext of a stored credential except when one is created or reset.
type TokenService struct{ store TokenStore }

func NewTokenService(store TokenStore) (*TokenService, error) {
	if store == nil {
		return nil, errors.New("token store is required")
	}
	return &TokenService{store: store}, nil
}

type TokenPageQuery struct {
	UserID   int64
	Keyword  string
	State    string
	Page     int
	PageSize int
}

type TokenCreateInput struct {
	ExpiresOn     string `json:"expiresOn"`
	Name          string `json:"name"`
	UserID        int64  `json:"userId"`
	ExpiresInDays int    `json:"expiresInDays"`
	Permanent     bool   `json:"permanent"`
}

type TokenExtendInput struct {
	Days      int  `json:"days"`
	Permanent bool `json:"permanent"`
}

type TokenStatusInput struct {
	Enabled *bool `json:"enabled"`
}

// TokenSecret carries the original value. It is returned only by create and
// reset, and by the dedicated administrator reveal endpoint.
type TokenSecret struct {
	ExpiresAt *time.Time `json:"expiresAt"`
	Permanent bool       `json:"permanent"`
	Token     string     `json:"token"`
	Record    TokenView  `json:"record"`
}

const (
	tokenStates          = "ENABLED,DISABLED,EXPIRED"
	maxTokenDays         = 3650
	tokenDateLayout      = "2006-01-02"
	tokenSecretInvisible = "令牌原值需要原配置才能恢复，请恢复原配置或重置该令牌"
)

func (s *TokenService) authorize(ctx context.Context) error {
	allowed, err := s.store.CanManage(ctx)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrForbidden
	}
	return nil
}

func (s *TokenService) Page(ctx context.Context, q TokenPageQuery) (Page[TokenView], error) {
	if err := s.authorize(ctx); err != nil {
		return Page[TokenView]{}, err
	}
	q.Page, q.PageSize = normalizeTokenPage(q.Page, q.PageSize)
	if q.UserID < 0 || !validTokenState(q.State) {
		return Page[TokenView]{}, ErrInvalid
	}
	return s.store.List(ctx, TokenQuery{UserID: q.UserID, Keyword: strings.TrimSpace(q.Keyword), State: q.State, Page: q.Page, PageSize: q.PageSize})
}

// Create issues a credential bound to an existing user. The default lifetime
// is permanent, expressed as no expiry at all.
func (s *TokenService) Create(ctx context.Context, meta audit.Metadata, in TokenCreateInput) (TokenSecret, error) {
	if err := s.authorize(ctx); err != nil {
		return TokenSecret{}, err
	}
	expires, err := resolveInitialExpiry(in)
	if err != nil {
		return TokenSecret{}, err
	}
	token, plaintext, err := s.store.Create(ctx, CreateOptions{UserID: in.UserID, Name: in.Name, ExpiresAt: expires}, event(meta, "mcp-token.create", "创建 MCP 令牌"))
	if err != nil {
		return TokenSecret{}, err
	}
	return s.secret(ctx, token, plaintext)
}

// Reveal decrypts the original value of one credential for its administrator.
func (s *TokenService) Reveal(ctx context.Context, meta audit.Metadata, id int64) (TokenSecret, error) {
	if err := s.authorize(ctx); err != nil {
		return TokenSecret{}, err
	}
	if id <= 0 {
		return TokenSecret{}, ErrInvalid
	}
	plaintext, err := s.store.Reveal(ctx, id)
	if err != nil {
		return TokenSecret{}, err
	}
	view, err := s.store.View(ctx, id)
	if err != nil {
		return TokenSecret{}, err
	}
	if err := s.store.Record(ctx, audit.Event{Action: "mcp-token.reveal", Resource: "mcp-token", ResourceID: id, Summary: "查看 MCP 令牌原值", Metadata: meta}); err != nil {
		return TokenSecret{}, err
	}
	return TokenSecret{Token: plaintext, Record: view, Permanent: view.Permanent, ExpiresAt: view.ExpiresAt}, nil
}

// Extend keeps the credential value and only moves the expiry. It never
// re-enables a disabled credential.
func (s *TokenService) Extend(ctx context.Context, meta audit.Metadata, id int64, in TokenExtendInput) (TokenView, error) {
	if err := s.authorize(ctx); err != nil {
		return TokenView{}, err
	}
	if id <= 0 {
		return TokenView{}, ErrInvalid
	}
	if in.Permanent && in.Days != 0 {
		return TokenView{}, ErrInvalid
	}
	if !in.Permanent && (in.Days < 1 || in.Days > maxTokenDays) {
		return TokenView{}, ErrInvalid
	}
	if _, err := s.store.Extend(ctx, id, in.Days, in.Permanent, event(meta, "mcp-token.extend", "延期 MCP 令牌")); err != nil {
		return TokenView{}, err
	}
	return s.store.View(ctx, id)
}

// SetEnabled toggles the disabled state independently of the expiry.
func (s *TokenService) SetEnabled(ctx context.Context, meta audit.Metadata, id int64, in TokenStatusInput) (TokenView, error) {
	if err := s.authorize(ctx); err != nil {
		return TokenView{}, err
	}
	if id <= 0 || in.Enabled == nil {
		return TokenView{}, ErrInvalid
	}
	if _, err := s.store.SetEnabled(ctx, id, *in.Enabled, event(meta, "mcp-token.status", "修改 MCP 令牌状态")); err != nil {
		return TokenView{}, err
	}
	return s.store.View(ctx, id)
}

// Reset replaces the value while keeping the name, bound user, expiry and
// enabled state; the previous value stops working immediately.
func (s *TokenService) Reset(ctx context.Context, meta audit.Metadata, id int64) (TokenSecret, error) {
	if err := s.authorize(ctx); err != nil {
		return TokenSecret{}, err
	}
	if id <= 0 {
		return TokenSecret{}, ErrInvalid
	}
	token, plaintext, err := s.store.Reset(ctx, id, event(meta, "mcp-token.reset", "重置 MCP 令牌"))
	if err != nil {
		return TokenSecret{}, err
	}
	return s.secret(ctx, token, plaintext)
}

func (s *TokenService) secret(ctx context.Context, token Token, plaintext string) (TokenSecret, error) {
	view, err := s.store.View(ctx, token.ID)
	if err != nil {
		return TokenSecret{}, err
	}
	return TokenSecret{Token: plaintext, Record: view, Permanent: view.Permanent, ExpiresAt: view.ExpiresAt}, nil
}

func event(meta audit.Metadata, action, summary string) audit.Event {
	return audit.Event{Action: action, Resource: "mcp-token", Summary: summary, Metadata: meta}
}

func normalizeTokenPage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 500 {
		pageSize = 500
	}
	return page, pageSize
}

func validTokenState(state string) bool {
	switch state {
	case "", "ENABLED", "DISABLED", "EXPIRED":
		return true
	}
	return false
}

// resolveInitialExpiry accepts exactly one validity choice. A custom date is
// read as an Asia/Shanghai calendar day that stays valid through its end, so
// the chosen day itself is still usable.
func resolveInitialExpiry(in TokenCreateInput) (*time.Time, error) {
	if in.Permanent {
		if in.ExpiresInDays != 0 || strings.TrimSpace(in.ExpiresOn) != "" {
			return nil, ErrInvalid
		}
		return nil, nil
	}
	hasDays := in.ExpiresInDays > 0
	hasDate := strings.TrimSpace(in.ExpiresOn) != ""
	if hasDays == hasDate {
		return nil, ErrInvalid
	}
	if hasDays {
		if in.ExpiresInDays > maxTokenDays {
			return nil, ErrInvalid
		}
		expires := time.Now().UTC().Add(time.Duration(in.ExpiresInDays) * 24 * time.Hour)
		return &expires, nil
	}
	return ParseExpiryDate(in.ExpiresOn)
}

// ParseExpiryDate reads a custom validity end date as an Asia/Shanghai calendar
// day that stays valid through its end, so the chosen day itself is usable.
func ParseExpiryDate(value string) (*time.Time, error) {
	if len(value) != len(tokenDateLayout) {
		return nil, ErrInvalid
	}
	day, err := time.ParseInLocation(tokenDateLayout, value, shanghai)
	if err != nil {
		return nil, ErrInvalid
	}
	today := time.Now().In(shanghai)
	if day.Before(time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, shanghai)) {
		return nil, ErrInvalid
	}
	expires := day.AddDate(0, 0, 1).UTC()
	return &expires, nil
}
