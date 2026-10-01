package product

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/EziosWJ/simple-inventory/server/internal/audit"
	"github.com/EziosWJ/simple-inventory/server/internal/auth"
	platform "github.com/EziosWJ/simple-inventory/server/internal/platform/http"
)

var (
	ErrNotFound = errors.New("资料不存在")
	ErrInvalid  = errors.New("参数错误")
	ErrConflict = errors.New("编码已存在")
	// ErrIdentityLocked guards the permanent inventory identity rule of
	// ADR-0012: once a product has any inventory ledger line, its type and base
	// unit can never be edited again, not even after the stock returns to zero.
	ErrIdentityLocked = errors.New("商品已产生库存流水，类型和基本单位不可修改")
)

type Product struct {
	ID                int64   `json:"id"`
	Code              string  `json:"code"`
	Name              string  `json:"name"`
	Type              string  `json:"type"`
	Brand             *string `json:"brand"`
	Model             *string `json:"model"`
	Specification     *string `json:"specification"`
	Category          *string `json:"category"`
	Unit              string  `json:"unit"`
	PurchasePrice     *int64  `json:"-" gorm:"column:purchase_price_cents"`
	SalePrice         *int64  `json:"-" gorm:"column:sale_price_cents"`
	PurchasePriceText *string `json:"purchasePrice" gorm:"-"`
	SalePriceText     *string `json:"salePrice" gorm:"-"`
	Remark            *string `json:"remark"`
	Status            int     `json:"status"`
	// InventoryLocked reports that the product already has inventory history,
	// so the interface can explain why type and unit are read-only.
	InventoryLocked bool      `json:"inventoryLocked" gorm:"->;-:migration"`
	CreateTime      time.Time `json:"createTime" gorm:"autoCreateTime"`
	UpdateTime      time.Time `json:"updateTime" gorm:"autoUpdateTime"`
}

func (Product) TableName() string { return "product" }

type Input struct {
	Code          string  `json:"code"`
	Name          string  `json:"name"`
	Type          string  `json:"type"`
	Brand         *string `json:"brand"`
	Model         *string `json:"model"`
	Specification *string `json:"specification"`
	Category      *string `json:"category"`
	Unit          string  `json:"unit"`
	PurchasePrice *string `json:"purchasePrice"`
	SalePrice     *string `json:"salePrice"`
	Remark        *string `json:"remark"`
}
type Query struct {
	Page, PageSize          int
	Keyword, Type, Category string
	Status                  *int
}
type Page struct {
	Records  []Product `json:"records"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"pageSize"`
}
type Store interface {
	Page(context.Context, Query) (Page, error)
	Find(context.Context, int64) (*Product, error)
	Save(context.Context, Product, bool, audit.Event) (Product, error)
	SetStatus(context.Context, int64, int, audit.Event) error
}
type Service struct{ store Store }

func NewService(s Store) *Service { return &Service{s} }
func (s *Service) Page(ctx context.Context, q Query) (Page, error) {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = 10
	}
	if q.PageSize > 500 {
		q.PageSize = 500
	}
	return s.store.Page(ctx, q)
}
func (s *Service) Detail(ctx context.Context, id int64) (*Product, error) {
	if id <= 0 {
		return nil, ErrInvalid
	}
	return s.store.Find(ctx, id)
}
func (s *Service) Save(ctx context.Context, meta audit.Metadata, id int64, in Input) (Product, error) {
	if id < 0 {
		return Product{}, ErrInvalid
	}
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	in.Name = strings.TrimSpace(in.Name)
	in.Type = strings.ToUpper(strings.TrimSpace(in.Type))
	in.Unit = strings.TrimSpace(in.Unit)
	brand, model, specification, category, remark := clean(in.Brand), clean(in.Model), clean(in.Specification), clean(in.Category), clean(in.Remark)
	if id < 0 || in.Name == "" || in.Type != "GOODS" && in.Type != "SERVICE" || in.Unit == "" ||
		!within(in.Code, 50) || !within(in.Name, 200) || !within(in.Unit, 50) ||
		!withinPtr(brand, 100) || !withinPtr(model, 100) || !withinPtr(specification, 200) || !withinPtr(category, 100) || !withinPtr(remark, 500) {
		return Product{}, ErrInvalid
	}
	pur, e := money(in.PurchasePrice)
	if e != nil {
		return Product{}, ErrInvalid
	}
	sale, e := money(in.SalePrice)
	if e != nil {
		return Product{}, ErrInvalid
	}
	v := Product{ID: id, Code: in.Code, Name: in.Name, Type: in.Type, Brand: brand, Model: model, Specification: specification, Category: category, Unit: in.Unit, PurchasePrice: pur, SalePrice: sale, Remark: remark, Status: 1}
	if id > 0 {
		old, e := s.store.Find(ctx, id)
		if e != nil {
			return Product{}, e
		}
		if v.Code == "" {
			v.Code = old.Code
		}
		v.Status = old.Status
	}
	event := audit.Event{Action: "product.save", Resource: "product", ResourceID: id, Summary: "维护商品与服务", Metadata: meta}
	return s.store.Save(ctx, v, id == 0, event)
}
func (s *Service) SetStatus(ctx context.Context, m audit.Metadata, id int64, status int) error {
	if id <= 0 || status != 0 && status != 1 {
		return ErrInvalid
	}
	if _, e := s.store.Find(ctx, id); e != nil {
		return e
	}
	return s.store.SetStatus(ctx, id, status, audit.Event{Action: "product.status", Resource: "product", ResourceID: id, Summary: "变更商品状态", Metadata: m})
}
func clean(p *string) *string {
	if p == nil {
		return nil
	}
	v := strings.TrimSpace(*p)
	if v == "" {
		return nil
	}
	return &v
}
func within(v string, max int) bool     { return utf8.RuneCountInString(v) <= max }
func withinPtr(v *string, max int) bool { return v == nil || within(*v, max) }
func money(p *string) (*int64, error) {
	if p == nil || strings.TrimSpace(*p) == "" {
		return nil, nil
	}
	v := strings.TrimSpace(*p)
	parts := strings.Split(v, ".")
	if len(parts) > 2 || parts[0] == "" {
		return nil, ErrInvalid
	}
	for _, r := range parts[0] {
		if r < '0' || r > '9' {
			return nil, ErrInvalid
		}
	}
	frac := ""
	if len(parts) == 2 {
		frac = parts[1]
		if len(frac) == 0 || len(frac) > 2 {
			return nil, ErrInvalid
		}
		for _, r := range frac {
			if r < '0' || r > '9' {
				return nil, ErrInvalid
			}
		}
	}
	for len(frac) < 2 {
		frac += "0"
	}
	whole, e := strconv.ParseInt(parts[0], 10, 64)
	if e != nil || whole < 0 {
		return nil, ErrInvalid
	}
	minor := int64(0)
	if frac != "" {
		minor, e = strconv.ParseInt(frac, 10, 64)
		if e != nil {
			return nil, ErrInvalid
		}
	}
	if whole > (math.MaxInt64-minor)/100 {
		return nil, ErrInvalid
	}
	n := whole*100 + minor
	return &n, nil
}
func priceText(n *int64) *string {
	if n == nil {
		return nil
	}
	v := fmt.Sprintf("%d.%02d", *n/100, *n%100)
	return &v
}
func present(v Product) Product {
	v.PurchasePriceText = priceText(v.PurchasePrice)
	v.SalePriceText = priceText(v.SalePrice)
	return v
}
func meta(ctx context.Context) audit.Metadata {
	m := audit.Metadata{RequestID: platform.RequestIDFromContext(ctx)}
	if p, ok := auth.PrincipalFromContext(ctx); ok {
		m.ActorID = p.UserID
	}
	if r, ok := platform.RequestMetaFromContext(ctx); ok {
		m.ClientIP = r.ClientIP
		m.UserAgent = r.UserAgent
		m.RequestMethod = r.RequestMethod
		m.RequestURL = r.RequestURL
	}
	return m
}
