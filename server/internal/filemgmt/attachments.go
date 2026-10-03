package filemgmt

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"time"

	"github.com/EziosWJ/simple-inventory/server/internal/auth"
	"gorm.io/gorm"
	"rsc.io/pdf"
)

const AttachmentPrefix = "partner-attachment:"
const MaxAttachmentSize = 10 * 1024 * 1024

func AttachmentPermission(module string) string {
	switch module {
	case AttachmentPrefix + "OPENING":
		return "business:opening-balance"
	case AttachmentPrefix + "SETTLEMENT":
		return "business:settlement"
	case AttachmentPrefix + "REFUND":
		return "business:refund"
	}
	return ""
}

func validateAttachment(input UploadInput) error {
	if input.Size <= 0 || input.Size > MaxAttachmentSize {
		return ErrAttachmentInvalid
	}
	ext := strings.ToLower(filepath.Ext(input.Filename))
	switch ext {
	case ".png", ".jpg", ".jpeg":
		_, err := validateContentType(imageMimeFromExtension(input.Filename), input.Filename, input.Reader)
		return err
	case ".pdf":
		// PDF header and final EOF marker must both be present; never trust a declared MIME type.
		data, err := io.ReadAll(io.LimitReader(input.Reader, MaxAttachmentSize+1))
		if _, seekErr := input.Reader.Seek(0, io.SeekStart); seekErr != nil {
			return seekErr
		}
		if err != nil || len(data) > MaxAttachmentSize || !bytes.HasPrefix(data, []byte("%PDF-")) || !bytes.HasSuffix(bytes.TrimSpace(data), []byte("%%EOF")) || !bytes.Contains(data, []byte("startxref")) {
			return ErrAttachmentInvalid
		}
		return validatePDF(data)
	}
	return ErrAttachmentInvalid
}

func (r *Repository) attachmentAccess(ctx context.Context, f File) error {
	if !strings.HasPrefix(f.BusinessModule, AttachmentPrefix) {
		return nil
	}
	permission := AttachmentPermission(f.BusinessModule)
	ok, err := canReadAttachment(ctx, r.db, f.BusinessModule)
	if err != nil {
		return err
	}
	if !ok || permission == "" {
		return ErrForbidden
	}
	var count int64
	if err := r.db.WithContext(ctx).Table("partner_entry_attachment").Where("file_id=?", f.ID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		p, _ := auth.PrincipalFromContext(ctx)
		if p.UserID != f.CreateBy {
			return ErrForbidden
		}
		// A live form renews the lease; abandoned temporary files can be collected safely.
		result := r.db.WithContext(ctx).Model(&File{}).Where("id=? AND deleted=0", f.ID).Update("update_time", time.Now().UTC())
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrNotFound
		}
		return nil
	}
	return nil
}

func protectAttachment(tx *gorm.DB, ids []int64, allowTemporaryDelete bool, actor int64) error {
	// The write reserves SQLite and shares the file row lock with binding and cleanup.
	if err := tx.Model(&File{}).Where("id IN ?", ids).UpdateColumn("deleted", gorm.Expr("deleted")).Error; err != nil {
		return err
	}
	var files []File
	if err := tx.Where("id IN ? AND deleted=0", ids).Find(&files).Error; err != nil {
		return err
	}
	for _, f := range files {
		if !strings.HasPrefix(f.BusinessModule, AttachmentPrefix) {
			continue
		}
		ok, err := auth.HasPermission(tx.Statement.Context, tx, AttachmentPermission(f.BusinessModule))
		if err != nil {
			return err
		}
		if !ok {
			return ErrForbidden
		}
		if !allowTemporaryDelete || actor != f.CreateBy {
			return ErrProtected
		}
		var n int64
		if err := tx.Table("partner_entry_attachment").Where("file_id=?", f.ID).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			return ErrProtected
		}
	}
	return nil
}

// CleanupAttachments expires only unbound, abandoned uploads. A form renews its lease every minute.
func (r *Repository) CleanupAttachments(ctx context.Context) ([]File, error) {
	files := []File{}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		cutoff := time.Now().UTC().Add(-24 * time.Hour)
		where := "business_module LIKE ? AND deleted=0 AND update_time<? AND NOT EXISTS (SELECT 1 FROM partner_entry_attachment a WHERE a.file_id=sys_file.id)"
		if err := tx.Model(&File{}).Where(where, AttachmentPrefix+"%", cutoff).UpdateColumn("deleted", gorm.Expr("deleted")).Error; err != nil {
			return err
		}
		if err := tx.Where(where, AttachmentPrefix+"%", cutoff).Find(&files).Error; err != nil {
			return err
		}
		for _, f := range files {
			if err := tx.Model(&File{}).Where("id=?", f.ID).Update("deleted", 1).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return files, err
	}
	var retry []File
	err = r.db.WithContext(ctx).Where("business_module LIKE ? AND deleted=1 AND NOT EXISTS (SELECT 1 FROM partner_entry_attachment a WHERE a.file_id=sys_file.id)", AttachmentPrefix+"%").Find(&retry).Error
	return retry, err
}

func (s *Service) cleanupAttachments(ctx context.Context) error {
	collector, ok := s.store.(interface {
		CleanupAttachments(context.Context) ([]File, error)
		PurgeTemporaryAttachment(context.Context, int64) error
	})
	if !ok {
		return fmt.Errorf("%w: 附件存储不支持清理", ErrInvalid)
	}
	files, err := collector.CleanupAttachments(ctx)
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := s.storage.Remove(ctx, f.StoragePath); err != nil {
			slog.Error("清理临时附件物理文件失败", "fileID", f.ID, "error", err)
			continue
		}
		if err := collector.PurgeTemporaryAttachment(ctx, f.ID); err != nil {
			slog.Error("清理临时附件元数据失败", "fileID", f.ID, "error", err)
		}
	}
	return nil
}

// Invalid PDFs must become validation errors even if the parser encounters a malformed object.
func validatePDF(data []byte) (err error) {
	defer func() {
		if recover() != nil {
			err = ErrAttachmentInvalid
		}
	}()
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || reader.Trailer().Key("Root").Key("Type").Name() != "Catalog" {
		return ErrAttachmentInvalid
	}
	pages := reader.NumPage()
	if pages <= 0 || pages > 10000 {
		return ErrAttachmentInvalid
	}
	for i := 1; i <= pages; i++ {
		if reader.Page(i).V.Key("Type").Name() != "Page" {
			return ErrAttachmentInvalid
		}
	}
	return nil
}

// RunAttachmentCleanup is owned by the API process lifecycle, not by HTTP router construction.
func (s *Service) RunAttachmentCleanup(ctx context.Context) {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		if err := s.cleanupAttachments(ctx); err != nil && ctx.Err() == nil {
			slog.Error("清理临时往来附件失败", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *Repository) PurgeTemporaryAttachment(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Where("id=? AND deleted=1 AND business_module LIKE ? AND NOT EXISTS (SELECT 1 FROM partner_entry_attachment a WHERE a.file_id=sys_file.id)", id, AttachmentPrefix+"%").Delete(&File{}).Error
}

// Ledger readers can view all posted business evidence; each business menu grant controls mutations.
func CheckAttachmentReadAccess(ctx context.Context, db *gorm.DB, module string) error {
	ok, err := canReadAttachment(ctx, db, module)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	return nil
}
func canReadAttachment(ctx context.Context, db *gorm.DB, module string) (bool, error) {
	permission := AttachmentPermission(module)
	if permission == "" {
		return false, nil
	}
	ok, err := auth.HasPermission(ctx, db, permission)
	if err != nil || ok {
		return ok, err
	}
	return auth.HasPermission(ctx, db, "business:partner-ledger")
}
