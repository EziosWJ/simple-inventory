package purchase

import (
	"database/sql/driver"
	"fmt"
	"github.com/EziosWJ/simple-inventory/server/internal/directdelivery"
	"time"
)

// BusinessDate is a civil calendar date, independent of a timezone.
type BusinessDate string

func (d *BusinessDate) Scan(value any) error {
	switch value := value.(type) {
	case time.Time:
		*d = BusinessDate(value.Format("2006-01-02"))
	case string:
		*d = BusinessDate(value)
	case []byte:
		*d = BusinessDate(string(value))
	case nil:
		*d = ""
	default:
		return fmt.Errorf("unsupported business date value %T", value)
	}
	return nil
}

func (d BusinessDate) Value() (driver.Value, error) { return string(d), nil }

type Draft struct {
	SaveReceipt     *SaveReceipt                 `json:"saveReceipt,omitempty" gorm:"-"`
	SaveRequest     *SaveRequest                 `json:"-" gorm:"-"`
	DirectTrace     *directdelivery.Trace        `json:"directTrace" gorm:"-"`
	DirectDelivery  bool                         `json:"directDelivery"`
	DirectDocuments []directdelivery.DocumentRef `json:"directDocuments" gorm:"-"`
	ID              int64                        `json:"id"`
	DocumentNo      string                       `json:"documentNo"`
	PartnerID       int64                        `json:"partnerId"`
	PartnerName     string                       `json:"partnerName" gorm:"->"`
	BusinessDate    BusinessDate                 `json:"businessDate"`
	Status          string                       `json:"status"`
	Version         int64                        `json:"version"`
	PostedBy        *int64                       `json:"postedBy"`
	PostedByName    string                       `json:"postedByName" gorm:"->"`
	PostedAt        *time.Time                   `json:"postedAt"`
	Remark          *string                      `json:"remark"`
	CreatedBy       int64                        `json:"createdBy"`
	CreatedByName   string                       `json:"createdByName" gorm:"->"`
	CancelledBy     *int64                       `json:"cancelledBy"`
	CancelledByName string                       `json:"cancelledByName" gorm:"->"`
	CancelReason    *string                      `json:"cancelReason"`
	CreateTime      time.Time                    `json:"createTime"`
	CancelledAt     *time.Time                   `json:"cancelledAt"`
	Items           []Line                       `json:"items" gorm:"-"`
	TotalAmount     string                       `json:"totalAmount" gorm:"-"`
}

func (Draft) TableName() string { return "purchase_document" }

type Line struct {
	ID                   int64   `json:"id"`
	DocumentID           int64   `json:"-" gorm:"column:document_id"`
	ProductID            int64   `json:"productId"`
	ProductCode          string  `json:"productCode"`
	ProductName          string  `json:"productName"`
	ProductModel         *string `json:"productModel"`
	ProductSpecification *string `json:"productSpecification"`
	ProductType          string  `json:"productType"`
	Unit                 string  `json:"unit"`
	QuantityMilli        int64   `json:"-" gorm:"column:quantity_milli"`
	Quantity             string  `json:"quantity" gorm:"-"`
	UnitPriceCents       int64   `json:"-" gorm:"column:unit_price_cents"`
	UnitPrice            string  `json:"unitPrice" gorm:"-"`
	AmountCents          int64   `json:"-" gorm:"column:amount_cents"`
	Amount               string  `json:"amount" gorm:"-"`
	Remark               *string `json:"remark"`
}

func (Line) TableName() string { return "purchase_document_item" }

type LineInput struct {
	ProductID   int64   `json:"productId"`
	ProductType string  `json:"productType"`
	Unit        string  `json:"unit"`
	Quantity    string  `json:"quantity"`
	UnitPrice   string  `json:"unitPrice"`
	Remark      *string `json:"remark"`
}
type Input struct {
	RequestKey     *string     `json:"requestKey,omitempty"`
	DirectDelivery bool        `json:"directDelivery"`
	PartnerID      int64       `json:"partnerId"`
	BusinessDate   string      `json:"businessDate"`
	Remark         *string     `json:"remark"`
	Items          []LineInput `json:"items"`
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
	PartnerID, ProductID     int64
	BusinessFrom, BusinessTo string
}
type Page struct {
	Records  []Draft `json:"records"`
	Total    int64   `json:"total"`
	Page     int     `json:"page"`
	PageSize int     `json:"pageSize"`
}
