package inventory

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/EziosWJ/simple-inventory/server/internal/audit"
)

var ErrInvalid = errors.New("参数错误")
var ErrNotFound = errors.New("调整单不存在")
var ErrConflict = errors.New("单据状态或版本已变化，请刷新后重试")

type ProductReference struct {
	ID         int64
	Type, Unit string
	Status     int
}
type Store interface {
	Create(context.Context, Adjustment, audit.Event, func(Item, ProductReference) error) (Adjustment, error)
	Edit(context.Context, int64, int64, []Item, audit.Event, func(Item, ProductReference) error) (Adjustment, error)
	Cancel(context.Context, int64, int64, string, audit.Event) (Adjustment, error)
	Find(context.Context, int64) (*Adjustment, error)
	Page(context.Context, Query) (Page, error)
}
type Service struct{ store Store }

func NewService(s Store) *Service  { return &Service{s} }
func invalid(message string) error { return fmt.Errorf("%w：%s", ErrInvalid, message) }
func (s *Service) Create(ctx context.Context, meta audit.Metadata, in Input) (Adjustment, error) {
	items, err := validatedItems(in)
	if err != nil {
		return Adjustment{}, err
	}
	v := Adjustment{Status: "DRAFT", Version: 1, CreatedBy: meta.ActorID, Items: items}
	return s.store.Create(ctx, v, audit.Event{Action: "inventory.adjustment.create", Resource: "inventory", Summary: "新建库存调整草稿", Metadata: meta}, validateProduct)
}
func validatedItems(in Input) ([]Item, error) {
	if len(in.Items) == 0 {
		return nil, invalid("至少填写一条明细")
	}
	items := make([]Item, 0, len(in.Items))
	seen := map[int64]bool{}
	for i, x := range in.Items {
		if x.ProductID <= 0 || seen[x.ProductID] {
			return nil, invalid(fmt.Sprintf("第%d行商品无效或重复", i+1))
		}
		seen[x.ProductID] = true
		if x.ProductType != "GOODS" || strings.TrimSpace(x.Unit) == "" || utf8.RuneCountInString(x.Unit) > 50 {
			return nil, invalid(fmt.Sprintf("第%d行必须选择实物商品并确认基本单位", i+1))
		}
		n, e := parseQuantity(x.Quantity)
		if e != nil {
			return nil, invalid(fmt.Sprintf("第%d行数量必须非零、最多三位小数且在存储范围内", i+1))
		}
		var remark *string
		if x.Remark != nil {
			value := strings.TrimSpace(*x.Remark)
			if utf8.RuneCountInString(value) > 500 {
				return nil, invalid("说明最多500字")
			}
			if value != "" {
				remark = &value
			}
		}
		switch x.Reason {
		case "OPENING", "SURPLUS":
			if n < 0 {
				return nil, invalid("期初录入与盘盈数量必须为正")
			}
		case "SHORTAGE", "DAMAGE":
			if n > 0 {
				return nil, invalid("盘亏与报损数量必须为负")
			}
		case "OTHER":
			if remark == nil {
				return nil, invalid("其他原因必须填写说明")
			}
		default:
			return nil, invalid("调整原因无效")
		}
		items = append(items, Item{ProductID: x.ProductID, ProductType: x.ProductType, Unit: x.Unit, QuantityMilli: n, Quantity: quantityText(n), Reason: x.Reason, Remark: remark})
	}
	return items, nil
}
func validateProduct(item Item, p ProductReference) error {
	if p.Status != 1 || p.Type != "GOODS" {
		return invalid(fmt.Sprintf("商品%d必须为启用实物商品", item.ProductID))
	}
	if p.Type != item.ProductType || p.Unit != item.Unit {
		return invalid(fmt.Sprintf("商品%d的类型或单位已变化，请重新选择并确认数量", item.ProductID))
	}
	return nil
}
func (s *Service) Edit(ctx context.Context, meta audit.Metadata, id int64, in EditInput) (Adjustment, error) {
	if id <= 0 || in.Version <= 0 || in.Version == math.MaxInt64 {
		return Adjustment{}, invalid("调整单ID或版本无效")
	}
	items, err := validatedItems(Input{Items: in.Items})
	if err != nil {
		return Adjustment{}, err
	}
	return s.store.Edit(ctx, id, in.Version, items, audit.Event{Action: "inventory.adjustment.edit", Resource: "inventory", ResourceID: id, Summary: "编辑库存调整草稿", Metadata: meta}, validateProduct)
}
func (s *Service) Cancel(ctx context.Context, meta audit.Metadata, id int64, in CancelInput) (Adjustment, error) {
	if id <= 0 || in.Version <= 0 || in.Version == math.MaxInt64 {
		return Adjustment{}, invalid("调整单ID或版本无效")
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" || utf8.RuneCountInString(reason) > 500 {
		return Adjustment{}, invalid("取消原因必填且最多500字")
	}
	return s.store.Cancel(ctx, id, in.Version, reason, audit.Event{Action: "inventory.adjustment.cancel", Resource: "inventory", ResourceID: id, Summary: "取消库存调整草稿", Metadata: meta})
}

func (s *Service) Detail(ctx context.Context, id int64) (*Adjustment, error) {
	if id <= 0 {
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
	if q.Page > int(^uint(0)>>1)/q.PageSize {
		return Page{}, ErrInvalid
	}
	if q.ProductID < 0 || q.Status != "" && q.Status != "DRAFT" && q.Status != "POSTED" && q.Status != "CANCELLED" {
		return Page{}, ErrInvalid
	}
	if q.CreatedFrom != nil && q.CreatedTo != nil && !q.CreatedFrom.Before(*q.CreatedTo) {
		return Page{}, invalid("创建时间范围无效")
	}
	return s.store.Page(ctx, q)
}

// Quantities use signed integer thousandths; parsing never passes through float.
func parseQuantity(v string) (int64, error) {
	if v == "" || strings.TrimSpace(v) != v {
		return 0, ErrInvalid
	}
	sign := ""
	if v[0] == '-' || v[0] == '+' {
		sign = v[:1]
		v = v[1:]
	}
	parts := strings.Split(v, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, ErrInvalid
	}
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
		if len(fraction) == 0 || len(fraction) > 3 {
			return 0, ErrInvalid
		}
	}
	for _, part := range parts {
		for _, r := range part {
			if r < '0' || r > '9' {
				return 0, ErrInvalid
			}
		}
	}
	fraction += strings.Repeat("0", 3-len(fraction))
	n, e := strconv.ParseInt(sign+strings.TrimLeft(parts[0]+fraction, "0"), 10, 64)
	if e != nil || n == 0 {
		return 0, ErrInvalid
	}
	return n, nil
}
func quantityText(n int64) string {
	whole := n / 1000
	fraction := n % 1000
	if fraction < 0 {
		fraction = -fraction
	}
	sign := ""
	if n < 0 && whole == 0 {
		sign = "-"
	}
	if fraction == 0 {
		return strconv.FormatInt(whole, 10)
	}
	return sign + strconv.FormatInt(whole, 10) + "." + strings.TrimRight(fmt.Sprintf("%03d", fraction), "0")
}
