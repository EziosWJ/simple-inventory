package database

import (
	"strings"

	"gorm.io/gorm"
)

func asciiFold(value string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}, value)
}
func foldedColumn(db *gorm.DB, column string) string {
	if db.Dialector.Name() == DriverPostgres {
		return "translate(" + column + ", 'ABCDEFGHIJKLMNOPQRSTUVWXYZ', 'abcdefghijklmnopqrstuvwxyz')"
	}
	return "lower(" + column + ")"
}

// WordsCondition accepts only trusted column expressions. All words must match
// the same query row, while each word may match any of that row's fields.
func WordsCondition(db *gorm.DB, keyword string, columns ...string) (string, []any) {
	var groups []string
	var args []any
	for _, word := range strings.Fields(keyword) {
		pattern := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(asciiFold(word)) + "%"
		var fields []string
		for _, col := range columns {
			fields = append(fields, foldedColumn(db, col)+" LIKE ? ESCAPE '!'")
			args = append(args, pattern)
		}
		groups = append(groups, "("+strings.Join(fields, " OR ")+")")
	}
	if len(groups) == 0 {
		return "TRUE", nil
	}
	return "(" + strings.Join(groups, " AND ") + ")", args
}
func LiteralWords(db *gorm.DB, keyword string, columns ...string) *gorm.DB {
	sql, args := WordsCondition(db, keyword, columns...)
	return db.Where(sql, args...)
}

// ExactRank is selected as a transient rank, never persisted in a business row.
func ExactRank(db *gorm.DB, keyword string, columns ...string) (string, []any) {
	var terms []string
	var args []any
	for _, col := range columns {
		terms = append(terms, foldedColumn(db, col)+"=?")
		args = append(args, asciiFold(strings.TrimSpace(keyword)))
	}
	return "CASE WHEN (" + strings.Join(terms, " OR ") + ") THEN 0 ELSE 1 END AS search_rank", args
}
