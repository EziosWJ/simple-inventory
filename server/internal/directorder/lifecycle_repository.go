package directorder

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/EziosWJ/simple-inventory/server/internal/audit"
	"github.com/EziosWJ/simple-inventory/server/internal/directdelivery"
	"github.com/EziosWJ/simple-inventory/server/internal/purchase"
	"github.com/EziosWJ/simple-inventory/server/internal/sale"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *Repository) Post(ctx context.Context, id, version int64, event audit.Event) (Draft, error) {
	var out Draft
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		h, e := lockDocument(tx, id)
		if e != nil {
			return e
		}
		// Retrying the exact original confirmation is safe; unrelated versions are stale.
		if h.Status == "POSTED" && h.Version == version+1 {
			v, e := find(tx, id)
			if e == nil {
				out = *v
			}
			return e
		}
		if h.Status != "DRAFT" || h.Version != version {
			return ErrConflict
		}
		saved, e := find(tx, id)
		if e != nil {
			return e
		}
		if e = validMaterials(tx, h, saved.Items); e != nil {
			return e
		}
		pi := purchase.Input{PartnerID: h.SupplierID, BusinessDate: string(h.BusinessDate), DirectDelivery: true, Remark: h.Remark}
		si := sale.Input{PartnerID: h.CustomerID, BusinessDate: string(h.BusinessDate), DirectDelivery: true, Remark: h.Remark, DeliveryContact: h.DeliveryContact, DeliveryPhone: h.DeliveryPhone, DeliveryAddress: h.DeliveryAddress}
		for _, l := range saved.Items {
			si.Items = append(si.Items, sale.LineInput{ProductID: l.ProductID, ProductType: l.ProductType, Unit: l.Unit, Quantity: l.Quantity, UnitPrice: l.SalePrice, Remark: l.Remark})
			if l.ProductType == "GOODS" {
				pi.Items = append(pi.Items, purchase.LineInput{ProductID: l.ProductID, ProductType: l.ProductType, Unit: l.Unit, Quantity: l.Quantity, UnitPrice: l.PurchasePrice, Remark: l.Remark})
			}
		}
		// Lock balances/products before child INSERT foreign keys acquire product key-share locks.
		if e = lockBusiness(tx, h, saved.Items); e != nil {
			return e
		}
		ps := purchase.NewService(purchase.NewOrderRepository(tx, id))
		p, e := ps.Create(ctx, event.Metadata, pi)
		if e != nil {
			return childError(e)
		}
		si.DirectPurchaseID = &p.ID
		ss := sale.NewService(sale.NewOrderRepository(tx, id))
		s, e := ss.Create(ctx, event.Metadata, si)
		if e != nil {
			return childError(e)
		}
		stamp := time.Now().UTC()
		businessCtx := directdelivery.WithEffectiveTime(ctx, stamp)
		// Both repositories use this same connection/transaction and effective time.
		p, e = ps.Post(businessCtx, event.Metadata, p.ID, purchase.PostInput{Version: p.Version})
		if e != nil {
			return childError(e)
		}
		s, e = ss.Post(businessCtx, event.Metadata, s.ID, sale.PostInput{Version: s.Version})
		if e != nil {
			return childError(e)
		}
		var supplier string
		if e = tx.Table("partner").Select("name").Where("id=?", h.SupplierID).Scan(&supplier).Error; e != nil {
			return e
		}
		for i, l := range s.Items {
			if e = tx.Table("direct_order_item").Where("id=?", saved.Items[i].ID).Updates(map[string]any{"product_code": l.ProductCode, "product_name": l.ProductName, "product_model": l.ProductModel, "product_specification": l.ProductSpecification}).Error; e != nil {
				return e
			}
		}
		result := tx.Model(&Draft{}).Where("id=? AND status='DRAFT' AND version=?", id, version).Updates(map[string]any{"status": "POSTED", "version": version + 1, "purchase_id": p.ID, "sale_id": s.ID, "posted_by": event.Metadata.ActorID, "posted_at": stamp, "supplier_snapshot": supplier, "customer_snapshot": s.PartnerName, "delivery_contact": s.DeliveryContact, "delivery_phone": s.DeliveryPhone, "delivery_address": s.DeliveryAddress})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrConflict
		}
		if e = audit.RecordOn(businessCtx, tx, event); e != nil {
			return e
		}
		v, e := find(tx, id)
		if e == nil {
			out = *v
		}
		return e
	})
	return out, mapErr(err)
}

func (r *Repository) Cancel(ctx context.Context, id, version int64, reason string, event audit.Event) (Draft, error) {
	var out Draft
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		h, e := lockDocument(tx, id)
		if e != nil {
			return e
		}
		if h.Status == "CANCELLED" && h.Version == version+1 && h.CancelReason != nil && *h.CancelReason == reason {
			v, e := find(tx, id)
			if e == nil {
				out = *v
			}
			return e
		}
		if (h.Status != "DRAFT" && h.Status != "POSTED") || h.Version != version {
			return ErrConflict
		}
		stamp := time.Now().UTC()
		if h.Status == "POSTED" {
			if h.PurchaseID == nil || h.SaleID == nil {
				return ErrConflict
			}
			var p purchase.Draft
			if e = directdelivery.LockPurchase(tx, *h.PurchaseID, &p); e != nil {
				return e
			}
			var s sale.Draft
			q := tx.Where("id=?", *h.SaleID)
			if tx.Dialector.Name() == "postgres" {
				q = q.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			if e = q.Take(&s).Error; e != nil {
				return e
			}
			if p.Status != "POSTED" || s.Status != "POSTED" || p.SourceOrderID == nil || s.SourceOrderID == nil || *p.SourceOrderID != id || *s.SourceOrderID != id {
				return ErrConflict
			}
			saved, e := find(tx, id)
			if e != nil {
				return e
			}
			if len(saved.ActiveSaleReturns) > 0 || len(saved.ActivePurchaseReturns) > 0 {
				return fmt.Errorf("%w：存在生效退货，请先查看关联退货单", ErrConflict)
			}
			if e = lockBusiness(tx, h, saved.Items); e != nil {
				return e
			}
			stamp = time.Now().UTC()
			businessCtx := directdelivery.WithEffectiveTime(ctx, stamp)
			ss := sale.NewService(sale.NewOrderRepository(tx, id))
			ps := purchase.NewService(purchase.NewOrderRepository(tx, id))
			if _, e = ss.Cancel(businessCtx, event.Metadata, s.ID, sale.CancelInput{Version: s.Version, Reason: reason}); e != nil {
				return childError(e)
			}
			if _, e = ps.Cancel(businessCtx, event.Metadata, p.ID, purchase.CancelInput{Version: p.Version, Reason: reason}); e != nil {
				return childError(e)
			}
		}
		result := tx.Model(&Draft{}).Where("id=? AND status=? AND version=?", id, h.Status, version).Updates(map[string]any{"status": "CANCELLED", "version": version + 1, "cancelled_by": event.Metadata.ActorID, "cancelled_at": stamp, "cancel_reason": reason})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrConflict
		}
		if e = audit.RecordOn(ctx, tx, event); e != nil {
			return e
		}
		v, e := find(tx, id)
		if e == nil {
			out = *v
		}
		return e
	})
	return out, mapErr(err)
}

// Acquire both financial directions before any product, including crossed suppliers/customers.
func lockBusiness(tx *gorm.DB, h Draft, lines []Line) error {
	keys := []struct {
		ID        int64
		Direction string
	}{{h.SupplierID, "SUPPLIER"}, {h.CustomerID, "CUSTOMER"}}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].ID == keys[j].ID {
			return keys[i].Direction < keys[j].Direction
		}
		return keys[i].ID < keys[j].ID
	})
	for _, key := range keys {
		if e := tx.Exec("INSERT INTO partner_balance(partner_id,direction,amount_cents,entry_count,update_time) VALUES (?,?,0,0,?) ON CONFLICT(partner_id,direction) DO NOTHING", key.ID, key.Direction, time.Now().UTC()).Error; e != nil {
			return e
		}
		var balance struct{ ID int64 }
		q := tx.Table("partner_balance").Where("partner_id=? AND direction=?", key.ID, key.Direction)
		if tx.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if e := q.Take(&balance).Error; e != nil {
			return e
		}
	}
	ids := []int64{}
	seen := map[int64]bool{}
	for _, l := range lines {
		if !seen[l.ProductID] {
			seen[l.ProductID] = true
			ids = append(ids, l.ProductID)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		var product struct{ ID int64 }
		q := tx.Table("product").Where("id=?", id)
		if tx.Dialector.Name() == "postgres" {
			q = q.Clauses(clause.Locking{Strength: "UPDATE"})
		}
		if e := q.Take(&product).Error; e != nil {
			return e
		}
	}
	return nil
}
func childError(e error) error {
	if errors.Is(e, purchase.ErrInvalid) || errors.Is(e, sale.ErrInvalid) {
		return fmt.Errorf("%w：%v", ErrInvalid, e)
	}
	if errors.Is(e, purchase.ErrConflict) || errors.Is(e, sale.ErrConflict) || errors.Is(e, purchase.ErrRequestConflict) || errors.Is(e, sale.ErrRequestConflict) {
		return fmt.Errorf("%w：%v", ErrConflict, e)
	}
	return e
}
