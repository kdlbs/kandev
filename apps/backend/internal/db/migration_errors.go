package db

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
)

const (
	postgresDuplicateColumn      = "42701"
	postgresDuplicateTable       = "42P07"
	postgresDuplicateObject      = "42710"
	postgresUndefinedTable       = "42P01"
	postgresForeignKeyViolation  = "23503"
	postgresSerializationFailure = "40001"
	postgresDeadlockDetected     = "40P01"
	sqliteAlreadyExistsText      = " already exists"
	sqliteForeignKeyViolationMsg = "FOREIGN KEY constraint failed"
	sqliteBusyText               = "database is locked"
	sqliteLockedText             = "database table is locked"
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
	return strings.HasPrefix(s, "table ") && strings.Contains(s, sqliteAlreadyExistsText) ||
		strings.HasPrefix(s, "index ") && strings.Contains(s, sqliteAlreadyExistsText) ||
		strings.HasPrefix(s, "trigger ") && strings.Contains(s, sqliteAlreadyExistsText) ||
		strings.HasPrefix(s, "view ") && strings.Contains(s, sqliteAlreadyExistsText)
}

// IsMissingTableError reports whether err means a query referenced a table
// that does not exist in this database — e.g. a cross-domain JOIN against a
// table owned by another package's schema that hasn't been initialised yet
// (isolated unit tests that only set up one domain's tables). Callers use
// this to fall back to a query that doesn't need the missing table, rather
// than failing outright.
func IsMissingTableError(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == postgresUndefinedTable
	}
	return strings.Contains(err.Error(), "no such table")
}

// IsForeignKeyViolation reports whether err means a write failed because it
// referenced a row that does not exist (or no longer exists) in a
// foreign-keyed table.
func IsForeignKeyViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == postgresForeignKeyViolation
	}
	return strings.Contains(err.Error(), sqliteForeignKeyViolationMsg)
}

// IsTransientError reports whether err means a write failed for a reason
// that is expected to clear on retry: a serialization failure or deadlock on
// PostgreSQL, or the single-writer connection being busy/locked on SQLite.
func IsTransientError(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case postgresSerializationFailure, postgresDeadlockDetected:
			return true
		default:
			return false
		}
	}
	s := err.Error()
	return strings.Contains(s, sqliteBusyText) || strings.Contains(s, sqliteLockedText)
}
