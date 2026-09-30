package warehouse

import (
	"context"
	"errors"
	"github.com/EziosWJ/simple-inventory/server/internal/audit"
	"github.com/EziosWJ/simple-inventory/server/internal/auth"
	platform "github.com/EziosWJ/simple-inventory/server/internal/platform/http"
	"gorm.io/gorm"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalid  = errors.New("参数错误")
	ErrNotFound = errors.New("仓库不存在")
)

type Warehouse struct {
	ID         int       `json:"-" gorm:"column:singleton_id;primaryKey"`
	Name       string    `json:"name"`
	Remark     *string   `json:"remark"`
	UpdateTime time.Time `json:"updateTime" gorm:"autoUpdateTime"`
}

func (Warehouse) TableName() string { return "warehouse" }

type Input struct {
	Name   string  `json:"name"`
	Remark *string `json:"remark"`
}
type Store interface {
	Get(context.Context) (Warehouse, error)
	Update(context.Context, Warehouse, audit.Event) error
}
type Service struct{ s Store }

func NewService(s Store) *Service                             { return &Service{s} }
func (s *Service) Get(ctx context.Context) (Warehouse, error) { return s.s.Get(ctx) }
func (s *Service) Update(ctx context.Context, m audit.Metadata, in Input) (Warehouse, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || utf8.RuneCountInString(in.Name) > 100 {
		return Warehouse{}, ErrInvalid
	}
	if in.Remark != nil {
		v := strings.TrimSpace(*in.Remark)
		if v == "" {
			in.Remark = nil
		} else {
			in.Remark = &v
		}
		if utf8.RuneCountInString(v) > 500 {
			return Warehouse{}, ErrInvalid
		}
	}
	w := Warehouse{ID: 1, Name: in.Name, Remark: in.Remark}
	if e := s.s.Update(ctx, w, audit.Event{Action: "warehouse.update", Resource: "warehouse", ResourceID: 1, Summary: "修改仓库", Metadata: m}); e != nil {
		return Warehouse{}, e
	}
	return s.s.Get(ctx)
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
func (r *Repository) Get(ctx context.Context) (Warehouse, error) {
	var v Warehouse
	e := r.db.WithContext(ctx).First(&v, 1).Error
	if errors.Is(e, gorm.ErrRecordNotFound) {
		return v, ErrNotFound
	}
	return v, e
}
func (r *Repository) Update(ctx context.Context, v Warehouse, event audit.Event) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&Warehouse{}).Where("singleton_id=1").Updates(map[string]any{"name": v.Name, "remark": v.Remark, "update_time": time.Now().UTC()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrNotFound
		}
		return audit.RecordOn(ctx, tx, event)
	})
}
