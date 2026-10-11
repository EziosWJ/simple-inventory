package receivable

import (
	"context"
	"database/sql"
	"math/big"

	"gorm.io/gorm"
)

type BalanceFilter struct {
	PartnerID int64
	Direction string
	Category  string
	Page      int
	PageSize  int
}

type BalanceSummary struct {
	Category     string `json:"category"`
	Amount       string `json:"amount"`
	PartnerCount int64  `json:"partnerCount"`
}

type BalanceOverview struct {
	Summaries      []BalanceSummary `json:"summaries"`
	TopReceivables []Balance        `json:"topReceivables"`
	TopPayables    []Balance        `json:"topPayables"`
}

func balanceCategory(direction string, amount int64) string {
	if direction == "CUSTOMER" {
		if amount > 0 {
			return "RECEIVABLE"
		}
		return "CUSTOMER_REFUND"
	}
	if amount > 0 {
		return "PAYABLE"
	}
	return "SUPPLIER_REFUND"
}

func validBalanceCategory(category string) bool {
	switch category {
	case "", "ALL", "RECEIVABLE", "PAYABLE", "CUSTOMER_REFUND", "SUPPLIER_REFUND":
		return true
	}
	return false
}

func (s *Service) FilterBalances(ctx context.Context, f BalanceFilter) (BalancePage, error) {
	if f.PartnerID < 0 || f.Page < 1 || f.PageSize < 1 || f.PageSize > 500 ||
		(f.Direction != "" && f.Direction != "CUSTOMER" && f.Direction != "SUPPLIER") || !validBalanceCategory(f.Category) {
		return BalancePage{}, ErrInvalid
	}
	return s.store.FilterBalances(ctx, f)
}

func (s *Service) Overview(ctx context.Context) (BalanceOverview, error) {
	return s.store.Overview(ctx)
}

// Include historical identities and inactive partners; directions without a
// balance record are represented by their current partner identity at zero.
const balanceQuery = `SELECT p.id AS partner_id,p.name AS partner_name,'CUSTOMER' AS direction,COALESCE(b.amount_cents,0) AS amount_cents,COALESCE(b.entry_count,0) AS entry_count,CASE WHEN b.id IS NULL THEN FALSE ELSE TRUE END AS has_records FROM partner p LEFT JOIN partner_balance b ON b.partner_id=p.id AND b.direction='CUSTOMER' WHERE p.is_customer=TRUE OR b.id IS NOT NULL
UNION ALL SELECT p.id AS partner_id,p.name AS partner_name,'SUPPLIER' AS direction,COALESCE(b.amount_cents,0) AS amount_cents,COALESCE(b.entry_count,0) AS entry_count,CASE WHEN b.id IS NULL THEN FALSE ELSE TRUE END AS has_records FROM partner p LEFT JOIN partner_balance b ON b.partner_id=p.id AND b.direction='SUPPLIER' WHERE p.is_supplier=TRUE OR b.id IS NOT NULL`

type balanceResult struct {
	PartnerID               int64
	PartnerName, Direction  string
	AmountCents, EntryCount int64
	HasRecords              bool
}

func balanceRows(db *gorm.DB, f BalanceFilter) *gorm.DB {
	q := db.Table("(" + balanceQuery + ") AS balances")
	if f.PartnerID > 0 {
		q = q.Where("partner_id=?", f.PartnerID)
	}
	if f.Direction != "" {
		q = q.Where("direction=?", f.Direction)
	}
	switch f.Category {
	case "RECEIVABLE":
		q = q.Where("direction=? AND amount_cents>0", "CUSTOMER")
	case "PAYABLE":
		q = q.Where("direction=? AND amount_cents>0", "SUPPLIER")
	case "CUSTOMER_REFUND":
		q = q.Where("direction=? AND amount_cents<0", "CUSTOMER")
	case "SUPPLIER_REFUND":
		q = q.Where("direction=? AND amount_cents<0", "SUPPLIER")
	}
	return q
}

func balanceOrder(category string) string {
	switch category {
	case "RECEIVABLE", "PAYABLE":
		return "amount_cents DESC,partner_id,direction"
	case "CUSTOMER_REFUND", "SUPPLIER_REFUND":
		// Sorting signed negatives ascending orders refund magnitudes descending
		// without negating the minimum signed integer in the database.
		return "amount_cents ASC,partner_id,direction"
	}
	return "partner_id,direction"
}

func readBalances(q *gorm.DB) ([]Balance, error) {
	rows := []balanceResult{}
	if err := q.Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]Balance, 0, len(rows))
	for _, r := range rows {
		out = append(out, Balance{PartnerID: r.PartnerID, PartnerName: r.PartnerName, Direction: r.Direction, Amount: money(r.AmountCents), EntryCount: r.EntryCount, HasRecords: r.HasRecords})
	}
	return out, nil
}

func (s *Repository) FilterBalances(ctx context.Context, f BalanceFilter) (BalancePage, error) {
	out := BalancePage{Page: f.Page, PageSize: f.PageSize}
	if err := balanceRows(s.db.WithContext(ctx), f).Count(&out.Total).Error; err != nil {
		return out, err
	}
	var err error
	out.Records, err = readBalances(balanceRows(s.db.WithContext(ctx), f).Order(balanceOrder(f.Category)).Offset((f.Page - 1) * f.PageSize).Limit(f.PageSize))
	return out, err
}

func (s *Repository) Overview(ctx context.Context) (BalanceOverview, error) {
	out := BalanceOverview{Summaries: []BalanceSummary{}, TopReceivables: []Balance{}, TopPayables: []Balance{}}
	options := &sql.TxOptions{ReadOnly: true}
	if s.db.Dialector.Name() == "postgres" {
		options.Isolation = sql.LevelRepeatableRead
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		categories := []string{"RECEIVABLE", "PAYABLE", "CUSTOMER_REFUND", "SUPPLIER_REFUND"}
		totals, counts := map[string]*big.Int{}, map[string]int64{}
		for _, category := range categories {
			totals[category] = new(big.Int)
		}
		// Sum integer cents exactly while streaming: several valid partner
		// balances may exceed int64 in aggregate (SQLite SUM would overflow).
		rows, err := balanceRows(tx, BalanceFilter{}).Select("direction,amount_cents").Where("amount_cents<>0").Rows()
		if err != nil {
			return err
		}
		for rows.Next() {
			var direction string
			var amount int64
			if err := rows.Scan(&direction, &amount); err != nil {
				rows.Close()
				return err
			}
			category := balanceCategory(direction, amount)
			totals[category].Add(totals[category], new(big.Int).Abs(big.NewInt(amount)))
			counts[category]++
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		for _, category := range categories {
			whole, fraction := new(big.Int), new(big.Int)
			whole.QuoRem(totals[category], big.NewInt(100), fraction)
			decimal := fraction.String()
			if len(decimal) == 1 {
				decimal = "0" + decimal
			}
			out.Summaries = append(out.Summaries, BalanceSummary{Category: category, Amount: whole.String() + "." + decimal, PartnerCount: counts[category]})
		}
		out.TopReceivables, err = readBalances(balanceRows(tx, BalanceFilter{Category: "RECEIVABLE"}).Order(balanceOrder("RECEIVABLE")).Limit(10))
		if err != nil {
			return err
		}
		out.TopPayables, err = readBalances(balanceRows(tx, BalanceFilter{Category: "PAYABLE"}).Order(balanceOrder("PAYABLE")).Limit(10))
		return err
	}, options)
	return out, err
}
