package database

import (
	"strings"

	"gorm.io/gorm"
)

// LiteralContains applies a trimmed substring search across trusted, static
// column names. Only ASCII A-Z is folded: SQLite's built-in case folding is
// ASCII-only, so PostgreSQL uses the same rule regardless of server locale.
// Other characters match literally, including LIKE wildcards and the escape.
func LiteralContains(db *gorm.DB, keyword string, columns ...string) *gorm.DB {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		return db
	}
	fold := func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		return r
	}
	keyword = strings.Map(fold, keyword)
	pattern := "%" + strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(keyword) + "%"
	conditions := make([]string, len(columns))
	args := make([]any, len(columns))
	for i, column := range columns {
		expression := "lower(" + column + ")"
		if db.Dialector.Name() == DriverPostgres {
			expression = "translate(" + column + ", 'ABCDEFGHIJKLMNOPQRSTUVWXYZ', 'abcdefghijklmnopqrstuvwxyz')"
		}
		conditions[i] = expression + " LIKE ? ESCAPE '!'"
		args[i] = pattern
	}
	return db.Where("("+strings.Join(conditions, " OR ")+")", args...)
}
