// Package directdelivery keeps purchase/sale association persistence local to repositories.
package directdelivery

import (
	"errors"
	"fmt"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"math"
	"time"
)

type Purchase struct {
	ID             int64
	DirectDelivery bool
	Status         string
}
type DocumentRef struct {
	ID              int64      `json:"id"`
	DocumentNo      string     `json:"documentNo"`
	Status          string     `json:"status"`
	PartnerName     string     `json:"partnerName"`
	PostedAt        *time.Time `json:"postedAt"`
	PostedByName    string     `json:"postedByName"`
	CancelledAt     *time.Time `json:"cancelledAt"`
	CancelledByName string     `json:"cancelledByName"`
	CancelReason    *string    `json:"cancelReason"`
}

// All association mutations lock purchase first, then sale, balances and products.
// SQLite serializes writes and optimistic document versions detect stale writers.
func LockPurchase(tx *gorm.DB, id int64, out any) error {
	if tx.Dialector.Name() == "sqlite" {
		if e := tx.Exec("UPDATE purchase_document SET version=version WHERE id=?", id).Error; e != nil {
			return e
		}
	}
	q := tx.Table("purchase_document").Where("id=?", id)
	if tx.Dialector.Name() == "postgres" {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	return q.Take(out).Error
}
func RejectActiveSale(tx *gorm.DB, id int64) error {
	var n int64
	if e := tx.Table("sale_document").Where("direct_purchase_id=? AND status<>'CANCELLED'", id).Count(&n).Error; e != nil {
		return e
	}
	if n > 0 {
		return errors.New("请先取消关联销售单，再取消采购或取消直送标记")
	}
	return nil
}
func PurchaseQuantities(tx *gorm.DB, id int64) (map[int64]int64, error) {
	var lines []struct{ ProductID, QuantityMilli int64 }
	if e := tx.Table("purchase_document_item").Select("product_id,quantity_milli").Where("document_id=?", id).Find(&lines).Error; e != nil {
		return nil, e
	}
	out := map[int64]int64{}
	for _, l := range lines {
		if out[l.ProductID] > math.MaxInt64-l.QuantityMilli {
			return nil, fmt.Errorf("采购汇总数量超出范围")
		}
		out[l.ProductID] += l.QuantityMilli
	}
	return out, nil
}
func refs(db *gorm.DB, table, condition string, args ...any) ([]DocumentRef, error) {
	out := []DocumentRef{}
	name := "p.name"
	if table == "sale_document" {
		name = "CASE WHEN d.posted_at IS NOT NULL THEN d.partner_name ELSE p.name END"
	}
	e := db.Table(table+" d").Select("d.id,d.document_no,d.status,"+name+" AS partner_name,d.posted_at,d.cancelled_at,d.cancel_reason,COALESCE(NULLIF(u.nickname,''),u.username) AS posted_by_name,COALESCE(NULLIF(c.nickname,''),c.username) AS cancelled_by_name").Joins("JOIN partner p ON p.id=d.partner_id").Joins("LEFT JOIN sys_user u ON u.id=d.posted_by").Joins("LEFT JOIN sys_user c ON c.id=d.cancelled_by").Where(condition, args...).Order("d.id").Scan(&out).Error
	return out, e
}
func PurchaseSales(db *gorm.DB, id int64) ([]DocumentRef, error) {
	return refs(db, "sale_document", "d.direct_purchase_id=?", id)
}
func SalePurchase(db *gorm.DB, id *int64) ([]DocumentRef, error) {
	if id == nil {
		return []DocumentRef{}, nil
	}
	return refs(db, "purchase_document", "d.id=?", *id)
}
