package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func seedPostgresForMsgTest(t *testing.T, db *sqlx.DB, taskID, sessionID, turnID string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, '', 'test task', ?, ?)
	`), taskID, now, now); err != nil {
		t.Fatalf("seed task %s: %v", taskID, err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_sessions (id, task_id, started_at, updated_at)
		VALUES (?, ?, ?, ?)
	`), sessionID, taskID, now, now); err != nil {
		t.Fatalf("seed session %s: %v", sessionID, err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`), turnID, sessionID, taskID, now, now, now); err != nil {
		t.Fatalf("seed turn %s: %v", turnID, err)
	}
}

// TestPostgresSubagentContextSchemaAndBackfill is the Postgres counterpart to
// TestSubagentContextSchemaFreshAndReplay / TestSubagentContextBackfill*: a
// fresh schema plus a migration replay produces the table, its indexes, the
// backfilled rows, and both activation keys, matching SQLite (AC-19). Skips
// unless KANDEV_TEST_POSTGRES_DSN is set.
func TestPostgresSubagentContextSchemaAndBackfill(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	// Production boot order (internal/persistence/provider.go) creates
	// kandev_meta before the task repository; mirror that here since this
	// package's repository never creates it itself.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS kandev_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatalf("create kandev_meta: %v", err)
	}
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()

	for _, index := range []string{"idx_subagents_session_id", "idx_subagents_task_id", "idx_subagents_turn_id"} {
		var name string
		if err := db.Get(&name, `SELECT indexname FROM pg_indexes WHERE indexname = $1`, index); err != nil {
			t.Fatalf("index %s is missing on postgres: %v", index, err)
		}
	}

	seedPostgresForMsgTest(t, db, "task-pg-schema", "session-pg-schema", "turn-pg-schema")
	createdAt := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Second)
	metadata := `{"tool_call_id":"tc-pg-backfill","status":"complete","normalized":{"kind":"subagent_task","subagent_task":{"status":"async_launched","is_async":true}}}`
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_session_messages
			(id, task_session_id, task_id, turn_id, author_type, content, type, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'agent', 'Subagent', 'tool_call', ?, ?, ?)
	`), "msg-pg-backfill", "session-pg-schema", "task-pg-schema", "turn-pg-schema", metadata, createdAt, updatedAt); err != nil {
		t.Fatalf("seed subagent message: %v", err)
	}

	// Reset the write-once activation keys to simulate this being the
	// database's first boot with historical data present — mirrors
	// TestSubagentContextActivationKeysWrittenOnce's SQLite technique, needed
	// because NewWithDB above already claimed both keys against the
	// then-empty database in this same boot.
	if _, err := db.Exec(`DELETE FROM kandev_meta WHERE key IN ('subagent_context_capture_since', 'subagent_context_backfill_through')`); err != nil {
		t.Fatalf("reset activation keys: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay runMigrations: %v", err)
	}

	rows, err := repo.ListSubagentContextsBySession(ctx, "session-pg-schema")
	if err != nil {
		t.Fatalf("ListSubagentContextsBySession: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("row count = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.Source != "backfill" {
		t.Errorf("source = %q, want backfill", row.Source)
	}
	if row.TotalTokens != nil || row.ToolUseCount != nil || row.DurationMs != nil {
		t.Errorf("metrics = tokens=%v count=%v duration=%v, want all NULL (async_launched)", row.TotalTokens, row.ToolUseCount, row.DurationMs)
	}
	if !row.IsAsync {
		t.Error("is_async = false, want true")
	}
	if row.SettledAt == nil || !row.SettledAt.Equal(updatedAt) {
		t.Errorf("settled_at = %v, want %v (tool_status complete is terminal)", row.SettledAt, updatedAt)
	}

	var captureSince, backfillThrough string
	if err := db.Get(&captureSince, db.Rebind(`SELECT value FROM kandev_meta WHERE key = ?`), "subagent_context_capture_since"); err != nil {
		t.Fatalf("read subagent_context_capture_since: %v", err)
	}
	if _, err := time.Parse(time.RFC3339, captureSince); err != nil {
		t.Fatalf("subagent_context_capture_since = %q, not RFC3339: %v", captureSince, err)
	}
	if err := db.Get(&backfillThrough, db.Rebind(`SELECT value FROM kandev_meta WHERE key = ?`), "subagent_context_backfill_through"); err != nil {
		t.Fatalf("read subagent_context_backfill_through: %v", err)
	}
	parsed, err := time.Parse(time.RFC3339, backfillThrough)
	if err != nil {
		t.Fatalf("subagent_context_backfill_through = %q, not RFC3339: %v", backfillThrough, err)
	}
	if !parsed.Equal(createdAt) {
		t.Errorf("subagent_context_backfill_through = %v, want %v", parsed, createdAt)
	}

	holds, err := repo.subagentContextExecutionShapeHolds()
	if err != nil {
		t.Fatalf("subagentContextExecutionShapeHolds: %v", err)
	}
	if !holds {
		t.Fatal("subagentContextExecutionShapeHolds = false on postgres, want true")
	}
	var executionSince string
	if err := db.Get(&executionSince, db.Rebind(`SELECT value FROM kandev_meta WHERE key = ?`), "subagent_context_execution_since"); err != nil {
		t.Fatalf("read subagent_context_execution_since: %v", err)
	}
	if _, err := time.Parse(time.RFC3339, executionSince); err != nil {
		t.Fatalf("subagent_context_execution_since = %q, not RFC3339: %v", executionSince, err)
	}
}

// TestPostgresSubagentContextExecutionIdentityMigratesLegacyShape is the
// PostgreSQL counterpart to
// TestSubagentContextExecutionIdentityMigratesLegacyShape: a database built
// before Amendment 1 (no agent_execution_id column, the old 2-column UNIQUE
// constraint under whatever name Postgres assigned it) is migrated to the
// new shape, its pre-existing row survives with the 'unknown' sentinel, and
// execution_since is written only once that shape is schema-observed to
// hold (AC-33, AC-19).
func TestPostgresSubagentContextExecutionIdentityMigratesLegacyShape(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS kandev_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatalf("create kandev_meta: %v", err)
	}
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}

	// Roll the table back to its pre-Amendment-1 legacy shape: drop the
	// column the migration adds and swap the 3-column key for the old
	// 2-column one, under a name the migration must discover dynamically
	// rather than assume. NewWithDB's own initial schema creation names this
	// inline UNIQUE(...) constraint whatever Postgres auto-assigns it, not
	// the name the migration would give it, so this test must look the name
	// up the same way — by column set — the production migration code does.
	var newConstraintName string
	if err := db.Get(&newConstraintName, `
		SELECT con.conname
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
	`); err != nil {
		t.Fatalf("find new unique constraint name: %v", err)
	}
	quotedConstraintName := `"` + strings.ReplaceAll(newConstraintName, `"`, `""`) + `"`
	if _, err := db.Exec(`ALTER TABLE task_session_subagents DROP CONSTRAINT ` + quotedConstraintName); err != nil {
		t.Fatalf("drop new unique constraint: %v", err)
	}
	if _, err := db.Exec(`ALTER TABLE task_session_subagents DROP COLUMN agent_execution_id`); err != nil {
		t.Fatalf("drop agent_execution_id: %v", err)
	}
	if _, err := db.Exec(`ALTER TABLE task_session_subagents ADD CONSTRAINT task_session_subagents_legacy_key UNIQUE (task_session_id, tool_call_id)`); err != nil {
		t.Fatalf("add legacy unique constraint: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM kandev_meta WHERE key = 'subagent_context_execution_since'`); err != nil {
		t.Fatalf("reset execution_since key: %v", err)
	}

	seedPostgresForMsgTest(t, db, "task-pg-legacy", "session-pg-legacy", "turn-pg-legacy")
	legacyObserved := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_session_subagents (id, task_session_id, task_id, tool_call_id, model, source, observed_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'live', ?, ?)
	`), "row-pg-legacy", "session-pg-legacy", "task-pg-legacy", "tc-pg-legacy", "legacy-model", legacyObserved, legacyObserved); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	repo.migrateSubagentContextExecutionIdentity()

	holds, err := repo.subagentContextExecutionShapeHolds()
	if err != nil {
		t.Fatalf("subagentContextExecutionShapeHolds: %v", err)
	}
	if !holds {
		t.Fatal("subagentContextExecutionShapeHolds = false after migrating a legacy shape, want true")
	}

	var executionID, model string
	if err := db.Get(&executionID, db.Rebind(`SELECT agent_execution_id FROM task_session_subagents WHERE id = ?`), "row-pg-legacy"); err != nil {
		t.Fatalf("read migrated legacy row's agent_execution_id: %v", err)
	}
	if err := db.Get(&model, db.Rebind(`SELECT model FROM task_session_subagents WHERE id = ?`), "row-pg-legacy"); err != nil {
		t.Fatalf("read migrated legacy row's model: %v", err)
	}
	if executionID != "unknown" {
		t.Errorf("agent_execution_id = %q, want unknown (pre-existing row must survive with the sentinel)", executionID)
	}
	if model != "legacy-model" {
		t.Errorf("model = %q, want legacy-model preserved", model)
	}

	var executionSince string
	if err := db.Get(&executionSince, db.Rebind(`SELECT value FROM kandev_meta WHERE key = ?`), "subagent_context_execution_since"); err != nil {
		t.Fatalf("subagent_context_execution_since must be written once the migrated shape is observed to hold: %v", err)
	}
	if _, err := time.Parse(time.RFC3339, executionSince); err != nil {
		t.Fatalf("subagent_context_execution_since = %q, not RFC3339: %v", executionSince, err)
	}
}

// resetSubagentContextExecutionMigrationToLegacyShape rolls
// task_session_subagents back to its pre-Amendment-1 shape (no
// agent_execution_id column, the old 2-column UNIQUE constraint) so a test
// can exercise migrateSubagentContextExecutionIdentityPostgres from a known
// starting point, mirroring
// TestPostgresSubagentContextExecutionIdentityMigratesLegacyShape's technique.
func resetSubagentContextExecutionMigrationToLegacyShape(t *testing.T, db *sqlx.DB) {
	t.Helper()
	var newConstraintName string
	if err := db.Get(&newConstraintName, `
		SELECT con.conname
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
	`); err != nil {
		t.Fatalf("find new unique constraint name: %v", err)
	}
	quotedConstraintName := `"` + strings.ReplaceAll(newConstraintName, `"`, `""`) + `"`
	if _, err := db.Exec(`ALTER TABLE task_session_subagents DROP CONSTRAINT ` + quotedConstraintName); err != nil {
		t.Fatalf("drop new unique constraint: %v", err)
	}
	if _, err := db.Exec(`ALTER TABLE task_session_subagents DROP COLUMN agent_execution_id`); err != nil {
		t.Fatalf("drop agent_execution_id: %v", err)
	}
	if _, err := db.Exec(`ALTER TABLE task_session_subagents ADD CONSTRAINT task_session_subagents_legacy_key UNIQUE (task_session_id, tool_call_id)`); err != nil {
		t.Fatalf("add legacy unique constraint: %v", err)
	}
}

// TestPostgresSubagentContextExecutionMigrationRollsBackAtEveryFailpoint
// covers the destructive cutover migration convention in AGENTS.md: a
// failure at either step of migrateSubagentContextExecutionIdentityPostgres
// must roll back the complete cutover, restoring the exact pre-migration
// legacy shape, rather than leaving agent_execution_id added without the new
// unique constraint (or any other partial state).
func TestPostgresSubagentContextExecutionMigrationRollsBackAtEveryFailpoint(t *testing.T) {
	for _, step := range []string{"add_column", "swap_constraint"} {
		t.Run(step, func(t *testing.T) {
			db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
			if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS kandev_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`); err != nil {
				t.Fatalf("create kandev_meta: %v", err)
			}
			repo, err := NewWithDB(db, db, nil)
			if err != nil {
				t.Fatalf("init postgres schema: %v", err)
			}
			resetSubagentContextExecutionMigrationToLegacyShape(t, db)
			repo.failSubagentContextExecutionMigrationAfter = step

			if err := repo.migrateSubagentContextExecutionIdentityPostgres(); err == nil {
				t.Fatalf("expected injected failure at %s", step)
			}

			var hasColumn bool
			if err := db.Get(&hasColumn, `
				SELECT EXISTS (
					SELECT 1 FROM information_schema.columns
					WHERE table_schema = current_schema()
						AND table_name = 'task_session_subagents'
						AND column_name = 'agent_execution_id'
				)`); err != nil {
				t.Fatalf("check agent_execution_id column: %v", err)
			}
			if hasColumn {
				t.Errorf("agent_execution_id column present after rollback at %s, want dropped", step)
			}

			var hasLegacyKey bool
			if err := db.Get(&hasLegacyKey, `
				SELECT EXISTS (
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
				)`); err != nil {
				t.Fatalf("check legacy unique constraint: %v", err)
			}
			if !hasLegacyKey {
				t.Errorf("legacy 2-column unique constraint missing after rollback at %s, want preserved", step)
			}

			holds, err := repo.subagentContextExecutionShapeHolds()
			if err != nil {
				t.Fatalf("subagentContextExecutionShapeHolds: %v", err)
			}
			if holds {
				t.Errorf("subagentContextExecutionShapeHolds = true after rollback at %s, want false", step)
			}
		})
	}
}

// TestPostgresUpsertSubagentContextConflictBehavior verifies the upsert's
// conflict clause behaviorally on Postgres — schema replay alone would not
// exercise ON CONFLICT DO UPDATE semantics (ADR 0027). Covers fill-forward,
// write-once settled_at, sticky is_async, and preserved original turn_id.
func TestPostgresUpsertSubagentContextConflictBehavior(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	seedPostgresForMsgTest(t, db, "task-pg-upsert", "session-pg-upsert", "turn-pg-upsert-a")
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_session_turns (id, task_session_id, task_id, started_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`), "turn-pg-upsert-b", "session-pg-upsert", "task-pg-upsert", time.Now().UTC(), time.Now().UTC(), time.Now().UTC()); err != nil {
		t.Fatalf("seed second turn: %v", err)
	}

	firstSettle := time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)
	turnA := "turn-pg-upsert-a"
	if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-pg-upsert", TaskID: "task-pg-upsert", TurnID: &turnA,
		ToolCallID: "tc-pg-upsert", Source: "live", ObservedAt: firstSettle, UpdatedAt: firstSettle,
		SubagentType: sp("security-reviewer"), IsAsync: true,
		ToolStatus: sp("completed"), SettledAt: &firstSettle,
	}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	turnB := "turn-pg-upsert-b"
	laterSettle := firstSettle.Add(time.Hour)
	if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
		TaskSessionID: "session-pg-upsert", TaskID: "task-pg-upsert", TurnID: &turnB,
		ToolCallID: "tc-pg-upsert", Source: "live", ObservedAt: firstSettle, UpdatedAt: laterSettle,
		IsAsync: false, ToolStatus: sp("completed"), SettledAt: &laterSettle,
		Model: sp("claude-opus-5"),
	}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	rows, err := repo.ListSubagentContextsBySession(ctx, "session-pg-upsert")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("row count = %d, want 1", len(rows))
	}
	row := rows[0]
	if row.TurnID == nil || *row.TurnID != "turn-pg-upsert-a" {
		t.Errorf("turn_id = %v, want turn-pg-upsert-a (original turn preserved)", row.TurnID)
	}
	if !row.IsAsync {
		t.Error("is_async = false, want sticky true")
	}
	if row.SettledAt == nil || !row.SettledAt.Equal(firstSettle) {
		t.Errorf("settled_at = %v, want %v (write-once)", row.SettledAt, firstSettle)
	}
	if row.SubagentType == nil || *row.SubagentType != "security-reviewer" {
		t.Errorf("subagent_type = %v, want preserved (fill-forward)", row.SubagentType)
	}
	if row.Model == nil || *row.Model != "claude-opus-5" {
		t.Errorf("model = %v, want claude-opus-5 (filled from second frame)", row.Model)
	}
}

// TestPostgresSubagentContextBackfillJSONHelpers verifies the backfill's
// dialect-aware JSON helpers over jsonb (AC-23b): malformed metadata (” and
// 'null') do not abort the migration, and the boolean spelling difference
// (#>> yields 'true'/'false' text on Postgres vs SQLite's 1/0) normalizes
// correctly.
func TestPostgresSubagentContextBackfillJSONHelpers(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	// Production boot order (internal/persistence/provider.go) creates
	// kandev_meta before the task repository; mirror that here since this
	// package's repository never creates it itself. Without this, the
	// backfill transaction's capture_since key write fails and rolls back
	// the whole migration, silently discarding the valid row along with the
	// malformed ones this test means to exercise.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS kandev_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatalf("create kandev_meta: %v", err)
	}
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	seedPostgresForMsgTest(t, db, "task-pg-malformed", "session-pg-malformed", "turn-pg-malformed")
	ts := time.Date(2026, 8, 5, 14, 0, 0, 0, time.UTC)

	// idx_messages_metadata_tool_call_id / idx_messages_metadata_pending_id
	// are expression indexes over metadata::jsonb; Postgres evaluates them on
	// every insert, so a literal empty-string metadata row (simulating
	// historical corruption that predates the index) can only be seeded with
	// the index dropped first — matching subagent_context_migration_test.go's
	// SQLite technique.
	for _, index := range []string{"idx_messages_metadata_tool_call_id", "idx_messages_metadata_pending_id"} {
		if _, err := db.Exec(`DROP INDEX IF EXISTS ` + index); err != nil {
			t.Fatalf("drop %s: %v", index, err)
		}
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_session_messages
			(id, task_session_id, task_id, turn_id, author_type, content, type, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'agent', 'Subagent', 'tool_call', ?, ?, ?)
	`), "msg-pg-blank", "session-pg-malformed", "task-pg-malformed", "turn-pg-malformed", "", ts, ts); err != nil {
		t.Fatalf("seed blank-metadata message: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_session_messages
			(id, task_session_id, task_id, turn_id, author_type, content, type, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'agent', 'Subagent', 'tool_call', ?, ?, ?)
	`), "msg-pg-null", "session-pg-malformed", "task-pg-malformed", "turn-pg-malformed", "null", ts, ts); err != nil {
		t.Fatalf("seed null-metadata message: %v", err)
	}
	validMetadata := `{"tool_call_id":"tc-pg-valid","status":"completed","normalized":{"kind":"subagent_task","subagent_task":{"status":"completed","is_async":true}}}`
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_session_messages
			(id, task_session_id, task_id, turn_id, author_type, content, type, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'agent', 'Subagent', 'tool_call', ?, ?, ?)
	`), "msg-pg-valid", "session-pg-malformed", "task-pg-malformed", "turn-pg-malformed", validMetadata, ts, ts); err != nil {
		t.Fatalf("seed valid message: %v", err)
	}

	// Reset the write-once activation keys to simulate this being the
	// database's first boot with historical data present — mirrors
	// TestPostgresSubagentContextSchemaAndBackfill's technique, needed
	// because NewWithDB above already claimed both keys against the
	// then-empty database in this same boot.
	if _, err := db.Exec(`DELETE FROM kandev_meta WHERE key IN ('subagent_context_capture_since', 'subagent_context_backfill_through')`); err != nil {
		t.Fatalf("reset activation keys: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations must not abort on malformed metadata rows: %v", err)
	}

	rows, err := repo.ListSubagentContextsBySession(context.Background(), "session-pg-malformed")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("row count = %d, want 1 (only the valid row backfills)", len(rows))
	}
	row := rows[0]
	if row.ToolCallID != "tc-pg-valid" {
		t.Errorf("tool_call_id = %q, want tc-pg-valid", row.ToolCallID)
	}
	if !row.IsAsync {
		t.Error("is_async = false, want true (Postgres #>> 'true' spelling must normalize to 1)")
	}
}

// TestPostgresSubagentContextHealthQueriesIgnoreSessionTimezone proves the
// health queries' timestamp comparisons produce the same result regardless
// of the Postgres session's `timezone` GUC. observed_at/created_at are naive
// TIMESTAMP columns storing a UTC wall clock (see subagentPreciseTimestamp's
// doc comment); casting one via ::timestamptz reinterprets its digits using
// whatever timezone the current session happens to have set, silently
// shifting the comparison by that session's UTC offset. A production server
// whose default timezone is not UTC — the common case outside test
// environments — would see the shortfall/excess anti-joins undercount and
// max-observed-since go blind exactly like a stalled writer would, which is
// what AC-29's "ok=false must mean genuinely nothing observed" guarantee
// depends on. TestPostgresSubagentContextHealthCountsAgreeAfterLiveWrites
// above never varies the session timezone, so it could not have caught this.
func TestPostgresSubagentContextHealthQueriesIgnoreSessionTimezone(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS kandev_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatalf("create kandev_meta: %v", err)
	}
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	seedPostgresForMsgTest(t, db, "task-pg-tz", "session-pg-tz", "turn-pg-tz")

	// OpenIsolatedPostgres pins the pool to a single connection, so this SET
	// holds for every query the test and the repository issue afterward.
	// Kiritimati (UTC+14) is far enough from UTC that the naive-TIMESTAMP-
	// as-timestamptz bug shifts a comparison by far more than the hour of
	// slack seeded below, regardless of the host running the test.
	if _, err := db.Exec(`SET TIME ZONE 'Pacific/Kiritimati'`); err != nil {
		t.Fatalf("set session timezone: %v", err)
	}

	since := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	after := since.Add(time.Hour)

	// A message with no accounting context row: shortfall must count it.
	seedSubagentMessage(t, repo, "msg-pg-tz-shortfall", "session-pg-tz", "task-pg-tz", "turn-pg-tz",
		"tc-pg-tz-shortfall", "", "completed", map[string]interface{}{"status": "completed"},
		after, after)

	// A context row with no accounting message: excess must count it, and
	// max-observed-since must report it.
	if err := repo.UpsertSubagentContext(context.Background(), &models.SubagentContext{
		TaskSessionID: "session-pg-tz", TaskID: "task-pg-tz", ToolCallID: "tc-pg-tz-excess",
		Source: "live", ObservedAt: after, UpdatedAt: after,
	}); err != nil {
		t.Fatalf("UpsertSubagentContext: %v", err)
	}

	shortfall, err := repo.subagentContextShortfall(since)
	if err != nil {
		t.Fatalf("subagentContextShortfall: %v", err)
	}
	if shortfall != 1 {
		t.Errorf("shortfall = %d, want 1 (a non-UTC session timezone must not hide an unmatched message)", shortfall)
	}

	excess, err := repo.subagentContextExcess(since)
	if err != nil {
		t.Fatalf("subagentContextExcess: %v", err)
	}
	if excess != 1 {
		t.Errorf("excess = %d, want 1 (a non-UTC session timezone must not hide an unmatched context row)", excess)
	}

	maxObserved, ok, err := repo.subagentContextMaxObservedSince(since)
	if err != nil {
		t.Fatalf("subagentContextMaxObservedSince: %v", err)
	}
	if !ok {
		t.Fatal("subagentContextMaxObservedSince: want a value, got none (a non-UTC session timezone must not make a live write invisible)")
	}
	if !maxObserved.Equal(after) {
		t.Errorf("subagentContextMaxObservedSince = %v, want %v", maxObserved, after)
	}
}

// TestPostgresSubagentContextHealthCountsAgreeAfterLiveWrites is the
// PostgreSQL counterpart to TestSubagentContextHealthCountsAgreeAfterLiveWrites:
// AC-28's shortfall/excess anti-joins and AC-29's max-observed-since use
// SQLite's json_extract in the spec's illustrative form, which does not exist
// on PostgreSQL — jsonKey builds the dialect-appropriate expression instead,
// and this test proves the resulting queries actually agree on PostgreSQL,
// not just on SQLite (AC-19).
func TestPostgresSubagentContextHealthCountsAgreeAfterLiveWrites(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS kandev_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatalf("create kandev_meta: %v", err)
	}
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	seedPostgresForMsgTest(t, db, "task-pg-health", "session-pg-health", "turn-pg-health")
	ctx := context.Background()
	since := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	ts := since.Add(time.Hour)

	var latest time.Time
	for i, toolCallID := range []string{"tc-pg-health-1", "tc-pg-health-2", "tc-pg-health-3"} {
		observedAt := ts.Add(time.Duration(i) * time.Minute)
		latest = observedAt
		seedSubagentMessage(t, repo, "msg-pg-health-"+toolCallID, "session-pg-health", "task-pg-health", "turn-pg-health",
			toolCallID, "", "completed", map[string]interface{}{"status": "completed"},
			observedAt, observedAt)
		if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
			TaskSessionID: "session-pg-health", TaskID: "task-pg-health", ToolCallID: toolCallID,
			Source: "live", ObservedAt: observedAt, UpdatedAt: observedAt,
		}); err != nil {
			t.Fatalf("UpsertSubagentContext %s: %v", toolCallID, err)
		}
	}

	shortfall, err := repo.subagentContextShortfall(since)
	if err != nil {
		t.Fatalf("subagentContextShortfall: %v", err)
	}
	if shortfall != 0 {
		t.Fatalf("shortfall = %d, want 0 after every message got a live context row", shortfall)
	}

	excess, err := repo.subagentContextExcess(since)
	if err != nil {
		t.Fatalf("subagentContextExcess: %v", err)
	}
	if excess != 0 {
		t.Fatalf("excess = %d, want 0 when every context row is accounted for by a message", excess)
	}

	maxObserved, ok, err := repo.subagentContextMaxObservedSince(since)
	if err != nil {
		t.Fatalf("subagentContextMaxObservedSince: %v", err)
	}
	if !ok {
		t.Fatal("subagentContextMaxObservedSince: want a value, got none")
	}
	if !maxObserved.Equal(latest) {
		t.Fatalf("subagentContextMaxObservedSince = %v, want %v (the last live write)", maxObserved, latest)
	}
}
