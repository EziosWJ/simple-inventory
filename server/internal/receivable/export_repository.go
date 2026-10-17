package receivable

import (
	"context"
	"database/sql"

	"github.com/EziosWJ/simple-inventory/server/internal/platform/exportfile"
	"gorm.io/gorm"
)

type LedgerExport struct {
	Records                 []Entry
	Statement               *Statement
	PartnerName, ExportedBy string
}

func (r *Repository) LedgerExport(ctx context.Context, q EntryFilter, actorID int64) (LedgerExport, error) {
	out := LedgerExport{Records: []Entry{}}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		repo := &Repository{db: tx}
		rows, err := repo.ledgerRows(ctx, q, ledgerOrderNewestFirst, exportfile.RowLimit+1)
		if err != nil {
			return err
		}
		if len(rows) > exportfile.RowLimit {
			return exportfile.ErrLimit
		}
		for _, row := range rows {
			out.Records = append(out.Records, fromRow(row))
		}
		if q.PartnerID > 0 {
			if err = tx.Table("partner").Select("name").Where("id=?", q.PartnerID).Scan(&out.PartnerName).Error; err != nil {
				return err
			}
			if q.Direction != "" && q.From != "" && q.To != "" {
				// The export prints the whole period, so it must not inherit the
				// listing page that shares this filter.
				statementQuery := q
				statementQuery.Page, statementQuery.PageSize = 0, 0
				statement, err := repo.Statement(ctx, statementQuery)
				if err != nil {
					return err
				}
				out.Statement = &statement
			}
		}
		return tx.Table("sys_user").Select("COALESCE(NULLIF(nickname,''),username)").Where("id=?", actorID).Scan(&out.ExportedBy).Error
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return out, err
}
