package sqlite

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	dbutil "github.com/kandev/kandev/internal/db"
)

// seedSubagentMessage inserts a task_session_messages row shaped like the
// orchestrator's real subagent_task tool-call message: tool_call_id and
// parent_tool_call_id are top-level metadata keys (matching
// messageCreatorAdapter.CreateToolCallMessage), and the agent-reported fields
// live under metadata.normalized.subagent_task.
func seedSubagentMessage(
	t *testing.T, repo *Repository,
	id, sessionID, taskID, turnID, toolCallID, parentToolCallID, toolStatus string,
	subagentTask map[string]interface{},
	createdAt, updatedAt time.Time,
) {
	t.Helper()
	metadata := map[string]interface{}{
		"tool_call_id": toolCallID,
		"status":       toolStatus,
		"normalized": map[string]interface{}{
			"kind":          "subagent_task",
			"subagent_task": subagentTask,
		},
	}
	if parentToolCallID != "" {
		metadata["parent_tool_call_id"] = parentToolCallID
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	seedRawSubagentMessage(t, repo, id, sessionID, taskID, turnID, string(metadataJSON), createdAt, updatedAt)
}

func seedRawSubagentMessage(
	t *testing.T, repo *Repository,
	id, sessionID, taskID, turnID, metadataJSON string,
	createdAt, updatedAt time.Time,
) {
	t.Helper()
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_session_messages
			(id, task_session_id, task_id, turn_id, author_type, content, type, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'agent', 'Subagent', 'tool_call', ?, ?, ?)
	`), id, sessionID, taskID, turnID, metadataJSON, createdAt, updatedAt)
	if err != nil {
		t.Fatalf("seed subagent message %s: %v", id, err)
	}
}

type subagentContextRow struct {
	id               string
	taskSessionID    string
	taskID           string
	turnID           sql.NullString
	agentExecutionID string
	parentToolCallID sql.NullString
	subagentType     sql.NullString
	description      sql.NullString
	agentID          sql.NullString
	childSessionID   sql.NullString
	model            sql.NullString
	agentStatus      sql.NullString
	toolStatus       sql.NullString
	isAsync          int
	totalTokens      sql.NullInt64
	toolUseCount     sql.NullInt64
	durationMs       sql.NullInt64
	source           string
	observedAt       time.Time
	settledAt        sql.NullTime
	updatedAt        time.Time
}

func getSubagentContextRow(t *testing.T, repo *Repository, sessionID, toolCallID string) *subagentContextRow {
	t.Helper()
	row := &subagentContextRow{}
	err := repo.db.QueryRow(repo.db.Rebind(`
		SELECT id, task_session_id, task_id, turn_id, agent_execution_id, parent_tool_call_id, subagent_type, description,
			agent_id, child_session_id, model, agent_status, tool_status, is_async,
			total_tokens, tool_use_count, duration_ms, source, observed_at, settled_at, updated_at
		FROM task_session_subagents WHERE task_session_id = ? AND tool_call_id = ?
	`), sessionID, toolCallID).Scan(
		&row.id, &row.taskSessionID, &row.taskID, &row.turnID, &row.agentExecutionID, &row.parentToolCallID, &row.subagentType, &row.description,
		&row.agentID, &row.childSessionID, &row.model, &row.agentStatus, &row.toolStatus, &row.isAsync,
		&row.totalTokens, &row.toolUseCount, &row.durationMs, &row.source, &row.observedAt, &row.settledAt, &row.updatedAt,
	)
	if err != nil {
		t.Fatalf("get subagent context row (%s,%s): %v", sessionID, toolCallID, err)
	}
	return row
}

func countSubagentContextRows(t *testing.T, repo *Repository, sessionID, toolCallID string) int {
	t.Helper()
	var count int
	if err := repo.db.QueryRow(repo.db.Rebind(`
		SELECT COUNT(*) FROM task_session_subagents WHERE task_session_id = ? AND tool_call_id = ?
	`), sessionID, toolCallID).Scan(&count); err != nil {
		t.Fatalf("count subagent context rows (%s,%s): %v", sessionID, toolCallID, err)
	}
	return count
}

func readMetaKey(t *testing.T, db *sqlx.DB, key string) (string, bool) {
	t.Helper()
	var value string
	err := db.QueryRow(db.Rebind(`SELECT value FROM kandev_meta WHERE key = ?`), key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false
	}
	if err != nil {
		t.Fatalf("read kandev_meta %q: %v", key, err)
	}
	return value, true
}

func newSubagentMigrationTestRepo(t *testing.T) (*Repository, *sqlx.DB) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "subagent-context-migration.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	// Production boot order (internal/persistence/provider.go) creates
	// kandev_meta before the task repository; mirror that here since this
	// package's repository never creates it itself.
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS kandev_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatalf("create kandev_meta: %v", err)
	}
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize schema: %v", err)
	}
	// NewWithDB's own initSchema() already ran the one-time backfill once,
	// against an empty database with no messages seeded yet, and claimed the
	// two activation keys (AC-24) — correct for a genuinely fresh install,
	// but not what this file's tests want: they seed subagent-shaped
	// messages AFTER construction and then call repo.runMigrations() again,
	// expecting THAT call to be the real (only) backfill run — mirroring a
	// DB upgrade where historical messages predate this boot. Reset to
	// "not yet activated" so every test in this file gets that semantics by
	// default. See subagentContextBackfillActivated in base_migrations.go,
	// which gates the backfill's full-table scan on these same two keys so
	// it runs at most once per installation (AC-23a).
	if _, err := db.Exec(`DELETE FROM kandev_meta WHERE key IN ('subagent_context_capture_since', 'subagent_context_backfill_through')`); err != nil {
		t.Fatalf("reset subagent context activation keys: %v", err)
	}
	return repo, db
}

// dropMessageMetadataIndex removes the expression index on
// task_session_messages(metadata) so a malformed-metadata row (used to prove
// AC-23b tolerates historical corruption) can be seeded at all: with the
// index present, SQLite evaluates json_extract on every insert and rejects
// invalid JSON outright, which real corrupt rows predate.
func dropMessageMetadataIndex(t *testing.T, db *sqlx.DB) {
	t.Helper()
	for _, index := range []string{"idx_messages_metadata_tool_call_id", "idx_messages_metadata_pending_id", "idx_messages_metadata_pending_id_lookup"} {
		if _, err := db.Exec(`DROP INDEX IF EXISTS ` + index); err != nil {
			t.Fatalf("drop %s: %v", index, err)
		}
	}
}

// TestSubagentContextSchemaFreshAndReplay covers AC-17/AC-18: a fresh database
// gets the table, its three indexes, and the UNIQUE constraint, and
// runMigrations() replays cleanly (schema and row set unchanged).
func TestSubagentContextSchemaFreshAndReplay(t *testing.T) {
	repo, db := newSubagentMigrationTestRepo(t)
	seedForMsgTest(t, repo, "task-schema", "session-schema", "turn-schema")

	for _, index := range []string{"idx_subagents_session_id", "idx_subagents_task_id", "idx_subagents_turn_id"} {
		var name string
		if err := db.Get(&name, `SELECT name FROM sqlite_master WHERE type = 'index' AND name = ?`, index); err != nil {
			t.Fatalf("index %s is missing: %v", index, err)
		}
	}

	now := time.Now().UTC()
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_session_subagents (id, task_session_id, task_id, tool_call_id, observed_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`), "row-1", "session-schema", "task-schema", "tc-schema", now, now); err != nil {
		t.Fatalf("first insert should succeed: %v", err)
	}
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_session_subagents (id, task_session_id, task_id, tool_call_id, observed_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`), "row-2", "session-schema", "task-schema", "tc-schema", now, now); err == nil {
		t.Fatal("duplicate (task_session_id, tool_call_id) should violate UNIQUE constraint")
	}

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay migrations: %v", err)
	}
	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay migrations twice: %v", err)
	}
	if count := countSubagentContextRows(t, repo, "session-schema", "tc-schema"); count != 1 {
		t.Fatalf("row count after replay = %d, want 1 (replay must not duplicate)", count)
	}
}

// TestSubagentContextBackfillAsyncLaunchedStoresNullNotZero is the spec's
// named must-not-get-wrong regression: 75% of real subagent invocations are
// async_launched with no reported usage, and a DEFAULT 0 anywhere would
// fabricate every one of them. (AC-7, AC-21)
//
// It also covers AC-21's per-column derivation for every text column the
// backfill's SELECT list populates: subagent_type, description, agent_id,
// child_session_id, and model are five adjacent same-typed columns in that
// SELECT with no other test giving each a distinct, individually-asserted
// value — a column-order mistake in the SELECT list (e.g. subagent_type and
// description swapped) would otherwise pass every existing test untouched.
func TestSubagentContextBackfillAsyncLaunchedStoresNullNotZero(t *testing.T) {
	repo, _ := newSubagentMigrationTestRepo(t)
	seedForMsgTest(t, repo, "task-async", "session-async", "turn-async")
	createdAt := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Second)

	seedSubagentMessage(t, repo, "msg-async", "session-async", "task-async", "turn-async",
		"tc-async", "", "complete",
		map[string]interface{}{
			"description":      "run the test suite",
			"subagent_type":    "test-runner",
			"agent_id":         "agent-xyz",
			"child_session_id": "child-session-xyz",
			"model":            "claude-async-model",
			"status":           "async_launched",
			"is_async":         true,
		},
		createdAt, updatedAt,
	)

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	row := getSubagentContextRow(t, repo, "session-async", "tc-async")
	if row.taskID != "task-async" {
		t.Errorf("task_id = %q, want task-async", row.taskID)
	}
	if !row.turnID.Valid || row.turnID.String != "turn-async" {
		t.Errorf("turn_id = %v, want turn-async", row.turnID)
	}
	if !row.subagentType.Valid || row.subagentType.String != "test-runner" {
		t.Errorf("subagent_type = %v, want test-runner", row.subagentType)
	}
	if !row.description.Valid || row.description.String != "run the test suite" {
		t.Errorf("description = %v, want %q", row.description, "run the test suite")
	}
	if !row.agentID.Valid || row.agentID.String != "agent-xyz" {
		t.Errorf("agent_id = %v, want agent-xyz", row.agentID)
	}
	if !row.childSessionID.Valid || row.childSessionID.String != "child-session-xyz" {
		t.Errorf("child_session_id = %v, want child-session-xyz", row.childSessionID)
	}
	if !row.model.Valid || row.model.String != "claude-async-model" {
		t.Errorf("model = %v, want claude-async-model", row.model)
	}
	if row.totalTokens.Valid {
		t.Errorf("total_tokens = %v, want NULL", row.totalTokens)
	}
	if row.toolUseCount.Valid {
		t.Errorf("tool_use_count = %v, want NULL", row.toolUseCount)
	}
	if row.durationMs.Valid {
		t.Errorf("duration_ms = %v, want NULL", row.durationMs)
	}
	if row.source != "backfill" {
		t.Errorf("source = %q, want backfill", row.source)
	}
	if !row.observedAt.Equal(createdAt) {
		t.Errorf("observed_at = %v, want %v (message created_at)", row.observedAt, createdAt)
	}
	if row.isAsync != 1 {
		t.Errorf("is_async = %d, want 1", row.isAsync)
	}
	if !row.agentStatus.Valid || row.agentStatus.String != "async_launched" {
		t.Errorf("agent_status = %v, want async_launched", row.agentStatus)
	}
	// tool_status is the ACP status of the launching Task call, terminal here,
	// so settled_at must be set from the message's updated_at.
	if !row.toolStatus.Valid || row.toolStatus.String != "complete" {
		t.Errorf("tool_status = %v, want complete", row.toolStatus)
	}
	if !row.settledAt.Valid || !row.settledAt.Time.Equal(updatedAt) {
		t.Errorf("settled_at = %v, want %v", row.settledAt, updatedAt)
	}
}

// TestSubagentContextBackfillReportedZeroToolUseCountSurvives covers AC-8: a
// genuinely reported 0 must remain distinguishable from "not reported".
func TestSubagentContextBackfillReportedZeroToolUseCountSurvives(t *testing.T) {
	repo, _ := newSubagentMigrationTestRepo(t)
	seedForMsgTest(t, repo, "task-zero", "session-zero", "turn-zero")
	ts := time.Date(2026, 8, 5, 11, 0, 0, 0, time.UTC)

	seedSubagentMessage(t, repo, "msg-zero", "session-zero", "task-zero", "turn-zero",
		"tc-zero", "", "completed",
		map[string]interface{}{
			"status":         "completed",
			"tool_use_count": 0,
		},
		ts, ts,
	)

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	row := getSubagentContextRow(t, repo, "session-zero", "tc-zero")
	if !row.toolUseCount.Valid || row.toolUseCount.Int64 != 0 {
		t.Errorf("tool_use_count = %v, want 0 (reported, not absent)", row.toolUseCount)
	}
}

// TestSubagentContextBackfillNegativeMetricBecomesNull covers AC-9/AC-23: a
// negative reported metric is not a measurement and must not be stored.
func TestSubagentContextBackfillNegativeMetricBecomesNull(t *testing.T) {
	repo, _ := newSubagentMigrationTestRepo(t)
	seedForMsgTest(t, repo, "task-neg", "session-neg", "turn-neg")
	ts := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)

	seedSubagentMessage(t, repo, "msg-neg", "session-neg", "task-neg", "turn-neg",
		"tc-neg", "", "completed",
		map[string]interface{}{
			"status":       "completed",
			"total_tokens": -5,
			"duration_ms":  1200,
		},
		ts, ts,
	)

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	row := getSubagentContextRow(t, repo, "session-neg", "tc-neg")
	if row.totalTokens.Valid {
		t.Errorf("total_tokens = %v, want NULL (negative reported value)", row.totalTokens)
	}
	if !row.durationMs.Valid || row.durationMs.Int64 != 1200 {
		t.Errorf("duration_ms = %v, want 1200 (other fields unaffected)", row.durationMs)
	}
}

// TestSubagentContextBackfillEmptyStringBecomesNull covers the empty-string
// normalization rule: SubagentTaskPayload uses "" for absent; the column uses
// NULL so COUNT(model) counts models, not blanks.
func TestSubagentContextBackfillEmptyStringBecomesNull(t *testing.T) {
	repo, _ := newSubagentMigrationTestRepo(t)
	seedForMsgTest(t, repo, "task-empty", "session-empty", "turn-empty")
	ts := time.Date(2026, 8, 5, 13, 0, 0, 0, time.UTC)

	seedSubagentMessage(t, repo, "msg-empty", "session-empty", "task-empty", "turn-empty",
		"tc-empty", "", "completed",
		map[string]interface{}{
			"status": "completed",
			"model":  "",
		},
		ts, ts,
	)

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	row := getSubagentContextRow(t, repo, "session-empty", "tc-empty")
	if row.model.Valid {
		t.Errorf("model = %q, want NULL not empty string", row.model.String)
	}
}

// TestSubagentContextBackfillMalformedMetadataDoesNotAbort covers AC-23b: a
// single row with unparsable metadata (” or 'null') must not abort the
// whole statement, so a valid row alongside it still lands.
func TestSubagentContextBackfillMalformedMetadataDoesNotAbort(t *testing.T) {
	repo, db := newSubagentMigrationTestRepo(t)
	seedForMsgTest(t, repo, "task-malformed", "session-malformed", "turn-malformed")
	ts := time.Date(2026, 8, 5, 14, 0, 0, 0, time.UTC)

	// A literal empty-string metadata row can only exist as historical
	// corruption predating idx_messages_metadata_tool_call_id (the index
	// itself rejects an empty-string insert via its json_extract expression).
	dropMessageMetadataIndex(t, db)
	seedRawSubagentMessage(t, repo, "msg-blank", "session-malformed", "task-malformed", "turn-malformed", "", ts, ts)
	seedRawSubagentMessage(t, repo, "msg-null", "session-malformed", "task-malformed", "turn-malformed", "null", ts, ts)
	seedSubagentMessage(t, repo, "msg-valid", "session-malformed", "task-malformed", "turn-malformed",
		"tc-malformed", "", "completed",
		map[string]interface{}{"status": "completed"},
		ts, ts,
	)

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations must not abort on malformed metadata rows: %v", err)
	}

	if count := countSubagentContextRows(t, repo, "session-malformed", "tc-malformed"); count != 1 {
		t.Fatalf("valid row alongside malformed metadata rows: count = %d, want 1", count)
	}
}

// TestSubagentContextBackfillNonTerminalSettledAtNull covers AC-23a's ELSE
// branch: a message whose derived ACP tool_status is NOT one of the terminal
// values must backfill with settled_at NULL, not a fabricated value from the
// message's updated_at. Every other backfill fixture in this file uses a
// terminal status, so nothing else exercises this branch — the spec notes 5
// real rows are in exactly this "started" (non-terminal) state.
func TestSubagentContextBackfillNonTerminalSettledAtNull(t *testing.T) {
	repo, _ := newSubagentMigrationTestRepo(t)
	seedForMsgTest(t, repo, "task-started", "session-started", "turn-started")
	ts := time.Date(2026, 8, 5, 14, 30, 0, 0, time.UTC)

	seedSubagentMessage(t, repo, "msg-started", "session-started", "task-started", "turn-started",
		"tc-started", "", "started",
		map[string]interface{}{"status": "started"},
		ts, ts,
	)

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	row := getSubagentContextRow(t, repo, "session-started", "tc-started")
	if row.settledAt.Valid {
		t.Errorf("settled_at = %v, want NULL (tool_status %q is not terminal)", row.settledAt, "started")
	}
	if !row.toolStatus.Valid || row.toolStatus.String != "started" {
		t.Errorf("tool_status = %v, want started", row.toolStatus)
	}
}

// TestSubagentContextBackfillLiveRowWins covers AC-22: a live write always
// wins over a backfilled one, a replay of the backfill is a true no-op, AND
// the ON CONFLICT DO NOTHING clause that makes that possible does not abort
// the rest of the single unbounded INSERT...SELECT (AC-23a) — a second,
// unconflicted message in the same statement must still land. Without the
// clause, SQLite's INSERT...SELECT aborts the WHOLE statement on the first
// UNIQUE violation, silently dropping every other backfillable row too.
func TestSubagentContextBackfillLiveRowWins(t *testing.T) {
	repo, db := newSubagentMigrationTestRepo(t)
	seedForMsgTest(t, repo, "task-live-wins", "session-live-wins", "turn-live-wins")
	ts := time.Date(2026, 8, 5, 15, 0, 0, 0, time.UTC)

	seedSubagentMessage(t, repo, "msg-live-wins", "session-live-wins", "task-live-wins", "turn-live-wins",
		"tc-live-wins", "", "completed",
		map[string]interface{}{"status": "completed", "model": "backfill-shape-model"},
		ts, ts,
	)
	seedSubagentMessage(t, repo, "msg-live-wins-sibling", "session-live-wins", "task-live-wins", "turn-live-wins",
		"tc-live-wins-sibling", "", "completed",
		map[string]interface{}{"status": "completed", "model": "sibling-model"},
		ts.Add(time.Minute), ts.Add(time.Minute),
	)

	liveObservedAt := ts.Add(time.Hour)
	if _, err := db.Exec(db.Rebind(`
		INSERT INTO task_session_subagents
			(id, task_session_id, task_id, tool_call_id, model, source, observed_at, updated_at)
		VALUES (?, ?, ?, ?, ?, 'live', ?, ?)
	`), "row-live-wins", "session-live-wins", "task-live-wins", "tc-live-wins", "live-model", liveObservedAt, liveObservedAt); err != nil {
		t.Fatalf("seed pre-existing live row: %v", err)
	}

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	row := getSubagentContextRow(t, repo, "session-live-wins", "tc-live-wins")
	if row.source != "live" {
		t.Errorf("source = %q, want live (backfill must not overwrite)", row.source)
	}
	if !row.model.Valid || row.model.String != "live-model" {
		t.Errorf("model = %v, want live-model preserved", row.model)
	}
	if count := countSubagentContextRows(t, repo, "session-live-wins", "tc-live-wins"); count != 1 {
		t.Fatalf("row count = %d, want exactly 1", count)
	}

	sibling := getSubagentContextRow(t, repo, "session-live-wins", "tc-live-wins-sibling")
	if sibling.source != "backfill" {
		t.Errorf("sibling source = %q, want backfill (the UNIQUE violation on the OTHER row must not abort this insert)", sibling.source)
	}
	if !sibling.model.Valid || sibling.model.String != "sibling-model" {
		t.Errorf("sibling model = %v, want sibling-model", sibling.model)
	}
}

// TestSubagentContextBackfillSkipsRowsWithoutIdentity covers AC-2: a message
// with no task_id or no tool_call_id must not backfill a row.
func TestSubagentContextBackfillSkipsRowsWithoutIdentity(t *testing.T) {
	repo, _ := newSubagentMigrationTestRepo(t)
	seedForMsgTest(t, repo, "task-identity", "session-identity", "turn-identity")
	ts := time.Date(2026, 8, 5, 16, 0, 0, 0, time.UTC)

	// No task_id: seed the message row directly with task_id = ''.
	seedRawSubagentMessage(t, repo, "msg-no-task", "session-identity", "", "turn-identity",
		`{"tool_call_id":"tc-no-task","status":"completed","normalized":{"kind":"subagent_task","subagent_task":{"status":"completed"}}}`,
		ts, ts,
	)
	// No tool_call_id.
	seedSubagentMessage(t, repo, "msg-no-toolcall", "session-identity", "task-identity", "turn-identity",
		"", "", "completed",
		map[string]interface{}{"status": "completed"},
		ts, ts,
	)

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	if count := countSubagentContextRows(t, repo, "session-identity", "tc-no-task"); count != 0 {
		t.Fatalf("row backfilled for message with no task_id: count = %d, want 0", count)
	}
	if count := countSubagentContextRows(t, repo, "session-identity", ""); count != 0 {
		t.Fatalf("row backfilled for message with no tool_call_id: count = %d, want 0", count)
	}
}

// TestSubagentContextBackfillNestedParentToolCallID covers AC-6: a nested
// subagent (parent_tool_call_id set) must be recorded, not suppressed.
func TestSubagentContextBackfillNestedParentToolCallID(t *testing.T) {
	repo, _ := newSubagentMigrationTestRepo(t)
	seedForMsgTest(t, repo, "task-nested", "session-nested", "turn-nested")
	ts := time.Date(2026, 8, 5, 17, 0, 0, 0, time.UTC)

	seedSubagentMessage(t, repo, "msg-nested", "session-nested", "task-nested", "turn-nested",
		"tc-nested-child", "tc-nested-parent", "completed",
		map[string]interface{}{"status": "completed"},
		ts, ts,
	)

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	row := getSubagentContextRow(t, repo, "session-nested", "tc-nested-child")
	if !row.parentToolCallID.Valid || row.parentToolCallID.String != "tc-nested-parent" {
		t.Errorf("parent_tool_call_id = %v, want tc-nested-parent", row.parentToolCallID)
	}
}

// TestSubagentContextActivationKeysWrittenOnce covers AC-24: both kandev_meta
// keys are written once, RFC3339-parseable (backfill_through may be empty),
// and never overwritten on replay. newSubagentMigrationTestRepo already
// resets both keys to "not yet activated" after construction (see its
// comment), so the first runMigrations() call below is the real backfill run
// this test exercises.
func TestSubagentContextActivationKeysWrittenOnce(t *testing.T) {
	repo, db := newSubagentMigrationTestRepo(t)
	seedForMsgTest(t, repo, "task-activation", "session-activation", "turn-activation")
	newest := time.Date(2026, 8, 9, 9, 0, 0, 0, time.UTC)
	older := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)

	seedSubagentMessage(t, repo, "msg-activation-older", "session-activation", "task-activation", "turn-activation",
		"tc-activation-older", "", "completed", map[string]interface{}{"status": "completed"}, older, older)
	seedSubagentMessage(t, repo, "msg-activation-newest", "session-activation", "task-activation", "turn-activation",
		"tc-activation-newest", "", "completed", map[string]interface{}{"status": "completed"}, newest, newest)

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	captureSince, ok := readMetaKey(t, db, "subagent_context_capture_since")
	if !ok {
		t.Fatal("subagent_context_capture_since must be present")
	}
	if _, err := time.Parse(time.RFC3339, captureSince); err != nil {
		t.Fatalf("subagent_context_capture_since = %q, not RFC3339: %v", captureSince, err)
	}

	backfillThrough, ok := readMetaKey(t, db, "subagent_context_backfill_through")
	if !ok {
		t.Fatal("subagent_context_backfill_through must be present")
	}
	parsed, err := time.Parse(time.RFC3339, backfillThrough)
	if err != nil {
		t.Fatalf("subagent_context_backfill_through = %q, not RFC3339: %v", backfillThrough, err)
	}
	if !parsed.Equal(newest) {
		t.Fatalf("subagent_context_backfill_through = %v, want newest message created_at %v", parsed, newest)
	}

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("replay runMigrations: %v", err)
	}
	captureSinceReplayed, _ := readMetaKey(t, db, "subagent_context_capture_since")
	backfillThroughReplayed, _ := readMetaKey(t, db, "subagent_context_backfill_through")
	if captureSinceReplayed != captureSince {
		t.Fatalf("subagent_context_capture_since changed on replay: %q -> %q", captureSince, captureSinceReplayed)
	}
	if backfillThroughReplayed != backfillThrough {
		t.Fatalf("subagent_context_backfill_through changed on replay: %q -> %q", backfillThrough, backfillThroughReplayed)
	}
}

// TestSubagentContextActivationBackfillThroughEmptyWithNoMessages covers the
// AC-24 edge case: a fresh DB with no subagent messages sets
// subagent_context_backfill_through to the empty string, not NULL and not a
// missing key.
func TestSubagentContextActivationBackfillThroughEmptyWithNoMessages(t *testing.T) {
	repo, db := newSubagentMigrationTestRepo(t)

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	backfillThrough, ok := readMetaKey(t, db, "subagent_context_backfill_through")
	if !ok {
		t.Fatal("subagent_context_backfill_through must be present even with no subagent messages")
	}
	if backfillThrough != "" {
		t.Fatalf("subagent_context_backfill_through = %q, want empty string", backfillThrough)
	}
}

// TestSubagentContextBackfillDoesNotRescanOnceActivated covers AC-23a: the
// backfill's full-table scan is "a real, accepted one-time cost ... taken
// once", not a per-boot cost. Once subagent_context_capture_since is set, a
// later runMigrations() call (a later boot) must skip the scan entirely —
// proven here by deleting an already-backfilled row and confirming a second
// runMigrations() call does not restore it. Under the pre-fix behaviour
// (migrateSubagentContextBackfill ran its INSERT...SELECT unconditionally on
// every call, relying only on ON CONFLICT DO NOTHING for row-level
// idempotence) nothing would have stopped that second call from re-scanning
// task_session_messages and re-inserting the deleted row, since deleting it
// removes the only thing ON CONFLICT DO NOTHING keys off.
func TestSubagentContextBackfillDoesNotRescanOnceActivated(t *testing.T) {
	repo, db := newSubagentMigrationTestRepo(t)
	seedForMsgTest(t, repo, "task-once", "session-once", "turn-once")
	ts := time.Date(2026, 8, 5, 18, 0, 0, 0, time.UTC)

	seedSubagentMessage(t, repo, "msg-once", "session-once", "task-once", "turn-once",
		"tc-once", "", "completed",
		map[string]interface{}{"status": "completed"},
		ts, ts,
	)

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("first runMigrations (the real, one-time backfill): %v", err)
	}
	if count := countSubagentContextRows(t, repo, "session-once", "tc-once"); count != 1 {
		t.Fatalf("row count after first backfill = %d, want 1", count)
	}

	if _, err := db.Exec(db.Rebind(`DELETE FROM task_session_subagents WHERE task_session_id = ? AND tool_call_id = ?`),
		"session-once", "tc-once"); err != nil {
		t.Fatalf("delete backfilled row: %v", err)
	}

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("second runMigrations (a later boot): %v", err)
	}
	if count := countSubagentContextRows(t, repo, "session-once", "tc-once"); count != 0 {
		t.Fatalf("row count after second boot = %d, want 0 (backfill must not re-scan once activated)", count)
	}
}

// TestSubagentContextBackfillStatementFailureLogsWarnWithMigrationName covers
// AC-20: because MigrateLogger.Apply swallows a migration statement's error
// (internal/db/migratelog.go), the only way a failure is observable is a WARN
// log carrying the migration's name. Forces the backfill INSERT to fail (its
// target table dropped out from under it) and asserts that WARN, using the
// same zaptest/observer pattern as
// TestCutover_RolledBackCutoverEmitsNoSuccessLog in
// worktree_ownership_election_test.go.
func TestSubagentContextBackfillStatementFailureLogsWarnWithMigrationName(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "subagent-context-migration-failure.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS kandev_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL DEFAULT '')`); err != nil {
		t.Fatalf("create kandev_meta: %v", err)
	}

	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observer logger: %v", err)
	}
	repo := &Repository{db: db, ro: db, log: log, migrate: dbutil.NewMigrateLogger(db, log)}
	if err := repo.initSchema(); err != nil {
		t.Fatalf("initSchema: %v", err)
	}

	seedForMsgTest(t, repo, "task-migfail", "session-migfail", "turn-migfail")
	seedSubagentMessage(t, repo, "msg-migfail", "session-migfail", "task-migfail", "turn-migfail",
		"tc-migfail", "", "completed",
		map[string]interface{}{"status": "completed"},
		time.Date(2026, 8, 5, 19, 0, 0, 0, time.UTC), time.Date(2026, 8, 5, 19, 0, 0, 0, time.UTC),
	)

	// Reset to "not yet activated" (see newSubagentMigrationTestRepo's
	// comment) so the next runMigrations() call actually attempts the
	// backfill, then break its INSERT statement so that attempt fails. Drop a
	// column the INSERT's column list needs instead of the whole table, so the
	// failure is isolated to the backfill statement.
	if _, err := db.Exec(`DELETE FROM kandev_meta WHERE key IN ('subagent_context_capture_since', 'subagent_context_backfill_through')`); err != nil {
		t.Fatalf("reset activation keys: %v", err)
	}
	if _, err := db.Exec(`ALTER TABLE task_session_subagents DROP COLUMN updated_at`); err != nil {
		t.Fatalf("drop updated_at column: %v", err)
	}

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations must swallow the failure, not return it: %v", err)
	}

	entries := logs.FilterMessage("migration failed").All()
	var found bool
	for _, entry := range entries {
		if entry.ContextMap()["name"] == "task_session_subagents.backfill" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no WARN log carrying migration name %q found among %d entries", "task_session_subagents.backfill", len(entries))
	}

	// AC-24d: the INSERT and both activation-key writes share one
	// transaction, so a failed INSERT must leave neither key written —
	// never just the backfill row-set incomplete while activation still
	// reports "done". Confirms this stays true rather than one write
	// slipping outside the transaction in a future edit.
	if _, ok := readMetaKey(t, db, subagentContextCaptureSinceKey); ok {
		t.Fatalf("%s must not be written when the backfill INSERT fails", subagentContextCaptureSinceKey)
	}
	if _, ok := readMetaKey(t, db, subagentContextBackfillThroughKey); ok {
		t.Fatalf("%s must not be written when the backfill INSERT fails", subagentContextBackfillThroughKey)
	}
}

// TestSubagentContextBackfillDoesNotRescanWhenBackfillThroughEmpty is SR45's
// named discriminating test: the activation guard must be a row-EXISTENCE
// check on both kandev_meta keys, not a value-non-empty check. AC-24f makes
// the empty string a legitimate, present value for backfill_through (no
// message matched the predicate at activation time) — a value-non-empty
// guard would misread that row as "not yet activated" and re-scan on every
// later boot, exactly the per-boot cost AC-23a forbids. This test activates
// with zero messages present (so backfill_through is written as ”) and
// then proves a second runMigrations() call, after a message now exists,
// still does not backfill it.
func TestSubagentContextBackfillDoesNotRescanWhenBackfillThroughEmpty(t *testing.T) {
	repo, db := newSubagentMigrationTestRepo(t)
	seedForMsgTest(t, repo, "task-empty-guard", "session-empty-guard", "turn-empty-guard")

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("first runMigrations (activates with no messages): %v", err)
	}
	backfillThrough, ok := readMetaKey(t, db, "subagent_context_backfill_through")
	if !ok {
		t.Fatal("subagent_context_backfill_through must be present after activation")
	}
	if backfillThrough != "" {
		t.Fatalf("subagent_context_backfill_through = %q, want empty string (no messages existed at activation)", backfillThrough)
	}

	ts := time.Date(2026, 8, 5, 20, 0, 0, 0, time.UTC)
	seedSubagentMessage(t, repo, "msg-empty-guard", "session-empty-guard", "task-empty-guard", "turn-empty-guard",
		"tc-empty-guard", "", "completed", map[string]interface{}{"status": "completed"}, ts, ts)

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("second runMigrations (a later boot, message now exists): %v", err)
	}
	if count := countSubagentContextRows(t, repo, "session-empty-guard", "tc-empty-guard"); count != 0 {
		t.Fatalf("row count after second boot = %d, want 0: an empty-string backfill_through is a PRESENT row and must not re-trigger the scan", count)
	}
}
