package directorder

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/EziosWJ/simple-inventory/server/internal/audit"
	"github.com/EziosWJ/simple-inventory/server/internal/auth"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"strings"
	"time"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db} }
func (r *Repository) Create(ctx context.Context, h Draft, lines []Line, event audit.Event) (Draft, error) {
	var out Draft
	h.CreateTime = time.Now().UTC()
	var nonce [10]byte
	if _, e := rand.Read(nonce[:]); e != nil {
		return out, e
	}
	h.DocumentNo = "DD" + h.CreateTime.Format("20060102") + "-" + hex.EncodeToString(nonce[:])
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if replay, e := beginSave(tx, event.Metadata.ActorID, h.SaveRequest); e != nil {
			return e
		} else if replay != nil {
			out = *replay
			return nil
		}
		if e := validMaterials(tx, h, lines); e != nil {
			return e
		}
		var customer struct{ Contact, Phone, Address *string }
		if e := tx.Table("partner").Select("contact,phone,address").Where("id=?", h.CustomerID).Take(&customer).Error; e != nil {
			return e
		}
		if h.DeliveryContact == nil {
			h.DeliveryContact = customer.Contact
		}
		if h.DeliveryPhone == nil {
			h.DeliveryPhone = customer.Phone
		}
		if h.DeliveryAddress == nil {
			h.DeliveryAddress = customer.Address
		}
		if e := tx.Create(&h).Error; e != nil {
			return e
		}
		if e := insertLines(tx, h.ID, lines); e != nil {
			return e
		}
		event.ResourceID = h.ID
		if e := audit.RecordOn(ctx, tx, event); e != nil {
			return e
		}
		v, e := find(tx, h.ID)
		if e != nil {
			return e
		}
		out = *v
		return finishSave(tx, event.Metadata.ActorID, h.SaveRequest, &out)
	})
	return out, mapErr(e)
}
func (r *Repository) Edit(ctx context.Context, id, version int64, h Draft, lines []Line, event audit.Event) (Draft, error) {
	var out Draft
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if replay, e := beginSave(tx, event.Metadata.ActorID, h.SaveRequest); e != nil {
			return e
		} else if replay != nil {
			out = *replay
			return nil
		}
		old, e := lockDocument(tx, id)
		if e != nil {
			return e
		}
		if old.Status != "DRAFT" || old.Version != version {
			return ErrConflict
		}
		if e := validMaterials(tx, h, lines); e != nil {
			return e
		}
		// Omitted edit fields retain saved delivery data; explicit empty values clear it.
		if h.DeliveryContact == nil {
			h.DeliveryContact = old.DeliveryContact
		}
		if h.DeliveryPhone == nil {
			h.DeliveryPhone = old.DeliveryPhone
		}
		if h.DeliveryAddress == nil {
			h.DeliveryAddress = old.DeliveryAddress
		}
		res := tx.Model(&Draft{}).Where("id=? AND status='DRAFT' AND version=?", id, version).Updates(map[string]any{"supplier_id": h.SupplierID, "customer_id": h.CustomerID, "business_date": h.BusinessDate, "remark": h.Remark, "delivery_contact": h.DeliveryContact, "delivery_phone": h.DeliveryPhone, "delivery_address": h.DeliveryAddress, "version": version + 1})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrConflict
		}
		if e := tx.Where("document_id=?", id).Delete(&Line{}).Error; e != nil {
			return e
		}
		if e := insertLines(tx, id, lines); e != nil {
			return e
		}
		if e := audit.RecordOn(ctx, tx, event); e != nil {
			return e
		}
		v, e := find(tx, id)
		if e != nil {
			return e
		}
		out = *v
		return finishSave(tx, event.Metadata.ActorID, h.SaveRequest, &out)
	})
	return out, mapErr(e)
}
func insertLines(tx *gorm.DB, id int64, lines []Line) error {
	for i := range lines {
		lines[i].ID = 0
		lines[i].DocumentID = id
		if e := tx.Create(&lines[i]).Error; e != nil {
			return e
		}
	}
	return nil
}
func lockDocument(tx *gorm.DB, id int64) (Draft, error) {
	var h Draft
	if tx.Dialector.Name() == "sqlite" {
		if e := tx.Exec("UPDATE direct_order_document SET version=version WHERE id=?", id).Error; e != nil {
			return h, e
		}
	}
	q := tx.Where("id=?", id)
	if tx.Dialector.Name() == "postgres" {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	e := q.Take(&h).Error
	return h, e
}
func validMaterials(tx *gorm.DB, h Draft, lines []Line) error {
	for _, x := range []struct {
		ID     int64
		Column string
	}{{h.SupplierID, "is_supplier"}, {h.CustomerID, "is_customer"}} {
		var n int64
		if e := tx.Table("partner").Where("id=? AND status=1 AND "+x.Column+"=?", x.ID, true).Count(&n).Error; e != nil {
			return e
		}
		if n != 1 {
			return fmt.Errorf("%w：客户或供应商已停用或身份已变化", ErrInvalid)
		}
	}
	for i, l := range lines {
		var n int64
		if e := tx.Table("product").Where("id=? AND status=1 AND type=? AND unit=?", l.ProductID, l.ProductType, l.Unit).Count(&n).Error; e != nil {
			return e
		}
		if n != 1 {
			return fmt.Errorf("%w：第%d行商品已停用或类型/单位已变化", ErrInvalid, i+1)
		}
	}
	return nil
}
func find(db *gorm.DB, id int64) (*Draft, error) {
	var h Draft
	if e := db.Table("direct_order_document d").Select("d.*,CASE WHEN d.posted_at IS NOT NULL THEN d.supplier_snapshot ELSE s.name END AS supplier_name,CASE WHEN d.posted_at IS NOT NULL THEN d.customer_snapshot ELSE c.name END AS customer_name,COALESCE(NULLIF(u.nickname,''),u.username) AS created_by_name,COALESCE(NULLIF(pp.nickname,''),pp.username) AS posted_by_name,COALESCE(NULLIF(cc.nickname,''),cc.username) AS cancelled_by_name").Joins("JOIN partner s ON s.id=d.supplier_id").Joins("JOIN partner c ON c.id=d.customer_id").Joins("JOIN sys_user u ON u.id=d.created_by").Joins("LEFT JOIN sys_user pp ON pp.id=d.posted_by").Joins("LEFT JOIN sys_user cc ON cc.id=d.cancelled_by").Where("d.id=?", id).Take(&h).Error; e != nil {
		return nil, e
	}
	h.Items = []Line{}
	if e := db.Table("direct_order_item i").Select("i.*,CASE WHEN d.posted_at IS NOT NULL THEN i.product_code ELSE p.code END AS product_code,CASE WHEN d.posted_at IS NOT NULL THEN i.product_name ELSE p.name END AS product_name").Joins("JOIN product p ON p.id=i.product_id").Joins("JOIN direct_order_document d ON d.id=i.document_id").Where("i.document_id=?", id).Order("i.id").Find(&h.Items).Error; e != nil {
		return nil, e
	}
	var purchaseTotal, saleTotal int64
	for i := range h.Items {
		l := &h.Items[i]
		l.Quantity = milliText(l.QuantityMilli)
		l.PurchasePrice = moneyText(l.PurchasePriceCents)
		l.SalePrice = moneyText(l.SalePriceCents)
		l.PurchaseAmount = moneyText(l.PurchaseAmountCents)
		l.SaleAmount = moneyText(l.SaleAmountCents)
		purchaseTotal += l.PurchaseAmountCents
		saleTotal += l.SaleAmountCents
	}
	h.ActiveSaleReturns = []int64{}
	h.ActivePurchaseReturns = []int64{}
	if h.SaleID != nil {
		if e := db.Table("sale_return_document").Where("sale_id=? AND status='POSTED'", *h.SaleID).Order("id").Pluck("id", &h.ActiveSaleReturns).Error; e != nil {
			return nil, e
		}
	}
	if h.PurchaseID != nil {
		if e := db.Table("purchase_return_document").Where("purchase_id=? AND status='POSTED'", *h.PurchaseID).Order("id").Pluck("id", &h.ActivePurchaseReturns).Error; e != nil {
			return nil, e
		}
	}
	h.PurchaseTotal = moneyText(purchaseTotal)
	h.SaleTotal = moneyText(saleTotal)
	return &h, nil
}
func (r *Repository) Find(ctx context.Context, id int64) (*Draft, error) {
	var out *Draft
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { var e error; out, e = find(tx, id); return e })
	return out, mapErr(e)
}
func (r *Repository) Page(ctx context.Context, q Query) (Page, error) {
	out := Page{Records: []Draft{}, Page: q.Page, PageSize: q.PageSize}
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		d := tx.Model(&Draft{})
		if q.DocumentNo != "" {
			d = d.Where("document_no LIKE ?", "%"+q.DocumentNo+"%")
		}
		if q.Status != "" {
			d = d.Where("status=?", q.Status)
		}
		if q.SupplierID > 0 {
			d = d.Where("supplier_id=?", q.SupplierID)
		}
		if q.CustomerID > 0 {
			d = d.Where("customer_id=?", q.CustomerID)
		}
		if q.BusinessFrom != "" {
			d = d.Where("business_date>=?", q.BusinessFrom)
		}
		if q.BusinessTo != "" {
			d = d.Where("business_date<=?", q.BusinessTo)
		}
		if e := d.Count(&out.Total).Error; e != nil {
			return e
		}
		var headers []Draft
		if e := d.Order("business_date DESC,id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&headers).Error; e != nil {
			return e
		}
		for _, h := range headers {
			v, e := find(tx, h.ID)
			if e != nil {
				return e
			}
			out.Records = append(out.Records, *v)
		}
		return nil
	})
	return out, mapErr(e)
}
func mapErr(e error) error {
	if e == nil {
		return nil
	}
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if strings.Contains(strings.ToLower(e.Error()), "database is locked") || strings.Contains(strings.ToLower(e.Error()), "database table is locked") {
		return fmt.Errorf("%w：数据库正在处理其他操作，请核实后重试", ErrConflict)
	}
	return e
}

func (r *Repository) Authorize(ctx context.Context) error {
	ok, e := auth.HasPermission(ctx, r.db, "business:direct-order")
	if e != nil {
		return e
	}
	if !ok {
		return ErrForbidden
	}
	return nil
}
