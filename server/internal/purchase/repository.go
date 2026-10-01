package purchase

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
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db} }
func (r *Repository) Create(ctx context.Context, h Draft, lines []Line, event audit.Event) (Draft, error) {
	var out Draft
	stamp := time.Now().UTC()
	var nonce [10]byte
	if _, e := rand.Read(nonce[:]); e != nil {
		return out, e
	}
	h.DocumentNo = "PI" + stamp.Format("20060102") + "-" + hex.EncodeToString(nonce[:])
	h.CreateTime = stamp
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := validPartnerProduct(tx, h.PartnerID, lines); e != nil {
			return e
		}
		if e := tx.Create(&h).Error; e != nil {
			return e
		}
		for i := range lines {
			lines[i].ID = 0
			lines[i].DocumentID = h.ID
			if e := tx.Create(&lines[i]).Error; e != nil {
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
func (r *Repository) Edit(ctx context.Context, id, version int64, h Draft, lines []Line, event audit.Event) (Draft, error) {
	var out Draft
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var old Draft
		if e := tx.Where("id=?", id).First(&old).Error; e != nil {
			return e
		}
		if old.Status != "DRAFT" || old.Version != version {
			return ErrConflict
		}
		if e := validPartnerProduct(tx, h.PartnerID, lines); e != nil {
			return e
		}
		res := tx.Model(&Draft{}).Where("id=? AND status='DRAFT' AND version=?", id, version).Updates(map[string]any{"partner_id": h.PartnerID, "business_date": h.BusinessDate, "remark": h.Remark, "version": version + 1})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrConflict
		}
		if e := tx.Where("document_id=?", id).Delete(&Line{}).Error; e != nil {
			return e
		}
		for i := range lines {
			lines[i].ID = 0
			lines[i].DocumentID = id
			if e := tx.Create(&lines[i]).Error; e != nil {
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
func (r *Repository) Cancel(ctx context.Context, id, version int64, reason string, event audit.Event) (Draft, error) {
	var out Draft
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		res := tx.Model(&Draft{}).Where("id=? AND status='DRAFT' AND version=?", id, version).Updates(map[string]any{"status": "CANCELLED", "version": version + 1, "cancelled_by": event.Metadata.ActorID, "cancelled_at": now, "cancel_reason": reason})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			var v Draft
			if x := tx.First(&v, id).Error; x != nil {
				return x
			}
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
func (r *Repository) Find(ctx context.Context, id int64) (*Draft, error) {
	var v *Draft
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { var x error; v, x = find(tx, id); return x })
	return v, mapErr(e)
}
func (r *Repository) Page(ctx context.Context, q Query) (Page, error) {
	p := Page{Records: []Draft{}, Page: q.Page, PageSize: q.PageSize}
	d := r.db.WithContext(ctx).Model(&Draft{})
	if q.DocumentNo != "" {
		s := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(q.DocumentNo)
		d = d.Where("document_no LIKE ? ESCAPE '\\'", "%"+s+"%")
	}
	if q.Status != "" {
		d = d.Where("status=?", q.Status)
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
	if q.ProductID > 0 {
		d = d.Where("EXISTS(SELECT 1 FROM purchase_document_item i WHERE i.document_id=purchase_document.id AND i.product_id=?)", q.ProductID)
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
func validPartnerProduct(tx *gorm.DB, partnerID int64, lines []Line) error {
	var n int64
	if e := tx.Table("partner").Where("id=? AND status=1 AND is_supplier=?", partnerID, true).Count(&n).Error; e != nil {
		return e
	}
	if n != 1 {
		return fmt.Errorf("%w：往来单位必须为启用供应商", ErrInvalid)
	}
	for i := range lines {
		l := &lines[i]
		var p struct {
			Code, Name           string
			Model, Specification *string
			Type, Unit           string
			Status               int
		}
		if e := tx.Table("product").Select("code,name,model,specification,type,unit,status").Where("id=?", l.ProductID).Take(&p).Error; e != nil {
			return fmt.Errorf("%w：商品不存在", ErrInvalid)
		}
		if p.Status != 1 || p.Type != "GOODS" {
			return fmt.Errorf("%w：采购仅支持启用实物商品", ErrInvalid)
		}
		if p.Type != l.ProductType || p.Unit != l.Unit {
			return fmt.Errorf("%w：商品类型或单位已变化，请重新确认", ErrInvalid)
		}
		l.ProductCode, l.ProductName, l.ProductModel, l.ProductSpecification = p.Code, p.Name, p.Model, p.Specification
	}
	return nil
}
func find(db *gorm.DB, id int64) (*Draft, error) {
	var h Draft
	e := db.Table("purchase_document d").Select("d.*,p.name AS partner_name,COALESCE(NULLIF(c.nickname,''),c.username) AS created_by_name,COALESCE(NULLIF(u.nickname,''),u.username) AS cancelled_by_name").Joins("JOIN partner p ON p.id=d.partner_id").Joins("LEFT JOIN sys_user c ON c.id=d.created_by").Joins("LEFT JOIN sys_user u ON u.id=d.cancelled_by").Where("d.id=?", id).Take(&h).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if e != nil {
		return nil, e
	}
	var lines []Line
	if e = db.Table("purchase_document_item i").Select("i.*").Where("i.document_id=?", id).Order("i.id ASC").Find(&lines).Error; e != nil {
		return nil, e
	}
	total := int64(0)
	for i := range lines {
		lines[i].Quantity = milliText(lines[i].QuantityMilli)
		lines[i].UnitPrice = moneyText(lines[i].UnitPriceCents)
		lines[i].Amount = moneyText(lines[i].AmountCents)
		total += lines[i].AmountCents
	}
	h.Items = lines
	h.TotalAmount = moneyText(total)
	return &h, nil
}
func milliText(n int64) string {
	whole, frac := n/1000, n%1000
	if frac == 0 {
		return fmt.Sprint(whole)
	}
	s := fmt.Sprintf("%03d", frac)
	s = strings.TrimRight(s, "0")
	return fmt.Sprintf("%d.%s", whole, s)
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
