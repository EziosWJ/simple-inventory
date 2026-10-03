package receivable

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/EziosWJ/simple-inventory/server/internal/audit"
	"github.com/EziosWJ/simple-inventory/server/internal/auth"
	"github.com/EziosWJ/simple-inventory/server/internal/filemgmt"
	"gorm.io/gorm"
)

type Attachment struct {
	BoundBy        int64      `json:"boundBy"`
	BoundAt        time.Time  `json:"boundAt"`
	ID             int64      `json:"id" gorm:"primaryKey"`
	EntryID        int64      `json:"entryId"`
	FileID         int64      `json:"fileId"`
	UploadedBy     int64      `json:"uploadedBy"`
	UploadedAt     time.Time  `json:"uploadedAt"`
	RemovedBy      *int64     `json:"removedBy"`
	RemovedAt      *time.Time `json:"removedAt"`
	InitialBinding bool       `json:"-"`
	OriginalName   string     `json:"originalName" gorm:"->"`
	FileSize       int64      `json:"fileSize" gorm:"->"`
	MimeType       string     `json:"mimeType" gorm:"->"`
	UploadedName   string     `json:"uploadedName" gorm:"->"`
	RemovedName    string     `json:"removedName" gorm:"->"`
}

func (Attachment) TableName() string { return "partner_entry_attachment" }

type attachmentStore interface {
	Attachments(context.Context, int64) ([]Attachment, error)
	AddAttachments(context.Context, audit.Metadata, int64, []int64) error
	RemoveAttachment(context.Context, audit.Metadata, int64, int64) error
}

func (s *Service) Attachments(ctx context.Context, id int64) ([]Attachment, error) {
	if id <= 0 {
		return nil, ErrInvalid
	}
	return s.store.Attachments(ctx, id)
}
func (s *Service) AddAttachments(ctx context.Context, m audit.Metadata, id int64, files []int64) error {
	if id <= 0 || len(files) == 0 {
		return ErrInvalid
	}
	if err := validateAttachmentIDs(files); err != nil {
		return err
	}
	return s.store.AddAttachments(ctx, m, id, files)
}
func (s *Service) RemoveAttachment(ctx context.Context, m audit.Metadata, id, attachment int64) error {
	if id <= 0 || attachment <= 0 {
		return ErrInvalid
	}
	return s.store.RemoveAttachment(ctx, m, id, attachment)
}
func attachmentKind(typ string) string {
	switch typ {
	case "OPENING":
		return "OPENING"
	case "RECEIPT", "PAYMENT":
		return "SETTLEMENT"
	case "CUSTOMER_REFUND", "SUPPLIER_REFUND":
		return "REFUND"
	}
	return ""
}
func attachmentPermission(ctx context.Context, tx *gorm.DB, typ string) error {
	kind := attachmentKind(typ)
	if kind == "" {
		return ErrInvalid
	}
	ok, err := auth.HasPermission(ctx, tx, filemgmt.AttachmentPermission(filemgmt.AttachmentPrefix+kind))
	if err != nil {
		return err
	}
	if !ok {
		return filemgmt.ErrForbidden
	}
	return nil
}
func validateAttachmentIDs(ids []int64) error {
	if len(ids) > 5 {
		return ErrInvalid
	}
	seen := map[int64]bool{}
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return ErrInvalid
		}
		seen[id] = true
	}
	return nil
}
func bindAttachments(ctx context.Context, tx *gorm.DB, m audit.Metadata, row entryRow, ids []int64, initial bool) error {
	if err := validateAttachmentIDs(ids); err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	if err := attachmentPermission(ctx, tx, row.EntryType); err != nil {
		return err
	}
	if row.ReversedByID != nil {
		return ErrConflict
	}
	var count int64
	if err := tx.Model(&Attachment{}).Where("entry_id=? AND removed_at IS NULL", row.ID).Count(&count).Error; err != nil {
		return err
	}
	sorted := append([]int64(nil), ids...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	for _, id := range sorted {
		if err := tx.Model(&filemgmt.File{}).Where("id=?", id).UpdateColumn("deleted", gorm.Expr("deleted")).Error; err != nil {
			return err
		}
		var f filemgmt.File
		if err := tx.Where("id=? AND deleted=0 AND status=1", id).Take(&f).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrInvalid
			}
			return err
		}
		var previous Attachment
		err := tx.Where("file_id=?", id).Take(&previous).Error
		if err == nil {
			if previous.EntryID == row.ID && previous.RemovedAt == nil {
				continue
			}
			return ErrConflict
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if f.CreateBy != m.ActorID || f.BusinessModule != filemgmt.AttachmentPrefix+attachmentKind(row.EntryType) || f.FileSize > filemgmt.MaxAttachmentSize {
			return ErrInvalid
		}
		count++
		if count > 5 {
			return ErrInvalid
		}
		a := Attachment{BoundBy: m.ActorID, BoundAt: time.Now().UTC(), EntryID: row.ID, FileID: id, UploadedBy: f.CreateBy, UploadedAt: f.CreateTime, InitialBinding: initial}
		if err := tx.Create(&a).Error; err != nil {
			return err
		}
		if err := audit.RecordOn(ctx, tx, audit.Event{Action: "partner.attachment.add", Resource: "partner_attachment", ResourceID: a.ID, Metadata: m}); err != nil {
			return err
		}
	}
	return nil
}
func checkInitialAttachments(tx *gorm.DB, row *entryRow, ids []int64) error {
	if len(ids) == 0 && !tx.Migrator().HasTable(&Attachment{}) {
		return nil
	}
	if err := validateAttachmentIDs(ids); err != nil {
		return err
	}
	var bound []int64
	if err := tx.Model(&Attachment{}).Where("entry_id=? AND initial_binding=TRUE", row.ID).Pluck("file_id", &bound).Error; err != nil {
		return err
	}
	a := append([]int64(nil), ids...)
	sort.Slice(a, func(i, j int) bool { return a[i] < a[j] })
	sort.Slice(bound, func(i, j int) bool { return bound[i] < bound[j] })
	if len(a) != len(bound) {
		return ErrConflict
	}
	for i := range a {
		if a[i] != bound[i] {
			return ErrConflict
		}
	}
	return tx.Model(&Attachment{}).Where("entry_id=? AND removed_at IS NULL", row.ID).Count(&row.AttachmentCount).Error
}
func (r *Repository) Attachments(ctx context.Context, id int64) ([]Attachment, error) {
	var row entryRow
	if err := r.db.WithContext(ctx).First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	kind := attachmentKind(row.EntryType)
	if kind == "" {
		return nil, ErrInvalid
	}
	if err := filemgmt.CheckAttachmentReadAccess(ctx, r.db, filemgmt.AttachmentPrefix+kind); err != nil {
		return nil, err
	}
	result := []Attachment{}
	err := r.db.WithContext(ctx).Table("partner_entry_attachment a").Select("a.*,f.original_name,f.file_size,f.mime_type,u.nickname AS uploaded_name,v.nickname AS removed_name").Joins("JOIN sys_file f ON f.id=a.file_id").Joins("LEFT JOIN sys_user u ON u.id=a.uploaded_by").Joins("LEFT JOIN sys_user v ON v.id=a.removed_by").Where("a.entry_id=?", id).Order("a.id").Scan(&result).Error
	return result, err
}
func lockAttachmentEntry(tx *gorm.DB, id int64) (entryRow, error) {
	var row entryRow
	if err := tx.Exec("UPDATE partner_balance SET amount_cents=amount_cents WHERE (partner_id,direction) IN (SELECT partner_id,direction FROM partner_balance_entry WHERE id=?)", id).Error; err != nil {
		return row, err
	}
	// Reversals acquire the same balance gate before changing the original record.
	if err := tx.First(&row, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return row, ErrNotFound
		}
		return row, err
	}
	if row.ReversedByID != nil {
		return row, ErrConflict
	}
	return row, nil
}
func (r *Repository) AddAttachments(ctx context.Context, m audit.Metadata, id int64, files []int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := lockAttachmentEntry(tx, id)
		if err != nil {
			return err
		}
		return bindAttachments(ctx, tx, m, row, files, false)
	})
}
func (r *Repository) RemoveAttachment(ctx context.Context, m audit.Metadata, id, attachment int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row, err := lockAttachmentEntry(tx, id)
		if err != nil {
			return err
		}
		if err := attachmentPermission(ctx, tx, row.EntryType); err != nil {
			return err
		}
		var a Attachment
		if err := tx.Where("id=? AND entry_id=?", attachment, id).Take(&a).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrNotFound
			}
			return err
		}
		if a.RemovedAt != nil {
			return nil
		}
		if err := tx.Model(&a).Updates(map[string]any{"removed_by": m.ActorID, "removed_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
		return audit.RecordOn(ctx, tx, audit.Event{Action: "partner.attachment.remove", Resource: "partner_attachment", ResourceID: a.ID, Metadata: m})
	})
}
