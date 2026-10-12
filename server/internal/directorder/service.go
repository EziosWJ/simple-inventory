package directorder

import (
	"context"
	"errors"
	"fmt"
	"github.com/EziosWJ/simple-inventory/server/internal/audit"
	"github.com/EziosWJ/simple-inventory/server/internal/purchase"
	"github.com/EziosWJ/simple-inventory/server/internal/sale"
	"math"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrForbidden       = errors.New("无直送单操作权限")
	ErrInvalid         = errors.New("直送单参数错误")
	ErrNotFound        = errors.New("直送单不存在")
	ErrConflict        = errors.New("直送单状态或版本已变化，请刷新核对")
	ErrRequestConflict = errors.New("保存标识已用于其他内容，请先核实原保存结果")
)

type Store interface {
	Post(context.Context, int64, int64, audit.Event) (Draft, error)
	Cancel(context.Context, int64, int64, string, audit.Event) (Draft, error)
	Authorize(context.Context) error
	Create(context.Context, Draft, []Line, audit.Event) (Draft, error)
	Edit(context.Context, int64, int64, Draft, []Line, audit.Event) (Draft, error)
	Find(context.Context, int64) (*Draft, error)
	Page(context.Context, Query) (Page, error)
	SaveResult(context.Context, int64, string, string) (SaveResult, error)
	ResolveSave(context.Context, audit.Metadata, string, string) (SaveResult, error)
}
type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store} }
func (s *Service) Create(ctx context.Context, m audit.Metadata, in Input) (Draft, error) {
	h, lines, e := validate(in)
	if e != nil {
		return Draft{}, e
	}
	h.CreatedBy = m.ActorID
	h.SaveRequest, e = prepareSaveRequest(in.RequestKey, "CREATE", 0, 0, h, lines)
	if e != nil {
		return Draft{}, e
	}
	return s.store.Create(ctx, h, lines, audit.Event{Action: "directorder.draft.create", Resource: "directorder", Summary: "新建直送草稿", Metadata: m})
}
func (s *Service) Edit(ctx context.Context, m audit.Metadata, id int64, in EditInput) (Draft, error) {
	if id < 1 || in.Version < 1 || in.Version == math.MaxInt64 {
		return Draft{}, ErrInvalid
	}
	h, lines, e := validate(in.Input)
	if e != nil {
		return Draft{}, e
	}
	h.SaveRequest, e = prepareSaveRequest(in.RequestKey, "EDIT", id, in.Version, h, lines)
	if e != nil {
		return Draft{}, e
	}
	return s.store.Edit(ctx, id, in.Version, h, lines, audit.Event{Action: "directorder.draft.edit", Resource: "directorder", ResourceID: id, Summary: "编辑直送草稿", Metadata: m})
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
	if q.Page > math.MaxInt/q.PageSize || q.SupplierID < 0 || q.CustomerID < 0 || len(q.DocumentNo) > 100 {
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
func validate(in Input) (Draft, []Line, error) {
	pi := purchase.Input{PartnerID: in.SupplierID, BusinessDate: in.BusinessDate, Remark: in.Remark}
	si := sale.Input{PartnerID: in.CustomerID, BusinessDate: in.BusinessDate, Remark: in.Remark, DeliveryContact: in.DeliveryContact, DeliveryPhone: in.DeliveryPhone, DeliveryAddress: in.DeliveryAddress}
	if len(in.Items) == 0 || len(in.Items) > 200 {
		return Draft{}, nil, ErrInvalid
	}
	for _, l := range in.Items {
		si.Items = append(si.Items, sale.LineInput{ProductID: l.ProductID, ProductType: l.ProductType, Unit: l.Unit, Quantity: l.Quantity, UnitPrice: l.SalePrice, Remark: l.Remark})
		if l.ProductType == "GOODS" {
			pi.Items = append(pi.Items, purchase.LineInput{ProductID: l.ProductID, ProductType: l.ProductType, Unit: l.Unit, Quantity: l.Quantity, UnitPrice: l.PurchasePrice, Remark: l.Remark})
		} else if l.ProductType == "SERVICE" && l.PurchasePrice != "" && l.PurchasePrice != "0" && l.PurchasePrice != "0.00" {
			return Draft{}, nil, fmt.Errorf("%w：服务不能填写采购价", ErrInvalid)
		}
	}
	ph, pl, e := purchase.ValidateDraftInput(pi)
	if e != nil {
		return Draft{}, nil, fmt.Errorf("%w：%v", ErrInvalid, e)
	}
	sh, sl, e := sale.ValidateDraftInput(si)
	if e != nil {
		return Draft{}, nil, fmt.Errorf("%w：%v", ErrInvalid, e)
	}
	h := Draft{SupplierID: in.SupplierID, CustomerID: in.CustomerID, BusinessDate: ph.BusinessDate, Status: "DRAFT", Version: 1, Remark: ph.Remark, DeliveryContact: sh.DeliveryContact, DeliveryPhone: sh.DeliveryPhone, DeliveryAddress: sh.DeliveryAddress, PurchaseTotal: ph.TotalAmount, SaleTotal: sh.TotalAmount}
	lines := make([]Line, 0, len(sl))
	j := 0
	for _, l := range sl {
		x := Line{ProductID: l.ProductID, ProductType: l.ProductType, Unit: l.Unit, QuantityMilli: l.QuantityMilli, SalePriceCents: l.UnitPriceCents, SaleAmountCents: l.AmountCents, Remark: l.Remark}
		if l.ProductType == "GOODS" {
			x.PurchasePriceCents = pl[j].UnitPriceCents
			x.PurchaseAmountCents = pl[j].AmountCents
			j++
		}
		lines = append(lines, x)
	}
	h.Items = lines
	return h, lines, nil
}
func milliText(n int64) string {
	v := fmt.Sprintf("%d.%03d", n/1000, n%1000)
	return strings.TrimRight(strings.TrimRight(v, "0"), ".")
}
func moneyText(n int64) string { return fmt.Sprintf("%d.%02d", n/100, n%100) }

func (s *Service) Authorize(ctx context.Context) error { return s.store.Authorize(ctx) }

func (s *Service) Post(ctx context.Context, m audit.Metadata, id int64, in PostInput) (Draft, error) {
	if id < 1 || in.Version < 1 || in.Version == math.MaxInt64 {
		return Draft{}, ErrInvalid
	}
	return s.store.Post(ctx, id, in.Version, audit.Event{Action: "directorder.post", Resource: "directorder", ResourceID: id, Summary: "确认直送并过账采购销售", Metadata: m})
}
func (s *Service) Cancel(ctx context.Context, m audit.Metadata, id int64, in CancelInput) (Draft, error) {
	reason := strings.TrimSpace(in.Reason)
	if id < 1 || in.Version < 1 || in.Version == math.MaxInt64 || reason == "" || utf8.RuneCountInString(reason) > 500 {
		return Draft{}, ErrInvalid
	}
	return s.store.Cancel(ctx, id, in.Version, reason, audit.Event{Action: "directorder.cancel", Resource: "directorder", ResourceID: id, Summary: "整笔取消直送", Metadata: m})
}
