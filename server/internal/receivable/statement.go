package receivable

import (
	"context"
	"database/sql"
	"math"
	"math/big"
	"strings"
	"time"

	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	"gorm.io/gorm"
)

// EntryFilter uses actual UTC instants: From is inclusive, To is exclusive.
type EntryFilter struct {
	EntryID        int64
	PartnerID      int64
	PartnerKeyword string
	DocumentNo     string
	ReversalStatus string
	BusinessFrom   string
	BusinessTo     string
	Direction      string
	Category       string
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
	Total          int64     `json:"total"`
	Page           int       `json:"page"`
	PageSize       int       `json:"pageSize"`
	Records        []Entry   `json:"records"`
}

func normalizeFilter(q EntryFilter, statement bool) (EntryFilter, error) {
	if (q.PartnerID > 0 && strings.TrimSpace(q.PartnerKeyword) != "") || q.PartnerID < 0 || (q.Direction != "" && q.Direction != "CUSTOMER" && q.Direction != "SUPPLIER") ||
		(q.Category != "" && q.Category != "OPENING" && q.Category != "SETTLEMENT" && q.Category != "REFUND") {
		return q, ErrInvalid
	}
	if q.ReversalStatus != "" && q.ReversalStatus != "EFFECTIVE" && q.ReversalStatus != "REVERSED" && q.ReversalStatus != "REVERSAL" {
		return q, ErrInvalid
	}
	if !statement && (q.Page < 1 || q.PageSize < 1 || q.PageSize > 500) {
		return q, ErrInvalid
	}
	// A statement returns every record unless the caller asks for one bounded
	// page; either way the totals cover the complete period.
	if statement && q.Page > 0 && (q.PageSize < 1 || q.PageSize > 500) {
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
	for _, value := range []*string{&q.BusinessFrom, &q.BusinessTo} {
		if *value == "" {
			continue
		}
		date, parseErr := time.Parse("2006-01-02", *value)
		if parseErr != nil {
			return q, ErrInvalid
		}
		*value = date.Format("2006-01-02")
	}
	if q.BusinessFrom != "" && q.BusinessTo != "" && q.BusinessFrom > q.BusinessTo {
		return q, ErrInvalid
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

// ledgerQuery builds the shared ledger filter. Every caller reuses it so the
// page, the count and the statement totals always describe the same rows.
func (r *Repository) ledgerQuery(db *gorm.DB, ctx context.Context, q EntryFilter) *gorm.DB {
	query := db.WithContext(ctx).Table("partner_balance_entry AS e").Joins("JOIN partner p ON p.id=e.partner_id").Joins("LEFT JOIN sys_user u ON u.id=e.operator_id").Joins("LEFT JOIN partner_balance_entry original ON original.id=e.reverses_id")
	if q.EntryID > 0 {
		query = query.Where("e.id=?", q.EntryID)
	}
	if q.PartnerID > 0 {
		query = query.Where("e.partner_id=?", q.PartnerID)
	}
	query = platformdatabase.LiteralWords(query, q.PartnerKeyword, "p.code", "p.name", "p.contact", "p.phone")
	query = platformdatabase.LiteralWords(query, q.DocumentNo, "e.document_no")
	if q.Direction != "" {
		query = query.Where("e.direction=?", q.Direction)
	}
	switch q.ReversalStatus {
	case "EFFECTIVE":
		query = query.Where("e.reverses_id IS NULL AND e.reversed_by_id IS NULL")
	case "REVERSED":
		query = query.Where("e.reversed_by_id IS NOT NULL")
	case "REVERSAL":
		query = query.Where("e.reverses_id IS NOT NULL")
	}
	if q.BusinessFrom != "" {
		query = query.Where("e.business_date>=?", q.BusinessFrom)
	}
	if q.BusinessTo != "" {
		query = query.Where("e.business_date<=?", q.BusinessTo)
	}
	switch q.Category {
	case "OPENING":
		query = query.Where("e.entry_type=? OR (e.entry_type='REVERSAL' AND original.entry_type=?)", "OPENING", "OPENING")
	case "SETTLEMENT":
		query = query.Where("e.entry_type IN ? OR (e.entry_type='REVERSAL' AND original.entry_type IN ?)", []string{"RECEIPT", "PAYMENT"}, []string{"RECEIPT", "PAYMENT"})
	case "REFUND":
		query = query.Where("e.entry_type IN ? OR (e.entry_type='REVERSAL' AND original.entry_type IN ?)", []string{"CUSTOMER_REFUND", "SUPPLIER_REFUND"}, []string{"CUSTOMER_REFUND", "SUPPLIER_REFUND"})
	}
	if q.From != "" {
		from, _ := time.Parse(time.RFC3339Nano, q.From)
		query = query.Where("e.effective_at>=?", from)
	}
	if q.To != "" {
		to, _ := time.Parse(time.RFC3339Nano, q.To)
		query = query.Where("e.effective_at<?", to)
	}
	return query
}

// Ledger listings default to the newest effective instant first; the printed
// statement reads oldest first because it builds a running balance in order.
const (
	ledgerOrderNewestFirst = "e.effective_at DESC,e.id DESC"
	ledgerOrderOldestFirst = "e.effective_at ASC,e.id ASC"
)

// ledgerRows reads at most one bounded window of ledger rows; partner and
// source identities come from the same query as their filters.
func (r *Repository) ledgerRows(ctx context.Context, q EntryFilter, order string, limit ...int) ([]entryRow, error) {
	query := r.ledgerQuery(r.db, ctx, q).Select(r.ledgerSelect(ctx))
	if len(limit) > 0 {
		query = query.Limit(limit[0])
	}
	rows := []entryRow{}
	err := query.Order(order).Scan(&rows).Error
	return rows, err
}

// FilterPage counts in the database and reads exactly one page, so a large
// ledger no longer has to be loaded to return its first page.
func (r *Repository) FilterPage(ctx context.Context, q EntryFilter) (Page, error) {
	out := Page{Records: []Entry{}, Page: q.Page, PageSize: q.PageSize}
	records := r.ledgerSelect(ctx)
	if err := r.ledgerQuery(r.db, ctx, q).Count(&out.Total).Error; err != nil {
		return Page{}, err
	}
	offset, beyond := pageOffset(q.Page, q.PageSize)
	if beyond {
		return out, nil
	}
	rows := []entryRow{}
	err := r.ledgerQuery(r.db, ctx, q).Select(records).Order(ledgerOrderNewestFirst).
		Offset(offset).Limit(q.PageSize).Scan(&rows).Error
	if err != nil {
		return Page{}, err
	}
	for _, row := range rows {
		out.Records = append(out.Records, fromRow(row))
	}
	return out, nil
}

// pageOffset converts a requested page into a row offset and reports whether
// the page lies beyond the result set. The guard keeps a maximal page number
// from wrapping into a valid offset.
func pageOffset(page, pageSize int) (int, bool) {
	if page < 1 || pageSize < 1 || page-1 > math.MaxInt/pageSize {
		return 0, true
	}
	return (page - 1) * pageSize, false
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

// Statement reads the whole period's totals and one bounded page of records
// from a single read-only snapshot. Totals never describe only the returned
// page, and the opening balance keeps counting rows before the period.
func (r *Repository) Statement(ctx context.Context, q EntryFilter) (Statement, error) {
	from, _ := time.Parse(time.RFC3339Nano, q.From)
	to, _ := time.Parse(time.RFC3339Nano, q.To)
	out := Statement{PartnerID: q.PartnerID, Direction: q.Direction, From: from, To: to, Records: []Entry{}, Page: q.Page, PageSize: q.PageSize}
	// The optional-table probe must run before the transaction: SQLite keeps a
	// single pooled connection, so a nested query would wait for the very
	// connection the transaction already holds.
	records := r.ledgerSelect(ctx)
	options := &sql.TxOptions{ReadOnly: true}
	if r.db.Dialector.Name() == "postgres" {
		options.Isolation = sql.LevelRepeatableRead
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var names []string
		if err := tx.Table("partner").Where("id=?", q.PartnerID).Pluck("name", &names).Error; err != nil {
			return err
		}
		if len(names) == 0 {
			return ErrNotFound
		}
		out.PartnerName = names[0]
		// Totals stream one integer amount per row, so a long ledger is never
		// held in memory while several sums may exceed int64.
		totals := q
		totals.From = ""
		rows, err := r.ledgerQuery(tx, ctx, totals).Select("e.amount_cents,e.effective_at").Rows()
		if err != nil {
			return err
		}
		opening, increase, decrease := new(big.Int), new(big.Int), new(big.Int)
		for rows.Next() {
			var amount int64
			var effective time.Time
			if err := rows.Scan(&amount, &effective); err != nil {
				rows.Close()
				return err
			}
			if effective.Before(from) {
				opening.Add(opening, big.NewInt(amount))
				continue
			}
			out.Total++
			if amount > 0 {
				increase.Add(increase, big.NewInt(amount))
			} else {
				decrease.Add(decrease, big.NewInt(-amount))
			}
		}
		scanErr := rows.Err()
		rows.Close()
		if scanErr != nil {
			return scanErr
		}
		page := []entryRow{}
		window := r.ledgerQuery(tx, ctx, q).Select(records).Order(ledgerOrderOldestFirst)
		skip := false
		if q.Page > 0 {
			offset, beyond := pageOffset(q.Page, q.PageSize)
			skip = beyond
			if !beyond {
				window = window.Offset(offset).Limit(q.PageSize)
			}
		}
		if !skip {
			if err := window.Scan(&page).Error; err != nil {
				return err
			}
		}
		for _, row := range page {
			out.Records = append(out.Records, fromRow(row))
		}
		net := new(big.Int).Sub(increase, decrease)
		closing := new(big.Int).Add(opening, net)
		out.OpeningAmount = moneyBig(opening)
		out.IncreaseAmount = moneyBig(increase)
		out.DecreaseAmount = moneyBig(decrease)
		out.NetChange = moneyBig(net)
		out.ClosingAmount = moneyBig(closing)
		return nil
	}, options)
	if err != nil {
		return Statement{}, err
	}
	return out, nil
}

func (r *Repository) ledgerSelect(ctx context.Context) string {
	if !r.db.WithContext(ctx).Migrator().HasTable(&Attachment{}) {
		return ledgerSelect + ", 0 AS attachment_count"
	}
	return ledgerSelect + ", (SELECT COUNT(*) FROM partner_entry_attachment a WHERE a.entry_id=e.id AND a.removed_at IS NULL) AS attachment_count"
}
