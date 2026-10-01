package purchase

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
	ErrInvalid  = errors.New("采购单参数错误")
	ErrNotFound = errors.New("采购单不存在")
	ErrConflict = errors.New("采购单状态或版本已变化，请刷新后重试")
)

type Store interface {
	Create(context.Context, Draft, []Line, audit.Event) (Draft, error)
	Edit(context.Context, int64, int64, Draft, []Line, audit.Event) (Draft, error)
	Cancel(context.Context, int64, int64, string, audit.Event) (Draft, error)
	Post(context.Context, int64, int64, audit.Event) (Draft, error)
	Find(context.Context, int64) (*Draft, error)
	Page(context.Context, Query) (Page, error)
}
type Service struct{ store Store }

func NewService(s Store) *Service { return &Service{s} }
func (s *Service) Create(ctx context.Context, m audit.Metadata, in Input) (Draft, error) {
	h, lines, e := validate(in)
	if e != nil {
		return Draft{}, e
	}
	h.CreatedBy = m.ActorID
	return s.store.Create(ctx, h, lines, audit.Event{Action: "purchase.draft.create", Resource: "purchase", Summary: "新建采购入库草稿", Metadata: m})
}
func (s *Service) Edit(ctx context.Context, m audit.Metadata, id int64, in EditInput) (Draft, error) {
	if id < 1 || in.Version < 1 || in.Version == math.MaxInt64 {
		return Draft{}, ErrInvalid
	}
	h, lines, e := validate(in.Input)
	if e != nil {
		return Draft{}, e
	}
	return s.store.Edit(ctx, id, in.Version, h, lines, audit.Event{Action: "purchase.draft.edit", Resource: "purchase", ResourceID: id, Summary: "编辑采购入库草稿", Metadata: m})
}
func (s *Service) Cancel(ctx context.Context, m audit.Metadata, id int64, in CancelInput) (Draft, error) {
	reason := strings.TrimSpace(in.Reason)
	if id < 1 || in.Version < 1 || in.Version == math.MaxInt64 || reason == "" || utf8.RuneCountInString(reason) > 500 {
		return Draft{}, ErrInvalid
	}
	return s.store.Cancel(ctx, id, in.Version, reason, audit.Event{Action: "purchase.draft.cancel", Resource: "purchase", ResourceID: id, Summary: "取消采购入库草稿", Metadata: m})
}
func (s *Service) Post(ctx context.Context, m audit.Metadata, id int64, in PostInput) (Draft, error) {
	if id < 1 || in.Version < 1 || in.Version == math.MaxInt64 {
		return Draft{}, ErrInvalid
	}
	return s.store.Post(ctx, id, in.Version, audit.Event{Action: "purchase.post", Resource: "purchase", ResourceID: id, Summary: "过账采购入库单", Metadata: m})
}
func (s *Service) Detail(ctx context.Context, id int64) (*Draft, error) {
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
	if q.Page > math.MaxInt/q.PageSize || q.ProductID < 0 || q.PartnerID < 0 {
		return Page{}, ErrInvalid
	}
	if q.Status != "" && q.Status != "DRAFT" && q.Status != "POSTED" && q.Status != "CANCELLED" {
		return Page{}, ErrInvalid
	}
	if q.BusinessFrom != "" {
		if _, e := time.Parse("2006-01-02", q.BusinessFrom); e != nil {
			return Page{}, ErrInvalid
		}
	}
	if q.BusinessTo != "" {
		if _, e := time.Parse("2006-01-02", q.BusinessTo); e != nil {
			return Page{}, ErrInvalid
		}
	}
	if q.BusinessFrom != "" && q.BusinessTo != "" && q.BusinessFrom > q.BusinessTo {
		return Page{}, ErrInvalid
	}
	return s.store.Page(ctx, q)
}

func validate(in Input) (Draft, []Line, error) {
	date, e := time.Parse("2006-01-02", in.BusinessDate)
	if e != nil || in.PartnerID < 1 || len(in.Items) == 0 || len(in.Items) > 200 {
		return Draft{}, nil, ErrInvalid
	}
	remark, e := clean(in.Remark, 500)
	if e != nil {
		return Draft{}, nil, e
	}
	h := Draft{PartnerID: in.PartnerID, BusinessDate: BusinessDate(date.Format("2006-01-02")), Status: "DRAFT", Version: 1, Remark: remark, Items: []Line{}}
	lines := make([]Line, 0, len(in.Items))
	total := int64(0)
	for i, x := range in.Items {
		if x.ProductID < 1 || x.ProductType != "GOODS" || strings.TrimSpace(x.Unit) == "" || utf8.RuneCountInString(x.Unit) > 50 {
			return Draft{}, nil, fmt.Errorf("%w：第%d行必须选择启用实物商品并确认单位", ErrInvalid, i+1)
		}
		qty, e := parseMilli(x.Quantity)
		if e != nil || qty <= 0 {
			return Draft{}, nil, fmt.Errorf("%w：第%d行数量必须为正且最多三位小数", ErrInvalid, i+1)
		}
		price, e := parseCents(x.UnitPrice)
		if e != nil || price < 0 {
			return Draft{}, nil, fmt.Errorf("%w：第%d行单价必须非负且最多两位小数", ErrInvalid, i+1)
		}
		amount, e := lineAmount(qty, price)
		if e != nil || total > math.MaxInt64-amount {
			return Draft{}, nil, fmt.Errorf("%w：单据金额超出范围", ErrInvalid)
		}
		total += amount
		lr, e := clean(x.Remark, 500)
		if e != nil {
			return Draft{}, nil, e
		}
		lines = append(lines, Line{ProductID: x.ProductID, ProductType: x.ProductType, Unit: strings.TrimSpace(x.Unit), QuantityMilli: qty, UnitPriceCents: price, AmountCents: amount, Remark: lr})
	}
	h.TotalAmount = moneyText(total)
	h.Items = lines
	return h, lines, nil
}
func clean(p *string, max int) (*string, error) {
	if p == nil {
		return nil, nil
	}
	v := strings.TrimSpace(*p)
	if utf8.RuneCountInString(v) > max {
		return nil, ErrInvalid
	}
	if v == "" {
		return nil, nil
	}
	return &v, nil
}
func parseMilli(s string) (int64, error) { return parseFixed(s, 3) }
func parseCents(s string) (int64, error) { return parseFixed(s, 2) }
func parseFixed(s string, scale int) (int64, error) {
	if s == "" || strings.TrimSpace(s) != s || strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") {
		return 0, ErrInvalid
	}
	p := strings.Split(s, ".")
	if len(p) > 2 || p[0] == "" || len(p) == 2 && (len(p[1]) == 0 || len(p[1]) > scale) {
		return 0, ErrInvalid
	}
	for _, part := range p {
		for _, r := range part {
			if r < '0' || r > '9' {
				return 0, ErrInvalid
			}
		}
	}
	whole, e := strconv.ParseInt(p[0], 10, 64)
	if e != nil {
		return 0, e
	}
	factor := int64(1)
	for i := 0; i < scale; i++ {
		factor *= 10
	}
	if whole > (math.MaxInt64 / factor) {
		return 0, ErrInvalid
	}
	frac := int64(0)
	if len(p) == 2 {
		digits := p[1]
		for len(digits) < scale {
			digits += "0"
		}
		frac, e = strconv.ParseInt(digits, 10, 64)
		if e != nil {
			return 0, e
		}
	}
	if whole*factor > math.MaxInt64-frac {
		return 0, ErrInvalid
	}
	return whole*factor + frac, nil
}
func lineAmount(qty, price int64) (int64, error) {
	n := new(big.Int).Mul(big.NewInt(qty), big.NewInt(price))
	n.Add(n, big.NewInt(500))
	n.Quo(n, big.NewInt(1000))
	if !n.IsInt64() {
		return 0, ErrInvalid
	}
	return n.Int64(), nil
}
func moneyText(c int64) string { return fmt.Sprintf("%d.%02d", c/100, c%100) }
