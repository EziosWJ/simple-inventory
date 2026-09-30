package inventory

import "time"

type Adjustment struct {
	ID              int64      `json:"id"`
	DocumentNo      string     `json:"documentNo"`
	Status          string     `json:"status"`
	Version         int64      `json:"version"`
	CreatedBy       int64      `json:"createdBy"`
	CreatedByName   string     `json:"createdByName,omitempty" gorm:"->;-:migration"`
	CreateTime      time.Time  `json:"createTime" gorm:"autoCreateTime"`
	CancelledBy     *int64     `json:"cancelledBy"`
	CancelledByName string     `json:"cancelledByName,omitempty" gorm:"->;-:migration"`
	CancelledAt     *time.Time `json:"cancelledAt"`
	CancelReason    *string    `json:"cancelReason"`
	Items           []Item     `json:"items,omitempty" gorm:"-"`
}

func (Adjustment) TableName() string { return "inventory_adjustment" }

type Item struct {
	ID                   int64   `json:"id"`
	AdjustmentID         int64   `json:"-"`
	ProductID            int64   `json:"productId"`
	ProductCode          string  `json:"productCode" gorm:"->;-:migration"`
	ProductName          string  `json:"productName" gorm:"->;-:migration"`
	ProductModel         *string `json:"productModel,omitempty" gorm:"->;-:migration"`
	ProductSpecification *string `json:"productSpecification,omitempty" gorm:"->;-:migration"`
	ProductType          string  `json:"productType"`
	Unit                 string  `json:"unit"`
	QuantityMilli        int64   `json:"-"`
	Quantity             string  `json:"quantity" gorm:"-"`
	Reason               string  `json:"reason"`
	Remark               *string `json:"remark,omitempty"`
}

func (Item) TableName() string { return "inventory_adjustment_item" }

type Input struct {
	Items []ItemInput `json:"items"`
}
type EditInput struct {
	Version int64       `json:"version"`
	Items   []ItemInput `json:"items"`
}
type CancelInput struct {
	Version int64  `json:"version"`
	Reason  string `json:"reason"`
}
type ItemInput struct {
	ProductID   int64   `json:"productId"`
	ProductType string  `json:"productType"`
	Unit        string  `json:"unit"`
	Quantity    string  `json:"quantity"`
	Reason      string  `json:"reason"`
	Remark      *string `json:"remark,omitempty"`
}
type Query struct {
	Page, PageSize         int
	DocumentNo, Status     string
	ProductID              int64
	CreatedFrom, CreatedTo *time.Time
}
type Page struct {
	Records  []Adjustment `json:"records"`
	Total    int64        `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"pageSize"`
}
