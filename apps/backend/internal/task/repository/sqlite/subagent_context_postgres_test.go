package sqlite

import (
	"context"
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

// TestPostgresSubagentContextHealthCountsAgreeAfterLiveWrites is the
// PostgreSQL counterpart to TestSubagentContextHealthCountsAgreeAfterLiveWrites:
// AC-28's message-side query uses SQLite's json_extract in the spec's
// illustrative form, which does not exist on PostgreSQL — subagentMessageCount
// builds the dialect-appropriate expression instead, and this test proves that
// expression actually agrees with the context-side count on PostgreSQL, not
// just on SQLite (AC-19).
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
	ts := time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)

	for i, toolCallID := range []string{"tc-pg-health-1", "tc-pg-health-2", "tc-pg-health-3"} {
		seedSubagentMessage(t, repo, "msg-pg-health-"+toolCallID, "session-pg-health", "task-pg-health", "turn-pg-health",
			toolCallID, "", "completed", map[string]interface{}{"status": "completed"},
			ts.Add(time.Duration(i)*time.Minute), ts.Add(time.Duration(i)*time.Minute))
		if err := repo.UpsertSubagentContext(ctx, &models.SubagentContext{
			TaskSessionID: "session-pg-health", TaskID: "task-pg-health", ToolCallID: toolCallID,
			Source: "live", ObservedAt: ts.Add(time.Duration(i) * time.Minute), UpdatedAt: ts.Add(time.Duration(i) * time.Minute),
		}); err != nil {
			t.Fatalf("UpsertSubagentContext %s: %v", toolCallID, err)
		}
	}

	messageCount := subagentMessageCount(t, repo)
	contextCount := subagentContextCount(t, repo)
	if messageCount != contextCount {
		t.Fatalf("message count = %d, context count = %d; want equal after live writes", messageCount, contextCount)
	}
	if messageCount != 3 {
		t.Fatalf("message count = %d, want 3", messageCount)
	}
}
