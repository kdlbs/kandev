package db

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	postgresDuplicateColumn = "42701"
	postgresDuplicateTable  = "42P07"
	postgresDuplicateObject = "42710"
)

// IsDuplicateColumnError reports whether err means an ADD COLUMN migration has
// already been applied.
func IsDuplicateColumnError(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == postgresDuplicateColumn
	}
	return strings.Contains(err.Error(), "duplicate column name")
}

// IsAlreadyExistsError reports whether err means a schema object already
// exists and the migration can be treated as an idempotent replay.
func IsAlreadyExistsError(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case postgresDuplicateColumn, postgresDuplicateTable, postgresDuplicateObject:
			return true
		default:
			return false
		}
	}

	s := err.Error()
	return strings.Contains(s, "duplicate column name") ||
		isSQLiteDuplicateObjectMessage(s)
}

func isSQLiteDuplicateObjectMessage(s string) bool {
	return strings.HasPrefix(s, "table ") && strings.Contains(s, " already exists") ||
		strings.HasPrefix(s, "index ") && strings.Contains(s, " already exists") ||
		strings.HasPrefix(s, "trigger ") && strings.Contains(s, " already exists") ||
		strings.HasPrefix(s, "view ") && strings.Contains(s, " already exists")
}
