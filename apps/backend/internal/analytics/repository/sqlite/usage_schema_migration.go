package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	"github.com/jmoiron/sqlx"
)

var usageTaskReference = regexp.MustCompile(`(?i)\s+REFERENCES\s+tasks\s*\(\s*id\s*\)\s+ON\s+DELETE\s+CASCADE`)
var usageSessionReference = regexp.MustCompile(`(?i)\s+REFERENCES\s+task_sessions\s*\(\s*id\s*\)\s+ON\s+DELETE\s+SET\s+NULL`)

func (r *Repository) removeUsageLiveForeignKeys(ctx context.Context) error {
	switch r.db.DriverName() {
	case usageDriverPostgres:
		return r.dropPostgresUsageLiveForeignKeys(ctx)
	case usageDriverSQLite:
		return r.rebuildSQLiteUsageTablesWithoutLiveForeignKeys(ctx)
	default:
		return fmt.Errorf("unsupported database driver for retained usage schema: %q", r.db.DriverName())
	}
}

func (r *Repository) dropPostgresUsageLiveForeignKeys(ctx context.Context) error {
	for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
		var constraints []string
		if err := r.db.SelectContext(ctx, &constraints, `
			SELECT c.conname
			FROM pg_constraint c
			JOIN pg_class child ON child.oid = c.conrelid
			JOIN pg_namespace ns ON ns.oid = child.relnamespace
			JOIN pg_class parent ON parent.oid = c.confrelid
			WHERE ns.nspname = current_schema() AND child.relname = $1 AND c.contype = 'f'
			  AND parent.relname IN ('tasks', 'task_sessions')`, table); err != nil {
			return fmt.Errorf("inspect retained usage foreign keys on %s: %w", table, err)
		}
		for _, constraint := range constraints {
			quoted := `"` + strings.ReplaceAll(constraint, `"`, `""`) + `"`
			if _, err := r.db.ExecContext(ctx, `ALTER TABLE `+table+` DROP CONSTRAINT `+quoted); err != nil {
				return fmt.Errorf("drop retained usage foreign key %s.%s: %w", table, constraint, err)
			}
		}
	}
	return nil
}

func (r *Repository) rebuildSQLiteUsageTablesWithoutLiveForeignKeys(ctx context.Context) error {
	conn, err := r.db.Connx(ctx)
	if err != nil {
		return fmt.Errorf("reserve SQLite connection for retained usage migration: %w", err)
	}
	defer func() { _ = conn.Close() }()
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return fmt.Errorf("disable SQLite foreign keys for retained usage migration: %w", err)
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), `PRAGMA foreign_keys = ON`) }()
	for _, table := range []string{usageMeasurementsTable, usageBucketsTable} {
		needsRebuild, err := sqliteUsageTableHasLiveForeignKey(ctx, conn, table)
		if err != nil {
			return err
		}
		if !needsRebuild {
			continue
		}
		if err := rebuildSQLiteUsageTable(ctx, conn, table); err != nil {
			return err
		}
	}
	for _, statement := range usageObservationIndexes {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("recreate retained usage index after foreign key migration: %w", err)
		}
	}
	return nil
}

func sqliteUsageTableHasLiveForeignKey(ctx context.Context, conn *sqlx.Conn, table string) (bool, error) {
	rows, err := conn.QueryxContext(ctx, `PRAGMA foreign_key_list("`+table+`")`)
	if err != nil {
		return false, fmt.Errorf("inspect SQLite retained usage foreign keys on %s: %w", table, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id, sequence int
		var parent, from, to, onUpdate, onDelete, match string
		if err := rows.Scan(&id, &sequence, &parent, &from, &to, &onUpdate, &onDelete, &match); err != nil {
			return false, err
		}
		if parent == "tasks" || parent == "task_sessions" {
			return true, nil
		}
	}
	return false, rows.Err()
}

func rebuildSQLiteUsageTable(ctx context.Context, conn *sqlx.Conn, table string) error {
	var sourceDDL string
	if err := conn.QueryRowxContext(ctx, `SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&sourceDDL); err != nil {
		return fmt.Errorf("read SQLite retained usage schema for %s: %w", table, err)
	}
	newTable := table + "_usagehistory_upgrade"
	ddl := strings.Replace(sourceDDL, "CREATE TABLE "+table, "CREATE TABLE "+newTable, 1)
	ddl = usageTaskReference.ReplaceAllString(ddl, "")
	ddl = usageSessionReference.ReplaceAllString(ddl, "")
	if strings.Contains(strings.ToLower(ddl), "references tasks") || strings.Contains(strings.ToLower(ddl), "references task_sessions") {
		return fmt.Errorf("cannot remove live foreign key from SQLite retained usage table %s", table)
	}
	var columns []string
	rows, err := conn.QueryxContext(ctx, `PRAGMA table_info("`+table+`")`)
	if err != nil {
		return fmt.Errorf("read SQLite retained usage columns for %s: %w", table, err)
	}
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			_ = rows.Close()
			return err
		}
		columns = append(columns, `"`+strings.ReplaceAll(name, `"`, `""`)+`"`)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(columns) == 0 {
		return fmt.Errorf("SQLite retained usage table %s has no columns", table)
	}
	columnList := strings.Join(columns, ", ")
	tx, err := conn.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin SQLite retained usage migration for %s: %w", table, err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, statement := range []string{
		ddl,
		`INSERT INTO "` + newTable + `" (` + columnList + `) SELECT ` + columnList + ` FROM "` + table + `"`,
		`DROP TABLE "` + table + `"`,
		`ALTER TABLE "` + newTable + `" RENAME TO "` + table + `"`,
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("rebuild SQLite retained usage table %s: %w", table, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit SQLite retained usage migration for %s: %w", table, err)
	}
	return nil
}

var usageObservationIndexes = []string{
	`CREATE INDEX IF NOT EXISTS idx_session_usage_measurements_workspace ON session_usage_measurements(workspace_id, collected_at)`,
	`CREATE INDEX IF NOT EXISTS idx_session_usage_measurements_session ON session_usage_measurements(workspace_id, session_id, model)`,
	`CREATE INDEX IF NOT EXISTS idx_session_usage_measurements_task ON session_usage_measurements(workspace_id, task_id, model)`,
	`CREATE INDEX IF NOT EXISTS idx_session_usage_buckets_workspace_date ON session_usage_buckets(workspace_id, coverage_start, source_date)`,
	`CREATE INDEX IF NOT EXISTS idx_session_usage_buckets_session ON session_usage_buckets(workspace_id, session_id, coverage_start)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_native_event ON session_usage_buckets(source_record_id) WHERE source = 'native'`,
}
