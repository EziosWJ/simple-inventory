package purchase

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
	"github.com/EziosWJ/simple-inventory/server/internal/directdelivery"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
		if e := directdelivery.LockPurchase(tx, id, &old); e != nil {
			return e
		}
		if old.Status != "DRAFT" || old.Version != version {
			return ErrConflict
		}
		if old.DirectDelivery && !h.DirectDelivery {
			if e := directdelivery.RejectActiveSale(tx, id); e != nil {
				return fmt.Errorf("%w：%v", ErrConflict, e)
			}
		}
		if e := validPartnerProduct(tx, h.PartnerID, lines); e != nil {
			return e
		}
		res := tx.Model(&Draft{}).Where("id=? AND status='DRAFT' AND version=?", id, version).Updates(map[string]any{"direct_delivery": h.DirectDelivery, "partner_id": h.PartnerID, "business_date": h.BusinessDate, "remark": h.Remark, "version": version + 1})
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
		var old Draft
		if x := directdelivery.LockPurchase(tx, id, &old); x != nil {
			return x
		}
		if old.Version != version || (old.Status != "DRAFT" && old.Status != "POSTED") {
			return ErrConflict
		}
		if e := directdelivery.RejectActiveSale(tx, id); e != nil {
			return fmt.Errorf("%w：%v", ErrConflict, e)
		}
		if old.Status == "POSTED" {
			var activeReturn struct{ ID int64 }
			if e := tx.Table("purchase_return_document").Select("id").Where("purchase_id=? AND status='POSTED'", id).Limit(1).Take(&activeReturn).Error; e == nil {
				return fmt.Errorf("%w：原采购单仍有已过账退货，必须先取消退货", ErrConflict)
			} else if !errors.Is(e, gorm.ErrRecordNotFound) {
				return e
			}
		}
		res := tx.Model(&Draft{}).Where("id=? AND status=? AND version=?", id, old.Status, version).Updates(map[string]any{"status": "CANCELLED", "version": version + 1, "cancelled_by": event.Metadata.ActorID, "cancelled_at": now, "cancel_reason": reason})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrConflict
		}
		if old.Status == "POSTED" {
			reasonCopy := reason
			old.CancelReason = &reasonCopy
			if x := reversePurchase(tx, old, id, now, event.Metadata.ActorID); x != nil {
				return x
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

func reversePurchase(tx *gorm.DB, h Draft, id int64, now time.Time, actor int64) error {
	// Keep balance-first ordering consistent with purchase posting and settlement.
	var b struct{ ID, AmountCents int64 }
	q := tx.Table("partner_balance").Where("partner_id=? AND direction='SUPPLIER'", h.PartnerID)
	if tx.Dialector.Name() == "postgres" {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if e := q.Take(&b).Error; e != nil {
		return e
	}
	lines := []Line{}
	if e := tx.Where("document_id=?", id).Order("id DESC").Find(&lines).Error; e != nil {
		return e
	}
	ids := uniqueProductIDs(lines)
	if e := lockProducts(tx, ids); e != nil {
		return e
	}
	need := map[int64]int64{}
	var total int64
	for _, l := range lines {
		if need[l.ProductID] > math.MaxInt64-l.QuantityMilli || total > math.MaxInt64-l.AmountCents {
			return ErrInvalid
		}
		need[l.ProductID] += l.QuantityMilli
		total += l.AmountCents
	}
	for productID, qty := range need {
		var current int64
		if e := tx.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", productID).Scan(&current).Error; e != nil {
			return e
		}
		if current < qty {
			return fmt.Errorf("%w：商品%d当前库存不足，采购单未取消", ErrConflict, productID)
		}
	}
	for _, l := range lines {
		before, after, e := decreaseStock(tx, l.ProductID, l.QuantityMilli, now)
		if e != nil {
			return e
		}
		row := map[string]any{"product_id": l.ProductID, "purchase_id": id, "purchase_item_id": l.ID, "entry_type": "REVERSAL", "quantity_milli": -l.QuantityMilli, "balance_before_milli": before, "balance_after_milli": after, "reason": "PURCHASE_CANCEL", "remark": h.CancelReason, "product_code": l.ProductCode, "product_name": l.ProductName, "product_model": l.ProductModel, "product_specification": l.ProductSpecification, "unit": l.Unit, "operator_id": actor, "occurred_at": now, "create_time": now}
		if e = tx.Table("inventory_entry").Create(row).Error; e != nil {
			return e
		}
	}
	if b.AmountCents < math.MinInt64+total {
		return ErrInvalid
	}
	if total == 0 {
		return nil
	}

	after := b.AmountCents - total
	res := tx.Table("partner_balance").Where("id=?", b.ID).Updates(map[string]any{"amount_cents": after, "entry_count": gorm.Expr("entry_count+1"), "update_time": now})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected != 1 {
		return ErrConflict
	}
	var original struct{ ID int64 }
	if e := tx.Table("partner_balance_entry").Select("id").Where("purchase_id=? AND entry_type='PURCHASE'", id).Take(&original).Error; e != nil {
		return e
	}
	docNo := fmt.Sprintf("PR%s-%d", now.Format("20060102150405"), id)
	rev := map[string]any{"partner_id": h.PartnerID, "direction": "SUPPLIER", "entry_type": "REVERSAL", "amount_cents": -total, "balance_before_cents": b.AmountCents, "balance_after_cents": after, "business_date": h.BusinessDate, "effective_at": now, "description": "取消采购入库 " + h.DocumentNo + "：" + *h.CancelReason, "document_no": docNo, "operator_id": actor, "reverses_id": original.ID, "create_time": now}
	return tx.Table("partner_balance_entry").Create(rev).Error
}
func decreaseStock(tx *gorm.DB, productID, delta int64, now time.Time) (int64, int64, error) {
	res := tx.Exec("UPDATE inventory_balance SET quantity_milli=quantity_milli-?,update_time=? WHERE product_id=? AND quantity_milli>=?", delta, now, productID, delta)
	if res.Error != nil {
		return 0, 0, res.Error
	}
	if res.RowsAffected != 1 {
		return 0, 0, fmt.Errorf("%w：库存不足", ErrConflict)
	}
	var after int64
	if e := tx.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", productID).Scan(&after).Error; e != nil {
		return 0, 0, e
	}
	return after + delta, after, nil
}

func (r *Repository) Post(ctx context.Context, id, version int64, event audit.Event) (Draft, error) {
	var out Draft
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		h, e := lockPurchase(tx, id, version)
		if e != nil {
			return e
		}
		lines := []Line{}
		if e = tx.Where("document_id=?", id).Order("id").Find(&lines).Error; e != nil {
			return e
		}
		if len(lines) == 0 {
			return ErrInvalid
		}
		var partner struct {
			ID         int64
			IsSupplier bool `gorm:"column:is_supplier"`
		}
		if e = tx.Table("partner").Select("id,is_supplier").Where("id=? AND status=1", h.PartnerID).Take(&partner).Error; e != nil || !partner.IsSupplier {
			return fmt.Errorf("%w：供应商已停用或身份已变化", ErrInvalid)
		}
		// Lock the first payable balance before any product balance, including
		// when the supplier has never had a balance row.
		if e = tx.Exec("INSERT INTO partner_balance(partner_id,direction,amount_cents,entry_count,update_time) VALUES (?, 'SUPPLIER', 0, 0, ?) ON CONFLICT(partner_id,direction) DO NOTHING", h.PartnerID, time.Now().UTC()).Error; e != nil {
			return e
		}
		var payable struct{ ID, AmountCents int64 }
		bq := tx.Table("partner_balance").Where("partner_id=? AND direction='SUPPLIER'", h.PartnerID)
		if tx.Dialector.Name() == "postgres" {
			bq = bq.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if e = bq.Take(&payable).Error; e != nil {
			return e
		}
		balanceBefore := payable.AmountCents
		productIDs := uniqueProductIDs(lines)
		if e = lockProducts(tx, productIDs); e != nil {
			return e
		}
		for i := range lines {
			var p struct {
				Code, Name           string
				Model, Specification *string
				Type, Unit           string
				Status               int
			}
			if e = tx.Table("product").Select("code,name,model,specification,type,unit,status").Where("id=?", lines[i].ProductID).Take(&p).Error; e != nil {
				return fmt.Errorf("%w：商品不存在", ErrInvalid)
			}
			if p.Status != 1 || p.Type != "GOODS" || p.Type != lines[i].ProductType || p.Unit != lines[i].Unit {
				return fmt.Errorf("%w：商品已停用或类型/单位已变化", ErrInvalid)
			}
			lines[i].ProductCode, lines[i].ProductName, lines[i].ProductModel, lines[i].ProductSpecification = p.Code, p.Name, p.Model, p.Specification
			if payable.AmountCents > math.MaxInt64-lines[i].AmountCents {
				return fmt.Errorf("%w：应付金额超出范围", ErrInvalid)
			}
			payable.AmountCents += lines[i].AmountCents
		}
		// Reject integer overflow before touching any ledger row.
		var total int64
		for _, line := range lines {
			if total > math.MaxInt64-line.AmountCents {
				return fmt.Errorf("%w：单据金额超出范围", ErrInvalid)
			}
			total += line.AmountCents
		}
		now := time.Now().UTC()
		for i := range lines {
			before, after, er := changeStock(tx, lines[i].ProductID, lines[i].QuantityMilli, now)
			if er != nil {
				return er
			}
			if er = tx.Model(&Line{}).Where("id=?", lines[i].ID).Updates(map[string]any{"product_code": lines[i].ProductCode, "product_name": lines[i].ProductName, "product_model": lines[i].ProductModel, "product_specification": lines[i].ProductSpecification}).Error; er != nil {
				return er
			}
			entry := map[string]any{"product_id": lines[i].ProductID, "purchase_id": id, "purchase_item_id": lines[i].ID, "entry_type": "ORIGINAL", "quantity_milli": lines[i].QuantityMilli, "balance_before_milli": before, "balance_after_milli": after, "reason": "PURCHASE", "remark": lines[i].Remark, "product_code": lines[i].ProductCode, "product_name": lines[i].ProductName, "product_model": lines[i].ProductModel, "product_specification": lines[i].ProductSpecification, "unit": lines[i].Unit, "operator_id": event.Metadata.ActorID, "occurred_at": now, "create_time": now}
			if er = tx.Table("inventory_entry").Create(entry).Error; er != nil {
				return er
			}
		}
		changed := tx.Model(&Draft{}).Where("id=? AND status='DRAFT' AND version=?", id, version).Updates(map[string]any{"status": "POSTED", "version": version + 1, "posted_by": event.Metadata.ActorID, "posted_at": now})
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected != 1 {
			return ErrConflict
		}
		if total > 0 {
			res := tx.Table("partner_balance").Where("id=?", payable.ID).Updates(map[string]any{"amount_cents": balanceBefore + total, "entry_count": gorm.Expr("entry_count+1"), "update_time": now})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return ErrConflict
			}
			financial := map[string]any{"partner_id": h.PartnerID, "direction": "SUPPLIER", "entry_type": "PURCHASE", "amount_cents": total, "balance_before_cents": balanceBefore, "balance_after_cents": balanceBefore + total, "business_date": h.BusinessDate, "effective_at": now, "description": "采购入库 " + h.DocumentNo, "document_no": h.DocumentNo, "operator_id": event.Metadata.ActorID, "purchase_id": id, "create_time": now}
			if e = tx.Table("partner_balance_entry").Create(financial).Error; e != nil {
				return e
			}
		}
		event.ResourceID = id
		if e = audit.RecordOn(ctx, tx, event); e != nil {
			return e
		}
		stored, e := find(tx, id)
		if e != nil {
			return e
		}
		out = *stored
		return nil
	})
	if err != nil {
		return out, mapErr(err)
	}
	return out, nil
}

func lockPurchase(tx *gorm.DB, id, version int64) (Draft, error) {
	var h Draft
	if e := directdelivery.LockPurchase(tx, id, &h); e != nil {
		return h, e
	}
	if h.Status != "DRAFT" || h.Version != version {
		return h, ErrConflict
	}
	return h, nil
}
func uniqueProductIDs(lines []Line) []int64 {
	m := map[int64]bool{}
	ids := make([]int64, 0, len(lines))
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
		var p struct{ ID int64 }
		if e := q.Take(&p).Error; e != nil {
			return e
		}
	}
	return nil
}
func changeStock(tx *gorm.DB, productID, delta int64, now time.Time) (int64, int64, error) {
	res := tx.Exec("INSERT INTO inventory_balance(product_id,quantity_milli,update_time) VALUES(?,?,?) ON CONFLICT(product_id) DO UPDATE SET quantity_milli=inventory_balance.quantity_milli+excluded.quantity_milli,update_time=excluded.update_time WHERE inventory_balance.quantity_milli<=?", productID, delta, now, math.MaxInt64-delta)
	if res.Error != nil {
		return 0, 0, res.Error
	}
	if res.RowsAffected != 1 {
		return 0, 0, fmt.Errorf("%w：库存数量超出范围", ErrInvalid)
	}
	var after int64
	if e := tx.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", productID).Scan(&after).Error; e != nil {
		return 0, 0, e
	}
	return after - delta, after, nil
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
	e := db.Table("purchase_document d").Select("d.*,p.name AS partner_name,COALESCE(NULLIF(c.nickname,''),c.username) AS created_by_name,COALESCE(NULLIF(u.nickname,''),u.username) AS cancelled_by_name,COALESCE(NULLIF(pp.nickname,''),pp.username) AS posted_by_name").Joins("JOIN partner p ON p.id=d.partner_id").Joins("LEFT JOIN sys_user c ON c.id=d.created_by").Joins("LEFT JOIN sys_user u ON u.id=d.cancelled_by").Joins("LEFT JOIN sys_user pp ON pp.id=d.posted_by").Where("d.id=?", id).Take(&h).Error
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
	h.DirectTrace, e = directdelivery.PurchaseTrace(db, id)
	if e != nil {
		return nil, e
	}
	h.DirectDocuments, e = directdelivery.PurchaseSales(db, id)
	if e != nil {
		return nil, e
	}
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
	if e != nil && (strings.Contains(strings.ToLower(e.Error()), "unique constraint") || strings.Contains(strings.ToLower(e.Error()), "database is locked") || strings.Contains(strings.ToLower(e.Error()), "database table is locked")) {
		return ErrConflict
	}
	return e
}
