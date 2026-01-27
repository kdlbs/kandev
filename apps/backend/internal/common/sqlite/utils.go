// Package sqlite provides common SQLite utility functions.
package sqlite

import (
	"database/sql"
	"fmt"
)

// BoolToInt converts a boolean to an integer (for SQLite).
func BoolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

// EnsureColumn adds a column to a table if it doesn't exist.
func EnsureColumn(db *sql.DB, table, column, definition string) error {
	exists, err := ColumnExists(db, table, column)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	query := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition)
	_, err = db.Exec(query)
	return err
}

// ColumnExists checks if a column exists in a table.
func ColumnExists(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, err
	}
	defer func() {
		_ = rows.Close()
	}()

	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

