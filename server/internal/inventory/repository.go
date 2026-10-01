package inventory

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/EziosWJ/simple-inventory/server/internal/audit"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db} }

// The description a document line shows is the live catalog text until the line
// is posted; posting freezes the confirmed text on the line and in the ledger.
// NULLIF keeps a stored empty value meaning "not frozen yet".
const itemSnapshotSelect = "i.*,COALESCE(NULLIF(i.product_code,''),p.code) AS product_code,COALESCE(NULLIF(i.product_name,''),p.name) AS product_name,COALESCE(NULLIF(i.product_model,''),p.model) AS product_model,COALESCE(NULLIF(i.product_specification,''),p.specification) AS product_specification"

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
	return &v, itemsOn(db, &v)
}

func itemsOn(db *gorm.DB, v *Adjustment) error {
	items, e := loadItems(db, v.ID)
	if e != nil {
		return e
	}
	v.Items = items
	return nil
}

// loadItems reads the lines with their confirmed quantities and, for posted
// lines, the frozen description and the impact recorded in the ledger.
func loadItems(db *gorm.DB, id int64) ([]Item, error) {
	items := []Item{}
	e := db.Table("inventory_adjustment_item i").Select(itemSnapshotSelect).
		Joins("JOIN product p ON p.id=i.product_id").Where("i.adjustment_id=?", id).Order("i.id ASC").Find(&items).Error
	if e != nil {
		return nil, e
	}
	ids := make([]int64, 0, len(items))
	for i := range items {
		items[i].Quantity = quantityText(items[i].QuantityMilli)
		ids = append(ids, items[i].ID)
	}
	if len(ids) == 0 {
		return items, nil
	}
	var entries []Entry
	// UNIQUE(adjustment_item_id,entry_type) keeps at most one original posting
	// per line, so the reported impact cannot be double counted.
	if e := db.Table("inventory_entry").Where("entry_type='ORIGINAL' AND adjustment_item_id IN ?", ids).Find(&entries).Error; e != nil {
		return nil, e
	}
	byItem := make(map[int64]Entry, len(entries))
	for _, entry := range entries {
		byItem[entry.AdjustmentItemID] = entry
	}
	for i := range items {
		entry, ok := byItem[items[i].ID]
		if !ok {
			continue
		}
		before, after := entry.BalanceBeforeMilli, entry.BalanceAfterMilli
		items[i].BalanceBeforeMilli, items[i].BalanceAfterMilli = &before, &after
		beforeText, afterText := quantityText(before), quantityText(after)
		items[i].BalanceBefore, items[i].BalanceAfter = &beforeText, &afterText
	}
	return items, nil
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

func insertItems(tx *gorm.DB, id int64, items []Item, validate func(Item, ProductReference) error) error {
	for i := range items {
		item := &items[i]
		p, e := readProduct(tx, item.ProductID)
		if e != nil {
			return e
		}
		if e := validate(*item, p); e != nil {
			return e
		}
		item.AdjustmentID = id
		item.ProductCode, item.ProductName = "", ""
		item.ProductModel, item.ProductSpecification = nil, nil
		if e := tx.Create(item).Error; e != nil {
			return e
		}
	}
	return nil
}

// readProduct compares type/unit/status inside the current transaction. The
// PostgreSQL SHARE lock blocks a concurrent catalog edit from committing until
// this transaction finishes, so the confirmation cannot go stale afterwards.
func readProduct(tx *gorm.DB, id int64) (ProductReference, error) {
	var p ProductReference
	d := tx.Table("product").Select("id,code,name,model,specification,type,unit,status").Where("id=?", id)
	if tx.Dialector.Name() == "postgres" {
		d = d.Clauses(clause.Locking{Strength: "SHARE"})
	}
	if e := d.Take(&p).Error; e != nil {
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return p, invalid("所选商品不存在")
		}
		return p, e
	}
	return p, nil
}

func adjustmentQuery(db *gorm.DB) *gorm.DB {
	return db.Table("inventory_adjustment a").Select("a.*,COALESCE(NULLIF(u.nickname,''),u.username) AS created_by_name,COALESCE(NULLIF(pp.nickname,''),pp.username) AS posted_by_name,COALESCE(NULLIF(c.nickname,''),c.username) AS cancelled_by_name").Joins("LEFT JOIN sys_user u ON u.id=a.created_by").Joins("LEFT JOIN sys_user pp ON pp.id=a.posted_by").Joins("LEFT JOIN sys_user c ON c.id=a.cancelled_by")
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

// Post applies a whole document in one transaction: revalidate every line,
// move each product balance, append the immutable ledger lines, flip the header
// to POSTED and write the audit row. Any failure rolls all of it back, leaving
// the draft and the stock exactly as they were.
func (r *Repository) Post(ctx context.Context, id, version int64, event audit.Event, validate func(Item, ProductReference) error) (Adjustment, error) {
	var v Adjustment
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		occurred := time.Now().UTC()
		// CAS first, as in Edit: it both locks the header and makes a repeated or
		// concurrent posting attempt a single effective transition. Status and
		// posting metadata move together because the posting check constraint
		// rejects POSTED without both.
		changed := tx.Model(&Adjustment{}).Where("id=? AND version=? AND status='DRAFT'", id, version).
			Updates(map[string]any{"status": "POSTED", "version": gorm.Expr("version+1"), "posted_by": event.Metadata.ActorID, "posted_at": occurred})
		if e := checkMutation(tx, id, changed); e != nil {
			return e
		}
		items, e := loadItems(tx, id)
		if e != nil {
			return e
		}
		if len(items) == 0 {
			return invalid("调整单没有明细，无法过账")
		}
		// A stable product order keeps concurrent multi-product postings from
		// deadlocking against each other on PostgreSQL row locks.
		sort.Slice(items, func(i, j int) bool { return items[i].ProductID < items[j].ProductID })
		for i := range items {
			item := &items[i]
			p, e := readProduct(tx, item.ProductID)
			if e != nil {
				return e
			}
			if e := validate(*item, p); e != nil {
				return e
			}
			before, after, e := applyDelta(tx, item.ProductID, item.QuantityMilli, occurred, postingLimits)
			if e != nil {
				return e
			}
			entry := Entry{
				ProductID: item.ProductID, AdjustmentID: id, AdjustmentItemID: item.ID,
				EntryType: "ORIGINAL", QuantityMilli: item.QuantityMilli,
				BalanceBeforeMilli: before, BalanceAfterMilli: after,
				Reason: item.Reason, Remark: item.Remark,
				ProductCode: p.Code, ProductName: p.Name, ProductModel: p.Model, ProductSpecification: p.Specification,
				Unit: item.Unit, OperatorID: event.Metadata.ActorID, OccurredAt: occurred,
			}
			if e := tx.Create(&entry).Error; e != nil {
				return e
			}
			// Posting freezes the confirmed description on the line as well; the
			// catalog keeps only the internal ID reference.
			if e := tx.Model(&Item{}).Where("id=?", item.ID).Updates(map[string]any{"product_code": p.Code, "product_name": p.Name, "product_model": p.Model, "product_specification": p.Specification}).Error; e != nil {
				return e
			}
		}
		event.ResourceID = id
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

// stockLimits names the errors that make a whole transaction fail when one
// product cannot move. Posting and reversal share the same guarded statement but
// must not tell the operator the same thing when it refuses.
type stockLimits struct{ insufficient, overflow error }

var (
	postingLimits  = stockLimits{insufficient: ErrStockInsufficient, overflow: ErrStockOverflow}
	reversalLimits = stockLimits{insufficient: ErrReversalInsufficient, overflow: ErrStockOverflow}
)

// applyDelta moves one product balance and reports the before/after values the
// ledger line must agree with. It never relies on a process-wide lock: the
// single guarded statement below is the only serialization point, so two API
// instances and two dialects share the same guarantee.
func applyDelta(tx *gorm.DB, productID, delta int64, now time.Time, limits stockLimits) (int64, int64, error) {
	var result *gorm.DB
	if delta > 0 {
		// One statement both creates the first balance and adds to an existing
		// one: concurrent first increases cannot produce two rows or lose an
		// update, because they serialize on the unique product_id rather than on
		// a read-then-write in the API process. The guard keeps the stored sum a
		// representable integer, so no dialect wraps silently.
		result = tx.Exec("INSERT INTO inventory_balance (product_id,quantity_milli,update_time) VALUES (?,?,?) ON CONFLICT (product_id) DO UPDATE SET quantity_milli = inventory_balance.quantity_milli + excluded.quantity_milli, update_time = excluded.update_time WHERE inventory_balance.quantity_milli <= ?", productID, delta, now, math.MaxInt64-delta)
	} else {
		// The non-negative guard is what enforces the rule; the CHECK constraint
		// on the balance table stays the last line of defence.
		result = tx.Exec("UPDATE inventory_balance SET quantity_milli = quantity_milli + ?, update_time = ? WHERE product_id = ? AND quantity_milli + ? >= 0", delta, now, productID, delta)
	}
	if result.Error != nil {
		return 0, 0, result.Error
	}
	if result.RowsAffected == 0 {
		// Nothing moved: for an increase the guard rejected an unrepresentable
		// sum, for a decrease the balance would have gone negative.
		if delta > 0 {
			return 0, 0, fmt.Errorf("%w：商品%d 结存超出可表示范围", limits.overflow, productID)
		}
		return 0, 0, fmt.Errorf("%w：商品%d 结存不足", limits.insufficient, productID)
	}
	// The statement above holds the balance row until commit, so this reading is
	// exactly the result of this line and the pair stays consistent.
	after, exists, e := balanceOn(tx, productID)
	if e != nil {
		return 0, 0, e
	}
	if !exists {
		return 0, 0, fmt.Errorf("库存余额写入后无法读取：商品%d", productID)
	}
	return after - delta, after, nil
}

// balanceOn reads one product balance inside the current transaction. A missing
// row means the product never had a balance, which is zero stock.
func balanceOn(tx *gorm.DB, productID int64) (int64, bool, error) {
	var row struct{ QuantityMilli int64 }
	e := tx.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", productID).Take(&row).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return 0, false, nil
	}
	if e != nil {
		return 0, false, e
	}
	return row.QuantityMilli, true, nil
}

// Cancel terminates a document. A draft is cancelled without touching stock; a
// posted document is cancelled by appending one reversal line per original line,
// keeping every original record. Both paths CAS on the confirmed version, so a
// repeated or racing cancellation is effective at most once; a cancellation that
// would drive any balance negative fails as a whole and leaves the document
// posted.
func (r *Repository) Cancel(ctx context.Context, id, version int64, reason string, event audit.Event) (Adjustment, error) {
	var v Adjustment
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		occurred := time.Now().UTC()
		// The posted branch is tried first because it is the only one that moves
		// stock; a document that has not been posted simply matches no row here.
		changed := tx.Model(&Adjustment{}).Where("id=? AND version=? AND status='POSTED'", id, version).
			Updates(map[string]any{"version": gorm.Expr("version+1"), "status": "CANCELLED", "cancelled_by": event.Metadata.ActorID, "cancelled_at": occurred, "cancel_reason": reason})
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected > 0 {
			if e := reverseOn(tx, id, occurred, event.Metadata.ActorID); e != nil {
				return e
			}
		} else {
			// The posting metadata stays untouched: a cancelled document keeps
			// whether it had been posted, and by whom.
			changed = tx.Model(&Adjustment{}).Where("id=? AND version=? AND status='DRAFT'", id, version).
				Updates(map[string]any{"version": gorm.Expr("version+1"), "status": "CANCELLED", "cancelled_by": event.Metadata.ActorID, "cancelled_at": occurred, "cancel_reason": reason})
			if e := checkMutation(tx, id, changed); e != nil {
				return e
			}
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

// reverseOn appends the cancellation reversal for a posted document: one
// REVERSAL line per original line with the opposite signed quantity, in the
// product order posting used, so concurrent cancellations cannot deadlock.
//
// The line refers to the same adjustment and the same adjustment item as its
// original entry, and entry_type tells the two apart; UNIQUE(adjustment_item_id,
// entry_type) makes that pair the exact link back to the original ledger row.
// Nothing is read from the product: a disabled product must still be cancelable,
// and the confirmed type, unit and description stay those of the posting.
func reverseOn(tx *gorm.DB, id int64, occurred time.Time, operatorID int64) error {
	items, e := loadStoredItems(tx, id)
	if e != nil {
		return e
	}
	if len(items) == 0 {
		return fmt.Errorf("已过账调整单%d缺少明细，无法冲销", id)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ProductID < items[j].ProductID })
	for i := range items {
		item := &items[i]
		if item.ProductCode == "" || item.ProductName == "" {
			return fmt.Errorf("已过账明细%d缺少商品快照，无法冲销", item.ID)
		}
		delta := -item.QuantityMilli
		before, after, e := applyDelta(tx, item.ProductID, delta, occurred, reversalLimits)
		if e != nil {
			return e
		}
		entry := Entry{
			ProductID: item.ProductID, AdjustmentID: id, AdjustmentItemID: item.ID,
			EntryType: "REVERSAL", QuantityMilli: delta,
			BalanceBeforeMilli: before, BalanceAfterMilli: after,
			Reason: item.Reason, Remark: item.Remark,
			ProductCode: item.ProductCode, ProductName: item.ProductName, ProductModel: item.ProductModel, ProductSpecification: item.ProductSpecification,
			Unit: item.Unit, OperatorID: operatorID, OccurredAt: occurred,
		}
		if e := tx.Create(&entry).Error; e != nil {
			return e
		}
	}
	return nil
}

// storedItemSelect reads only the columns frozen on the line. The snapshot must
// come from the stored copy, never from the live catalog text a JOIN would add.
const storedItemSelect = "id,adjustment_id,product_id,product_code,product_name,product_model,product_specification,product_type,unit,quantity_milli,reason,remark"

func loadStoredItems(tx *gorm.DB, id int64) ([]Item, error) {
	items := []Item{}
	e := tx.Table("inventory_adjustment_item").Select(storedItemSelect).Where("adjustment_id=?", id).Order("id ASC").Find(&items).Error
	if e != nil {
		return nil, e
	}
	return items, nil
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
