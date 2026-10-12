package sale

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/EziosWJ/simple-inventory/server/internal/audit"
	"github.com/EziosWJ/simple-inventory/server/internal/auth"
	"github.com/EziosWJ/simple-inventory/server/internal/directdelivery"
	"gorm.io/gorm"
)

// Lock every referenced purchase before the sales document, matching legacy direct delivery.
func lockSourcePurchases(tx *gorm.DB, id int64, lines []Line, extra *int64) error {
	if !tx.Migrator().HasTable(&PurchaseSource{}) {
		return nil
	}
	itemIDs := []int64{}
	for _, l := range lines {
		if l.SourceChange != nil {
			for _, s := range *l.SourceChange {
				itemIDs = append(itemIDs, s.PurchaseItemID)
			}
		}
	}
	if id > 0 {
		var existing []int64
		if e := tx.Table("sale_purchase_source s").Joins("JOIN sale_document_item i ON i.id=s.sale_item_id").Where("i.document_id=?", id).Pluck("s.purchase_item_id", &existing).Error; e != nil {
			return e
		}
		itemIDs = append(itemIDs, existing...)
	}
	ids := []int64{}
	if len(itemIDs) > 0 {
		if e := tx.Table("purchase_document_item").Where("id IN ?", itemIDs).Distinct("document_id").Pluck("document_id", &ids).Error; e != nil {
			return e
		}
	}
	if extra != nil {
		ids = append(ids, *extra)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	var last int64
	for _, pid := range ids {
		if pid == last {
			continue
		}
		last = pid
		var p directdelivery.Purchase
		if e := directdelivery.LockPurchase(tx, pid, &p); e != nil {
			return e
		}
	}
	return nil
}

func applySourceChanges(ctx context.Context, tx *gorm.DB, h Draft, lines []Line, actor int64) error {
	available := tx.Migrator().HasTable(&PurchaseSource{})
	if !available {
		for _, l := range lines {
			if l.SourceChange != nil && len(*l.SourceChange) > 0 {
				return ErrConflict
			}
		}
		return nil
	}
	changes := false
	for _, l := range lines {
		if l.SourceChange != nil {
			changes = true
		}
	}
	if changes {
		if h.DirectDelivery {
			for _, l := range lines {
				if l.SourceChange != nil && len(*l.SourceChange) > 0 {
					return fmt.Errorf("%w：直送来源由直送业务管理", ErrInvalid)
				}
			}
		}

	}
	for _, l := range lines {
		before := []PurchaseSource{}
		if e := tx.Where("sale_item_id=?", l.ID).Order("id").Find(&before).Error; e != nil {
			return e
		}
		old := map[int64]PurchaseSource{}
		desired := []SourceValue{}
		for _, s := range before {
			old[s.PurchaseItemID] = s
			desired = append(desired, SourceValue{s.PurchaseItemID, s.QuantityMilli, s.ConfirmLaterDate})
		}
		if l.SourceChange != nil {
			desired = *l.SourceChange
		}
		var total int64
		for _, s := range desired {
			if l.ProductType != "GOODS" || s.QuantityMilli > l.QuantityMilli-total {
				return fmt.Errorf("%w：已有采购来源与销售数量或商品不符，请同时更正来源", ErrInvalid)
			}
			total += s.QuantityMilli
			var p struct {
				ProductID    int64
				Unit, Status string
				BusinessDate BusinessDate
			}
			if e := tx.Table("purchase_document_item i").Select("i.product_id,i.unit,d.status,d.business_date").Joins("JOIN purchase_document d ON d.id=i.document_id").Where("i.id=?", s.PurchaseItemID).Take(&p).Error; e != nil {
				if errors.Is(e, gorm.ErrRecordNotFound) {
					return fmt.Errorf("%w：来源采购明细不存在", ErrInvalid)
				}
				return e
			}
			if p.ProductID != l.ProductID || p.Unit != l.Unit {
				return fmt.Errorf("%w：来源必须对应相同实物商品及单位", ErrInvalid)
			}
			previous, existed := old[s.PurchaseItemID]
			unchanged := existed && previous.QuantityMilli == s.QuantityMilli
			if !unchanged && p.Status != "POSTED" {
				return fmt.Errorf("%w：只能新增或更正已过账采购来源，原取消来源可保留", ErrInvalid)
			}
			if p.BusinessDate > h.BusinessDate && !s.ConfirmLaterDate {
				return fmt.Errorf("%w：采购日期晚于销售，请明确确认后关联", ErrInvalid)
			}
		}
		if l.SourceChange == nil {
			continue
		}
		previousValues := []SourceValue{}
		for _, s := range before {
			previousValues = append(previousValues, SourceValue{s.PurchaseItemID, s.QuantityMilli, s.ConfirmLaterDate})
		}
		oldJSON, e := json.Marshal(sourceAuditValues(previousValues))
		if e != nil {
			return e
		}
		newJSON, e := json.Marshal(sourceAuditValues(desired))
		if e != nil {
			return e
		}
		if string(oldJSON) == string(newJSON) {
			continue
		}
		ok, e := auth.HasPermission(ctx, tx, "business:sale")
		if e != nil {
			return e
		}
		if !ok {
			return ErrForbidden
		}
		if e = tx.Where("sale_item_id=?", l.ID).Delete(&PurchaseSource{}).Error; e != nil {
			return e
		}
		for _, s := range desired {
			row := PurchaseSource{SaleItemID: l.ID, PurchaseItemID: s.PurchaseItemID, QuantityMilli: s.QuantityMilli, ConfirmLaterDate: s.ConfirmLaterDate}
			if e = tx.Create(&row).Error; e != nil {
				return e
			}
		}
		history := SourceHistory{SaleID: h.ID, SaleItemID: l.ID, BeforeData: string(oldJSON), AfterData: string(newJSON), OperatorID: actor, CreateTime: time.Now().UTC()}
		if e = tx.Create(&history).Error; e != nil {
			return e
		}
	}
	return nil
}
func loadSources(db *gorm.DB, lines []Line) error {
	for i := range lines {
		lines[i].PurchaseSources = []PurchaseSource{}
	}
	if len(lines) == 0 || !db.Migrator().HasTable(&PurchaseSource{}) {
		return nil
	}
	ids := lineIDs(lines)
	sources := []PurchaseSource{}
	query := db.Table("sale_purchase_source s").Select("s.*,i.document_id AS purchase_id,d.document_no,p.name AS partner_name,d.business_date,d.status,COALESCE(r.quantity_milli,0) AS returned_quantity_milli").Joins("JOIN purchase_document_item i ON i.id=s.purchase_item_id").Joins("JOIN purchase_document d ON d.id=i.document_id").Joins("JOIN partner p ON p.id=d.partner_id").Joins("LEFT JOIN (SELECT ri.purchase_item_id,SUM(ri.quantity_milli) AS quantity_milli FROM purchase_return_document_item ri JOIN purchase_return_document rd ON rd.id=ri.document_id WHERE rd.status='POSTED' GROUP BY ri.purchase_item_id) r ON r.purchase_item_id=i.id").Where("s.sale_item_id IN ?", ids).Order("s.id")
	if e := query.Scan(&sources).Error; e != nil {
		return e
	}
	byLine := map[int64][]PurchaseSource{}
	for _, s := range sources {
		s.Quantity = milliText(s.QuantityMilli)
		s.ReturnedQuantity = milliText(s.ReturnedQuantityMilli)
		byLine[s.SaleItemID] = append(byLine[s.SaleItemID], s)
	}
	for i := range lines {
		if list := byLine[lines[i].ID]; list != nil {
			lines[i].PurchaseSources = list
		}
	}
	return nil
}
func (r *Repository) UpdateSources(ctx context.Context, id, version int64, items map[int64][]SourceInput, event audit.Event) (Draft, error) {
	ok, e := auth.HasPermission(ctx, r.db, "business:sale")
	if e != nil {
		return Draft{}, e
	}
	if !ok {
		return Draft{}, ErrForbidden
	}
	var out Draft
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var prior Draft
		if e := tx.Where("id=?", id).Take(&prior).Error; e != nil {
			return e
		}
		proposed := []Line{}
		for lineID, in := range items {
			v, e := normalizeSources(in, "GOODS", int64(^uint64(0)>>1))
			if e != nil {
				return e
			}
			proposed = append(proposed, Line{ID: lineID, SourceChange: &v})
		}
		if e := lockSourcePurchases(tx, id, proposed, prior.DirectPurchaseID); e != nil {
			return e
		}
		h, e := lockDocument(tx, id, version)
		if e != nil {
			return e
		}
		if h.SourceOrderID != nil || h.DirectDelivery {
			return fmt.Errorf("%w：直送来源由直送业务管理，普通采购来源用于普通销售", ErrConflict)
		}
		lines := []Line{}
		if e = tx.Where("document_id=?", id).Find(&lines).Error; e != nil {
			return e
		}
		seen := 0
		for i := range lines {
			if in, ok := items[lines[i].ID]; ok {
				v, e := normalizeSources(in, lines[i].ProductType, lines[i].QuantityMilli)
				if e != nil {
					return e
				}
				lines[i].SourceChange = &v
				seen++
			}
		}
		if seen != len(items) {
			return ErrInvalid
		}
		if e = applySourceChanges(ctx, tx, h, lines, event.Metadata.ActorID); e != nil {
			return e
		}
		result := tx.Model(&Draft{}).Where("id=? AND version=? AND status IN ('DRAFT','POSTED')", id, version).Update("version", version+1)
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
func (r *Repository) SourceHistory(ctx context.Context, id int64) ([]SourceHistory, error) {
	ok, e := auth.HasPermission(ctx, r.db, "business:sale")
	if e != nil {
		return nil, e
	}
	if !ok {
		return nil, ErrForbidden
	}
	var n int64
	if e = r.db.WithContext(ctx).Model(&Draft{}).Where("id=?", id).Count(&n).Error; e != nil {
		return nil, e
	}
	if n == 0 {
		return nil, ErrNotFound
	}
	out := []SourceHistory{}
	e = r.db.WithContext(ctx).Table("sale_purchase_source_history h").Select("h.*,COALESCE(NULLIF(u.nickname,''),u.username) AS operator_name").Joins("JOIN sys_user u ON u.id=h.operator_id").Where("h.sale_id=?", id).Order("h.id DESC").Scan(&out).Error
	return out, e
}

func sourceAuditValues(values []SourceValue) []SourceInput {
	out := []SourceInput{}
	for _, v := range values {
		out = append(out, SourceInput{PurchaseItemID: v.PurchaseItemID, Quantity: milliText(v.QuantityMilli), ConfirmLaterDate: v.ConfirmLaterDate})
	}
	return out
}
