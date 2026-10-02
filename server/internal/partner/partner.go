package partner

import (
	"context"
	"errors"
	"fmt"
	"github.com/EziosWJ/simple-inventory/server/internal/audit"
	"github.com/EziosWJ/simple-inventory/server/internal/auth"
	platformdatabase "github.com/EziosWJ/simple-inventory/server/internal/platform/database"
	platform "github.com/EziosWJ/simple-inventory/server/internal/platform/http"
	"gorm.io/gorm"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalid  = errors.New("参数错误")
	ErrNotFound = errors.New("资料不存在")
	ErrConflict = errors.New("编码已存在")
)

type Partner struct {
	ID                int64     `json:"id"`
	Code              string    `json:"code"`
	Name              string    `json:"name"`
	Type              string    `json:"type"`
	IsCustomer        bool      `json:"isCustomer" gorm:"column:is_customer"`
	IsSupplier        bool      `json:"isSupplier" gorm:"column:is_supplier"`
	Contact           *string   `json:"contact"`
	Phone             *string   `json:"phone"`
	Address           *string   `json:"address"`
	Remark            *string   `json:"remark"`
	InvoiceName       *string   `json:"invoiceName"`
	TaxNumber         *string   `json:"taxNumber"`
	RegisteredAddress *string   `json:"registeredAddress"`
	RegisteredPhone   *string   `json:"registeredPhone"`
	BankName          *string   `json:"bankName"`
	BankAccount       *string   `json:"bankAccount"`
	Status            int       `json:"status"`
	CreateTime        time.Time `json:"createTime" gorm:"autoCreateTime"`
	UpdateTime        time.Time `json:"updateTime" gorm:"autoUpdateTime"`
}

func (Partner) TableName() string { return "partner" }

type Input struct {
	Code              string  `json:"code"`
	Name              string  `json:"name"`
	Type              string  `json:"type"`
	IsCustomer        bool    `json:"isCustomer"`
	IsSupplier        bool    `json:"isSupplier"`
	Contact           *string `json:"contact"`
	Phone             *string `json:"phone"`
	Address           *string `json:"address"`
	Remark            *string `json:"remark"`
	InvoiceName       *string `json:"invoiceName"`
	TaxNumber         *string `json:"taxNumber"`
	RegisteredAddress *string `json:"registeredAddress"`
	RegisteredPhone   *string `json:"registeredPhone"`
	BankName          *string `json:"bankName"`
	BankAccount       *string `json:"bankAccount"`
}
type Query struct {
	Page, PageSize          int
	Keyword, Type, Identity string
	Status                  *int
}
type Page struct {
	Records  []Partner `json:"records"`
	Total    int64     `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"pageSize"`
}
type Store interface {
	Page(context.Context, Query) (Page, error)
	Find(context.Context, int64) (*Partner, error)
	Save(context.Context, Partner, bool, audit.Event) (Partner, error)
	SetStatus(context.Context, int64, int, audit.Event) error
}
type Service struct{ s Store }

func NewService(s Store) *Service { return &Service{s} }
func (s *Service) Page(ctx context.Context, q Query) (Page, error) {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 {
		q.PageSize = 10
	}
	if q.PageSize > 500 {
		q.PageSize = 500
	}
	return s.s.Page(ctx, q)
}
func (s *Service) Detail(ctx context.Context, id int64) (*Partner, error) {
	if id <= 0 {
		return nil, ErrInvalid
	}
	return s.s.Find(ctx, id)
}
func (s *Service) Save(ctx context.Context, m audit.Metadata, id int64, in Input) (Partner, error) {
	if id < 0 {
		return Partner{}, ErrInvalid
	}
	in.Code = strings.ToUpper(strings.TrimSpace(in.Code))
	in.Name = strings.TrimSpace(in.Name)
	in.Type = strings.ToUpper(strings.TrimSpace(in.Type))
	contact, phone, address, remark := clean(in.Contact), clean(in.Phone), clean(in.Address), clean(in.Remark)
	invoiceName, taxNumber, registeredAddress, registeredPhone, bankName, bankAccount := clean(in.InvoiceName), clean(in.TaxNumber), clean(in.RegisteredAddress), clean(in.RegisteredPhone), clean(in.BankName), clean(in.BankAccount)
	if in.Name == "" || (in.Type != "COMPANY" && in.Type != "PERSON") || (!in.IsCustomer && !in.IsSupplier) ||
		!partnerText(in.Code, 50) || !partnerText(in.Name, 200) || !partnerPtr(contact, 100) || !partnerPtr(phone, 50) || !partnerPtr(address, 500) || !partnerPtr(remark, 500) ||
		!partnerPtr(invoiceName, 200) || !partnerPtr(taxNumber, 100) || !partnerPtr(registeredAddress, 500) || !partnerPtr(registeredPhone, 50) || !partnerPtr(bankName, 200) || !partnerPtr(bankAccount, 100) {
		return Partner{}, ErrInvalid
	}
	v := Partner{ID: id, Code: in.Code, Name: in.Name, Type: in.Type, IsCustomer: in.IsCustomer, IsSupplier: in.IsSupplier, Contact: contact, Phone: phone, Address: address, Remark: remark, InvoiceName: invoiceName, TaxNumber: taxNumber, RegisteredAddress: registeredAddress, RegisteredPhone: registeredPhone, BankName: bankName, BankAccount: bankAccount, Status: 1}
	if id > 0 {
		old, e := s.s.Find(ctx, id)
		if e != nil {
			return Partner{}, e
		}
		if v.Code == "" {
			v.Code = old.Code
		}
		v.Status = old.Status
	}
	return s.s.Save(ctx, v, id == 0, audit.Event{Action: "partner.save", Resource: "partner", ResourceID: id, Summary: "维护往来单位", Metadata: m})
}
func (s *Service) SetStatus(ctx context.Context, m audit.Metadata, id int64, st int) error {
	if id <= 0 || st != 0 && st != 1 {
		return ErrInvalid
	}
	if _, e := s.s.Find(ctx, id); e != nil {
		return e
	}
	return s.s.SetStatus(ctx, id, st, audit.Event{Action: "partner.status", Resource: "partner", ResourceID: id, Summary: "变更往来单位状态", Metadata: m})
}
func partnerText(v string, max int) bool { return utf8.RuneCountInString(v) <= max }
func partnerPtr(v *string, max int) bool { return v == nil || partnerText(*v, max) }
func clean(p *string) *string {
	if p == nil {
		return nil
	}
	v := strings.TrimSpace(*p)
	if v == "" {
		return nil
	}
	return &v
}
func meta(ctx context.Context) audit.Metadata {
	m := audit.Metadata{RequestID: platform.RequestIDFromContext(ctx)}
	if p, ok := auth.PrincipalFromContext(ctx); ok {
		m.ActorID = p.UserID
	}
	if r, ok := platform.RequestMetaFromContext(ctx); ok {
		m.ClientIP = r.ClientIP
		m.UserAgent = r.UserAgent
		m.RequestMethod = r.RequestMethod
		m.RequestURL = r.RequestURL
	}
	return m
}

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db} }
func (r *Repository) Page(ctx context.Context, q Query) (Page, error) {
	p := Page{Records: []Partner{}, Page: q.Page, PageSize: q.PageSize}
	d := r.db.WithContext(ctx).Model(&Partner{})
	d = platformdatabase.LiteralContains(d, q.Keyword, "code", "name", "contact", "phone")
	if q.Type != "" {
		d = d.Where("type=?", q.Type)
	}
	if q.Identity == "CUSTOMER" {
		d = d.Where("is_customer = ?", true)
	} else if q.Identity == "SUPPLIER" {
		d = d.Where("is_supplier = ?", true)
	} else if q.Identity != "" {
		return p, ErrInvalid
	}
	if q.Status != nil {
		d = d.Where("status=?", *q.Status)
	}
	if e := d.Count(&p.Total).Error; e != nil {
		return p, e
	}
	e := d.Order("id DESC").Offset((q.Page - 1) * q.PageSize).Limit(q.PageSize).Find(&p.Records).Error
	return p, e
}
func (r *Repository) Find(ctx context.Context, id int64) (*Partner, error) {
	var v Partner
	e := r.db.WithContext(ctx).First(&v, id).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return nil, ErrNotFound
	}
	return &v, e
}
func (r *Repository) Save(ctx context.Context, v Partner, create bool, event audit.Event) (Partner, error) {
	e := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if create {
			autoCode := v.Code == ""
			if autoCode {
				v.Code = fmt.Sprintf("PENDING-%d", time.Now().UnixNano())
			}
			if e := tx.Create(&v).Error; e != nil {
				return e
			}
			if autoCode {
				base := fmt.Sprintf("PT%08d", v.ID)
				for suffix := 0; ; suffix++ {
					v.Code = base
					if suffix > 0 {
						v.Code = fmt.Sprintf("%s-%d", base, suffix)
					}
					var count int64
					if e := tx.Model(&Partner{}).Where("code=?", v.Code).Count(&count).Error; e != nil {
						return e
					}
					if count == 0 {
						break
					}
				}
				if e := tx.Model(&v).Update("code", v.Code).Error; e != nil {
					return e
				}
			}
			event.ResourceID = v.ID
		} else {
			v.UpdateTime = time.Now().UTC()
			result := tx.Model(&Partner{}).Where("id=?", v.ID).Updates(map[string]any{"code": v.Code, "name": v.Name, "type": v.Type, "is_customer": v.IsCustomer, "is_supplier": v.IsSupplier, "contact": v.Contact, "phone": v.Phone, "address": v.Address, "remark": v.Remark, "invoice_name": v.InvoiceName, "tax_number": v.TaxNumber, "registered_address": v.RegisteredAddress, "registered_phone": v.RegisteredPhone, "bank_name": v.BankName, "bank_account": v.BankAccount, "update_time": v.UpdateTime})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return ErrNotFound
			}
		}
		return audit.RecordOn(ctx, tx, event)
	})
	if e != nil {
		if strings.Contains(strings.ToLower(e.Error()), "unique") || strings.Contains(strings.ToLower(e.Error()), "duplicate") {
			return Partner{}, ErrConflict
		}
		return Partner{}, e
	}
	stored, err := r.Find(ctx, v.ID)
	if err != nil {
		return Partner{}, err
	}
	return *stored, nil
}
func (r *Repository) SetStatus(ctx context.Context, id int64, st int, event audit.Event) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Partner{}).Where("id=?", id).Updates(map[string]any{"status": st, "update_time": time.Now().UTC()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return audit.RecordOn(ctx, tx, event)
	})
}
