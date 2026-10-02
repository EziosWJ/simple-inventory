package sale

import (
	"context"
	"errors"

	"github.com/EziosWJ/simple-inventory/server/internal/audit"
	"gorm.io/gorm"
)

type saveRow struct {
	ActorID                            int64
	Operation, RequestKey, Fingerprint string
	DocumentID, SavedVersion           *int64
}

func (saveRow) TableName() string { return "sale_save_request" }

func beginSave(tx *gorm.DB, actor int64, request *SaveRequest) (*Draft, error) {
	if request == nil {
		return nil, nil
	}
	insert := tx.Exec("INSERT INTO sale_save_request(actor_id,operation,request_key,fingerprint) VALUES (?,?,?,?) ON CONFLICT(actor_id,operation,request_key) DO NOTHING", actor, request.Operation, request.Key, request.Fingerprint)
	if insert.Error != nil {
		return nil, insert.Error
	}
	if insert.RowsAffected == 1 {
		return nil, nil
	}
	var row saveRow
	if e := tx.Where("actor_id=? AND operation=? AND request_key=?", actor, request.Operation, request.Key).Take(&row).Error; e != nil {
		return nil, e
	}
	if row.Fingerprint != request.Fingerprint {
		return nil, ErrRequestConflict
	}
	if row.DocumentID == nil || row.SavedVersion == nil {
		return nil, ErrConflict
	}
	document, e := find(tx, *row.DocumentID)
	if e != nil {
		return nil, e
	}
	document.SaveReceipt = receipt(row)
	return document, nil
}
func finishSave(tx *gorm.DB, actor int64, request *SaveRequest, document *Draft) error {
	if request == nil {
		return nil
	}
	result := tx.Table("sale_save_request").Where("actor_id=? AND operation=? AND request_key=?", actor, request.Operation, request.Key).Updates(map[string]any{"document_id": document.ID, "saved_version": document.Version})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrConflict
	}
	document.SaveReceipt = &SaveReceipt{request.Key, request.Operation, document.ID, document.Version}
	return nil
}
func receipt(row saveRow) *SaveReceipt {
	return &SaveReceipt{row.RequestKey, row.Operation, *row.DocumentID, *row.SavedVersion}
}

func (r *Repository) SaveResult(ctx context.Context, actor int64, operation, key string) (SaveResult, error) {
	var row saveRow
	db := r.db.WithContext(ctx)
	e := db.Where("actor_id=? AND operation=? AND request_key=?", actor, operation, key).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		// Absence is not failure: the original transaction may still be running.
		// A definite rejection is conveyed by the save's validation/conflict error.
		return SaveResult{State: "UNCONFIRMED"}, nil
	}
	if e != nil {
		return SaveResult{}, e
	}
	if row.DocumentID == nil || row.SavedVersion == nil {
		if row.Fingerprint == "" {
			return SaveResult{State: "NOT_COMMITTED"}, nil
		}
		return SaveResult{State: "UNCONFIRMED"}, nil
	}
	document, e := find(db, *row.DocumentID)
	if e != nil {
		return SaveResult{}, e
	}
	return SaveResult{State: "COMMITTED", Receipt: receipt(row), Document: document}, nil
}

// Use the saving unique key to wait for a running transaction. If absent,
// permanently close the key before declaring NOT_COMMITTED; any late save
// conflicts with the empty fingerprint, allowing safe recovery after refresh.
func (r *Repository) ResolveSave(ctx context.Context, m audit.Metadata, operation, key string) (SaveResult, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		insert := tx.Exec("INSERT INTO sale_save_request(actor_id,operation,request_key,fingerprint) VALUES (?,?,?,'') ON CONFLICT(actor_id,operation,request_key) DO NOTHING", m.ActorID, operation, key)
		if insert.Error != nil {
			return insert.Error
		}
		if insert.RowsAffected == 0 {
			return nil
		}
		return audit.RecordOn(ctx, tx, audit.Event{Action: "sale.save.resolve", Resource: "sale", Summary: "核实销售草稿保存未提交，关闭原保存标识", Metadata: m})
	})
	if err != nil {
		return SaveResult{}, mapErr(err)
	}
	return r.SaveResult(ctx, m.ActorID, operation, key)
}
