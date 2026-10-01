package receivable

import (
	"context"
	"math/big"
	"time"
)

// EntryFilter uses actual UTC instants: From is inclusive, To is exclusive.
type EntryFilter struct {
	EntryID        int64
	PartnerID      int64
	Direction      string
	From, To       string
	Page, PageSize int
}
type Statement struct {
	PartnerID      int64     `json:"partnerId"`
	PartnerName    string    `json:"partnerName"`
	Direction      string    `json:"direction"`
	From           time.Time `json:"from"`
	To             time.Time `json:"to"`
	OpeningAmount  string    `json:"openingAmount"`
	IncreaseAmount string    `json:"increaseAmount"`
	DecreaseAmount string    `json:"decreaseAmount"`
	NetChange      string    `json:"netChange"`
	ClosingAmount  string    `json:"closingAmount"`
	Records        []Entry   `json:"records"`
}

func normalizeFilter(q EntryFilter, statement bool) (EntryFilter, error) {
	if q.PartnerID < 0 || (q.Direction != "" && q.Direction != "CUSTOMER" && q.Direction != "SUPPLIER") {
		return q, ErrInvalid
	}
	if !statement && (q.Page < 1 || q.PageSize < 1 || q.PageSize > 500) {
		return q, ErrInvalid
	}
	if statement && (q.PartnerID <= 0 || q.Direction == "" || q.From == "" || q.To == "") {
		return q, ErrInvalid
	}
	var from, to time.Time
	var err error
	if q.From != "" {
		from, err = time.Parse(time.RFC3339Nano, q.From)
		if err != nil {
			return q, ErrInvalid
		}
		q.From = from.UTC().Format(time.RFC3339Nano)
	}
	if q.To != "" {
		to, err = time.Parse(time.RFC3339Nano, q.To)
		if err != nil {
			return q, ErrInvalid
		}
		q.To = to.UTC().Format(time.RFC3339Nano)
	}
	if !from.IsZero() && !to.IsZero() && !from.Before(to) {
		return q, ErrInvalid
	}
	return q, nil
}
func (s *Service) FilterPage(ctx context.Context, q EntryFilter) (Page, error) {
	q, e := normalizeFilter(q, false)
	if e != nil {
		return Page{}, e
	}
	return s.store.FilterPage(ctx, q)
}
func (s *Service) Statement(ctx context.Context, q EntryFilter) (Statement, error) {
	q, e := normalizeFilter(q, true)
	if e != nil {
		return Statement{}, e
	}
	return s.store.Statement(ctx, q)
}

const ledgerSelect = "e.id,e.partner_id,e.direction,e.entry_type,e.amount_cents,e.balance_before_cents,e.balance_after_cents,e.business_date,e.effective_at,e.description,e.document_no,e.operator_id,e.reverses_id,e.reversed_by_id,e.request_key,e.payment_method,e.transaction_no, p.name AS partner_name, u.nickname AS operator_name, COALESCE(e.purchase_id,original.purchase_id) AS purchase_id, COALESCE(e.sale_id,original.sale_id) AS sale_id, COALESCE(e.purchase_return_id,original.purchase_return_id) AS purchase_return_id, COALESCE(e.sale_return_id,original.sale_return_id) AS sale_return_id, original.document_no AS reversed_document_no"

// The complete result is read by one SQL statement, including partner and source
// identities. Pagination, counts and statement totals share this same snapshot.
func (r *Repository) ledgerRows(ctx context.Context, q EntryFilter, beforeOnly bool) ([]entryRow, error) {
	query := r.db.WithContext(ctx).Table("partner_balance_entry AS e").Select(ledgerSelect).Joins("JOIN partner p ON p.id=e.partner_id").Joins("LEFT JOIN sys_user u ON u.id=e.operator_id").Joins("LEFT JOIN partner_balance_entry original ON original.id=e.reverses_id")
	if q.EntryID > 0 {
		query = query.Where("e.id=?", q.EntryID)
	}
	if q.PartnerID > 0 {
		query = query.Where("e.partner_id=?", q.PartnerID)
	}
	if q.Direction != "" {
		query = query.Where("e.direction=?", q.Direction)
	}
	if q.From != "" && !beforeOnly {
		from, _ := time.Parse(time.RFC3339Nano, q.From)
		query = query.Where("e.effective_at>=?", from)
	}
	if q.To != "" {
		to, _ := time.Parse(time.RFC3339Nano, q.To)
		query = query.Where("e.effective_at<?", to)
	}
	rows := []entryRow{}
	err := query.Order("e.effective_at ASC,e.id ASC").Scan(&rows).Error
	return rows, err
}
func (r *Repository) FilterPage(ctx context.Context, q EntryFilter) (Page, error) {
	rows, err := r.ledgerRows(ctx, q, false)
	if err != nil {
		return Page{}, err
	}
	out := Page{Records: []Entry{}, Total: int64(len(rows)), Page: q.Page, PageSize: q.PageSize}
	if q.Page-1 > len(rows)/q.PageSize {
		return out, nil
	}
	start64 := int64(q.Page-1) * int64(q.PageSize)
	if start64 >= int64(len(rows)) {
		return out, nil
	}
	start := int(start64)
	end := start + q.PageSize
	if end > len(rows) {
		end = len(rows)
	}
	for _, row := range rows[start:end] {
		out.Records = append(out.Records, fromRow(row))
	}
	return out, nil
}
func moneyBig(value *big.Int) string {
	n := new(big.Int).Abs(value)
	quotient, remainder := new(big.Int), new(big.Int)
	quotient.QuoRem(n, big.NewInt(100), remainder)
	sign := ""
	if value.Sign() < 0 {
		sign = "-"
	}
	fraction := remainder.String()
	if len(fraction) == 1 {
		fraction = "0" + fraction
	}
	return sign + quotient.String() + "." + fraction
}
func (r *Repository) Statement(ctx context.Context, q EntryFilter) (Statement, error) {
	from, _ := time.Parse(time.RFC3339Nano, q.From)
	to, _ := time.Parse(time.RFC3339Nano, q.To)
	rows := []entryRow{}
	// Anchor on the partner to return its identity even for a historical period
	// with no entries. Every amount and label comes from this one SQL snapshot.
	err := r.db.WithContext(ctx).Table("partner p").Select(ledgerSelect).Joins("LEFT JOIN partner_balance_entry e ON e.partner_id=p.id AND e.direction=? AND e.effective_at<?", q.Direction, to).Joins("LEFT JOIN sys_user u ON u.id=e.operator_id").Joins("LEFT JOIN partner_balance_entry original ON original.id=e.reverses_id").Where("p.id=?", q.PartnerID).Order("e.effective_at ASC,e.id ASC").Scan(&rows).Error
	if err != nil {
		return Statement{}, err
	}
	if len(rows) == 0 {
		return Statement{}, ErrNotFound
	}
	out := Statement{PartnerID: q.PartnerID, PartnerName: rows[0].PartnerName, Direction: q.Direction, From: from, To: to, Records: []Entry{}}
	opening, increase, decrease := new(big.Int), new(big.Int), new(big.Int)
	for _, row := range rows {
		if row.ID == 0 {
			continue
		}
		amount := big.NewInt(row.AmountCents)
		if row.EffectiveAt.Before(from) {
			opening.Add(opening, amount)
			continue
		}
		out.Records = append(out.Records, fromRow(row))
		if row.AmountCents > 0 {
			increase.Add(increase, amount)
		} else {
			decrease.Sub(decrease, amount)
		}
	}
	net := new(big.Int).Sub(increase, decrease)
	closing := new(big.Int).Add(opening, net)
	out.OpeningAmount = moneyBig(opening)
	out.IncreaseAmount = moneyBig(increase)
	out.DecreaseAmount = moneyBig(decrease)
	out.NetChange = moneyBig(net)
	out.ClosingAmount = moneyBig(closing)
	return out, nil
}
