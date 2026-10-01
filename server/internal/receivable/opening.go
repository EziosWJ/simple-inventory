package receivable

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/EziosWJ/simple-inventory/server/internal/audit"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvalid  = errors.New("参数错误")
	ErrNotFound = errors.New("往来记录不存在")
	ErrConflict = errors.New("记录已冲销或余额不足，操作未生效")
)

type Balance struct {
	PartnerID   int64  `json:"partnerId"`
	PartnerName string `json:"partnerName"`
	Direction   string `json:"direction"`
	Amount      string `json:"amount"`
	EntryCount  int64  `json:"entryCount"`
	HasRecords  bool   `json:"hasRecords"`
}
type BalancePage struct {
	Records  []Balance `json:"records"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"pageSize"`
}
type balanceRow struct {
	ID          int64 `gorm:"primaryKey"`
	PartnerID   int64
	Direction   string
	AmountCents int64 `gorm:"column:amount_cents"`
	EntryCount  int64
	UpdateTime  time.Time
}

func (balanceRow) TableName() string { return "partner_balance" }

type Entry struct {
	PartnerName        string    `json:"partnerName"`
	OperatorName       string    `json:"operatorName"`
	PurchaseReturnID   *int64    `json:"purchaseReturnId,omitempty"`
	SaleReturnID       *int64    `json:"saleReturnId,omitempty"`
	ReversedDocumentNo string    `json:"reversedDocumentNo,omitempty"`
	ID                 int64     `json:"id"`
	PartnerID          int64     `json:"partnerId"`
	Direction          string    `json:"direction"`
	EntryType          string    `json:"entryType"`
	Amount             string    `json:"amount"`
	BalanceBefore      string    `json:"balanceBefore"`
	BalanceAfter       string    `json:"balanceAfter"`
	BusinessDate       string    `json:"businessDate"`
	EffectiveAt        time.Time `json:"effectiveAt"`
	Description        string    `json:"description"`
	DocumentNo         string    `json:"documentNo"`
	OperatorID         int64     `json:"operatorId"`
	ReversesID         *int64    `json:"reversesId"`
	ReversedByID       *int64    `json:"reversedById"`
	PaymentMethod      string    `json:"paymentMethod,omitempty"`
	TransactionNo      string    `json:"transactionNo,omitempty"`
	PurchaseID         *int64    `json:"purchaseId,omitempty"`
	SaleID             *int64    `json:"saleId,omitempty"`
}

func (Entry) TableName() string { return "partner_balance_entry" }

type Input struct {
	RequestKey   string `json:"requestKey"`
	PartnerID    int64  `json:"partnerId"`
	Direction    string `json:"direction"`
	Amount       string `json:"amount"`
	BusinessDate string `json:"businessDate"`
	Description  string `json:"description"`
}
type SettlementInput struct {
	RequestKey    string `json:"requestKey"`
	PartnerID     int64  `json:"partnerId"`
	Direction     string `json:"direction"`
	Amount        string `json:"amount"`
	BusinessDate  string `json:"businessDate"`
	PaymentMethod string `json:"paymentMethod"`
	TransactionNo string `json:"transactionNo"`
	Remark        string `json:"remark"`
}
type Page struct {
	Records  []Entry `json:"records"`
	Total    int64   `json:"total"`
	Page     int     `json:"page"`
	PageSize int     `json:"pageSize"`
}
type Store interface {
	FilterPage(context.Context, EntryFilter) (Page, error)
	Statement(context.Context, EntryFilter) (Statement, error)
	Create(context.Context, audit.Metadata, Input) (Entry, error)
	Find(context.Context, int64) (Entry, error)
	Page(context.Context, int64, string, int, int) (Page, error)
	Balances(context.Context, int64, int, int, string) (BalancePage, error)
	Reverse(context.Context, audit.Metadata, int64, string) (Entry, error)
	Settle(context.Context, audit.Metadata, SettlementInput) (Entry, error)
	Refund(context.Context, audit.Metadata, SettlementInput) (Entry, error)
}
type Service struct{ store Store }
type Repository struct{ db *gorm.DB }

func NewService(store Store) *Service       { return &Service{store: store} }
func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }
func cents(raw string) (int64, error) {
	s := strings.TrimSpace(raw)
	if s == "" || strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") {
		return 0, ErrInvalid
	}
	for _, ch := range s {
		if ch != '.' && (ch < '0' || ch > '9') {
			return 0, ErrInvalid
		}
	}
	p := strings.Split(s, ".")
	if len(p) > 2 || p[0] == "" || (len(p) == 2 && (len(p[1]) == 0 || len(p[1]) > 2)) {
		return 0, ErrInvalid
	}
	whole, e := strconv.ParseInt(p[0], 10, 64)
	if e != nil || whole > math.MaxInt64/100 {
		return 0, ErrInvalid
	}
	frac := "00"
	if len(p) == 2 {
		frac = p[1]
		if len(frac) == 1 {
			frac += "0"
		}
	}
	f, e := strconv.ParseInt(frac, 10, 64)
	if e != nil || whole > (math.MaxInt64-f)/100 {
		return 0, ErrInvalid
	}
	n := whole*100 + f
	if n <= 0 {
		return 0, ErrInvalid
	}
	return n, nil
}
func money(n int64) string {
	var magnitude uint64
	sign := ""
	if n < 0 {
		sign = "-"
		magnitude = uint64(-(n + 1)) + 1
	} else {
		magnitude = uint64(n)
	}
	return fmt.Sprintf("%s%d.%02d", sign, magnitude/100, magnitude%100)
}
func validateInput(in Input) error {
	amount, e := cents(in.Amount)
	if e != nil || in.PartnerID <= 0 || (in.Direction != "CUSTOMER" && in.Direction != "SUPPLIER") || strings.TrimSpace(in.Description) == "" || utf8.RuneCountInString(strings.TrimSpace(in.Description)) > 500 || strings.TrimSpace(in.RequestKey) == "" || utf8.RuneCountInString(in.RequestKey) > 100 {
		return ErrInvalid
	}
	if _, e = time.Parse("2006-01-02", in.BusinessDate); e != nil {
		return ErrInvalid
	}
	_ = amount
	return nil
}
func (s *Service) Create(ctx context.Context, meta audit.Metadata, in Input) (Entry, error) {
	if e := validateInput(in); e != nil {
		return Entry{}, e
	}
	return s.store.Create(ctx, meta, in)
}
func validateFunds(in SettlementInput) error {
	_, err := cents(in.Amount)
	if err != nil || in.PartnerID <= 0 || (in.Direction != "CUSTOMER" && in.Direction != "SUPPLIER") || strings.TrimSpace(in.RequestKey) == "" || len(in.RequestKey) > 100 || !validMethod(in.PaymentMethod) || utf8.RuneCountInString(in.TransactionNo) > 100 || utf8.RuneCountInString(in.Remark) > 500 {
		return ErrInvalid
	}
	if _, err = time.Parse("2006-01-02", in.BusinessDate); err != nil {
		return ErrInvalid
	}
	return nil
}
func (s *Service) Settle(ctx context.Context, meta audit.Metadata, in SettlementInput) (Entry, error) {
	if err := validateFunds(in); err != nil {
		return Entry{}, err
	}
	return s.store.Settle(ctx, meta, in)
}
func (s *Service) Refund(ctx context.Context, meta audit.Metadata, in SettlementInput) (Entry, error) {
	if err := validateFunds(in); err != nil {
		return Entry{}, err
	}
	return s.store.Refund(ctx, meta, in)
}
func validMethod(v string) bool {
	switch v {
	case "CASH", "WECHAT", "ALIPAY", "BANK_TRANSFER", "OTHER":
		return true
	}
	return false
}
func (s *Service) Page(ctx context.Context, partner int64, direction string, page, size int) (Page, error) {
	if partner < 0 || page < 1 || size < 1 || size > 500 || (direction != "" && direction != "CUSTOMER" && direction != "SUPPLIER") {
		return Page{}, ErrInvalid
	}
	return s.store.Page(ctx, partner, direction, page, size)
}
func (s *Service) Detail(ctx context.Context, id int64) (Entry, error) {
	if id <= 0 {
		return Entry{}, ErrInvalid
	}
	return s.store.Find(ctx, id)
}
func (s *Service) Balances(ctx context.Context, partnerID int64, page, size int, direction string) (BalancePage, error) {
	if partnerID < 0 || page < 1 || size < 1 || size > 500 || (direction != "" && direction != "CUSTOMER" && direction != "SUPPLIER") {
		return BalancePage{}, ErrInvalid
	}
	return s.store.Balances(ctx, partnerID, page, size, direction)
}
func (s *Service) Reverse(ctx context.Context, meta audit.Metadata, id int64, reason string) (Entry, error) {
	if id <= 0 || strings.TrimSpace(reason) == "" || utf8.RuneCountInString(strings.TrimSpace(reason)) > 500 {
		return Entry{}, ErrInvalid
	}
	return s.store.Reverse(ctx, meta, id, strings.TrimSpace(reason))
}
func (s *Repository) Create(ctx context.Context, meta audit.Metadata, in Input) (Entry, error) {
	amount, e := cents(in.Amount)
	if e != nil || in.PartnerID <= 0 || (in.Direction != "CUSTOMER" && in.Direction != "SUPPLIER") || strings.TrimSpace(in.Description) == "" || utf8.RuneCountInString(strings.TrimSpace(in.Description)) > 500 {
		return Entry{}, ErrInvalid
	}
	date, e := time.Parse("2006-01-02", in.BusinessDate)
	if e != nil {
		return Entry{}, ErrInvalid
	}
	desc := strings.TrimSpace(in.Description)
	var out Entry
	e = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		checkRequestKey := func() (bool, error) {
			var previous entryRow
			er := tx.Where("request_key=?", strings.TrimSpace(in.RequestKey)).Take(&previous).Error
			if errors.Is(er, gorm.ErrRecordNotFound) {
				return false, nil
			}
			if er != nil {
				return false, er
			}
			previousDate := previous.BusinessDate
			if len(previousDate) > 10 {
				previousDate = previousDate[:10]
			}
			if previous.PartnerID != in.PartnerID || previous.Direction != in.Direction || previous.AmountCents != amount || previousDate != date.Format("2006-01-02") || previous.Description != desc {
				return false, ErrConflict
			}
			out = fromRow(previous)
			return true, nil
		}
		if done, er := checkRequestKey(); er != nil {
			return er
		} else if done {
			return nil
		}
		var p struct {
			ID         int64
			IsCustomer bool `gorm:"column:is_customer"`
			IsSupplier bool `gorm:"column:is_supplier"`
		}
		if er := tx.Table("partner").Select("id,is_customer,is_supplier").Where("id=? AND status=1", in.PartnerID).Take(&p).Error; er != nil {
			return ErrInvalid
		}
		if in.Direction == "CUSTOMER" && !p.IsCustomer || in.Direction == "SUPPLIER" && !p.IsSupplier {
			return ErrInvalid
		}
		b := balanceRow{PartnerID: in.PartnerID, Direction: in.Direction}
		if er := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "partner_id"}, {Name: "direction"}}, DoNothing: true}).Create(&b).Error; er != nil {
			return er
		}
		if er := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("partner_id=? AND direction=?", in.PartnerID, in.Direction).Take(&b).Error; er != nil {
			return er
		}
		if done, er := checkRequestKey(); er != nil {
			return er
		} else if done {
			return nil
		}
		before := int64(0)
		if er := tx.Model(&Entry{}).Select("COALESCE(SUM(amount_cents),0)").Where("partner_id=? AND direction=?", in.PartnerID, in.Direction).Scan(&before).Error; er != nil {
			return er
		}
		if before > math.MaxInt64-amount {
			return ErrInvalid
		}
		now := time.Now().UTC()
		out = Entry{PartnerID: in.PartnerID, Direction: in.Direction, EntryType: "OPENING", Amount: money(amount), BalanceBefore: money(before), BalanceAfter: money(before + amount), BusinessDate: date.Format("2006-01-02"), EffectiveAt: now, Description: desc, DocumentNo: fmt.Sprintf("OB%s-%d", now.Format("20060102150405"), now.UnixNano()), OperatorID: meta.ActorID}
		requestKey := strings.TrimSpace(in.RequestKey)
		row := entryRow{PartnerID: out.PartnerID, Direction: out.Direction, EntryType: out.EntryType, AmountCents: amount, BalanceBeforeCents: before, BalanceAfterCents: before + amount, BusinessDate: out.BusinessDate, EffectiveAt: now, Description: desc, DocumentNo: out.DocumentNo, OperatorID: meta.ActorID, RequestKey: &requestKey}
		if er := tx.Create(&row).Error; er != nil {
			return er
		}
		out.ID = row.ID
		if er := tx.Model(&b).Updates(map[string]any{"amount_cents": before + amount, "entry_count": gorm.Expr("entry_count+1"), "update_time": now}).Error; er != nil {
			return er
		}
		return audit.RecordOn(ctx, tx, audit.Event{Action: "partner.opening.create", Resource: "partner_balance", ResourceID: out.ID, Summary: "录入期初往来", Metadata: meta})
	})
	return out, e
}

type entryRow struct {
	PartnerName, OperatorName, ReversedDocumentNo      string `gorm:"->"`
	PurchaseReturnID, SaleReturnID                     *int64
	ID                                                 int64 `gorm:"primaryKey"`
	PartnerID                                          int64
	Direction, EntryType                               string
	AmountCents, BalanceBeforeCents, BalanceAfterCents int64
	BusinessDate                                       string
	EffectiveAt                                        time.Time
	Description, DocumentNo                            string
	OperatorID                                         int64
	ReversesID, ReversedByID                           *int64
	RequestKey                                         *string
	PaymentMethod, TransactionNo                       string
	PurchaseID, SaleID                                 *int64
}

func (entryRow) TableName() string { return "partner_balance_entry" }
func (s *Repository) Find(ctx context.Context, id int64) (Entry, error) {
	rows, e := s.ledgerRows(ctx, EntryFilter{EntryID: id}, false)
	if e != nil {
		return Entry{}, e
	}
	for _, r := range rows {
		if r.ID == id {
			return fromRow(r), nil
		}
	}
	return Entry{}, ErrNotFound
}

func (s *Repository) Page(ctx context.Context, partner int64, direction string, page, size int) (Page, error) {
	return s.FilterPage(ctx, EntryFilter{PartnerID: partner, Direction: direction, Page: page, PageSize: size})
}
func fromRow(r entryRow) Entry {
	return Entry{PartnerName: r.PartnerName, OperatorName: r.OperatorName, PurchaseReturnID: r.PurchaseReturnID, SaleReturnID: r.SaleReturnID, ReversedDocumentNo: r.ReversedDocumentNo, ID: r.ID, PartnerID: r.PartnerID, Direction: r.Direction, EntryType: r.EntryType, Amount: money(r.AmountCents), BalanceBefore: money(r.BalanceBeforeCents), BalanceAfter: money(r.BalanceAfterCents), BusinessDate: r.BusinessDate, EffectiveAt: r.EffectiveAt, Description: r.Description, DocumentNo: r.DocumentNo, OperatorID: r.OperatorID, ReversesID: r.ReversesID, ReversedByID: r.ReversedByID, PaymentMethod: r.PaymentMethod, TransactionNo: r.TransactionNo, PurchaseID: r.PurchaseID, SaleID: r.SaleID}
}

func (s *Repository) Balances(ctx context.Context, partnerID int64, page, size int, direction string) (BalancePage, error) {
	if page < 1 || size < 1 || size > 500 {
		return BalancePage{}, ErrInvalid
	}
	if direction != "" && direction != "CUSTOMER" && direction != "SUPPLIER" {
		return BalancePage{}, ErrInvalid
	}
	whereCustomer, whereSupplier := "(p.is_customer = TRUE OR b.id IS NOT NULL)", "(p.is_supplier = TRUE OR b.id IS NOT NULL)"
	if direction == "SUPPLIER" {
		whereCustomer = "1=0"
	}
	if direction == "CUSTOMER" {
		whereSupplier = "1=0"
	}
	if partnerID > 0 {
		whereCustomer = fmt.Sprintf("(%s) AND p.id=%d", whereCustomer, partnerID)
		whereSupplier = fmt.Sprintf("(%s) AND p.id=%d", whereSupplier, partnerID)
	}
	query := `SELECT p.id AS partner_id,p.name AS partner_name,'CUSTOMER' AS direction,COALESCE(b.amount_cents,0) AS amount_cents,COALESCE(b.entry_count,0) AS entry_count,CASE WHEN b.id IS NULL THEN FALSE ELSE TRUE END AS has_records FROM partner p LEFT JOIN partner_balance b ON b.partner_id=p.id AND b.direction='CUSTOMER' WHERE ` + whereCustomer + ` UNION ALL SELECT p.id AS partner_id,p.name AS partner_name,'SUPPLIER' AS direction,COALESCE(b.amount_cents,0) AS amount_cents,COALESCE(b.entry_count,0) AS entry_count,CASE WHEN b.id IS NULL THEN FALSE ELSE TRUE END AS has_records FROM partner p LEFT JOIN partner_balance b ON b.partner_id=p.id AND b.direction='SUPPLIER' WHERE ` + whereSupplier
	var total int64
	if e := s.db.WithContext(ctx).Table("(" + query + ") AS balances").Count(&total).Error; e != nil {
		return BalancePage{}, e
	}
	type row struct {
		PartnerID               int64 `gorm:"column:partner_id"`
		PartnerName, Direction  string
		AmountCents, EntryCount int64
		HasRecords              bool
	}
	rows := []row{}
	if e := s.db.WithContext(ctx).Table("(" + query + ") AS balances").Select("partner_id,partner_name,direction,amount_cents,entry_count,has_records").Order("partner_id,direction").Offset((page - 1) * size).Limit(size).Scan(&rows).Error; e != nil {
		return BalancePage{}, e
	}
	out := BalancePage{Records: []Balance{}, Total: total, Page: page, PageSize: size}
	for _, r := range rows {
		out.Records = append(out.Records, Balance{PartnerID: r.PartnerID, PartnerName: r.PartnerName, Direction: r.Direction, Amount: money(r.AmountCents), EntryCount: r.EntryCount, HasRecords: r.HasRecords})
	}
	return out, nil
}
func (s *Repository) Reverse(ctx context.Context, meta audit.Metadata, id int64, reason string) (Entry, error) {
	reason = strings.TrimSpace(reason)
	if id <= 0 || reason == "" || utf8.RuneCountInString(reason) > 500 {
		return Entry{}, ErrInvalid
	}
	var out Entry
	e := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Lock the direction before reading the original; the write also reserves SQLite.
		if er := tx.Exec("UPDATE partner_balance SET amount_cents=amount_cents WHERE (partner_id,direction) IN (SELECT partner_id,direction FROM partner_balance_entry WHERE id=?)", id).Error; er != nil {
			return er
		}
		var orig entryRow
		if er := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&orig, id).Error; er != nil {
			return ErrNotFound
		}
		if (orig.EntryType != "OPENING" && orig.EntryType != "RECEIPT" && orig.EntryType != "PAYMENT" && orig.EntryType != "CUSTOMER_REFUND" && orig.EntryType != "SUPPLIER_REFUND") || orig.ReversedByID != nil {
			return ErrConflict
		}
		amount := -orig.AmountCents
		var b balanceRow
		if er := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("partner_id=? AND direction=?", orig.PartnerID, orig.Direction).Take(&b).Error; er != nil {
			return er
		}
		var current int64
		if er := tx.Model(&entryRow{}).Select("COALESCE(SUM(amount_cents),0)").Where("partner_id=? AND direction=?", orig.PartnerID, orig.Direction).Scan(&current).Error; er != nil {
			return er
		}
		if (amount > 0 && current > math.MaxInt64-amount) || (amount < 0 && current < math.MinInt64-amount) || (orig.EntryType == "OPENING" && current+amount < 0) {
			return ErrConflict
		}
		now := time.Now().UTC()
		rev := entryRow{PartnerID: orig.PartnerID, Direction: orig.Direction, EntryType: "REVERSAL", AmountCents: amount, BalanceBeforeCents: current, BalanceAfterCents: current + amount, BusinessDate: now.Format("2006-01-02"), EffectiveAt: now, Description: reason, DocumentNo: fmt.Sprintf("RV%s-%d", now.Format("20060102150405"), now.UnixNano()), OperatorID: meta.ActorID, ReversesID: &orig.ID}
		if er := tx.Create(&rev).Error; er != nil {
			return er
		}
		res := tx.Model(&entryRow{}).Where("id=? AND reversed_by_id IS NULL", orig.ID).Update("reversed_by_id", rev.ID)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrConflict
		}
		if er := tx.Model(&b).Updates(map[string]any{"amount_cents": current + amount, "entry_count": gorm.Expr("entry_count+1"), "update_time": now}).Error; er != nil {
			return er
		}
		if er := audit.RecordOn(ctx, tx, audit.Event{Action: "partner.balance.reverse", Resource: "partner_balance", ResourceID: orig.ID, Summary: "冲销往来记录：" + reason, Metadata: meta}); er != nil {
			return er
		}
		out = fromRow(rev)
		return nil
	})
	return out, e
}
