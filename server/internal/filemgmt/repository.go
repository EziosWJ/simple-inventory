package filemgmt

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/EziosWJ/simple-inventory/server/internal/audit"
	"github.com/EziosWJ/simple-inventory/server/internal/auth"
	"gorm.io/gorm"
)

type Repository struct{ db *gorm.DB }

var _ Store = (*Repository)(nil)

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Page(ctx context.Context, q FilePageQuery) (Page[File], error) {
	var p Page[File]
	db := r.db.WithContext(ctx).Model(&File{}).Where("deleted=0 AND (business_module IS NULL OR business_module NOT LIKE ?)", AttachmentPrefix+"%")
	if q.OriginalName != "" {
		db = db.Where("original_name LIKE ?", "%"+q.OriginalName+"%")
	}
	if q.BusinessModule != "" {
		db = db.Where("business_module=?", q.BusinessModule)
	}
	if q.MimeType != "" {
		db = db.Where("mime_type LIKE ?", "%"+q.MimeType+"%")
	}
	if q.Status != nil {
		db = db.Where("status=?", *q.Status)
	}
	if err := db.Count(&p.Total).Error; err != nil {
		return p, err
	}
	p.Page, p.PageSize = q.Page, q.PageSize
	err := db.Order("create_time DESC,id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&p.Records).Error
	return p, err
}

func (r *Repository) Find(ctx context.Context, id int64) (*File, error) {
	var f File
	err := r.db.WithContext(ctx).Where("id=? AND deleted=0", id).Take(&f).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := r.attachmentAccess(ctx, f); err != nil {
		return nil, err
	}
	return &f, nil
}

func (r *Repository) Create(ctx context.Context, f File, e AuditEvent) (File, error) {
	if strings.HasPrefix(f.BusinessModule, AttachmentPrefix) {
		ok, err := auth.HasPermission(ctx, r.db, AttachmentPermission(f.BusinessModule))
		if err != nil {
			return f, err
		}
		if !ok {
			return f, ErrForbidden
		}
	}
	if !validMD5(f.FileMD5) {
		return f, ErrInvalid
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&f).Error; err != nil {
			return err
		}
		e.ResourceID = f.ID
		f.AccessURL = "/api/system/file/" + strconv.FormatInt(f.ID, 10) + "/view"
		if err := tx.Model(&File{}).Where("id=? AND deleted=0", f.ID).Update("access_url", f.AccessURL).Error; err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, e)
	})
	return f, err
}

func (r *Repository) Update(ctx context.Context, id int64, in UpdateInput, e AuditEvent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if strings.HasPrefix(in.BusinessModule, AttachmentPrefix) {
			return ErrProtected
		}
		if err := protectAttachment(tx, []int64{id}, false, e.Metadata.ActorID); err != nil {
			return err
		}
		if err := tx.Model(&File{}).Where("id=? AND deleted=0", id).Updates(map[string]any{"business_module": in.BusinessModule, "remark": in.Remark}).Error; err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, e)
	})
}

func (r *Repository) Delete(ctx context.Context, id int64, e AuditEvent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := protectAttachment(tx, []int64{id}, true, e.Metadata.ActorID); err != nil {
			return err
		}
		if err := tx.Model(&File{}).Where("id=? AND deleted=0", id).Update("deleted", 1).Error; err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, e)
	})
}

func (r *Repository) DeleteBatch(ctx context.Context, ids []int64, e AuditEvent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := protectAttachment(tx, ids, true, e.Metadata.ActorID); err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&File{}).Where("id IN ? AND deleted=0", ids).Count(&count).Error; err != nil {
			return err
		}
		if count != int64(len(ids)) {
			return ErrNotFound
		}
		if err := tx.Model(&File{}).Where("id IN ? AND deleted=0", ids).Update("deleted", 1).Error; err != nil {
			return err
		}
		for _, id := range ids {
			e.ResourceID = id
			if err := audit.RecordOn(ctx, tx, e); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *Repository) SetStatus(ctx context.Context, id int64, status int, e AuditEvent) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := protectAttachment(tx, []int64{id}, false, e.Metadata.ActorID); err != nil {
			return err
		}
		if err := tx.Model(&File{}).Where("id=? AND deleted=0", id).Update("status", status).Error; err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, e)
	})
}
