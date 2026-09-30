package inventory

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/EziosWJ/simple-inventory/server/internal/audit"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db} }
func (r *Repository) Create(ctx context.Context, v Adjustment, event audit.Event, validate func(Item, ProductReference) error) (Adjustment, error) {
	bytes := make([]byte, 16)
	if _, e := rand.Read(bytes); e != nil {
		return Adjustment{}, e
	}
	v.CreateTime = time.Now().UTC()
	v.DocumentNo = "IA" + time.Now().UTC().Format("20060102") + "-" + hex.EncodeToString(bytes)
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Insert first reserves the SQLite writer before reading product references.
		// PostgreSQL row SHARE locks below block type/unit/status changes until commit.
		if e := tx.Create(&v).Error; e != nil {
			return e
		}
		if e := insertItems(tx, v.ID, v.Items, validate); e != nil {
			return e
		}
		event.ResourceID = v.ID
		if e := audit.RecordOn(ctx, tx, event); e != nil {
			return e
		}
		// Read the full response before commit; no post-commit read can turn a
		// successful creation into an apparent failure.
		stored, e := findOn(tx, v.ID)
		if e != nil {
			return e
		}
		v = *stored
		return nil
	})
	if e != nil {
		if errors.Is(e, gorm.ErrDuplicatedKey) || strings.Contains(strings.ToLower(e.Error()), "unique") {
			return Adjustment{}, ErrConflict
		}
		return Adjustment{}, e
	}
	return v, nil
}
func findOn(db *gorm.DB, id int64) (*Adjustment, error) {
	var v Adjustment
	d := adjustmentQuery(db).Where("a.id=?", id)
	// Reading header and items holds the header lock until the aggregate is read,
	// so an edit cannot return a new version with old or partially replaced items.
	if db.Dialector.Name() == "postgres" {
		d = d.Clauses(clause.Locking{Strength: "SHARE", Table: clause.Table{Name: "a"}})
	}
	e := d.Take(&v).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if e != nil {
		return nil, e
	}
	v.Items = []Item{}
	e = db.Table("inventory_adjustment_item i").Select("i.*,p.code AS product_code,p.name AS product_name,p.model AS product_model,p.specification AS product_specification").Joins("JOIN product p ON p.id=i.product_id").Where("i.adjustment_id=?", id).Order("i.id ASC").Find(&v.Items).Error
	for i := range v.Items {
		v.Items[i].Quantity = quantityText(v.Items[i].QuantityMilli)
	}
	return &v, e
}
func (r *Repository) Find(ctx context.Context, id int64) (*Adjustment, error) {
	var v *Adjustment
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error { var e error; v, e = findOn(tx, id); return e })
	return v, err
}
func (r *Repository) Page(ctx context.Context, q Query) (Page, error) {
	p := Page{Records: []Adjustment{}, Page: q.Page, PageSize: q.PageSize}
	d := r.db.WithContext(ctx).Table("inventory_adjustment a")
	if q.DocumentNo != "" {
		v := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(q.DocumentNo)
		d = d.Where("a.document_no LIKE ? ESCAPE '\\'", "%"+v+"%")
	}
	if q.Status != "" {
		d = d.Where("a.status=?", q.Status)
	}
	if q.ProductID > 0 {
		d = d.Where("EXISTS (SELECT 1 FROM inventory_adjustment_item i WHERE i.adjustment_id=a.id AND i.product_id=?)", q.ProductID)
	}
	if q.CreatedFrom != nil {
		d = d.Where("a.create_time>=?", *q.CreatedFrom)
	}
	if q.CreatedTo != nil {
		d = d.Where("a.create_time<?", *q.CreatedTo)
	}
	if e := d.Count(&p.Total).Error; e != nil {
		return p, e
	}
	e := adjustmentQuery(d).Order("a.id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&p.Records).Error
	return p, e
}

// The same transactional product-reference check applies to create and edit.
func insertItems(tx *gorm.DB, id int64, items []Item, validate func(Item, ProductReference) error) error {
	for i := range items {
		item := &items[i]
		var p ProductReference
		d := tx.Table("product").Select("id,type,unit,status").Where("id=?", item.ProductID)
		if tx.Dialector.Name() == "postgres" {
			d = d.Clauses(clause.Locking{Strength: "SHARE"})
		}
		if e := d.Take(&p).Error; e != nil {
			if errors.Is(e, gorm.ErrRecordNotFound) {
				return invalid("所选商品不存在")
			}
			return e
		}
		if e := validate(*item, p); e != nil {
			return e
		}
		item.AdjustmentID = id
		if e := tx.Create(item).Error; e != nil {
			return e
		}
	}
	return nil
}

func adjustmentQuery(db *gorm.DB) *gorm.DB {
	return db.Table("inventory_adjustment a").Select("a.*,COALESCE(NULLIF(u.nickname,''),u.username) AS created_by_name,COALESCE(NULLIF(c.nickname,''),c.username) AS cancelled_by_name").Joins("LEFT JOIN sys_user u ON u.id=a.created_by").Joins("LEFT JOIN sys_user c ON c.id=a.cancelled_by")
}
func (r *Repository) Edit(ctx context.Context, id, version int64, items []Item, event audit.Event, validate func(Item, ProductReference) error) (Adjustment, error) {
	var v Adjustment
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// CAS is the first database operation: it locks this header (PostgreSQL)
		// or reserves the writer (SQLite) before any product or item reads.
		changed := tx.Model(&Adjustment{}).Where("id=? AND version=? AND status='DRAFT'", id, version).Update("version", gorm.Expr("version+1"))
		if e := checkMutation(tx, id, changed); e != nil {
			return e
		}
		if e := tx.Where("adjustment_id=?", id).Delete(&Item{}).Error; e != nil {
			return e
		}
		if e := insertItems(tx, id, items, validate); e != nil {
			return e
		}
		if e := audit.RecordOn(ctx, tx, event); e != nil {
			return e
		}
		stored, e := findOn(tx, id)
		if e != nil {
			return e
		}
		v = *stored
		return nil
	})
	return v, err
}
func (r *Repository) Cancel(ctx context.Context, id, version int64, reason string, event audit.Event) (Adjustment, error) {
	var v Adjustment
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		changed := tx.Model(&Adjustment{}).Where("id=? AND version=? AND status='DRAFT'", id, version).Updates(map[string]any{"version": gorm.Expr("version+1"), "status": "CANCELLED", "cancelled_by": event.Metadata.ActorID, "cancelled_at": time.Now().UTC(), "cancel_reason": reason})
		if e := checkMutation(tx, id, changed); e != nil {
			return e
		}
		if e := audit.RecordOn(ctx, tx, event); e != nil {
			return e
		}
		stored, e := findOn(tx, id)
		if e != nil {
			return e
		}
		v = *stored
		return nil
	})
	return v, err
}
func checkMutation(tx *gorm.DB, id int64, result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		return nil
	}
	var count int64
	if e := tx.Model(&Adjustment{}).Where("id=?", id).Count(&count).Error; e != nil {
		return e
	}
	if count == 0 {
		return ErrNotFound
	}
	return ErrConflict
}
