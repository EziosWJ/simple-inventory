package receivable

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/EziosWJ/simple-inventory/server/internal/audit"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (r *Repository) Settle(ctx context.Context, meta audit.Metadata, in SettlementInput) (Entry, error) {
	return r.recordFunds(ctx, meta, in, false)
}
func (r *Repository) Refund(ctx context.Context, meta audit.Metadata, in SettlementInput) (Entry, error) {
	return r.recordFunds(ctx, meta, in, true)
}

// Both kinds of actual funds share the same immutable ledger and serialization
// gate as purchases, sales, returns and reversals in this direction.
func (r *Repository) recordFunds(ctx context.Context, meta audit.Metadata, in SettlementInput, refund bool) (Entry, error) {
	if err := validateFunds(in); err != nil {
		return Entry{}, err
	}
	amount, _ := cents(in.Amount)
	delta := -amount
	typ, prefix, action, summary := "RECEIPT", "ST", "partner.settlement.create", "往来收付款"
	if in.Direction == "SUPPLIER" {
		typ = "PAYMENT"
	}
	if refund {
		delta = amount
		typ = in.Direction + "_REFUND"
		prefix = "RF"
		action = "partner.refund.create"
		summary = "往来退款"
	}
	key, remark, transaction := strings.TrimSpace(in.RequestKey), strings.TrimSpace(in.Remark), strings.TrimSpace(in.TransactionNo)
	if remark == "" {
		if refund {
			remark = summary
		} else {
			remark = "往来结算"
		}
	}
	var out Entry
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Reserve SQLite's writer before any reads; PostgreSQL locks the balance row.
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "partner_id"}, {Name: "direction"}}, DoNothing: true}).Create(&balanceRow{PartnerID: in.PartnerID, Direction: in.Direction}).Error; err != nil {
			return err
		}
		var b balanceRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("partner_id=? AND direction=?", in.PartnerID, in.Direction).Take(&b).Error; err != nil {
			return err
		}
		var previous entryRow
		if err := tx.Where("request_key=?", key).Take(&previous).Error; err == nil {
			previousDate := previous.BusinessDate
			if len(previousDate) > 10 {
				previousDate = previousDate[:10]
			}
			if previous.PartnerID != in.PartnerID || previous.Direction != in.Direction || previous.EntryType != typ || previous.AmountCents != delta || previousDate != in.BusinessDate || (previous.Description != remark && !(!refund && strings.TrimSpace(in.Remark) == "" && previous.Description == "")) || previous.PaymentMethod != in.PaymentMethod || previous.TransactionNo != transaction {
				return ErrConflict
			}
			out = fromRow(previous)
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if refund {
			// Comparing before with -amount avoids abs(MinInt64) overflow.
			if b.AmountCents >= 0 || b.AmountCents > -amount {
				return fmt.Errorf("%w：退款金额超过当前待退款", ErrConflict)
			}
		} else if b.AmountCents <= 0 || b.AmountCents < amount {
			return fmt.Errorf("%w：收付款金额超过当前欠款", ErrConflict)
		}
		now := time.Now().UTC()
		after := b.AmountCents + delta
		row := entryRow{PartnerID: in.PartnerID, Direction: in.Direction, EntryType: typ, AmountCents: delta, BalanceBeforeCents: b.AmountCents, BalanceAfterCents: after, BusinessDate: in.BusinessDate, EffectiveAt: now, Description: remark, DocumentNo: fmt.Sprintf("%s%s-%d", prefix, now.Format("20060102150405"), now.UnixNano()), OperatorID: meta.ActorID, RequestKey: &key, PaymentMethod: in.PaymentMethod, TransactionNo: transaction}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		res := tx.Model(&balanceRow{}).Where("id=? AND amount_cents=?", b.ID, b.AmountCents).Updates(map[string]any{"amount_cents": after, "entry_count": gorm.Expr("entry_count+1"), "update_time": now})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrConflict
		}
		if err := audit.RecordOn(ctx, tx, audit.Event{Action: action, Resource: "partner_balance", ResourceID: row.ID, Summary: summary, Metadata: meta}); err != nil {
			return err
		}
		out = fromRow(row)
		return nil
	})
	return out, err
}
