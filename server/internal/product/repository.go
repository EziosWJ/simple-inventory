package product

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/EziosWJ/simple-inventory/server/internal/audit"
	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db} }
func (r *Repository) Page(ctx context.Context, q Query) (Page, error) {
	p := Page{Records: []Product{}, Page: q.Page, PageSize: q.PageSize}
	d := r.db.WithContext(ctx).Model(&Product{})
	if q.Keyword != "" {
		like := "%" + q.Keyword + "%"
		d = d.Where("code LIKE ? OR name LIKE ? OR brand LIKE ? OR model LIKE ? OR specification LIKE ?", like, like, like, like, like)
	}
	if q.Type != "" {
		d = d.Where("type=?", q.Type)
	}
	if q.Category != "" {
		d = d.Where("category=?", q.Category)
	}
	if q.Status != nil {
		d = d.Where("status=?", *q.Status)
	}
	if e := d.Count(&p.Total).Error; e != nil {
		return p, e
	}
	e := d.Order("id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&p.Records).Error
	for i := range p.Records {
		p.Records[i] = present(p.Records[i])
	}
	return p, e
}
func (r *Repository) Find(ctx context.Context, id int64) (*Product, error) {
	var v Product
	e := r.db.WithContext(ctx).First(&v, id).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	v = present(v)
	return &v, e
}
func (r *Repository) Save(ctx context.Context, v Product, create bool, event audit.Event) (Product, error) {
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if create {
			autoCode := v.Code == ""
			if autoCode {
				v.Code = fmt.Sprintf("PENDING-%d-%d", time.Now().UnixNano(), v.ID)
			}
			if err := tx.Create(&v).Error; err != nil {
				return err
			}
			if autoCode {
				base := fmt.Sprintf("SP%08d", v.ID)
				for suffix := 0; ; suffix++ {
					v.Code = base
					if suffix > 0 {
						v.Code = fmt.Sprintf("%s-%d", base, suffix)
					}
					var count int64
					if err := tx.Model(&Product{}).Where("code=?", v.Code).Count(&count).Error; err != nil {
						return err
					}
					if count == 0 {
						break
					}
				}
				if err := tx.Model(&v).Update("code", v.Code).Error; err != nil {
					return err
				}
			}
			event.ResourceID = v.ID
		} else {
			v.UpdateTime = time.Now().UTC()
			result := tx.Model(&Product{}).Where("id=?", v.ID).Updates(map[string]any{"code": v.Code, "name": v.Name, "type": v.Type, "brand": v.Brand, "model": v.Model, "specification": v.Specification, "category": v.Category, "unit": v.Unit, "purchase_price_cents": v.PurchasePrice, "sale_price_cents": v.SalePrice, "remark": v.Remark, "update_time": v.UpdateTime})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return ErrNotFound
			}
		}
		return audit.RecordOn(ctx, tx, event)
	})
	if e != nil {
		if strings.Contains(strings.ToLower(e.Error()), "unique") || strings.Contains(strings.ToLower(e.Error()), "duplicate") {
			return Product{}, ErrConflict
		}
		return Product{}, e
	}
	stored, err := r.Find(ctx, v.ID)
	if err != nil {
		return Product{}, err
	}
	return *stored, nil
}
func (r *Repository) SetStatus(ctx context.Context, id int64, status int, event audit.Event) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Product{}).Where("id=?", id).Updates(map[string]any{"status": status, "update_time": time.Now().UTC()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return audit.RecordOn(ctx, tx, event)
	})
}
