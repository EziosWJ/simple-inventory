// Package printprofile stores the single operator identity used on printed
// business documents.
package printprofile

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/EziosWJ/simple-inventory/server/internal/audit"
	"gorm.io/gorm"
)

const (
	nameKey    = "business.print-profile.name"
	phoneKey   = "business.print-profile.phone"
	addressKey = "business.print-profile.address"
)

var ErrInvalid = errors.New("经营者打印资料参数错误")

type Profile struct {
	Name    string `json:"name"`
	Phone   string `json:"phone"`
	Address string `json:"address"`
}

type Store interface {
	Get(context.Context) (Profile, error)
	Update(context.Context, Profile, audit.Event) (Profile, error)
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func (s *Service) Get(ctx context.Context) (Profile, error) { return s.store.Get(ctx) }

func (s *Service) Update(ctx context.Context, actor audit.Metadata, in Profile) (Profile, error) {
	in.Name = strings.TrimSpace(in.Name)
	in.Phone = strings.TrimSpace(in.Phone)
	in.Address = strings.TrimSpace(in.Address)
	if in.Name == "" || !within(in.Name, 200) || !within(in.Phone, 50) || !within(in.Address, 500) {
		return Profile{}, ErrInvalid
	}
	return s.store.Update(ctx, in, audit.Event{
		Action: "print-profile.update", Resource: "print-profile",
		Summary: "维护经营者打印资料", Metadata: actor,
	})
}

func within(s string, limit int) bool { return utf8.RuneCountInString(s) <= limit }

type Repository struct{ db *gorm.DB }

func NewRepository(db *gorm.DB) *Repository { return &Repository{db: db} }

func (r *Repository) Get(ctx context.Context) (Profile, error) {
	values, err := r.read(ctx, r.db.WithContext(ctx))
	return values, err
}

func (r *Repository) read(ctx context.Context, db *gorm.DB) (Profile, error) {
	var rows []struct {
		ConfigKey   string `gorm:"column:config_key"`
		ConfigValue string `gorm:"column:config_value"`
	}
	err := db.WithContext(ctx).Table("sys_config").Select("config_key, config_value").Where("config_key IN ? AND deleted=0", []string{nameKey, phoneKey, addressKey}).Find(&rows).Error
	if err != nil {
		return Profile{}, err
	}
	profile := Profile{}
	for _, row := range rows {
		switch row.ConfigKey {
		case nameKey:
			profile.Name = row.ConfigValue
		case phoneKey:
			profile.Phone = row.ConfigValue
		case addressKey:
			profile.Address = row.ConfigValue
		}
	}
	return profile, nil
}

func (r *Repository) Update(ctx context.Context, profile Profile, event audit.Event) (Profile, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, pair := range []struct{ key, value string }{{nameKey, profile.Name}, {phoneKey, profile.Phone}, {addressKey, profile.Address}} {
			result := tx.Table("sys_config").Where("config_key=? AND deleted=0", pair.key).Updates(map[string]any{
				"config_value": pair.value,
				"update_time":  time.Now().UTC(),
			})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return errors.New("经营者打印资料配置未初始化")
			}
		}
		return audit.RecordOn(ctx, tx, event)
	})
	if err != nil {
		return Profile{}, err
	}
	return r.Get(ctx)
}
