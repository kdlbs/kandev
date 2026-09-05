package db

import "database/sql"

// SchemaQuerier is the subset of sqlx database and transaction methods used
// by the portable schema probes.
type SchemaQuerier interface {
	DriverName() string
	Query(string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
	Rebind(string) string
}

// TableExists reports whether table exists in the active database schema.
// Missing tables are reported as false without an error.
func TableExists(conn SchemaQuerier, table string) (bool, error) {
	query := `SELECT EXISTS (
		SELECT 1 FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_name = ?
	)`
	if !isPostgresSchema(conn) {
		query = `SELECT EXISTS (
			SELECT 1 FROM sqlite_master
			WHERE type = 'table' AND name = ?
		)`
	}

	var exists bool
	if err := conn.QueryRow(conn.Rebind(query), table).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

// TableColumns returns the column names declared by table. An absent table is
// reported as an empty map without an error.
func TableColumns(conn SchemaQuerier, table string) (map[string]bool, error) {
	query := `SELECT column_name
		FROM information_schema.columns
		WHERE table_schema = current_schema() AND table_name = ?
		ORDER BY ordinal_position`
	if !isPostgresSchema(conn) {
		query = `SELECT name FROM pragma_table_info(?) ORDER BY cid`
	}

	rows, err := conn.Query(conn.Rebind(query), table)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	columns := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return columns, nil
}

// ColumnExists reports whether table declares column. Missing tables and
// columns are reported as false without an error.
func ColumnExists(conn SchemaQuerier, table, column string) (bool, error) {
	columns, err := TableColumns(conn, table)
	if err != nil {
		return false, err
	}
	return columns[column], nil
}

func isPostgresSchema(conn SchemaQuerier) bool {
	return conn.DriverName() == "pgx"
}
