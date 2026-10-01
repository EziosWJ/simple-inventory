package salereturn

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/EziosWJ/simple-inventory/server/internal/audit"
)

var (
	ErrInvalid  = errors.New("销售退货参数错误")
	ErrNotFound = errors.New("销售退货单不存在")
	ErrConflict = errors.New("销售退货单状态或版本已变化，请刷新后重试")
)

type Store interface {
	Source(context.Context, int64) (*Document, error)
	Create(context.Context, Document, []Item, audit.Event) (Document, error)
	Edit(context.Context, int64, int64, Document, []Item, audit.Event) (Document, error)
	Cancel(context.Context, int64, int64, string, audit.Event) (Document, error)
	Post(context.Context, int64, int64, audit.Event) (Document, error)
	Find(context.Context, int64) (*Document, error)
	Page(context.Context, Query) (Page, error)
}
type Service struct{ store Store }

func NewService(s Store) *Service { return &Service{s} }
func (s *Service) Source(ctx context.Context, id int64) (*Document, error) {
	if id < 1 {
		return nil, ErrInvalid
	}
	return s.store.Source(ctx, id)
}
func (s *Service) Create(ctx context.Context, m audit.Metadata, in Input) (Document, error) {
	h, items, e := validate(in)
	if e != nil {
		return Document{}, e
	}
	h.CreatedBy = m.ActorID
	return s.store.Create(ctx, h, items, audit.Event{Action: "sale_return.draft.create", Resource: "sale_return", Summary: "新建销售退货草稿", Metadata: m})
}
func (s *Service) Edit(ctx context.Context, m audit.Metadata, id int64, in EditInput) (Document, error) {
	if id < 1 || in.Version < 1 || in.Version == math.MaxInt64 {
		return Document{}, ErrInvalid
	}
	h, items, e := validate(in.Input)
	if e != nil {
		return Document{}, e
	}
	return s.store.Edit(ctx, id, in.Version, h, items, audit.Event{Action: "sale_return.draft.edit", Resource: "sale_return", ResourceID: id, Summary: "编辑销售退货草稿", Metadata: m})
}
func (s *Service) Cancel(ctx context.Context, m audit.Metadata, id int64, in CancelInput) (Document, error) {
	r := strings.TrimSpace(in.Reason)
	if id < 1 || in.Version < 1 || in.Version == math.MaxInt64 || r == "" || utf8.RuneCountInString(r) > 500 {
		return Document{}, ErrInvalid
	}
	return s.store.Cancel(ctx, id, in.Version, r, audit.Event{Action: "sale_return.cancel", Resource: "sale_return", ResourceID: id, Summary: "取消销售退货单", Metadata: m})
}
func (s *Service) Post(ctx context.Context, m audit.Metadata, id int64, in PostInput) (Document, error) {
	if id < 1 || in.Version < 1 || in.Version == math.MaxInt64 {
		return Document{}, ErrInvalid
	}
	return s.store.Post(ctx, id, in.Version, audit.Event{Action: "sale_return.post", Resource: "sale_return", ResourceID: id, Summary: "过账销售退货单", Metadata: m})
}
func (s *Service) Detail(ctx context.Context, id int64) (*Document, error) {
	if id < 1 {
		return nil, ErrInvalid
	}
	return s.store.Find(ctx, id)
}
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
	if q.Page > math.MaxInt/q.PageSize || q.SaleID < 0 || q.PartnerID < 0 {
		return Page{}, ErrInvalid
	}
	if q.Status != "" && q.Status != "DRAFT" && q.Status != "POSTED" && q.Status != "CANCELLED" {
		return Page{}, ErrInvalid
	}
	for _, d := range []string{q.BusinessFrom, q.BusinessTo} {
		if d != "" {
			if _, e := time.Parse("2006-01-02", d); e != nil {
				return Page{}, ErrInvalid
			}
		}
	}
	if q.BusinessFrom != "" && q.BusinessTo != "" && q.BusinessFrom > q.BusinessTo {
		return Page{}, ErrInvalid
	}
	return s.store.Page(ctx, q)
}
func validate(in Input) (Document, []Item, error) {
	d, e := time.Parse("2006-01-02", in.BusinessDate)
	if e != nil || in.SaleID < 1 || len(in.Items) == 0 || len(in.Items) > 200 {
		return Document{}, nil, ErrInvalid
	}
	remark, e := clean(in.Remark)
	if e != nil {
		return Document{}, nil, e
	}
	h := Document{SaleID: in.SaleID, BusinessDate: d.Format("2006-01-02"), Status: "DRAFT", Version: 1, Remark: remark, Items: []Item{}}
	items := make([]Item, 0, len(in.Items))
	seen := map[int64]bool{}
	for i, x := range in.Items {
		if x.SaleItemID < 1 || seen[x.SaleItemID] {
			return Document{}, nil, fmt.Errorf("%w：原销售明细不能重复选择", ErrInvalid)
		}
		seen[x.SaleItemID] = true
		q, e := parseMilli(x.Quantity)
		if e != nil || q <= 0 {
			return Document{}, nil, fmt.Errorf("%w：第%d行退货数量必须为正且最多三位小数", ErrInvalid, i+1)
		}
		r, e := clean(x.Remark)
		if e != nil {
			return Document{}, nil, e
		}
		items = append(items, Item{SaleItemID: x.SaleItemID, QuantityMilli: q, Remark: r})
	}
	h.Remark = remark
	return h, items, nil
}
func clean(p *string) (*string, error) {
	if p == nil {
		return nil, nil
	}
	v := strings.TrimSpace(*p)
	if utf8.RuneCountInString(v) > 500 {
		return nil, ErrInvalid
	}
	if v == "" {
		return nil, nil
	}
	return &v, nil
}
func parseMilli(s string) (int64, error) {
	if s == "" || strings.TrimSpace(s) != s || strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") {
		return 0, ErrInvalid
	}
	p := strings.Split(s, ".")
	if len(p) > 2 || p[0] == "" || len(p) == 2 && (len(p[1]) == 0 || len(p[1]) > 3) {
		return 0, ErrInvalid
	}
	for _, z := range p {
		for _, r := range z {
			if r < '0' || r > '9' {
				return 0, ErrInvalid
			}
		}
	}
	w, e := strconv.ParseInt(p[0], 10, 64)
	if e != nil || w > math.MaxInt64/1000 {
		return 0, ErrInvalid
	}
	f := int64(0)
	if len(p) == 2 {
		z := p[1]
		for len(z) < 3 {
			z += "0"
		}
		f, e = strconv.ParseInt(z, 10, 64)
		if e != nil {
			return 0, ErrInvalid
		}
	}
	if w*1000 > math.MaxInt64-f {
		return 0, ErrInvalid
	}
	return w*1000 + f, nil
}
func roundAmount(q, p int64) (int64, error) {
	n := new(big.Int).Mul(big.NewInt(q), big.NewInt(p))
	n.Add(n, big.NewInt(500))
	n.Quo(n, big.NewInt(1000))
	if !n.IsInt64() {
		return 0, ErrInvalid
	}
	return n.Int64(), nil
}
func milliText(n int64) string {
	whole, frac := n/1000, n%1000
	if frac == 0 {
		return fmt.Sprint(whole)
	}
	f := strings.TrimRight(fmt.Sprintf("%03d", frac), "0")
	return fmt.Sprintf("%d.%s", whole, f)
}
func moneyText(n int64) string { return fmt.Sprintf("%d.%02d", n/100, n%100) }
