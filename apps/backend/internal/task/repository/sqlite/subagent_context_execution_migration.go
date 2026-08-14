// Package sqlite provides SQLite-based repository implementations.
package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/db/dialect"
)

// subagentContextShapeMigrationName is shared between the WARN log this
// migration emits on failure and the test asserting that message/field shape
// — see MigrateLogger.Apply's exact contract (internal/db/migratelog.go).
const subagentContextShapeMigrationName = "task_session_subagents.execution_identity"

// migrateSubagentContextExecutionIdentity brings task_session_subagents to
// the AC-33 end state (agent_execution_id NOT NULL column, the 3-column
// UNIQUE key, the legacy 2-column UNIQUE key gone) and, only once that shape
// is schema-observed to hold, writes the execution_since activation key
// (AC-33b) — a fresh DB reaches the same shape via initSubagentContextSchema
// directly, so this still writes the key for it (AC-33 clause 6).
//
// Deliberately does NOT use the r.migrate.Apply / propagate-the-error
// convention every other recreateTableNamed-based migration in this package
// uses: AC-33a requires boot to never abort on this specific migration's
// failure, only WARN and retry next boot, with the table left in its
// pre-migration shape (recreateTable's own transaction rollback guarantees
// that) and execution_since left unwritten. So the error is caught and
// logged here instead of returned.
//
// Returns whether the AC-33 end state is schema-observed to hold once this
// call returns — AC-33c's caller (runMigrations) uses this to decide whether
// the backfill may run at all this pass.
func (r *Repository) migrateSubagentContextExecutionIdentity() bool {
	if err := r.migrateSubagentContextExecutionIdentityUnsafe(); err != nil {
		r.warnSubagentContextMigration(subagentContextShapeMigrationName, err)
		return false
	}

	holds, err := r.subagentContextExecutionShapeHolds()
	if err != nil {
		r.warnSubagentContextMigration(subagentContextShapeMigrationName+".shape_check", err)
		return false
	}
	if !holds {
		return false
	}
	r.migrate.Apply("kandev_meta.subagent_context_execution_since",
		subagentContextActivationKeyInsertSQL("subagent_context_execution_since", subagentContextNowRFC3339Nano()))
	return true
}

func (r *Repository) migrateSubagentContextExecutionIdentityUnsafe() error {
	if dialect.IsPostgres(r.db.DriverName()) {
		return r.migrateSubagentContextExecutionIdentityPostgres()
	}
	return r.recreateTableNamed(
		subagentContextShapeMigrationName,
		"task_session_subagents",
		"UNIQUE (task_session_id, tool_call_id)",
		[]string{
			`CREATE TABLE task_session_subagents_new (
				id                  TEXT PRIMARY KEY,
				task_session_id     TEXT NOT NULL,
				task_id             TEXT NOT NULL,
				turn_id             TEXT,
				tool_call_id        TEXT NOT NULL,
				agent_execution_id  TEXT NOT NULL DEFAULT 'unknown',
				parent_tool_call_id TEXT,
				subagent_type       TEXT,
				description         TEXT,
				agent_id            TEXT,
				child_session_id    TEXT,
				model               TEXT,
				agent_status        TEXT,
				tool_status         TEXT,
				is_async            INTEGER NOT NULL DEFAULT 0,
				total_tokens        INTEGER,
				tool_use_count      INTEGER,
				duration_ms         INTEGER,
				source              TEXT NOT NULL DEFAULT 'live',
				observed_at         TIMESTAMP NOT NULL,
				settled_at          TIMESTAMP,
				updated_at          TIMESTAMP NOT NULL,
				UNIQUE (task_session_id, agent_execution_id, tool_call_id),
				FOREIGN KEY (task_session_id) REFERENCES task_sessions(id) ON DELETE CASCADE
			)`,
			`INSERT INTO task_session_subagents_new (
				id, task_session_id, task_id, turn_id, tool_call_id, agent_execution_id,
				parent_tool_call_id, subagent_type, description, agent_id, child_session_id, model,
				agent_status, tool_status, is_async, total_tokens, tool_use_count,
				duration_ms, source, observed_at, settled_at, updated_at
			)
			SELECT
				id, task_session_id, task_id, turn_id, tool_call_id, 'unknown',
				parent_tool_call_id, subagent_type, description, agent_id, child_session_id, model,
				agent_status, tool_status, is_async, total_tokens, tool_use_count,
				duration_ms, source, observed_at, settled_at, updated_at
			FROM task_session_subagents`,
			`DROP TABLE task_session_subagents`,
			`ALTER TABLE task_session_subagents_new RENAME TO task_session_subagents`,
			`CREATE INDEX IF NOT EXISTS idx_subagents_session_id ON task_session_subagents(task_session_id)`,
			`CREATE INDEX IF NOT EXISTS idx_subagents_task_id    ON task_session_subagents(task_id)`,
			`CREATE INDEX IF NOT EXISTS idx_subagents_turn_id    ON task_session_subagents(turn_id)`,
		},
	)
}

// migrateSubagentContextExecutionIdentityPostgres runs the column addition
// and the unique-constraint swap in one transaction, so a failure partway
// through (the constraint swap failing after the column was added, or vice
// versa) rolls back the complete cutover rather than leaving the schema in a
// state this function never intended and subagentContextExecutionShapeHolds
// would not recognize as either "old" or "new" (per the destructive cutover
// migration convention in AGENTS.md).
func (r *Repository) migrateSubagentContextExecutionIdentityPostgres() error {
	tx, err := r.db.Beginx()
	if err != nil {
		return fmt.Errorf("begin subagent context execution migration transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`ALTER TABLE task_session_subagents
		ADD COLUMN IF NOT EXISTS agent_execution_id TEXT NOT NULL DEFAULT 'unknown'`); err != nil {
		return fmt.Errorf("add task_session_subagents.agent_execution_id: %w", err)
	}
	if err := r.maybeFailSubagentContextExecutionMigration("add_column"); err != nil {
		return err
	}
	if _, err := tx.Exec(`
DO $$
DECLARE
	old_constraint_name text;
BEGIN
	SELECT con.conname INTO old_constraint_name
	FROM pg_constraint con
	JOIN pg_class rel ON rel.oid = con.conrelid
	JOIN pg_namespace nsp ON nsp.oid = rel.relnamespace
	WHERE rel.relname = 'task_session_subagents'
		AND nsp.nspname = current_schema()
		AND con.contype = 'u'
		AND (
			SELECT array_agg(attr.attname::text ORDER BY cols.ordinality)
			FROM unnest(con.conkey) WITH ORDINALITY AS cols(attnum, ordinality)
			JOIN pg_attribute attr ON attr.attrelid = con.conrelid AND attr.attnum = cols.attnum
		) = ARRAY['task_session_id', 'tool_call_id'];

	IF old_constraint_name IS NOT NULL THEN
		EXECUTE format('ALTER TABLE task_session_subagents DROP CONSTRAINT %I', old_constraint_name);
	END IF;

	IF NOT EXISTS (
		SELECT 1
		FROM pg_constraint con
		JOIN pg_class rel ON rel.oid = con.conrelid
		JOIN pg_namespace nsp ON nsp.oid = rel.relnamespace
		WHERE rel.relname = 'task_session_subagents'
			AND nsp.nspname = current_schema()
			AND con.contype = 'u'
			AND (
				SELECT array_agg(attr.attname::text ORDER BY cols.ordinality)
				FROM unnest(con.conkey) WITH ORDINALITY AS cols(attnum, ordinality)
				JOIN pg_attribute attr ON attr.attrelid = con.conrelid AND attr.attnum = cols.attnum
			) = ARRAY['task_session_id', 'agent_execution_id', 'tool_call_id']
	) THEN
		ALTER TABLE task_session_subagents
			ADD CONSTRAINT task_session_subagents_session_exec_tool_key
			UNIQUE (task_session_id, agent_execution_id, tool_call_id);
	END IF;
END $$;
	`); err != nil {
		return fmt.Errorf("migrate task_session_subagents unique constraint: %w", err)
	}
	if err := r.maybeFailSubagentContextExecutionMigration("swap_constraint"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit subagent context execution migration transaction: %w", err)
	}
	return nil
}

// maybeFailSubagentContextExecutionMigration is a test-only failpoint: when
// the repository's failSubagentContextExecutionMigrationAfter step matches,
// the migration aborts so tests can prove the transaction restores the
// complete pre-migration schema.
func (r *Repository) maybeFailSubagentContextExecutionMigration(step string) error {
	if r.failSubagentContextExecutionMigrationAfter == "" {
		return nil
	}
	if r.failSubagentContextExecutionMigrationAfter == step {
		r.failSubagentContextExecutionMigrationAfter = ""
		return errors.New("injected subagent context execution migration failure at " + step)
	}
	return nil
}

// subagentContextExecutionShapeHolds reports whether the AC-33 end state
// (clauses 1-3 only: the NOT NULL column, the 3-column UNIQUE key, the
// legacy 2-column UNIQUE key gone) is schema-observed to hold right now.
// AC-33b requires this check rather than recreateTable's own fired-boolean,
// which is ambiguous across "table doesn't exist", "already correct", and
// "Postgres no-op" — and cannot detect a 4th reachable state: a committed
// shape migration whose subsequent execution_since write failed on an
// earlier boot. A schema-observation check self-heals that state too, since
// it holds true on the very next boot with no rebuild needed.
func (r *Repository) subagentContextExecutionShapeHolds() (bool, error) {
	if dialect.IsPostgres(r.db.DriverName()) {
		return r.subagentContextExecutionShapeHoldsPostgres()
	}
	return r.subagentContextExecutionShapeHoldsSQLite()
}

func (r *Repository) subagentContextExecutionShapeHoldsSQLite() (bool, error) {
	notNull, err := r.subagentContextColumnNotNullSQLite()
	if err != nil || !notNull {
		return false, err
	}
	uniqueSets, err := r.subagentContextUniqueIndexColumnsSQLite()
	if err != nil {
		return false, err
	}
	hasNewKey, hasOldKey := false, false
	for _, cols := range uniqueSets {
		switch strings.Join(cols, ",") {
		case "task_session_id,agent_execution_id,tool_call_id":
			hasNewKey = true
		case "task_session_id,tool_call_id":
			hasOldKey = true
		}
	}
	return hasNewKey && !hasOldKey, nil
}

func (r *Repository) subagentContextColumnNotNullSQLite() (bool, error) {
	rows, err := r.db.Query(`PRAGMA table_info(task_session_subagents)`)
	if err != nil {
		return false, fmt.Errorf("subagent context shape: table_info: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var cid, notnull, pk int
		var name, ctype string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, fmt.Errorf("subagent context shape: scan table_info: %w", err)
		}
		if name == "agent_execution_id" {
			return notnull == 1, rows.Err()
		}
	}
	return false, rows.Err()
}

// subagentContextUniqueIndexColumnsSQLite returns, for every UNIQUE
// constraint on task_session_subagents, its columns in declared order.
func (r *Repository) subagentContextUniqueIndexColumnsSQLite() ([][]string, error) {
	idxRows, err := r.db.Query(`PRAGMA index_list(task_session_subagents)`)
	if err != nil {
		return nil, fmt.Errorf("subagent context shape: index_list: %w", err)
	}
	defer func() { _ = idxRows.Close() }()

	var indexNames []string
	for idxRows.Next() {
		var seq int
		var name string
		var unique, partial int
		var origin string
		if err := idxRows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			return nil, fmt.Errorf("subagent context shape: scan index_list: %w", err)
		}
		if unique == 1 && origin == "u" {
			indexNames = append(indexNames, name)
		}
	}
	if err := idxRows.Err(); err != nil {
		return nil, err
	}

	result := make([][]string, 0, len(indexNames))
	for _, name := range indexNames {
		cols, err := r.subagentContextIndexInfoColumnsSQLite(name)
		if err != nil {
			return nil, err
		}
		result = append(result, cols)
	}
	return result, nil
}

func (r *Repository) subagentContextIndexInfoColumnsSQLite(indexName string) ([]string, error) {
	// PRAGMA index_info does not support bind parameters for the index name;
	// string concatenation is the only option. indexName comes from PRAGMA
	// index_list above, which reflects this application's own DDL, so this
	// is not a SQL injection surface.
	infoRows, err := r.db.Query(`PRAGMA index_info(` + indexName + `)`)
	if err != nil {
		return nil, fmt.Errorf("subagent context shape: index_info(%s): %w", indexName, err)
	}
	defer func() { _ = infoRows.Close() }()

	var cols []string
	for infoRows.Next() {
		var seqno, cid int
		var name string
		if err := infoRows.Scan(&seqno, &cid, &name); err != nil {
			return nil, fmt.Errorf("subagent context shape: scan index_info(%s): %w", indexName, err)
		}
		cols = append(cols, name)
	}
	return cols, infoRows.Err()
}

func (r *Repository) subagentContextExecutionShapeHoldsPostgres() (bool, error) {
	var holds bool
	err := r.db.QueryRow(`
		SELECT
			EXISTS (
				SELECT 1 FROM information_schema.columns
				WHERE table_schema = current_schema()
					AND table_name = 'task_session_subagents'
					AND column_name = 'agent_execution_id'
					AND is_nullable = 'NO'
			)
			AND EXISTS (
				SELECT 1
				FROM pg_constraint con
				JOIN pg_class rel ON rel.oid = con.conrelid
				JOIN pg_namespace nsp ON nsp.oid = rel.relnamespace
				WHERE rel.relname = 'task_session_subagents'
					AND nsp.nspname = current_schema()
					AND con.contype = 'u'
					AND (
						SELECT array_agg(attr.attname::text ORDER BY cols.ordinality)
						FROM unnest(con.conkey) WITH ORDINALITY AS cols(attnum, ordinality)
						JOIN pg_attribute attr ON attr.attrelid = con.conrelid AND attr.attnum = cols.attnum
					) = ARRAY['task_session_id', 'agent_execution_id', 'tool_call_id']
			)
			AND NOT EXISTS (
				SELECT 1
				FROM pg_constraint con
				JOIN pg_class rel ON rel.oid = con.conrelid
				JOIN pg_namespace nsp ON nsp.oid = rel.relnamespace
				WHERE rel.relname = 'task_session_subagents'
					AND nsp.nspname = current_schema()
					AND con.contype = 'u'
					AND (
						SELECT array_agg(attr.attname::text ORDER BY cols.ordinality)
						FROM unnest(con.conkey) WITH ORDINALITY AS cols(attnum, ordinality)
						JOIN pg_attribute attr ON attr.attrelid = con.conrelid AND attr.attnum = cols.attnum
					) = ARRAY['task_session_id', 'tool_call_id']
			)
	`).Scan(&holds)
	if err != nil {
		return false, fmt.Errorf("subagent context shape (postgres): %w", err)
	}
	return holds, nil
}

// warnSubagentContextMigration logs a failure in the exact "migration
// failed" / zap.String("name", ...) / zap.Error(...) shape MigrateLogger.Apply
// uses (internal/db/migratelog.go), so tests asserting on that shape work
// identically whether the failure came from r.migrate.Apply or from one of
// this file's/subagent_context_backfill.go's hand-rolled transactions.
func (r *Repository) warnSubagentContextMigration(name string, err error) {
	if r.log == nil {
		return
	}
	r.log.Warn("migration failed", zap.String("name", name), zap.Error(err))
}
