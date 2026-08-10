package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/db/dialect"
)

// migrateTaskPriorityToTextPostgres brings legacy PostgreSQL databases in line
// with the canonical task model. The original schema declared priority as an
// INTEGER, but task creation stores the string values critical/high/medium/low.
// SQLite already has a table-rebuild migration for this change; PostgreSQL
// needs its own ALTER because the SQLite migration checks sqlite_master and
// PRAGMA table_info.
//
// Existing integer values are mapped to medium, matching the SQLite migration.
// The transaction makes the type change and constraint update atomic, and the
// integer-type guard makes replay a no-op after the first successful migration.
func (r *Repository) migrateTaskPriorityToTextPostgres() error {
	if !dialect.IsPostgres(r.db.DriverName()) {
		return nil
	}

	var dataType string
	err := r.db.Get(&dataType, `
		SELECT data_type
		FROM information_schema.columns
		WHERE table_schema = current_schema()
		  AND table_name = 'tasks'
		  AND column_name = 'priority'
	`)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect tasks.priority type: %w", err)
	}
	if dataType != "integer" && dataType != "smallint" && dataType != "bigint" {
		return nil
	}

	tx, err := r.db.BeginTxx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin tasks.priority migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	statements := []struct {
		name string
		sql  string
	}{
		{"drop default", `ALTER TABLE tasks ALTER COLUMN priority DROP DEFAULT`},
		{"change type", `ALTER TABLE tasks ALTER COLUMN priority TYPE TEXT USING 'medium'`},
		{"set default", `ALTER TABLE tasks ALTER COLUMN priority SET DEFAULT 'medium'`},
		{"set not null", `ALTER TABLE tasks ALTER COLUMN priority SET NOT NULL`},
		{"add constraint", `ALTER TABLE tasks ADD CONSTRAINT tasks_priority_check CHECK (priority IN ('critical','high','medium','low'))`},
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement.sql); err != nil {
			return fmt.Errorf("tasks.priority migration (%s): %w", statement.name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit tasks.priority migration: %w", err)
	}
	return nil
}
