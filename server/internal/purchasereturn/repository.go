package purchasereturn

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/EziosWJ/simple-inventory/server/internal/audit"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db} }
func (r *Repository) Create(ctx context.Context, h Document, items []Item, event audit.Event) (Document, error) {
	var out Document
	now := time.Now().UTC()
	var b [10]byte
	if _, e := rand.Read(b[:]); e != nil {
		return out, e
	}
	h.DocumentNo = "PR" + now.Format("20060102") + "-" + hex.EncodeToString(b[:])
	h.CreateTime = now
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := validateSource(tx, h.PurchaseID, items); e != nil {
			return e
		}
		if e := tx.Table("purchase_document").Select("partner_id").Where("id=? AND status='POSTED'", h.PurchaseID).Scan(&h.PartnerID).Error; e != nil {
			return e
		}
		if e := tx.Create(&h).Error; e != nil {
			return e
		}
		for i := range items {
			items[i].ID = 0
			items[i].DocumentID = h.ID
			if e := tx.Create(&items[i]).Error; e != nil {
				return e
			}
		}
		event.ResourceID = h.ID
		if e := audit.RecordOn(ctx, tx, event); e != nil {
			return e
		}
		v, e := find(tx, h.ID)
		if e == nil {
			out = *v
		}
		return e
	})
	if e != nil {
		return out, mapErr(e)
	}
	return out, nil
}
func (r *Repository) Edit(ctx context.Context, id, version int64, h Document, items []Item, event audit.Event) (Document, error) {
	var out Document
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var old Document
		q := tx.Where("id=?", id)
		if tx.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if e := q.Take(&old).Error; e != nil {
			return e
		}
		if old.Status != "DRAFT" || old.Version != version {
			return ErrConflict
		}
		if e := validateSource(tx, h.PurchaseID, items); e != nil {
			return e
		}
		var partnerID int64
		if e := tx.Table("purchase_document").Select("partner_id").Where("id=? AND status='POSTED'", h.PurchaseID).Scan(&partnerID).Error; e != nil {
			return e
		}
		res := tx.Model(&Document{}).Where("id=? AND status='DRAFT' AND version=?", id, version).Updates(map[string]any{"purchase_id": h.PurchaseID, "partner_id": partnerID, "business_date": h.BusinessDate, "remark": h.Remark, "version": version + 1})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrConflict
		}
		if e := tx.Where("document_id=?", id).Delete(&Item{}).Error; e != nil {
			return e
		}
		for i := range items {
			items[i].ID = 0
			items[i].DocumentID = id
			if e := tx.Create(&items[i]).Error; e != nil {
				return e
			}
		}
		event.ResourceID = id
		if e := audit.RecordOn(ctx, tx, event); e != nil {
			return e
		}
		v, e := find(tx, id)
		if e == nil {
			out = *v
		}
		return e
	})
	if e != nil {
		return out, mapErr(e)
	}
	return out, nil
}
func (r *Repository) Cancel(ctx context.Context, id, version int64, reason string, event audit.Event) (Document, error) {
	var out Document
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var old Document
		q := tx.Where("id=?", id)
		if tx.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if e := q.Take(&old).Error; e != nil {
			return e
		}
		if old.Version != version || old.Status != "DRAFT" {
			return ErrConflict
		}
		now := time.Now().UTC()
		res := tx.Model(&Document{}).Where("id=? AND status='DRAFT' AND version=?", id, version).Updates(map[string]any{"status": "CANCELLED", "version": version + 1, "cancelled_by": event.Metadata.ActorID, "cancelled_at": now, "cancel_reason": reason})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrConflict
		}
		event.ResourceID = id
		if e := audit.RecordOn(ctx, tx, event); e != nil {
			return e
		}
		v, e := find(tx, id)
		if e == nil {
			out = *v
		}
		return e
	})
	if e != nil {
		return out, mapErr(e)
	}
	return out, nil
}
func validateSource(tx *gorm.DB, purchaseID int64, items []Item) error {
	var p struct {
		ID     int64
		Status string
	}
	origin := tx.Table("purchase_document").Select("id,status").Where("id=?", purchaseID)
	if tx.Dialector.Name() == "postgres" {
		origin = origin.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if e := origin.Take(&p).Error; e != nil || p.Status != "POSTED" {
		return fmt.Errorf("%w：只能从已过账采购单建立退货", ErrInvalid)
	}
	for i := range items {
		var src struct {
			ID, ProductID, QuantityMilli, UnitPriceCents int64
			ProductCode, ProductName                     string
			ProductModel, ProductSpecification           *string
			ProductType, Unit                            string
			Remark                                       *string
		}
		if e := tx.Table("purchase_document_item").Where("id=? AND document_id=?", items[i].PurchaseItemID, purchaseID).Take(&src).Error; e != nil {
			return fmt.Errorf("%w：第%d行原采购明细不属于所选已过账采购单", ErrInvalid, i+1)
		}
		if src.ProductType != "GOODS" {
			return fmt.Errorf("%w：采购服务明细不能退货", ErrInvalid)
		}
		items[i].ProductID = src.ProductID
		items[i].ProductCode = src.ProductCode
		items[i].ProductName = src.ProductName
		items[i].ProductModel = src.ProductModel
		items[i].ProductSpecification = src.ProductSpecification
		items[i].Unit = src.Unit
		items[i].UnitPriceCents = src.UnitPriceCents
	}
	return nil
}
func (r *Repository) Find(ctx context.Context, id int64) (*Document, error) {
	var out *Document
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { var x error; out, x = find(tx, id); return x })
	return out, mapErr(e)
}
func (r *Repository) Page(ctx context.Context, q Query) (Page, error) {
	p := Page{Records: []Document{}, Page: q.Page, PageSize: q.PageSize}
	d := r.db.WithContext(ctx).Model(&Document{})
	if q.DocumentNo != "" {
		s := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(q.DocumentNo)
		d = d.Where("document_no LIKE ? ESCAPE '\\'", "%"+s+"%")
	}
	if q.Status != "" {
		d = d.Where("status=?", q.Status)
	}
	if q.PurchaseID > 0 {
		d = d.Where("purchase_id=?", q.PurchaseID)
	}
	if q.PartnerID > 0 {
		d = d.Where("partner_id=?", q.PartnerID)
	}
	if q.BusinessFrom != "" {
		d = d.Where("business_date>=?", q.BusinessFrom)
	}
	if q.BusinessTo != "" {
		d = d.Where("business_date<=?", q.BusinessTo)
	}
	if e := d.Count(&p.Total).Error; e != nil {
		return p, e
	}
	var ids []int64
	if e := d.Select("id").Order("id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Scan(&ids).Error; e != nil {
		return p, e
	}
	for _, id := range ids {
		v, e := find(r.db.WithContext(ctx), id)
		if e != nil {
			return p, e
		}
		p.Records = append(p.Records, *v)
	}
	return p, nil
}
func find(db *gorm.DB, id int64) (*Document, error) {
	var h Document
	e := db.Table("purchase_return_document d").Select("d.*,pd.document_no AS purchase_no,p.name AS partner_name,COALESCE(NULLIF(c.nickname,''),c.username) AS created_by_name,COALESCE(NULLIF(u.nickname,''),u.username) AS cancelled_by_name").Joins("JOIN purchase_document pd ON pd.id=d.purchase_id").Joins("JOIN partner p ON p.id=d.partner_id").Joins("LEFT JOIN sys_user c ON c.id=d.created_by").Joins("LEFT JOIN sys_user u ON u.id=d.cancelled_by").Where("d.id=?", id).Take(&h).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if e != nil {
		return nil, e
	}
	var items []Item
	e = db.Table("purchase_return_document_item i").Select("i.*").Where("i.document_id=?", id).Joins("JOIN purchase_document_item pi ON pi.id=i.purchase_item_id").Order("i.id").Find(&items).Error
	if e != nil {
		return nil, e
	}
	total := int64(0)
	for i := range items {
		l := &items[i]
		l.OriginalQuantityMilli = 0
		var original int64
		db.Table("purchase_document_item").Select("quantity_milli").Where("id=?", l.PurchaseItemID).Scan(&original)
		l.OriginalQuantityMilli = original
		l.OriginalQuantity = milliText(original)
		var sums struct{ Qty, Amount int64 }
		if e = db.Table("purchase_return_document_item i").Select("COALESCE(SUM(i.quantity_milli),0) AS qty,COALESCE(SUM(i.amount_cents),0) AS amount").Joins("JOIN purchase_return_document d ON d.id=i.document_id").Where("i.purchase_item_id=? AND d.status='POSTED' AND d.id<>?", l.PurchaseItemID, id).Scan(&sums).Error; e != nil {
			return nil, e
		}
		l.ReturnedQuantityMilli = sums.Qty
		l.ReturnedQuantity = milliText(sums.Qty)
		l.RemainingQuantityMilli = original - sums.Qty
		if l.RemainingQuantityMilli < 0 {
			l.RemainingQuantityMilli = 0
		}
		l.RemainingQuantity = milliText(l.RemainingQuantityMilli)
		l.Quantity = milliText(l.QuantityMilli)
		l.UnitPrice = moneyText(l.UnitPriceCents)
		l.PriorAmount = moneyText(sums.Amount)
		if sums.Qty > int64(^uint64(0)>>1)-l.QuantityMilli {
			return nil, ErrInvalid
		}
		target, e := roundAmount(sums.Qty+l.QuantityMilli, l.UnitPriceCents)
		if e != nil {
			return nil, e
		}
		l.AmountCents = target - sums.Amount
		if l.AmountCents < 0 {
			l.AmountCents = 0
		}
		l.Amount = moneyText(l.AmountCents)
		if total > int64(^uint64(0)>>1)-l.AmountCents {
			return nil, ErrInvalid
		}
		total += l.AmountCents
	}
	h.Items = items
	h.TotalAmount = moneyText(total)
	return &h, nil
}
func mapErr(e error) error {
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if e != nil && strings.Contains(strings.ToLower(e.Error()), "unique constraint") {
		return ErrConflict
	}
	return e
}

func (r *Repository) Source(ctx context.Context, purchaseID int64) (*Document, error) {
	var out Document
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var h struct {
			ID, PartnerID                    int64
			DocumentNo, BusinessDate, Status string
		}
		if e := tx.Table("purchase_document").Select("id,partner_id,document_no,business_date,status").Where("id=?", purchaseID).Take(&h).Error; e != nil {
			return e
		}
		if h.Status != "POSTED" {
			return ErrInvalid
		}
		out = Document{PurchaseID: h.ID, PurchaseNo: h.DocumentNo, PartnerID: h.PartnerID, BusinessDate: time.Now().UTC().Format("2006-01-02"), Status: "DRAFT", Version: 1, Items: []Item{}}
		tx.Table("partner").Select("name").Where("id=?", h.PartnerID).Scan(&out.PartnerName)
		var src []struct {
			ID, ProductID, QuantityMilli, UnitPriceCents, AmountCents int64
			ProductCode, ProductName                                  string
			ProductModel, ProductSpecification                        *string
			ProductType, Unit                                         string
			Remark                                                    *string
		}
		if e := tx.Table("purchase_document_item").Where("document_id=? AND product_type='GOODS'", purchaseID).Order("id").Find(&src).Error; e != nil {
			return e
		}
		for _, v := range src {
			l := Item{PurchaseItemID: v.ID, ProductID: v.ProductID, ProductCode: v.ProductCode, ProductName: v.ProductName, ProductModel: v.ProductModel, ProductSpecification: v.ProductSpecification, Unit: v.Unit, OriginalQuantityMilli: v.QuantityMilli, OriginalQuantity: milliText(v.QuantityMilli), UnitPriceCents: v.UnitPriceCents, UnitPrice: moneyText(v.UnitPriceCents), AmountCents: v.AmountCents, Amount: moneyText(v.AmountCents), ReturnedQuantity: "0", RemainingQuantityMilli: v.QuantityMilli, RemainingQuantity: milliText(v.QuantityMilli), QuantityMilli: 0, Quantity: "0", Remark: v.Remark}
			var sums struct{ Qty, Amount int64 }
			if e := tx.Table("purchase_return_document_item i").Select("COALESCE(SUM(i.quantity_milli),0) AS qty,COALESCE(SUM(i.amount_cents),0) AS amount").Joins("JOIN purchase_return_document d ON d.id=i.document_id").Where("i.purchase_item_id=? AND d.status='POSTED'", v.ID).Scan(&sums).Error; e != nil {
				return e
			}
			l.ReturnedQuantityMilli = sums.Qty
			l.ReturnedQuantity = milliText(sums.Qty)
			l.RemainingQuantityMilli = v.QuantityMilli - sums.Qty
			if l.RemainingQuantityMilli < 0 {
				l.RemainingQuantityMilli = 0
			}
			l.RemainingQuantity = milliText(l.RemainingQuantityMilli)
			l.PriorAmount = moneyText(sums.Amount)
			if l.RemainingQuantityMilli > 0 {
				out.Items = append(out.Items, l)
			}
		}
		out.TotalAmount = "0.00"
		if len(out.Items) == 0 {
			return ErrConflict
		}
		return nil
	})
	if e != nil {
		return nil, mapErr(e)
	}
	return &out, nil
}
