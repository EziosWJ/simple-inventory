package salereturn

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

func (r *Repository) Create(ctx context.Context, h Document, items []Item, event audit.Event) (Document, error) {
	var out Document
	now := time.Now().UTC()
	var b [10]byte
	if _, e := rand.Read(b[:]); e != nil {
		return out, e
	}
	h.DocumentNo = "SR" + now.Format("20060102") + "-" + hex.EncodeToString(b[:])
	h.CreateTime = now
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := validateSource(tx, h.SaleID, items); e != nil {
			return e
		}
		if e := tx.Table("sale_document").Select("partner_id").Where("id=? AND status='POSTED'", h.SaleID).Scan(&h.PartnerID).Error; e != nil {
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
		if e := validateSource(tx, h.SaleID, items); e != nil {
			return e
		}
		var partnerID int64
		if e := tx.Table("sale_document").Select("partner_id").Where("id=? AND status='POSTED'", h.SaleID).Scan(&partnerID).Error; e != nil {
			return e
		}
		res := tx.Model(&Document{}).Where("id=? AND status='DRAFT' AND version=?", id, version).Updates(map[string]any{"sale_id": h.SaleID, "partner_id": partnerID, "business_date": h.BusinessDate, "remark": h.Remark, "version": version + 1})
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
	var originID int64
	if e := r.db.WithContext(ctx).Table("sale_return_document").Select("sale_id").Where("id=?", id).Scan(&originID).Error; e != nil {
		return out, e
	}
	if originID < 1 {
		return out, ErrNotFound
	}
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock the origin first so this reversal cannot race the original sale's
		// cancellation or another return post against the same sale.
		if e := lockOrigin(tx, originID); e != nil {
			return e
		}
		var old Document
		q := tx.Where("id=?", id)
		if tx.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if e := q.Take(&old).Error; e != nil {
			return e
		}
		if old.Version != version || (old.Status != "DRAFT" && old.Status != "POSTED") {
			return ErrConflict
		}
		oldStatus := old.Status
		old.CancelReason = &reason
		if old.Status == "POSTED" {
			if e := reversePostedReturn(tx, id, old, event.Metadata.ActorID); e != nil {
				return e
			}
		}
		now := time.Now().UTC()
		res := tx.Model(&Document{}).Where("id=? AND status=? AND version=?", id, oldStatus, version).Updates(map[string]any{"status": "CANCELLED", "version": version + 1, "cancelled_by": event.Metadata.ActorID, "cancelled_at": now, "cancel_reason": reason})
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

// Post adds the returned goods back to shared stock and reduces the customer's
// total receivable in one transaction. The origin sale is locked first, so a
// concurrent sale cancellation or a second return cannot double-count quota.
func (r *Repository) Post(ctx context.Context, id, version int64, event audit.Event) (Document, error) {
	var out Document
	var originID int64
	if e := r.db.WithContext(ctx).Table("sale_return_document").Select("sale_id").Where("id=?", id).Scan(&originID).Error; e != nil {
		return out, e
	}
	if originID < 1 {
		return out, ErrNotFound
	}
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if e := lockOrigin(tx, originID); e != nil {
			return e
		}
		var h Document
		q := tx.Where("id=?", id)
		if tx.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if e := q.Take(&h).Error; e != nil {
			return e
		}
		if h.Status != "DRAFT" || h.Version != version {
			return ErrConflict
		}
		if h.SaleID != originID {
			return ErrConflict
		}
		var origin struct {
			ID, PartnerID                    int64
			Status, DocumentNo, BusinessDate string
		}
		if e := tx.Table("sale_document").Select("id,partner_id,status,document_no,business_date").Where("id=?", h.SaleID).Take(&origin).Error; e != nil || origin.Status != "POSTED" {
			return fmt.Errorf("%w：原销售单必须仍为已过账", ErrConflict)
		}
		if origin.PartnerID != h.PartnerID {
			return ErrConflict
		}
		var lines []Item
		if e := tx.Where("document_id=?", id).Order("id").Find(&lines).Error; e != nil {
			return e
		}
		if len(lines) == 0 {
			return ErrInvalid
		}
		now := time.Now().UTC()
		if e := tx.Exec("INSERT INTO partner_balance(partner_id,direction,amount_cents,entry_count,update_time) VALUES (?, 'CUSTOMER', 0, 0, ?) ON CONFLICT(partner_id,direction) DO NOTHING", h.PartnerID, now).Error; e != nil {
			return e
		}
		var balance struct{ ID, AmountCents int64 }
		bq := tx.Table("partner_balance").Where("partner_id=? AND direction='CUSTOMER'", h.PartnerID)
		if tx.Dialector.Name() == "postgres" {
			bq = bq.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if e := bq.Take(&balance).Error; e != nil {
			return e
		}
		if e := lockProducts(tx, uniqueProductIDs(lines)); e != nil {
			return e
		}
		needed := map[int64]int64{}
		var total int64
		for i := range lines {
			l := &lines[i]
			var src struct {
				ID, ProductID, QuantityMilli, UnitPriceCents int64
				ProductCode, ProductName, Unit, ProductType  string
				ProductModel, ProductSpecification           *string
			}
			if e := tx.Table("sale_document_item").Where("id=? AND document_id=? AND product_type='GOODS'", l.SaleItemID, h.SaleID).Take(&src).Error; e != nil {
				return ErrConflict
			}
			if src.ProductID != l.ProductID || src.UnitPriceCents != l.UnitPriceCents || src.Unit != l.Unit {
				return ErrConflict
			}
			var sums struct{ Qty, Amount int64 }
			if e := tx.Table("sale_return_document_item i").Select("COALESCE(SUM(i.quantity_milli),0) AS qty,COALESCE(SUM(i.amount_cents),0) AS amount").Joins("JOIN sale_return_document d ON d.id=i.document_id").Where("i.sale_item_id=? AND d.status='POSTED'", l.SaleItemID).Scan(&sums).Error; e != nil {
				return e
			}
			if sums.Qty < 0 || sums.Qty > src.QuantityMilli || l.QuantityMilli > src.QuantityMilli-sums.Qty {
				return fmt.Errorf("%w：原销售明细可退数量不足", ErrConflict)
			}
			if sums.Qty > math.MaxInt64-l.QuantityMilli {
				return ErrInvalid
			}
			target, e := roundAmount(sums.Qty+l.QuantityMilli, l.UnitPriceCents)
			if e != nil || target < sums.Amount {
				return ErrInvalid
			}
			amount := target - sums.Amount
			if total > math.MaxInt64-amount {
				return ErrInvalid
			}
			total += amount
			l.AmountCents = amount
			if needed[l.ProductID] > math.MaxInt64-l.QuantityMilli {
				return ErrInvalid
			}
			needed[l.ProductID] += l.QuantityMilli
		}
		if balance.AmountCents < math.MinInt64+total {
			return ErrInvalid
		}
		for product, qty := range needed {
			var current int64
			if e := tx.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", product).Scan(&current).Error; e != nil {
				return e
			}
			if current > math.MaxInt64-qty {
				return fmt.Errorf("%w：商品%d退货入库后库存超出范围", ErrInvalid, product)
			}
		}
		for i := range lines {
			l := &lines[i]
			before, after, e := increase(tx, l.ProductID, l.QuantityMilli, now)
			if e != nil {
				return e
			}
			if e = tx.Model(&Item{}).Where("id=?", l.ID).Update("amount_cents", l.AmountCents).Error; e != nil {
				return e
			}
			entry := map[string]any{"product_id": l.ProductID, "sale_return_id": id, "sale_return_item_id": l.ID, "entry_type": "ORIGINAL", "quantity_milli": l.QuantityMilli, "balance_before_milli": before, "balance_after_milli": after, "reason": "SALE_RETURN", "remark": l.Remark, "product_code": l.ProductCode, "product_name": l.ProductName, "product_model": l.ProductModel, "product_specification": l.ProductSpecification, "unit": l.Unit, "operator_id": event.Metadata.ActorID, "occurred_at": now, "create_time": now}
			if e = tx.Table("inventory_entry").Create(entry).Error; e != nil {
				return e
			}
		}
		res := tx.Model(&Document{}).Where("id=? AND status='DRAFT' AND version=?", id, version).Updates(map[string]any{"status": "POSTED", "version": version + 1, "posted_by": event.Metadata.ActorID, "posted_at": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrConflict
		}
		after := balance.AmountCents - total
		balanceChanges := map[string]any{"amount_cents": after, "update_time": now}
		if total > 0 {
			balanceChanges["entry_count"] = gorm.Expr("entry_count+1")
		}
		if e := tx.Table("partner_balance").Where("id=?", balance.ID).Updates(balanceChanges).Error; e != nil {
			return e
		}
		if total > 0 {
			financial := map[string]any{"partner_id": h.PartnerID, "direction": "CUSTOMER", "entry_type": "SALE_RETURN", "amount_cents": -total, "balance_before_cents": balance.AmountCents, "balance_after_cents": after, "business_date": h.BusinessDate, "effective_at": now, "description": "销售退货 " + h.DocumentNo, "document_no": h.DocumentNo, "operator_id": event.Metadata.ActorID, "sale_return_id": id, "create_time": now}
			if e := tx.Table("partner_balance_entry").Create(financial).Error; e != nil {
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

// reversePostedReturn undoes a posted sale return: stock goes back out and the
// customer owes the returned amount again. It only accepts the newest still
// effective return per original line, so earlier records stay untouched.
func reversePostedReturn(tx *gorm.DB, id int64, h Document, actor int64) error {
	var origin struct {
		ID, PartnerID int64
		Status        string
	}
	if e := tx.Table("sale_document").Select("id,partner_id,status").Where("id=?", h.SaleID).Take(&origin).Error; e != nil || origin.Status != "POSTED" {
		return ErrConflict
	}
	var bal struct{ ID, AmountCents int64 }
	bq := tx.Table("partner_balance").Where("partner_id=? AND direction='CUSTOMER'", h.PartnerID)
	if tx.Dialector.Name() == "postgres" {
		bq = bq.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if e := bq.Take(&bal).Error; e != nil {
		return e
	}
	var lines []Item
	if e := tx.Where("document_id=?", id).Order("id").Find(&lines).Error; e != nil {
		return e
	}
	if e := lockProducts(tx, uniqueProductIDs(lines)); e != nil {
		return e
	}
	var total int64
	need := map[int64]int64{}
	for i := range lines {
		l := lines[i]
		var latest int64
		e := tx.Table("sale_return_document_item i").Select("i.document_id").Joins("JOIN sale_return_document d ON d.id=i.document_id").Where("i.sale_item_id=? AND d.status='POSTED'", l.SaleItemID).Order("d.posted_at DESC,d.id DESC").Limit(1).Scan(&latest).Error
		if e != nil {
			return e
		}
		if latest != id {
			return fmt.Errorf("%w：同一原明细必须按退货过账逆序取消", ErrConflict)
		}
		if total > math.MaxInt64-l.AmountCents || need[l.ProductID] > math.MaxInt64-l.QuantityMilli {
			return ErrInvalid
		}
		total += l.AmountCents
		need[l.ProductID] += l.QuantityMilli
	}
	if bal.AmountCents > math.MaxInt64-total {
		return ErrInvalid
	}
	now := time.Now().UTC()
	for product, qty := range need {
		var current int64
		if e := tx.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", product).Scan(&current).Error; e != nil {
			return e
		}
		if current < qty {
			return fmt.Errorf("%w：商品%d取消退货后库存不足", ErrConflict, product)
		}
	}
	for _, l := range lines {
		before, after, e := decrease(tx, l.ProductID, l.QuantityMilli, now)
		if e != nil {
			return e
		}
		row := map[string]any{"product_id": l.ProductID, "sale_return_id": id, "sale_return_item_id": l.ID, "entry_type": "REVERSAL", "quantity_milli": -l.QuantityMilli, "balance_before_milli": before, "balance_after_milli": after, "reason": "SALE_RETURN_CANCEL", "remark": h.CancelReason, "product_code": l.ProductCode, "product_name": l.ProductName, "product_model": l.ProductModel, "product_specification": l.ProductSpecification, "unit": l.Unit, "operator_id": actor, "occurred_at": now, "create_time": now}
		if e = tx.Table("inventory_entry").Create(row).Error; e != nil {
			return e
		}
	}
	after := bal.AmountCents + total
	balanceChanges := map[string]any{"amount_cents": after, "update_time": now}
	if total > 0 {
		balanceChanges["entry_count"] = gorm.Expr("entry_count+1")
	}
	if e := tx.Table("partner_balance").Where("id=?", bal.ID).Updates(balanceChanges).Error; e != nil {
		return e
	}
	if total > 0 {
		var orig struct{ ID int64 }
		if e := tx.Table("partner_balance_entry").Select("id").Where("sale_return_id=? AND entry_type='SALE_RETURN'", id).Take(&orig).Error; e != nil {
			return e
		}
		row := map[string]any{"partner_id": h.PartnerID, "direction": "CUSTOMER", "entry_type": "REVERSAL", "amount_cents": total, "balance_before_cents": bal.AmountCents, "balance_after_cents": after, "business_date": h.BusinessDate, "effective_at": now, "description": "取消销售退货 " + h.DocumentNo, "document_no": fmt.Sprintf("SRC%s-%d", now.Format("20060102150405"), id), "operator_id": actor, "reverses_id": orig.ID, "create_time": now}
		if e := tx.Table("partner_balance_entry").Create(row).Error; e != nil {
			return e
		}
	}
	return nil
}

func lockOrigin(tx *gorm.DB, id int64) error {
	if tx.Dialector.Name() == "sqlite" {
		// SQLite has a single writer; the no-op update takes the row write lock
		// so a concurrent post/cancel serializes the same way PostgreSQL does.
		return tx.Exec("UPDATE sale_document SET version=version WHERE id=?", id).Error
	}
	var row struct{ ID int64 }
	return tx.Table("sale_document").Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id=?", id).Take(&row).Error
}

func uniqueProductIDs(lines []Item) []int64 {
	m := map[int64]bool{}
	ids := []int64{}
	for _, l := range lines {
		if !m[l.ProductID] {
			m[l.ProductID] = true
			ids = append(ids, l.ProductID)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func lockProducts(tx *gorm.DB, ids []int64) error {
	for _, id := range ids {
		q := tx.Table("product").Where("id=?", id)
		if tx.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		var row struct{ ID int64 }
		if e := q.Take(&row).Error; e != nil {
			return e
		}
	}
	return nil
}

func decrease(tx *gorm.DB, id, qty int64, now time.Time) (int64, int64, error) {
	res := tx.Exec("UPDATE inventory_balance SET quantity_milli=quantity_milli-?,update_time=? WHERE product_id=? AND quantity_milli>=?", qty, now, id, qty)
	if res.Error != nil {
		return 0, 0, res.Error
	}
	if res.RowsAffected != 1 {
		return 0, 0, fmt.Errorf("%w：库存不足", ErrConflict)
	}
	var after int64
	if e := tx.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", id).Scan(&after).Error; e != nil {
		return 0, 0, e
	}
	return after + qty, after, nil
}

func increase(tx *gorm.DB, id, qty int64, now time.Time) (int64, int64, error) {
	res := tx.Exec("INSERT INTO inventory_balance(product_id,quantity_milli,update_time) VALUES(?,?,?) ON CONFLICT(product_id) DO UPDATE SET quantity_milli=inventory_balance.quantity_milli+excluded.quantity_milli,update_time=excluded.update_time WHERE inventory_balance.quantity_milli<=?", id, qty, now, math.MaxInt64-qty)
	if res.Error != nil {
		return 0, 0, res.Error
	}
	if res.RowsAffected != 1 {
		return 0, 0, fmt.Errorf("%w：库存数量超出范围", ErrInvalid)
	}
	var after int64
	if e := tx.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", id).Scan(&after).Error; e != nil {
		return 0, 0, e
	}
	return after - qty, after, nil
}
func validateSource(tx *gorm.DB, saleID int64, items []Item) error {
	var origin struct {
		ID     int64
		Status string
	}
	q := tx.Table("sale_document").Select("id,status").Where("id=?", saleID)
	if tx.Dialector.Name() == "postgres" {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if e := q.Take(&origin).Error; e != nil || origin.Status != "POSTED" {
		return fmt.Errorf("%w：只能从已过账销售单建立退货", ErrInvalid)
	}
	for i := range items {
		var src struct {
			ID, ProductID, QuantityMilli, UnitPriceCents int64
			ProductCode, ProductName, ProductType, Unit  string
			ProductModel, ProductSpecification           *string
			Remark                                       *string
		}
		if e := tx.Table("sale_document_item").Where("id=? AND document_id=?", items[i].SaleItemID, saleID).Take(&src).Error; e != nil {
			return fmt.Errorf("%w：第%d行原销售明细不属于所选已过账销售单", ErrInvalid, i+1)
		}
		if src.ProductType != "GOODS" {
			return fmt.Errorf("%w：销售服务明细不能退货", ErrInvalid)
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
	if q.SaleID > 0 {
		d = d.Where("sale_id=?", q.SaleID)
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
	e := db.Table("sale_return_document d").Select("d.*,sd.document_no AS sale_no,p.name AS partner_name,COALESCE(NULLIF(c.nickname,''),c.username) AS created_by_name,COALESCE(NULLIF(u.nickname,''),u.username) AS cancelled_by_name").Joins("JOIN sale_document sd ON sd.id=d.sale_id").Joins("JOIN partner p ON p.id=d.partner_id").Joins("LEFT JOIN sys_user c ON c.id=d.created_by").Joins("LEFT JOIN sys_user u ON u.id=d.cancelled_by").Where("d.id=?", id).Take(&h).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if e != nil {
		return nil, e
	}
	var items []Item
	e = db.Table("sale_return_document_item i").Select("i.*").Where("i.document_id=?", id).Joins("JOIN sale_document_item si ON si.id=i.sale_item_id").Order("i.id").Find(&items).Error
	if e != nil {
		return nil, e
	}
	total := int64(0)
	for i := range items {
		l := &items[i]
		var original int64
		if e = db.Table("sale_document_item").Select("quantity_milli").Where("id=?", l.SaleItemID).Scan(&original).Error; e != nil {
			return nil, e
		}
		l.OriginalQuantityMilli = original
		l.OriginalQuantity = milliText(original)
		var sums struct{ Qty, Amount int64 }
		if e = db.Table("sale_return_document_item i").Select("COALESCE(SUM(i.quantity_milli),0) AS qty,COALESCE(SUM(i.amount_cents),0) AS amount").Joins("JOIN sale_return_document d ON d.id=i.document_id").Where("i.sale_item_id=? AND d.status='POSTED' AND d.id<>?", l.SaleItemID, id).Scan(&sums).Error; e != nil {
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
		if sums.Qty > math.MaxInt64-l.QuantityMilli {
			return nil, ErrInvalid
		}
		target, e := roundAmount(sums.Qty+l.QuantityMilli, l.UnitPriceCents)
		if e != nil {
			return nil, e
		}
		if h.Status == "DRAFT" {
			l.AmountCents = target - sums.Amount
			if l.AmountCents < 0 {
				l.AmountCents = 0
			}
		}
		l.Amount = moneyText(l.AmountCents)
		if total > math.MaxInt64-l.AmountCents {
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
	if e != nil && strings.Contains(strings.ToLower(e.Error()), "duplicate key") {
		return ErrConflict
	}
	return e
}

func (r *Repository) Source(ctx context.Context, saleID int64) (*Document, error) {
	var out Document
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var origin struct {
			ID, PartnerID                    int64
			DocumentNo, BusinessDate, Status string
		}
		if e := tx.Table("sale_document").Select("id,partner_id,document_no,business_date,status").Where("id=?", saleID).Take(&origin).Error; e != nil {
			return e
		}
		if origin.Status != "POSTED" {
			return ErrInvalid
		}
		out = Document{SaleID: origin.ID, SaleNo: origin.DocumentNo, PartnerID: origin.PartnerID, BusinessDate: time.Now().UTC().Format("2006-01-02"), Status: "DRAFT", Version: 1, Items: []Item{}}
		tx.Table("partner").Select("name").Where("id=?", origin.PartnerID).Scan(&out.PartnerName)
		var src []struct {
			ID, ProductID, QuantityMilli, UnitPriceCents, AmountCents int64
			ProductCode, ProductName                                  string
			ProductModel, ProductSpecification                        *string
			ProductType, Unit                                         string
			Remark                                                    *string
		}
		if e := tx.Table("sale_document_item").Where("document_id=? AND product_type='GOODS'", saleID).Order("id").Find(&src).Error; e != nil {
			return e
		}
		for _, v := range src {
			l := Item{SaleItemID: v.ID, ProductID: v.ProductID, ProductCode: v.ProductCode, ProductName: v.ProductName, ProductModel: v.ProductModel, ProductSpecification: v.ProductSpecification, Unit: v.Unit, OriginalQuantityMilli: v.QuantityMilli, OriginalQuantity: milliText(v.QuantityMilli), UnitPriceCents: v.UnitPriceCents, UnitPrice: moneyText(v.UnitPriceCents), AmountCents: v.AmountCents, Amount: moneyText(v.AmountCents), ReturnedQuantity: "0", RemainingQuantityMilli: v.QuantityMilli, RemainingQuantity: milliText(v.QuantityMilli), QuantityMilli: 0, Quantity: "0", Remark: v.Remark}
			var sums struct{ Qty, Amount int64 }
			if e := tx.Table("sale_return_document_item i").Select("COALESCE(SUM(i.quantity_milli),0) AS qty,COALESCE(SUM(i.amount_cents),0) AS amount").Joins("JOIN sale_return_document d ON d.id=i.document_id").Where("i.sale_item_id=? AND d.status='POSTED'", v.ID).Scan(&sums).Error; e != nil {
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
