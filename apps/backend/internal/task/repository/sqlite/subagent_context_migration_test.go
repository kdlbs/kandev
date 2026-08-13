package sqlite

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

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
		SELECT id, task_session_id, task_id, turn_id, parent_tool_call_id, subagent_type, description,
			agent_id, child_session_id, model, agent_status, tool_status, is_async,
			total_tokens, tool_use_count, duration_ms, source, observed_at, settled_at, updated_at
		FROM task_session_subagents WHERE task_session_id = ? AND tool_call_id = ?
	`), sessionID, toolCallID).Scan(
		&row.id, &row.taskSessionID, &row.taskID, &row.turnID, &row.parentToolCallID, &row.subagentType, &row.description,
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
	return repo, db
}

// dropMessageMetadataIndex removes the expression index on
// task_session_messages(metadata) so a malformed-metadata row (used to prove
// AC-23b tolerates historical corruption) can be seeded at all: with the
// index present, SQLite evaluates json_extract on every insert and rejects
// invalid JSON outright, which real corrupt rows predate.
func dropMessageMetadataIndex(t *testing.T, db *sqlx.DB) {
	t.Helper()
	for _, index := range []string{"idx_messages_metadata_tool_call_id", "idx_messages_metadata_pending_id"} {
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
func TestSubagentContextBackfillAsyncLaunchedStoresNullNotZero(t *testing.T) {
	repo, _ := newSubagentMigrationTestRepo(t)
	seedForMsgTest(t, repo, "task-async", "session-async", "turn-async")
	createdAt := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	updatedAt := createdAt.Add(time.Second)

	seedSubagentMessage(t, repo, "msg-async", "session-async", "task-async", "turn-async",
		"tc-async", "", "complete",
		map[string]interface{}{
			"description":   "run tests",
			"subagent_type": "test-runner",
			"status":        "async_launched",
			"is_async":      true,
		},
		createdAt, updatedAt,
	)

	if err := repo.runMigrations(); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}

	row := getSubagentContextRow(t, repo, "session-async", "tc-async")
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

// TestSubagentContextBackfillLiveRowWins covers AC-22: a live write always
// wins over a backfilled one, and a replay of the backfill is a true no-op.
func TestSubagentContextBackfillLiveRowWins(t *testing.T) {
	repo, db := newSubagentMigrationTestRepo(t)
	seedForMsgTest(t, repo, "task-live-wins", "session-live-wins", "turn-live-wins")
	ts := time.Date(2026, 8, 5, 15, 0, 0, 0, time.UTC)

	seedSubagentMessage(t, repo, "msg-live-wins", "session-live-wins", "task-live-wins", "turn-live-wins",
		"tc-live-wins", "", "completed",
		map[string]interface{}{"status": "completed", "model": "backfill-shape-model"},
		ts, ts,
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
// and never overwritten on replay.
//
// initSchema() runs runMigrations() as one of its steps, so NewWithDB already
// claims both keys once against the empty database created in this same
// boot — correctly recording "no history to backfill" for a genuinely fresh
// install. To exercise the "existing DB, historical messages predate this
// boot" case AC-24 is actually written for, this test deletes the two
// write-once rows (simulating that this is the database's first boot with
// the new migration) before seeding messages and re-running.
func TestSubagentContextActivationKeysWrittenOnce(t *testing.T) {
	repo, db := newSubagentMigrationTestRepo(t)
	seedForMsgTest(t, repo, "task-activation", "session-activation", "turn-activation")
	newest := time.Date(2026, 8, 9, 9, 0, 0, 0, time.UTC)
	older := time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC)

	seedSubagentMessage(t, repo, "msg-activation-older", "session-activation", "task-activation", "turn-activation",
		"tc-activation-older", "", "completed", map[string]interface{}{"status": "completed"}, older, older)
	seedSubagentMessage(t, repo, "msg-activation-newest", "session-activation", "task-activation", "turn-activation",
		"tc-activation-newest", "", "completed", map[string]interface{}{"status": "completed"}, newest, newest)

	if _, err := db.Exec(`DELETE FROM kandev_meta WHERE key IN ('subagent_context_capture_since', 'subagent_context_backfill_through')`); err != nil {
		t.Fatalf("reset activation keys to simulate first boot with data present: %v", err)
	}
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
