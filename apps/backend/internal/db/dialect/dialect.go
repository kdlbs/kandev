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

// AutoIncrementIDColumn returns the dialect-specific column definition for an
// auto-incrementing integer primary key named "id", for use in CREATE TABLE
// statements shared between SQLite and PostgreSQL.
func AutoIncrementIDColumn(driver string) string {
	if IsPostgres(driver) {
		return "id BIGSERIAL PRIMARY KEY"
	}
	return "id INTEGER PRIMARY KEY AUTOINCREMENT"
}

// ByteOrderedText wraps a column or expression reference so ORDER BY compares
// it byte-for-byte on every supported dialect, regardless of a database-level
// locale collation. SQLite's TEXT columns already default to the BINARY
// collation, so this is a no-op there; PostgreSQL's default template
// collation can be locale-aware, so this appends an explicit COLLATE "C" to
// force the same byte order there too.
func ByteOrderedText(driver, expr string) string {
	if IsPostgres(driver) {
		return expr + ` COLLATE "C"`
	}
	return expr
}

// BoolToInt converts a boolean to an integer for SQL storage.
func BoolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
