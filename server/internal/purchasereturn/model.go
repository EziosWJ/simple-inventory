package purchasereturn

import (
	"github.com/EziosWJ/simple-inventory/server/internal/directdelivery"
	"time"
)

type Document struct {
	DirectTrace     *directdelivery.Trace `json:"directTrace" gorm:"-"`
	ID              int64                 `json:"id"`
	DocumentNo      string                `json:"documentNo"`
	PurchaseID      int64                 `json:"purchaseId"`
	PurchaseNo      string                `json:"purchaseNo" gorm:"->"`
	PartnerID       int64                 `json:"partnerId"`
	PartnerName     string                `json:"partnerName" gorm:"->"`
	BusinessDate    string                `json:"businessDate"`
	Status          string                `json:"status"`
	Version         int64                 `json:"version"`
	PostedBy        *int64                `json:"postedBy"`
	PostedByName    string                `json:"postedByName" gorm:"->"`
	PostedAt        *time.Time            `json:"postedAt"`
	Remark          *string               `json:"remark"`
	CreatedBy       int64                 `json:"createdBy"`
	CreatedByName   string                `json:"createdByName" gorm:"->"`
	CancelledBy     *int64                `json:"cancelledBy"`
	CancelledByName string                `json:"cancelledByName" gorm:"->"`
	CancelReason    *string               `json:"cancelReason"`
	CreateTime      time.Time             `json:"createTime"`
	CancelledAt     *time.Time            `json:"cancelledAt"`
	Items           []Item                `json:"items" gorm:"-"`
	TotalAmount     string                `json:"totalAmount" gorm:"-"`
}

func (Document) TableName() string { return "purchase_return_document" }

type Item struct {
	ID                     int64   `json:"id"`
	DocumentID             int64   `json:"-" gorm:"column:document_id"`
	PurchaseItemID         int64   `json:"purchaseItemId" gorm:"column:purchase_item_id"`
	ProductID              int64   `json:"productId"`
	ProductCode            string  `json:"productCode"`
	ProductName            string  `json:"productName"`
	ProductModel           *string `json:"productModel"`
	ProductSpecification   *string `json:"productSpecification"`
	Unit                   string  `json:"unit"`
	OriginalQuantityMilli  int64   `json:"-" gorm:"-"`
	OriginalQuantity       string  `json:"originalQuantity" gorm:"-"`
	ReturnedQuantityMilli  int64   `json:"-" gorm:"-"`
	ReturnedQuantity       string  `json:"returnedQuantity" gorm:"-"`
	RemainingQuantityMilli int64   `json:"-" gorm:"-"`
	RemainingQuantity      string  `json:"remainingQuantity" gorm:"-"`
	QuantityMilli          int64   `json:"-" gorm:"column:quantity_milli"`
	Quantity               string  `json:"quantity" gorm:"-"`
	UnitPriceCents         int64   `json:"-" gorm:"column:unit_price_cents"`
	UnitPrice              string  `json:"unitPrice" gorm:"-"`
	AmountCents            int64   `json:"-" gorm:"column:amount_cents"`
	Amount                 string  `json:"amount" gorm:"-"`
	PriorAmount            string  `json:"priorReturnAmount" gorm:"-"`
	Remark                 *string `json:"remark"`
}

func (Item) TableName() string { return "purchase_return_document_item" }

type ItemInput struct {
	PurchaseItemID int64   `json:"purchaseItemId"`
	Quantity       string  `json:"quantity"`
	Remark         *string `json:"remark"`
}
type Input struct {
	PurchaseID   int64       `json:"purchaseId"`
	BusinessDate string      `json:"businessDate"`
	Remark       *string     `json:"remark"`
	Items        []ItemInput `json:"items"`
}
type EditInput struct {
	Version int64 `json:"version"`
	Input
}
type CancelInput struct {
	Version int64  `json:"version"`
	Reason  string `json:"reason"`
}
type PostInput struct {
	Version int64 `json:"version"`
}
type Query struct {
	Page, PageSize           int
	DocumentNo, Status       string
	PurchaseID, PartnerID    int64
	BusinessFrom, BusinessTo string
}
type Page struct {
	Records  []Document `json:"records"`
	Total    int64      `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"pageSize"`
}
