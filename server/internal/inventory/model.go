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
	ID                   int64   `json:"id"`
	ProductID            int64   `json:"productId"`
	AdjustmentID         int64   `json:"adjustmentId"`
	AdjustmentItemID     int64   `json:"adjustmentItemId"`
	EntryType            string  `json:"entryType"`
	QuantityMilli        int64   `json:"-"`
	Quantity             string  `json:"quantity" gorm:"-"`
	BalanceBeforeMilli   int64   `json:"-"`
	BalanceBefore        string  `json:"balanceBefore" gorm:"-"`
	BalanceAfterMilli    int64   `json:"-"`
	BalanceAfter         string  `json:"balanceAfter" gorm:"-"`
	Reason               string  `json:"reason"`
	Remark               *string `json:"remark,omitempty"`
	ProductCode          string  `json:"productCode"`
	ProductName          string  `json:"productName"`
	ProductModel         *string `json:"productModel,omitempty"`
	ProductSpecification *string `json:"productSpecification,omitempty"`
	Unit                 string  `json:"unit"`
	OperatorID           int64   `json:"operatorId"`
	// OperatorName and DocumentNo are read-only joins, so a ledger row is
	// traceable back to its source document and its actual operator.
	OperatorName string    `json:"operatorName,omitempty" gorm:"->;-:migration"`
	DocumentNo   string    `json:"documentNo,omitempty" gorm:"->;-:migration"`
	OccurredAt   time.Time `json:"occurredAt"`
	CreateTime   time.Time `json:"createTime" gorm:"autoCreateTime"`
}

func (Entry) TableName() string { return "inventory_entry" }

// present fills the decimal text of one ledger row. Quantities never pass
// through float, so a stored thousandth is reported exactly.
func (e *Entry) present() {
	e.Quantity = quantityText(e.QuantityMilli)
	e.BalanceBefore = quantityText(e.BalanceBeforeMilli)
	e.BalanceAfter = quantityText(e.BalanceAfterMilli)
}

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

// Balance is one row of the current-stock view. It reports the live catalog
// description, not a posting snapshot: renaming a product changes this list
// while the stored quantity stays. Stock is exposed as a decimal string for the
// same reason document quantities are.
type Balance struct {
	ProductID     int64   `json:"productId"`
	Code          string  `json:"code"`
	Name          string  `json:"name"`
	Model         *string `json:"model"`
	Specification *string `json:"specification"`
	Category      *string `json:"category"`
	Unit          string  `json:"unit"`
	Status        int     `json:"status"`
	QuantityMilli int64   `json:"-"`
	Quantity      string  `json:"quantity" gorm:"-"`
}

// EntryQuery filters the immutable ledger. EntryType selects original postings
// or cancellation reversals; times compare the stored occurrence instant.
type EntryQuery struct {
	Page, PageSize           int
	ProductID                int64
	EntryType                string
	OccurredFrom, OccurredTo *time.Time
}
type EntryPage struct {
	Records  []Entry `json:"records"`
	Total    int64   `json:"total"`
	Page     int     `json:"page"`
	PageSize int     `json:"pageSize"`
}

// BalanceQuery filters the current-stock view. Stock selects the non-zero view
// by default; "all" also lists products that never had a posting, "zero" lists
// exactly the products whose stock is zero or absent.
type BalanceQuery struct {
	Page, PageSize           int
	Keyword, Category, Stock string
	Status                   *int
}
type BalancePage struct {
	Records  []Balance `json:"records"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"pageSize"`
}
