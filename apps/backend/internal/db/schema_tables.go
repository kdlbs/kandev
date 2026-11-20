package db

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// ExistingSQLiteTablesContext returns the requested tables present in SQLite.
// An error discards the entire inventory, including any rows already read.
func ExistingSQLiteTablesContext(ctx context.Context, conn *sqlx.DB, tables []string) (map[string]bool, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if conn.DriverName() != "sqlite3" {
		return nil, fmt.Errorf("SQLite table inventory requires the sqlite3 driver")
	}
	existing := make(map[string]bool)
	names := make([]string, 0, len(tables))
	seen := make(map[string]bool, len(tables))
	for _, table := range tables {
		if !seen[table] {
			names = append(names, table)
			seen[table] = true
		}
	}
	if len(names) == 0 {
		return existing, nil
	}
	query, args, err := sqlx.In("SELECT name FROM sqlite_master WHERE type = 'table' AND name IN (?)", names)
	if err != nil {
		return nil, err
	}
	rows, err := conn.QueryContext(ctx, conn.Rebind(query), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return existing, nil
}
