package directdelivery

import (
	"fmt"
	"gorm.io/gorm"
	"math"
	"math/big"
	"strings"
	"time"
)

// Trace preserves each side's own item identity, price and money; it does not
// allocate refunds, combine amounts or infer that both documents took effect.
type Trace struct {
	Purchase        TraceDocument   `json:"purchase"`
	Sales           []TraceDocument `json:"sales"`
	PurchaseReturns []TraceDocument `json:"purchaseReturns"`
	SaleReturns     []TraceDocument `json:"saleReturns"`
}
type TraceDocument struct {
	DocumentRef
	Kind          string      `json:"kind"`
	OriginalID    *int64      `json:"originalId"`
	PartnerID     int64       `json:"partnerId"`
	BusinessDate  string      `json:"businessDate"`
	CreatedByName string      `json:"createdByName"`
	CreateTime    time.Time   `json:"createTime"`
	Items         []TraceItem `json:"items" gorm:"-"`
	TotalAmount   string      `json:"totalAmount" gorm:"-"`
}
type TraceItem struct {
	ID             int64  `json:"id"`
	OriginalItemID *int64 `json:"originalItemId"`
	ProductID      int64  `json:"productId"`
	ProductCode    string `json:"productCode"`
	ProductName    string `json:"productName"`
	Unit           string `json:"unit"`
	QuantityMilli  int64  `json:"-"`
	Quantity       string `json:"quantity" gorm:"-"`
	UnitPriceCents int64  `json:"-"`
	UnitPrice      string `json:"unitPrice" gorm:"-"`
	AmountCents    int64  `json:"-"`
	Amount         string `json:"amount" gorm:"-"`
}

func PurchaseTrace(db *gorm.DB, id int64) (*Trace, error) {
	var p Purchase
	if e := db.Table("purchase_document").Select("id,direct_delivery,status").Where("id=?", id).Take(&p).Error; e != nil {
		return nil, e
	}
	var history int64
	if !p.DirectDelivery {
		if e := db.Table("sale_document").Where("direct_purchase_id=?", id).Count(&history).Error; e != nil {
			return nil, e
		}
		if history == 0 {
			return nil, nil
		}
	}
	purchases, e := traceDocs(db, "PURCHASE", "d.id=?", id)
	if e != nil {
		return nil, e
	}
	out := &Trace{Purchase: purchases[0], Sales: []TraceDocument{}, PurchaseReturns: []TraceDocument{}, SaleReturns: []TraceDocument{}}
	if out.Sales, e = traceDocs(db, "SALE", "d.direct_purchase_id=?", id); e != nil {
		return nil, e
	}
	if out.PurchaseReturns, e = traceDocs(db, "PURCHASE_RETURN", "d.purchase_id=?", id); e != nil {
		return nil, e
	}
	if out.SaleReturns, e = traceDocs(db, "SALE_RETURN", "d.sale_id IN (SELECT id FROM sale_document WHERE direct_purchase_id=?)", id); e != nil {
		return nil, e
	}
	return out, nil
}
func SaleTrace(db *gorm.DB, id int64) (*Trace, error) {
	var h struct{ DirectPurchaseID *int64 }
	if e := db.Table("sale_document").Select("direct_purchase_id").Where("id=?", id).Take(&h).Error; e != nil {
		return nil, e
	}
	if h.DirectPurchaseID == nil {
		return nil, nil
	}
	return PurchaseTrace(db, *h.DirectPurchaseID)
}
func traceDocs(db *gorm.DB, kind, condition string, args ...any) ([]TraceDocument, error) {
	table := map[string]string{"PURCHASE": "purchase_document", "SALE": "sale_document", "PURCHASE_RETURN": "purchase_return_document", "SALE_RETURN": "sale_return_document"}[kind]
	original := "NULL"
	originItem := "NULL"
	if kind == "PURCHASE_RETURN" {
		original = "d.purchase_id"
		originItem = "purchase_item_id"
	}
	if kind == "SALE_RETURN" {
		original = "d.sale_id"
		originItem = "sale_item_id"
	}
	name := "p.name"
	if kind == "SALE" {
		name = "CASE WHEN d.posted_at IS NOT NULL THEN d.partner_name ELSE p.name END"
	}
	out := []TraceDocument{}
	e := db.Table(table+" d").Select("d.id,d.document_no,d.status,d.partner_id,"+name+" AS partner_name,CAST(d.business_date AS TEXT) AS business_date,d.create_time,d.posted_at,d.cancelled_at,d.cancel_reason,"+original+" AS original_id,COALESCE(NULLIF(u.nickname,''),u.username) AS posted_by_name,COALESCE(NULLIF(c.nickname,''),c.username) AS cancelled_by_name,COALESCE(NULLIF(cr.nickname,''),cr.username) AS created_by_name").Joins("JOIN partner p ON p.id=d.partner_id").Joins("LEFT JOIN sys_user u ON u.id=d.posted_by").Joins("LEFT JOIN sys_user c ON c.id=d.cancelled_by").Joins("LEFT JOIN sys_user cr ON cr.id=d.created_by").Where(condition, args...).Order("d.id").Scan(&out).Error
	if e != nil {
		return nil, e
	}
	for i := range out {
		d := &out[i]
		d.Kind = kind
		d.Items = []TraceItem{}
		if e := db.Table(table+"_item").Select("id,"+originItem+" AS original_item_id,product_id,product_code,product_name,unit,quantity_milli,unit_price_cents,amount_cents").Where("document_id=?", d.ID).Order("id").Scan(&d.Items).Error; e != nil {
			return nil, e
		}
		var total int64
		for j := range d.Items {
			l := &d.Items[j]
			if d.Status == "DRAFT" && l.OriginalItemID != nil {
				var prior struct{ Qty, Amount int64 }
				if e := db.Table(table+"_item i").Select("COALESCE(SUM(i.quantity_milli),0) AS qty,COALESCE(SUM(i.amount_cents),0) AS amount").Joins("JOIN "+table+" r ON r.id=i.document_id").Where("i."+originItem+"=? AND r.status='POSTED'", *l.OriginalItemID).Scan(&prior).Error; e != nil {
					return nil, e
				}
				n := new(big.Int).Add(big.NewInt(prior.Qty), big.NewInt(l.QuantityMilli))
				n.Mul(n, big.NewInt(l.UnitPriceCents))
				n.Add(n, big.NewInt(500))
				n.Quo(n, big.NewInt(1000))
				n.Sub(n, big.NewInt(prior.Amount))
				if !n.IsInt64() {
					return nil, fmt.Errorf("退货金额超出范围")
				}
				l.AmountCents = n.Int64()
				if l.AmountCents < 0 {
					l.AmountCents = 0
				}
			}
			if total > math.MaxInt64-l.AmountCents {
				return nil, fmt.Errorf("单据金额超出范围")
			}
			total += l.AmountCents
			l.Quantity = quantityText(l.QuantityMilli)
			l.UnitPrice = money(l.UnitPriceCents)
			l.Amount = money(l.AmountCents)
		}
		d.TotalAmount = money(total)
	}
	return out, nil
}
func money(n int64) string { return fmt.Sprintf("%d.%02d", n/100, n%100) }
func quantityText(n int64) string {
	if n%1000 == 0 {
		return fmt.Sprint(n / 1000)
	}
	return fmt.Sprintf("%d.%s", n/1000, strings.TrimRight(fmt.Sprintf("%03d", n%1000), "0"))
}
