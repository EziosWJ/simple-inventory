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
	PostedBy        *int64     `json:"postedBy"`
	PostedByName    string     `json:"postedByName,omitempty" gorm:"->;-:migration"`
	PostedAt        *time.Time `json:"postedAt"`
	CancelledBy     *int64     `json:"cancelledBy"`
	CancelledByName string     `json:"cancelledByName,omitempty" gorm:"->;-:migration"`
	CancelledAt     *time.Time `json:"cancelledAt"`
	CancelReason    *string    `json:"cancelReason"`
	Items           []Item     `json:"items,omitempty" gorm:"-"`
}

func (Adjustment) TableName() string { return "inventory_adjustment" }

// ProductCode/ProductName/ProductModel/ProductSpecification are stored copies of
// the confirmed product description. Posting refreshes them inside the posting
// transaction, so a posted document keeps the description it was posted with.
type Item struct {
	ID                   int64   `json:"id"`
	AdjustmentID         int64   `json:"-"`
	ProductID            int64   `json:"productId"`
	ProductCode          string  `json:"productCode"`
	ProductName          string  `json:"productName"`
	ProductModel         *string `json:"productModel,omitempty"`
	ProductSpecification *string `json:"productSpecification,omitempty"`
	ProductType          string  `json:"productType"`
	Unit                 string  `json:"unit"`
	QuantityMilli        int64   `json:"-"`
	Quantity             string  `json:"quantity" gorm:"-"`
	Reason               string  `json:"reason"`
	Remark               *string `json:"remark,omitempty"`
	// Balances are read back from the original ledger entry, so a posted
	// document reports the impact it actually had.
	BalanceBeforeMilli *int64  `json:"-" gorm:"column:balance_before_milli;->;-:migration"`
	BalanceAfterMilli  *int64  `json:"-" gorm:"column:balance_after_milli;->;-:migration"`
	BalanceBefore      *string `json:"balanceBefore,omitempty" gorm:"-"`
	BalanceAfter       *string `json:"balanceAfter,omitempty" gorm:"-"`
}

func (Item) TableName() string { return "inventory_adjustment_item" }

// Entry is one line of the append-only inventory ledger. It has no update or
// delete entry point: corrections happen through cancellation reversals.
type Entry struct {
	ID                   int64     `json:"id"`
	ProductID            int64     `json:"productId"`
	AdjustmentID         int64     `json:"adjustmentId"`
	AdjustmentItemID     int64     `json:"adjustmentItemId"`
	EntryType            string    `json:"entryType"`
	QuantityMilli        int64     `json:"-"`
	Quantity             string    `json:"quantity" gorm:"-"`
	BalanceBeforeMilli   int64     `json:"-"`
	BalanceBefore        string    `json:"balanceBefore" gorm:"-"`
	BalanceAfterMilli    int64     `json:"-"`
	BalanceAfter         string    `json:"balanceAfter" gorm:"-"`
	Reason               string    `json:"reason"`
	Remark               *string   `json:"remark,omitempty"`
	ProductCode          string    `json:"productCode"`
	ProductName          string    `json:"productName"`
	ProductModel         *string   `json:"productModel,omitempty"`
	ProductSpecification *string   `json:"productSpecification,omitempty"`
	Unit                 string    `json:"unit"`
	OperatorID           int64     `json:"operatorId"`
	OccurredAt           time.Time `json:"occurredAt"`
	CreateTime           time.Time `json:"createTime" gorm:"autoCreateTime"`
}

func (Entry) TableName() string { return "inventory_entry" }

type Input struct {
	Items []ItemInput `json:"items"`
}
type EditInput struct {
	Version int64       `json:"version"`
	Items   []ItemInput `json:"items"`
}
type PostInput struct {
	Version int64 `json:"version"`
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
