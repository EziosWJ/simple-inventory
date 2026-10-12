package sale

import (
	"context"
	"fmt"
	"github.com/EziosWJ/simple-inventory/server/internal/audit"
	"math"
	"time"
)

type SourceInput struct {
	PurchaseItemID   int64  `json:"purchaseItemId"`
	Quantity         string `json:"quantity"`
	ConfirmLaterDate bool   `json:"confirmLaterDate"`
}
type SourceValue struct {
	PurchaseItemID   int64 `json:"purchaseItemId"`
	QuantityMilli    int64 `json:"quantityMilli"`
	ConfirmLaterDate bool  `json:"confirmLaterDate"`
}
type PurchaseSource struct {
	ID                    int64        `json:"id"`
	SaleItemID            int64        `json:"saleItemId"`
	PurchaseItemID        int64        `json:"purchaseItemId"`
	QuantityMilli         int64        `json:"-"`
	Quantity              string       `json:"quantity" gorm:"-"`
	ConfirmLaterDate      bool         `json:"confirmLaterDate"`
	PurchaseID            int64        `json:"purchaseId" gorm:"->"`
	DocumentNo            string       `json:"documentNo" gorm:"->"`
	PartnerName           string       `json:"partnerName" gorm:"->"`
	BusinessDate          BusinessDate `json:"businessDate" gorm:"->"`
	Status                string       `json:"status" gorm:"->"`
	ReturnedQuantityMilli int64        `json:"-" gorm:"->"`
	ReturnedQuantity      string       `json:"returnedQuantity" gorm:"-"`
}

func (PurchaseSource) TableName() string { return "sale_purchase_source" }

type SourceHistory struct {
	ID           int64     `json:"id"`
	SaleID       int64     `json:"saleId"`
	SaleItemID   int64     `json:"saleItemId"`
	BeforeData   string    `json:"beforeData"`
	AfterData    string    `json:"afterData"`
	OperatorID   int64     `json:"operatorId"`
	OperatorName string    `json:"operatorName" gorm:"->"`
	CreateTime   time.Time `json:"createTime"`
}

func (SourceHistory) TableName() string { return "sale_purchase_source_history" }

type SourceLineInput struct {
	SaleItemID int64         `json:"saleItemId"`
	Sources    []SourceInput `json:"sources"`
}
type SourceEditInput struct {
	Version int64             `json:"version"`
	Items   []SourceLineInput `json:"items"`
}
type sourceStore interface {
	UpdateSources(context.Context, int64, int64, map[int64][]SourceInput, audit.Event) (Draft, error)
	SourceHistory(context.Context, int64) ([]SourceHistory, error)
}

func normalizeSources(in []SourceInput, productType string, quantity int64) ([]SourceValue, error) {
	if len(in) > 200 || productType != "GOODS" && len(in) > 0 {
		return nil, fmt.Errorf("%w：服务不能填写实物采购来源", ErrInvalid)
	}
	out := []SourceValue{}
	seen := map[int64]bool{}
	var total int64
	for _, x := range in {
		qty, e := parseMilli(x.Quantity)
		if e != nil || qty <= 0 || x.PurchaseItemID < 1 || seen[x.PurchaseItemID] || total > math.MaxInt64-qty {
			return nil, fmt.Errorf("%w：来源数量或采购明细重复，请核对", ErrInvalid)
		}
		total += qty
		seen[x.PurchaseItemID] = true
		out = append(out, SourceValue{x.PurchaseItemID, qty, x.ConfirmLaterDate})
	}
	if total > quantity {
		return nil, fmt.Errorf("%w：来源数量合计不能超过销售明细原数量", ErrInvalid)
	}
	return out, nil
}
func (s *Service) UpdateSources(ctx context.Context, m audit.Metadata, id int64, in SourceEditInput) (Draft, error) {
	if id < 1 || in.Version < 1 || in.Version == math.MaxInt64 || len(in.Items) == 0 || len(in.Items) > 200 {
		return Draft{}, ErrInvalid
	}
	items := map[int64][]SourceInput{}
	for _, x := range in.Items {
		if x.SaleItemID < 1 {
			return Draft{}, ErrInvalid
		}
		if _, ok := items[x.SaleItemID]; ok {
			return Draft{}, ErrInvalid
		}
		items[x.SaleItemID] = x.Sources
	}
	store, ok := s.store.(sourceStore)
	if !ok {
		return Draft{}, ErrConflict
	}
	return store.UpdateSources(ctx, id, in.Version, items, audit.Event{Action: "sale.sources.edit", Resource: "sale", ResourceID: id, Summary: "更正销售采购来源", Metadata: m})
}
func (s *Service) SourceHistory(ctx context.Context, id int64) ([]SourceHistory, error) {
	if id < 1 {
		return nil, ErrInvalid
	}
	store, ok := s.store.(sourceStore)
	if !ok {
		return nil, ErrConflict
	}
	return store.SourceHistory(ctx, id)
}
