// Package dialect provides SQL fragment helpers for SQLite/PostgreSQL portability.
package dialect

const (
	SQLite3 = "sqlite3"
	PGX     = "pgx"
)

// IsPostgres returns true if the driver is PostgreSQL (pgx).
func IsPostgres(driver string) bool {
	return driver == PGX
}

// BoolToInt converts a boolean to an integer for SQL storage.
func BoolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
