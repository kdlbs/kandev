package sqlite

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
)

const recoveryBlockIncidentUniqueIndex = `CREATE UNIQUE INDEX IF NOT EXISTS idx_session_recovery_blocks_open_reason
	ON session_recovery_blocks(session_id, incarnation_id, expected_generation, reason)
	WHERE state = 'open'`

func (r *Repository) migrateSessionRecoveryBlockIncidentUniqueness(ctx context.Context) error {
	if dialect.IsPostgres(r.db.DriverName()) {
		if err := r.dropPostgresRecoveryBlockIncidentConstraint(ctx); err != nil {
			return err
		}
		return r.ensureRecoveryBlockIndexes(ctx)
	}
	if r.db.DriverName() != dialect.SQLite3 {
		return fmt.Errorf("unsupported recovery block migration driver %q", r.db.DriverName())
	}
	legacy, err := r.sqliteHasLegacyRecoveryBlockIncidentConstraint(ctx)
	if err != nil {
		return err
	}
	if legacy {
		if err := r.rebuildSQLiteRecoveryBlocks(ctx); err != nil {
			return err
		}
	}
	return r.ensureRecoveryBlockIndexes(ctx)
}

func (r *Repository) dropPostgresRecoveryBlockIncidentConstraint(ctx context.Context) error {
	rows, err := r.db.QueryxContext(ctx, r.db.Rebind(`SELECT conname FROM pg_constraint
		WHERE conrelid = 'session_recovery_blocks'::regclass AND contype = 'u'
		AND pg_get_constraintdef(oid) = ?`),
		"UNIQUE (session_id, incarnation_id, expected_generation, reason)")
	if err != nil {
		return err
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return err
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, name := range names {
		if _, err := r.db.ExecContext(ctx, "ALTER TABLE session_recovery_blocks DROP CONSTRAINT "+quoteSQLIdentifier(name)); err != nil {
			return fmt.Errorf("drop legacy recovery block constraint %q: %w", name, err)
		}
	}
	return nil
}

func quoteSQLIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func (r *Repository) sqliteHasLegacyRecoveryBlockIncidentConstraint(ctx context.Context) (bool, error) {
	rows, err := r.db.QueryxContext(ctx, `PRAGMA index_list('session_recovery_blocks')`)
	if err != nil {
		return false, err
	}
	type index struct {
		Seq     int    `db:"seq"`
		Name    string `db:"name"`
		Unique  int    `db:"unique"`
		Origin  string `db:"origin"`
		Partial int    `db:"partial"`
	}
	var indexes []index
	for rows.Next() {
		var item index
		if err := rows.StructScan(&item); err != nil {
			_ = rows.Close()
			return false, err
		}
		indexes = append(indexes, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return false, err
	}
	if err := rows.Close(); err != nil {
		return false, err
	}
	for _, item := range indexes {
		if item.Unique == 0 || item.Partial != 0 {
			continue
		}
		columns, err := r.sqliteIndexColumns(ctx, item.Name)
		if err != nil {
			return false, err
		}
		if equalRecoveryBlockIncidentKey(columns) {
			return true, nil
		}
	}
	return false, nil
}

func (r *Repository) sqliteIndexColumns(ctx context.Context, name string) ([]string, error) {
	rows, err := r.db.QueryxContext(ctx, "PRAGMA index_info("+quoteSQLIdentifier(name)+")")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var columns []string
	for rows.Next() {
		var sequence, columnID int
		var column string
		if err := rows.Scan(&sequence, &columnID, &column); err != nil {
			return nil, err
		}
		columns = append(columns, column)
	}
	return columns, rows.Err()
}

//nolint:goconst // Keep schema identifiers readable in the SQL migration and column comparison.
func equalRecoveryBlockIncidentKey(columns []string) bool {
	return len(columns) == 4 && columns[0] == "session_id" && columns[1] == "incarnation_id" &&
		columns[2] == "expected_generation" && columns[3] == "reason"
}

func (r *Repository) rebuildSQLiteRecoveryBlocks(ctx context.Context) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	triggers, err := suspendSQLiteRecoveryBlockTriggers(ctx, tx)
	if err != nil {
		return err
	}
	statements := []string{
		`CREATE TABLE session_recovery_blocks_migration (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			incarnation_id TEXT NOT NULL,
			expected_generation BIGINT NOT NULL,
			reason TEXT NOT NULL,
			state TEXT NOT NULL,
			consumer_reference TEXT NOT NULL DEFAULT '',
			delivery_submission_id TEXT NOT NULL DEFAULT '',
			delivery_stream_id TEXT NOT NULL DEFAULT '',
			delivery_sequence BIGINT NOT NULL DEFAULT 0,
			delivery_turn_id TEXT NOT NULL DEFAULT '',
			delivery_outcome TEXT NOT NULL DEFAULT '',
			authorized_action TEXT NOT NULL DEFAULT '',
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
			resolved_at TIMESTAMP,
			FOREIGN KEY (session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
		)`,
		`INSERT INTO session_recovery_blocks_migration
			(id, session_id, incarnation_id, expected_generation, reason, state, consumer_reference,
			delivery_submission_id, delivery_stream_id, delivery_sequence, delivery_turn_id, delivery_outcome,
			authorized_action, created_at, updated_at, resolved_at)
			SELECT id, session_id, incarnation_id, expected_generation, reason, state, consumer_reference,
			delivery_submission_id, delivery_stream_id, delivery_sequence, delivery_turn_id, delivery_outcome,
			authorized_action, created_at, updated_at, resolved_at FROM session_recovery_blocks`,
		`DROP TABLE session_recovery_blocks`,
		`ALTER TABLE session_recovery_blocks_migration RENAME TO session_recovery_blocks`,
	}
	for _, statement := range statements {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("rebuild session recovery blocks: %w", err)
		}
	}
	if err := ensureSQLiteRecoveryBlockIndexes(ctx, tx); err != nil {
		return err
	}
	for _, trigger := range triggers {
		if _, err := tx.ExecContext(ctx, trigger.SQL); err != nil {
			return fmt.Errorf("restore recovery block trigger %q: %w", trigger.Name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func ensureSQLiteRecoveryBlockIndexes(ctx context.Context, executor *sqlx.Tx) error {
	for _, statement := range []string{
		`CREATE INDEX IF NOT EXISTS idx_session_recovery_blocks_open
			ON session_recovery_blocks(session_id, incarnation_id, state)`,
		recoveryBlockIncidentUniqueIndex,
	} {
		if _, err := executor.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) ensureRecoveryBlockIndexes(ctx context.Context) error {
	for _, statement := range []string{
		`CREATE INDEX IF NOT EXISTS idx_session_recovery_blocks_open
			ON session_recovery_blocks(session_id, incarnation_id, state)`,
		recoveryBlockIncidentUniqueIndex,
	} {
		if _, err := r.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("ensure session recovery block index: %w", err)
		}
	}
	return nil
}

// Recovery-block references in triggers must remain valid when SQLite validates
// the replacement table rename. Trigger DDL shares the rebuild transaction.
type sqliteRecoveryBlockTrigger struct {
	Name string `db:"name"`
	SQL  string `db:"sql"`
}

func suspendSQLiteRecoveryBlockTriggers(ctx context.Context, tx *sqlx.Tx) ([]sqliteRecoveryBlockTrigger, error) {
	var triggers []sqliteRecoveryBlockTrigger
	if err := tx.SelectContext(ctx, &triggers, `SELECT name, sql FROM sqlite_schema
		WHERE type = 'trigger' AND instr(lower(sql), 'session_recovery_blocks') > 0
		ORDER BY name`); err != nil {
		return nil, fmt.Errorf("read recovery block triggers: %w", err)
	}
	for _, trigger := range triggers {
		if _, err := tx.ExecContext(ctx, "DROP TRIGGER "+quoteSQLIdentifier(trigger.Name)); err != nil {
			return nil, fmt.Errorf("suspend recovery block trigger %q: %w", trigger.Name, err)
		}
	}
	return triggers, nil
}
