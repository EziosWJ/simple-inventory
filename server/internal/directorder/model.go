// Package directorder manages independent supplier-to-customer delivery documents.
package directorder

import (
	"github.com/EziosWJ/simple-inventory/server/internal/purchase"
	"time"
)

type Draft struct {
	PurchaseID            *int64     `json:"purchaseId"`
	SaleID                *int64     `json:"saleId"`
	PostedBy              *int64     `json:"postedBy"`
	PostedAt              *time.Time `json:"postedAt"`
	PostedByName          string     `json:"postedByName" gorm:"->"`
	CancelledBy           *int64     `json:"cancelledBy"`
	CancelledAt           *time.Time `json:"cancelledAt"`
	CancelledByName       string     `json:"cancelledByName" gorm:"->"`
	CancelReason          *string    `json:"cancelReason"`
	SupplierSnapshot      *string    `json:"-"`
	CustomerSnapshot      *string    `json:"-"`
	ActiveSaleReturns     []int64    `json:"activeSaleReturns" gorm:"-"`
	ActivePurchaseReturns []int64    `json:"activePurchaseReturns" gorm:"-"`

	ID              int64                 `json:"id"`
	DocumentNo      string                `json:"documentNo"`
	SupplierID      int64                 `json:"supplierId"`
	CustomerID      int64                 `json:"customerId"`
	SupplierName    string                `json:"supplierName" gorm:"->"`
	CustomerName    string                `json:"customerName" gorm:"->"`
	BusinessDate    purchase.BusinessDate `json:"businessDate"`
	Status          string                `json:"status"`
	Version         int64                 `json:"version"`
	DeliveryContact *string               `json:"deliveryContact"`
	DeliveryPhone   *string               `json:"deliveryPhone"`
	DeliveryAddress *string               `json:"deliveryAddress"`
	Remark          *string               `json:"remark"`
	CreatedBy       int64                 `json:"createdBy"`
	CreatedByName   string                `json:"createdByName" gorm:"->"`
	CreateTime      time.Time             `json:"createTime"`
	Items           []Line                `json:"items" gorm:"-"`
	PurchaseTotal   string                `json:"purchaseTotal" gorm:"-"`
	SaleTotal       string                `json:"saleTotal" gorm:"-"`
	SaveRequest     *SaveRequest          `json:"-" gorm:"-"`
	SaveReceipt     *SaveReceipt          `json:"saveReceipt,omitempty" gorm:"-"`
}

func (Draft) TableName() string { return "direct_order_document" }

type Line struct {
	ProductModel         *string `json:"productModel"`
	ProductSpecification *string `json:"productSpecification"`

	ID                  int64   `json:"id"`
	DocumentID          int64   `json:"-"`
	ProductID           int64   `json:"productId"`
	ProductType         string  `json:"productType"`
	ProductName         string  `json:"productName" gorm:"->"`
	ProductCode         string  `json:"productCode" gorm:"->"`
	Unit                string  `json:"unit"`
	QuantityMilli       int64   `json:"-"`
	Quantity            string  `json:"quantity" gorm:"-"`
	PurchasePriceCents  int64   `json:"-"`
	PurchasePrice       string  `json:"purchasePrice" gorm:"-"`
	SalePriceCents      int64   `json:"-"`
	SalePrice           string  `json:"salePrice" gorm:"-"`
	PurchaseAmountCents int64   `json:"-"`
	PurchaseAmount      string  `json:"purchaseAmount" gorm:"-"`
	SaleAmountCents     int64   `json:"-"`
	SaleAmount          string  `json:"saleAmount" gorm:"-"`
	Remark              *string `json:"remark"`
}

func (Line) TableName() string { return "direct_order_item" }

type LineInput struct {
	ProductID     int64   `json:"productId"`
	ProductType   string  `json:"productType"`
	Unit          string  `json:"unit"`
	Quantity      string  `json:"quantity"`
	PurchasePrice string  `json:"purchasePrice"`
	SalePrice     string  `json:"salePrice"`
	Remark        *string `json:"remark"`
}
type Input struct {
	RequestKey      *string     `json:"requestKey"`
	SupplierID      int64       `json:"supplierId"`
	CustomerID      int64       `json:"customerId"`
	BusinessDate    string      `json:"businessDate"`
	DeliveryContact *string     `json:"deliveryContact"`
	DeliveryPhone   *string     `json:"deliveryPhone"`
	DeliveryAddress *string     `json:"deliveryAddress"`
	Remark          *string     `json:"remark"`
	Items           []LineInput `json:"items"`
}
type EditInput struct {
	Version int64 `json:"version"`
	Input
}
type Query struct {
	Page, PageSize                               int
	DocumentNo, Status, BusinessFrom, BusinessTo string
	SupplierID, CustomerID                       int64
}
type Page struct {
	Records  []Draft `json:"records"`
	Total    int64   `json:"total"`
	Page     int     `json:"page"`
	PageSize int     `json:"pageSize"`
}

type PostInput struct {
	Version int64 `json:"version"`
}
type CancelInput struct {
	Version int64  `json:"version"`
	Reason  string `json:"reason"`
}
