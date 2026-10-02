package sale

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
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db} }

func (r *Repository) Create(ctx context.Context, h Draft, lines []Line, event audit.Event) (Draft, error) {
	var out Draft
	var nonce [10]byte
	if _, e := rand.Read(nonce[:]); e != nil {
		return out, e
	}
	h.DocumentNo = "SO" + time.Now().UTC().Format("20060102") + "-" + hex.EncodeToString(nonce[:])
	h.CreateTime = time.Now().UTC()
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if replay, e := beginSave(tx, event.Metadata.ActorID, h.SaveRequest); e != nil {
			return e
		} else if replay != nil {
			out = *replay
			return nil
		}
		if e := validateDirect(tx, h, 0, false, lines); e != nil {
			return e
		}
		if e := validPartnerProduct(tx, h.PartnerID, lines); e != nil {
			return e
		}
		var p struct{ Contact, Phone, Address *string }
		if e := tx.Table("partner").Select("contact,phone,address").Where("id=?", h.PartnerID).Take(&p).Error; e != nil {
			return e
		}
		if h.DeliveryContact == nil {
			h.DeliveryContact = copyString(p.Contact)
		}
		if h.DeliveryPhone == nil {
			h.DeliveryPhone = copyString(p.Phone)
		}
		if h.DeliveryAddress == nil {
			h.DeliveryAddress = copyString(p.Address)
		}
		if e := tx.Create(&h).Error; e != nil {
			return e
		}
		for i := range lines {
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
			e = finishSave(tx, event.Metadata.ActorID, h.SaveRequest, &out)
		}
		return e
	})
	if e != nil {
		return out, mapErr(e)
	}
	return out, nil
}
func copyString(v *string) *string {
	if v == nil {
		return nil
	}
	x := *v
	return &x
}
func (r *Repository) Edit(ctx context.Context, id, version int64, h Draft, lines []Line, event audit.Event) (Draft, error) {
	var out Draft
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if replay, e := beginSave(tx, event.Metadata.ActorID, h.SaveRequest); e != nil {
			return e
		} else if replay != nil {
			out = *replay
			return nil
		}
		var prior Draft
		if e := tx.Where("id=?", id).Take(&prior).Error; e != nil {
			return e
		}
		if prior.DirectPurchaseID != nil && !samePurchase(prior.DirectPurchaseID, h.DirectPurchaseID) {
			return fmt.Errorf("%w：已关联销售须取消后重新建立，保留历史关联", ErrConflict)
		}
		if e := validateDirect(tx, h, id, false, lines); e != nil {
			return e
		}
		old, e := lockDraft(tx, id, version)
		if e != nil {
			return e
		}
		if old.DirectPurchaseID != nil && (h.DirectPurchaseID == nil || *old.DirectPurchaseID != *h.DirectPurchaseID) {
			return fmt.Errorf("%w：已关联销售须取消后重新建立，保留历史关联", ErrConflict)
		}
		if e = validPartnerProduct(tx, h.PartnerID, lines); e != nil {
			return e
		}
		res := tx.Model(&Draft{}).Where("id=? AND status='DRAFT' AND version=?", id, version).Updates(map[string]any{"direct_delivery": h.DirectDelivery, "direct_purchase_id": h.DirectPurchaseID, "partner_id": h.PartnerID, "business_date": h.BusinessDate, "remark": h.Remark, "delivery_contact": h.DeliveryContact, "delivery_phone": h.DeliveryPhone, "delivery_address": h.DeliveryAddress, "version": version + 1})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrConflict
		}
		// Keep each submitted line's identity stable when its product/price remains; replace the full set atomically.
		var previous []Line
		if e = tx.Where("document_id=?", id).Order("id ASC").Find(&previous).Error; e != nil {
			return e
		}
		oldByID := make(map[int64]Line, len(previous))
		for _, oldLine := range previous {
			oldByID[oldLine.ID] = oldLine
		}
		kept := map[int64]bool{}
		for i := range lines {
			lines[i].DocumentID = id
			if lines[i].ID != 0 {
				if _, ok := oldByID[lines[i].ID]; !ok {
					return ErrConflict
				}
				kept[lines[i].ID] = true
				if e = tx.Model(&Line{}).Where("id=? AND document_id=?", lines[i].ID, id).Updates(map[string]any{"product_id": lines[i].ProductID, "product_code": lines[i].ProductCode, "product_name": lines[i].ProductName, "product_model": lines[i].ProductModel, "product_specification": lines[i].ProductSpecification, "product_type": lines[i].ProductType, "unit": lines[i].Unit, "quantity_milli": lines[i].QuantityMilli, "unit_price_cents": lines[i].UnitPriceCents, "amount_cents": lines[i].AmountCents, "remark": lines[i].Remark}).Error; e != nil {
					return e
				}
			} else if e = tx.Create(&lines[i]).Error; e != nil {
				return e
			}
		}
		var removed []int64
		for _, oldLine := range previous {
			if !kept[oldLine.ID] {
				removed = append(removed, oldLine.ID)
			}
		}
		if len(removed) > 0 {
			if e = tx.Where("document_id=? AND id IN ?", id, removed).Delete(&Line{}).Error; e != nil {
				return e
			}
		}
		event.ResourceID = id
		if e = audit.RecordOn(ctx, tx, event); e != nil {
			return e
		}
		v, e := find(tx, id)
		if e == nil {
			out = *v
			e = finishSave(tx, event.Metadata.ActorID, h.SaveRequest, &out)
		}
		_ = old
		return e
	})
	if e != nil {
		return out, mapErr(e)
	}
	return out, nil
}
func lineIDs(v []Line) []int64 {
	ids := make([]int64, len(v))
	for i, x := range v {
		ids[i] = x.ID
	}
	return ids
}
func (r *Repository) Cancel(ctx context.Context, id, version int64, reason string, event audit.Event) (Draft, error) {
	var out Draft
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		h, e := lockDocument(tx, id, version)
		if e != nil {
			return e
		}
		now := time.Now().UTC()
		if h.Status == "POSTED" {
			if err := rejectSaleWithReturns(tx, id); err != nil {
				return err
			}
			if err := reverseSale(tx, h, id, reason, &now, event.Metadata.ActorID); err != nil {
				return err
			}
		}
		res := tx.Model(&Draft{}).Where("id=? AND status=? AND version=?", id, h.Status, version).Updates(map[string]any{"status": "CANCELLED", "version": version + 1, "cancelled_by": event.Metadata.ActorID, "cancelled_at": now, "cancel_reason": reason})
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
func lockDocument(tx *gorm.DB, id, version int64) (Draft, error) {
	var h Draft
	var before Draft
	if e := tx.Where("id=?", id).Take(&before).Error; e != nil {
		return h, e
	}
	if before.DirectPurchaseID != nil {
		var p directdelivery.Purchase
		if e := directdelivery.LockPurchase(tx, *before.DirectPurchaseID, &p); e != nil {
			return h, e
		}
	}
	q := tx.Where("id=?", id)
	if tx.Dialector.Name() == "postgres" {
		q = q.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if e := q.Take(&h).Error; e != nil {
		return h, e
	}
	if !samePurchase(before.DirectPurchaseID, h.DirectPurchaseID) {
		return h, ErrConflict
	}
	if (h.Status != "DRAFT" && h.Status != "POSTED") || h.Version != version {
		return h, ErrConflict
	}
	return h, nil
}

func lockDraft(tx *gorm.DB, id, version int64) (Draft, error) {
	h, e := lockDocument(tx, id, version)
	if e != nil {
		return h, e
	}
	if h.Status != "DRAFT" {
		return h, ErrConflict
	}
	return h, nil
}

// Post locks the receivable first and products in ascending ID order, matching
// purchase posting/cancellation and the settlement write path.
func (r *Repository) Post(ctx context.Context, id, version int64, event audit.Event) (Draft, error) {
	var out Draft
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		h, e := lockDraft(tx, id, version)
		if e != nil {
			return e
		}
		lines := []Line{}
		if e = tx.Where("document_id=?", id).Order("id ASC").Find(&lines).Error; e != nil {
			return e
		}
		if len(lines) == 0 {
			return ErrInvalid
		}
		if e = validateDirect(tx, h, id, true, lines); e != nil {
			return e
		}
		var partner struct {
			ID         int64
			Name       string
			IsCustomer bool `gorm:"column:is_customer"`
		}
		if e = tx.Table("partner").Select("id,name,is_customer").Where("id=? AND status=1", h.PartnerID).Take(&partner).Error; e != nil || !partner.IsCustomer {
			return fmt.Errorf("%w：客户已停用或身份已变化", ErrInvalid)
		}
		if e = tx.Exec("INSERT INTO partner_balance(partner_id,direction,amount_cents,entry_count,update_time) VALUES (?, 'CUSTOMER', 0, 0, ?) ON CONFLICT(partner_id,direction) DO NOTHING", h.PartnerID, time.Now().UTC()).Error; e != nil {
			return e
		}
		var balance struct{ ID, AmountCents int64 }
		bq := tx.Table("partner_balance").Where("partner_id=? AND direction='CUSTOMER'", h.PartnerID)
		if tx.Dialector.Name() == "postgres" {
			bq = bq.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if e = bq.Take(&balance).Error; e != nil {
			return e
		}
		productIDs := uniqueProductIDs(lines)
		if e = lockProducts(tx, productIDs); e != nil {
			return e
		}
		need := map[int64]int64{}
		var total int64
		for i := range lines {
			l := &lines[i]
			var p struct {
				Code, Name           string
				Model, Specification *string
				Type, Unit           string
				Status               int
			}
			if e = tx.Table("product").Select("code,name,model,specification,type,unit,status").Where("id=?", l.ProductID).Take(&p).Error; e != nil {
				return fmt.Errorf("%w：商品或服务不存在", ErrInvalid)
			}
			if p.Status != 1 || p.Type != l.ProductType || p.Unit != l.Unit || (p.Type != "GOODS" && p.Type != "SERVICE") {
				return fmt.Errorf("%w：商品或服务已停用或类型/单位已变化", ErrInvalid)
			}
			l.ProductCode, l.ProductName, l.ProductModel, l.ProductSpecification = p.Code, p.Name, copyString(p.Model), copyString(p.Specification)
			if total > math.MaxInt64-l.AmountCents {
				return fmt.Errorf("%w：单据金额超出范围", ErrInvalid)
			}
			total += l.AmountCents
			if p.Type == "GOODS" {
				if need[l.ProductID] > math.MaxInt64-l.QuantityMilli {
					return fmt.Errorf("%w：商品数量超出范围", ErrInvalid)
				}
				need[l.ProductID] += l.QuantityMilli
			}
		}
		if balance.AmountCents > math.MaxInt64-total {
			return fmt.Errorf("%w：应收金额超出范围", ErrInvalid)
		}
		for productID, qty := range need {
			var current int64
			if e = tx.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", productID).Scan(&current).Error; e != nil {
				return e
			}
			if current < qty {
				return fmt.Errorf("%w：商品%d库存不足，销售单未过账", ErrConflict, productID)
			}
		}
		profile, er := readOwnerProfile(tx)
		if er != nil {
			return er
		}
		now := time.Now().UTC()
		for _, l := range lines {
			if l.ProductType != "GOODS" {
				continue
			}
			before, after, er := decreaseSaleStock(tx, l.ProductID, l.QuantityMilli, now)
			if er != nil {
				return er
			}
			if er = tx.Model(&Line{}).Where("id=? AND document_id=?", l.ID, id).Updates(map[string]any{"product_code": l.ProductCode, "product_name": l.ProductName, "product_model": l.ProductModel, "product_specification": l.ProductSpecification}).Error; er != nil {
				return er
			}
			entry := map[string]any{"product_id": l.ProductID, "sale_id": id, "sale_item_id": l.ID, "entry_type": "ORIGINAL", "quantity_milli": -l.QuantityMilli, "balance_before_milli": before, "balance_after_milli": after, "reason": "SALE", "remark": l.Remark, "product_code": l.ProductCode, "product_name": l.ProductName, "product_model": l.ProductModel, "product_specification": l.ProductSpecification, "unit": l.Unit, "operator_id": event.Metadata.ActorID, "occurred_at": now, "create_time": now}
			if er = tx.Table("inventory_entry").Create(entry).Error; er != nil {
				return er
			}
		}
		for _, l := range lines {
			if l.ProductType == "SERVICE" {
				if e = tx.Model(&Line{}).Where("id=? AND document_id=?", l.ID, id).Updates(map[string]any{"product_code": l.ProductCode, "product_name": l.ProductName, "product_model": l.ProductModel, "product_specification": l.ProductSpecification}).Error; e != nil {
					return e
				}
			}
		}
		changed := tx.Model(&Draft{}).Where("id=? AND status='DRAFT' AND version=?", id, version).Updates(map[string]any{"status": "POSTED", "version": version + 1, "posted_by": event.Metadata.ActorID, "posted_at": now, "partner_name": partner.Name, "owner_name": profile.Name, "owner_phone": profile.Phone, "owner_address": profile.Address})
		if changed.Error != nil {
			return changed.Error
		}
		if changed.RowsAffected != 1 {
			return ErrConflict
		}
		after := balance.AmountCents + total
		if total > 0 {
			res := tx.Table("partner_balance").Where("id=?", balance.ID).Updates(map[string]any{"amount_cents": after, "entry_count": gorm.Expr("entry_count+1"), "update_time": now})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return ErrConflict
			}
			financial := map[string]any{"partner_id": h.PartnerID, "direction": "CUSTOMER", "entry_type": "SALE", "amount_cents": total, "balance_before_cents": balance.AmountCents, "balance_after_cents": after, "business_date": h.BusinessDate, "effective_at": now, "description": "销售出库 " + h.DocumentNo, "document_no": h.DocumentNo, "operator_id": event.Metadata.ActorID, "sale_id": id, "create_time": now}
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

func readOwnerProfile(tx *gorm.DB) (v struct{ Name, Phone, Address string }, err error) {
	var rows []struct {
		ConfigKey   string `gorm:"column:config_key"`
		ConfigValue string `gorm:"column:config_value"`
	}
	err = tx.Table("sys_config").Select("config_key,config_value").Where("config_key IN ? AND deleted=0", []string{"business.print-profile.name", "business.print-profile.phone", "business.print-profile.address"}).Find(&rows).Error
	if err != nil {
		return v, err
	}
	for _, r := range rows {
		switch r.ConfigKey {
		case "business.print-profile.name":
			v.Name = r.ConfigValue
		case "business.print-profile.phone":
			v.Phone = r.ConfigValue
		case "business.print-profile.address":
			v.Address = r.ConfigValue
		}
	}
	return v, nil
}

func decreaseSaleStock(tx *gorm.DB, productID, delta int64, now time.Time) (int64, int64, error) {
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

func uniqueProductIDs(lines []Line) []int64 {
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
		var p struct{ ID int64 }
		if e := q.Take(&p).Error; e != nil {
			return e
		}
	}
	return nil
}

func rejectSaleWithReturns(tx *gorm.DB, id int64) error {
	// The return module lands in a later Phase 4 ticket. Keeping this guard at
	// the cancellation boundary makes the constraint effective as soon as that
	// table is installed without creating a parallel return model here.
	if !tx.Migrator().HasTable("sale_return_document") {
		return nil
	}
	var count int64
	if e := tx.Table("sale_return_document").Where("sale_id=? AND status='POSTED'", id).Count(&count).Error; e != nil {
		return e
	}
	if count > 0 {
		return fmt.Errorf("%w：该销售单存在已生效退货，不能取消", ErrConflict)
	}
	return nil
}

func reverseSale(tx *gorm.DB, h Draft, id int64, reason string, occurred *time.Time, actor int64) error {
	var b struct{ ID, AmountCents int64 }
	bq := tx.Table("partner_balance").Where("partner_id=? AND direction='CUSTOMER'", h.PartnerID)
	if tx.Dialector.Name() == "postgres" {
		bq = bq.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	if e := bq.Take(&b).Error; e != nil {
		return e
	}
	lines := []Line{}
	if e := tx.Where("document_id=?", id).Order("id DESC").Find(&lines).Error; e != nil {
		return e
	}
	productIDs := uniqueProductIDs(lines)
	if e := lockProducts(tx, productIDs); e != nil {
		return e
	}
	need := map[int64]int64{}
	var total int64
	for _, l := range lines {
		if total > math.MaxInt64-l.AmountCents {
			return ErrInvalid
		}
		total += l.AmountCents
		if l.ProductType == "GOODS" {
			if need[l.ProductID] > math.MaxInt64-l.QuantityMilli {
				return ErrInvalid
			}
			need[l.ProductID] += l.QuantityMilli
		}
	}
	for productID, qty := range need {
		var current int64
		if e := tx.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", productID).Scan(&current).Error; e != nil {
			return e
		}
		if current > math.MaxInt64-qty {
			return fmt.Errorf("%w：商品%d冲销后库存超出范围，销售单未取消", ErrInvalid, productID)
		}
	}
	now := time.Now().UTC()
	*occurred = now
	for _, l := range lines {
		if l.ProductType != "GOODS" {
			continue
		}
		before, after, e := increaseSaleStock(tx, l.ProductID, l.QuantityMilli, now)
		if e != nil {
			return e
		}
		row := map[string]any{"product_id": l.ProductID, "sale_id": id, "sale_item_id": l.ID, "entry_type": "REVERSAL", "quantity_milli": l.QuantityMilli, "balance_before_milli": before, "balance_after_milli": after, "reason": "SALE_CANCEL", "remark": reason, "product_code": l.ProductCode, "product_name": l.ProductName, "product_model": l.ProductModel, "product_specification": l.ProductSpecification, "unit": l.Unit, "operator_id": actor, "occurred_at": now, "create_time": now}
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
	if e := tx.Table("partner_balance_entry").Select("id").Where("sale_id=? AND entry_type='SALE'", id).Take(&original).Error; e != nil {
		return e
	}
	docNo := fmt.Sprintf("SR%s-%d", now.Format("20060102150405"), id)
	rev := map[string]any{"partner_id": h.PartnerID, "direction": "CUSTOMER", "entry_type": "REVERSAL", "amount_cents": -total, "balance_before_cents": b.AmountCents, "balance_after_cents": after, "business_date": h.BusinessDate, "effective_at": now, "description": "取消销售出库 " + h.DocumentNo + "：" + reason, "document_no": docNo, "operator_id": actor, "reverses_id": original.ID, "create_time": now}
	return tx.Table("partner_balance_entry").Create(rev).Error
}

func increaseSaleStock(tx *gorm.DB, productID, delta int64, now time.Time) (int64, int64, error) {
	res := tx.Exec("INSERT INTO inventory_balance(product_id,quantity_milli,update_time) VALUES(?,?,?) ON CONFLICT(product_id) DO UPDATE SET quantity_milli=inventory_balance.quantity_milli+excluded.quantity_milli,update_time=excluded.update_time WHERE inventory_balance.quantity_milli<=?", productID, delta, now, math.MaxInt64-delta)
	if res.Error != nil {
		return 0, 0, res.Error
	}
	if res.RowsAffected != 1 {
		return 0, 0, fmt.Errorf("%w：冲销后库存数量超出范围", ErrInvalid)
	}
	var after int64
	if e := tx.Table("inventory_balance").Select("quantity_milli").Where("product_id=?", productID).Scan(&after).Error; e != nil {
		return 0, 0, e
	}
	return after - delta, after, nil
}
func (r *Repository) Find(ctx context.Context, id int64) (*Draft, error) {
	v, e := find(r.db.WithContext(ctx), id)
	return v, mapErr(e)
}

// DeliveryNote reads the saved document into the print projection. A posted or
// cancelled sale uses the frozen snapshot columns, including empty values; a
// draft falls back to the current partner and operator profile because it has
// not frozen anything yet. This never touches stock or receivables.
func (r *Repository) DeliveryNote(ctx context.Context, id int64) (*DeliveryNote, error) {
	var out *DeliveryNote
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var h struct {
			ID, PartnerID                                   int64
			DocumentNo, Status                              string
			BusinessDate                                    BusinessDate
			PartnerName                                     string
			DeliveryContact, DeliveryPhone, DeliveryAddress *string
			OwnerName, OwnerPhone, OwnerAddress             string
			Remark                                          *string
		}
		e := tx.Table("sale_document d").Select("d.id,d.document_no,d.status,d.business_date,d.partner_id,d.remark,d.delivery_contact,d.delivery_phone,d.delivery_address,d.partner_name,d.owner_name,d.owner_phone,d.owner_address").Where("d.id=?", id).Take(&h).Error
		if errors.Is(e, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		if e != nil {
			return e
		}
		n := &DeliveryNote{DocumentNo: h.DocumentNo, Status: h.Status, Posted: h.Status == "POSTED", BusinessDate: string(h.BusinessDate), PartnerID: h.PartnerID, DeliveryContact: h.DeliveryContact, DeliveryPhone: h.DeliveryPhone, DeliveryAddress: h.DeliveryAddress, OwnerName: h.OwnerName, OwnerPhone: h.OwnerPhone, OwnerAddress: h.OwnerAddress, Remark: h.Remark, Items: []DeliveryNoteLine{}}
		n.PartnerName = h.PartnerName
		if n.PartnerName == "" {
			if e := tx.Table("partner").Select("name").Where("id=?", h.PartnerID).Scan(&n.PartnerName).Error; e != nil {
				return e
			}
		}
		if h.Status == "DRAFT" {
			profile, e := readOwnerProfile(tx)
			if e != nil {
				return e
			}
			n.OwnerName, n.OwnerPhone, n.OwnerAddress = profile.Name, profile.Phone, profile.Address
		}
		var lines []Line
		if e := tx.Table("sale_document_item").Where("document_id=?", id).Order("id ASC").Find(&lines).Error; e != nil {
			return e
		}
		totalQty, totalAmount := int64(0), int64(0)
		for i := range lines {
			l := &lines[i]
			qty := milliText(l.QuantityMilli)
			amount := moneyText(l.AmountCents)
			n.Items = append(n.Items, DeliveryNoteLine{ProductID: l.ProductID, ProductCode: l.ProductCode, ProductName: l.ProductName, ProductModel: l.ProductModel, ProductSpecification: l.ProductSpecification, Unit: l.Unit, Quantity: qty, UnitPrice: moneyText(l.UnitPriceCents), Amount: amount, Remark: l.Remark})
			if totalQty > math.MaxInt64-l.QuantityMilli {
				return ErrInvalid
			}
			totalQty += l.QuantityMilli
			if totalAmount > math.MaxInt64-l.AmountCents {
				return ErrInvalid
			}
			totalAmount += l.AmountCents
		}
		n.TotalQuantity = milliText(totalQty)
		n.TotalAmount = moneyText(totalAmount)
		out = n
		return nil
	})
	if e != nil {
		return nil, mapErr(e)
	}
	return out, nil
}
func (r *Repository) Page(ctx context.Context, q Query) (Page, error) {
	p := Page{Records: []Draft{}, Page: q.Page, PageSize: q.PageSize}
	d := r.db.WithContext(ctx).Model(&Draft{})
	if strings.TrimSpace(q.DocumentNo) != "" {
		d = platformdatabase.LiteralContains(d, q.DocumentNo, "document_no")
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
		d = d.Where("EXISTS(SELECT 1 FROM sale_document_item i WHERE i.document_id=sale_document.id AND i.product_id=?)", q.ProductID)
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
	if e := tx.Table("partner").Where("id=? AND status=1 AND is_customer=?", partnerID, true).Count(&n).Error; e != nil {
		return e
	}
	if n != 1 {
		return fmt.Errorf("%w：往来单位必须为启用客户", ErrInvalid)
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
			return fmt.Errorf("%w：商品或服务不存在", ErrInvalid)
		}
		if p.Status != 1 || (p.Type != "GOODS" && p.Type != "SERVICE") {
			return fmt.Errorf("%w：销售仅支持启用实物商品或服务", ErrInvalid)
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
	e := db.Table("sale_document d").Select("d.*,COALESCE(NULLIF(d.partner_name,''),p.name) AS partner_name,COALESCE(NULLIF(c.nickname,''),c.username) AS created_by_name,COALESCE(NULLIF(u.nickname,''),u.username) AS cancelled_by_name,COALESCE(NULLIF(pb.nickname,''),pb.username) AS posted_by_name").Joins("JOIN partner p ON p.id=d.partner_id").Joins("LEFT JOIN sys_user c ON c.id=d.created_by").Joins("LEFT JOIN sys_user u ON u.id=d.cancelled_by").Joins("LEFT JOIN sys_user pb ON pb.id=d.posted_by").Where("d.id=?", id).Take(&h).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	if e != nil {
		return nil, e
	}
	var lines []Line
	if e = db.Table("sale_document_item").Where("document_id=?", id).Order("id ASC").Find(&lines).Error; e != nil {
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
	h.DirectTrace, e = directdelivery.SaleTrace(db, id)
	if e != nil {
		return nil, e
	}
	h.DirectDocuments, e = directdelivery.SalePurchase(db, h.DirectPurchaseID)
	if e != nil {
		return nil, e
	}
	return &h, nil
}
func mapErr(e error) error {
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	if e != nil && (strings.Contains(strings.ToLower(e.Error()), "unique constraint") || strings.Contains(strings.ToLower(e.Error()), "database is locked")) {
		return ErrConflict
	}
	return e
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

func samePurchase(a, b *int64) bool { return a == nil && b == nil || a != nil && b != nil && *a == *b }
func validateDirect(tx *gorm.DB, h Draft, id int64, posting bool, lines []Line) error {
	if h.DirectPurchaseID == nil {
		return nil
	}
	var p directdelivery.Purchase
	if e := directdelivery.LockPurchase(tx, *h.DirectPurchaseID, &p); e != nil {
		return fmt.Errorf("%w：关联采购不存在", ErrInvalid)
	}
	if !p.DirectDelivery || p.Status == "CANCELLED" {
		return fmt.Errorf("%w：必须关联未取消的直送采购单", ErrConflict)
	}
	var n int64
	if e := tx.Table("sale_document").Where("direct_purchase_id=? AND status<>'CANCELLED' AND id<>?", p.ID, id).Count(&n).Error; e != nil {
		return e
	}
	if n != 0 {
		return fmt.Errorf("%w：该直送采购已有未取消销售单", ErrConflict)
	}
	if !posting {
		return nil
	}
	if p.Status != "POSTED" {
		return fmt.Errorf("%w：请先单独过账关联采购单", ErrConflict)
	}
	sale := map[int64]int64{}
	for _, l := range lines {
		if l.ProductType == "GOODS" {
			if sale[l.ProductID] > math.MaxInt64-l.QuantityMilli {
				return ErrInvalid
			}
			sale[l.ProductID] += l.QuantityMilli
		}
	}
	purchase, e := directdelivery.PurchaseQuantities(tx, p.ID)
	if e != nil {
		return fmt.Errorf("%w：%v", ErrConflict, e)
	}
	if len(sale) != len(purchase) {
		return fmt.Errorf("%w：直送实物商品汇总数量不一致", ErrConflict)
	}
	for id, n := range purchase {
		if sale[id] != n {
			return fmt.Errorf("%w：直送商品%d汇总数量不一致", ErrConflict, id)
		}
	}
	return nil
}
