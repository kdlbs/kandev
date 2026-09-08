package sqlite

// Schema-level coverage for task_usage_events (docs/specs/task-cost-ledger/spec.md):
// table shape, the named unique index AC-32's detector matches on, and the two
// FK behaviors that make this table an append-only durable record rather than
// a table Cascade quietly loses attribution from.

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	dbutil "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
)

func newUsageEventsTestRepo(t *testing.T) *Repository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "usage-events.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("initialize schema: %v", err)
	}
	return repo
}

func createUsageEventsTestTask(t *testing.T, repo *Repository, taskID string) *models.Task {
	t.Helper()
	task := &models.Task{
		ID:          taskID,
		WorkspaceID: "ws-1",
		Title:       "Usage events test task",
		Priority:    "medium",
	}
	if err := repo.CreateTask(context.Background(), task); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	return task
}

func createUsageEventsTestSession(t *testing.T, repo *Repository, sessionID, taskID string) {
	t.Helper()
	if err := repo.CreateTaskSession(context.Background(), &models.TaskSession{
		ID: sessionID, TaskID: taskID, State: models.TaskSessionStateWaitingForInput,
	}); err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
}

func insertUsageEventRow(t *testing.T, repo *Repository, eventID, taskID, sessionID string) {
	t.Helper()
	now := time.Now().UTC()
	var sessionArg interface{}
	if sessionID != "" {
		sessionArg = sessionID
	}
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_usage_events (
			usage_event_id, task_id, session_id, turn_id,
			agent_profile_id, agent_type, model, provider,
			tokens_in, tokens_cached_read, tokens_cached_write, tokens_out, tokens_thought,
			tokens_total, cost_subcents, cost_source, estimated,
			contract_version, occurred_at, created_at
		) VALUES (?, ?, ?, NULL, '', '', '', '', 0, 0, 0, 0, 0, 0, 0, 'unpriced', 0, 1, ?, ?)
	`), eventID, taskID, sessionArg, now, now)
	if err != nil {
		t.Fatalf("insert task_usage_events row %s: %v", eventID, err)
	}
}

// TestTaskUsageEventsSchema_FreshDB proves the table, its named unique index,
// and its three supporting indexes exist on a fresh database.
func TestTaskUsageEventsSchema_FreshDB(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-1")

	insertUsageEventRow(t, repo, "evt-1", "task-1", "")

	var count int
	if err := repo.db.Get(&count, `SELECT COUNT(*) FROM task_usage_events`); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("row count = %d, want 1", count)
	}

	var indexNames []string
	if err := repo.db.Select(&indexNames, `
		SELECT name FROM sqlite_master
		WHERE type = 'index' AND tbl_name = 'task_usage_events'
	`); err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	want := map[string]bool{
		"uniq_task_usage_events_usage_event_id": false,
		"idx_task_usage_events_task":            false,
		"idx_task_usage_events_session_turn":    false,
	}
	for _, name := range indexNames {
		if name == "idx_task_usage_events_occurred" {
			t.Errorf("unexpected standalone occurred_at index %s, got indexes %v", name, indexNames)
		}
		if _, ok := want[name]; ok {
			want[name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("expected index %s to exist, got indexes %v", name, indexNames)
		}
	}
}

// TestTaskUsageEventsSchema_Replay proves re-running schema init against an
// already-initialized database (a restart) does not error - CREATE TABLE IF
// NOT EXISTS / CREATE INDEX IF NOT EXISTS must both be idempotent - and that
// a usage event written before the replay, along with the rollup it drove,
// survives the replay untouched, and that the ledger still accepts new
// writes afterward (docs/specs/task-cost-ledger/spec.md AC-29, AC-36 row 3).
func TestTaskUsageEventsSchema_Replay(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "usage-events-replay.db")
	dbConn, err := dbutil.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db := sqlx.NewDb(dbConn, "sqlite3")
	t.Cleanup(func() { _ = db.Close() })

	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("first schema init: %v", err)
	}
	createUsageEventsTestTask(t, repo, "task-replay")
	createUsageEventsTestSession(t, repo, "session-replay", "task-replay")
	preReplayEvent := newTestUsageEvent("evt-replay-pre", "task-replay", "session-replay")
	if err := repo.CreateTaskUsageEvent(context.Background(), preReplayEvent); err != nil {
		t.Fatalf("CreateTaskUsageEvent before replay: %v", err)
	}

	wantRows := countTaskUsageEventRows(t, repo)
	wantTokensIn, wantTokensCachedIn, wantTokensOut, wantCostSubcents := readTaskSessionRollup(t, repo, "session-replay")

	repo, err = NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("replayed schema init must be a no-op, got error: %v", err)
	}

	if got := countTaskUsageEventRows(t, repo); got != wantRows {
		t.Errorf("row count after replay = %d, want %d (replay must not touch existing rows)", got, wantRows)
	}
	tokensIn, tokensCachedIn, tokensOut, costSubcents := readTaskSessionRollup(t, repo, "session-replay")
	if tokensIn != wantTokensIn || tokensCachedIn != wantTokensCachedIn || tokensOut != wantTokensOut || costSubcents != wantCostSubcents {
		t.Errorf("rollup after replay = (%d,%d,%d,%d), want (%d,%d,%d,%d) (replay must not touch the existing rollup)",
			tokensIn, tokensCachedIn, tokensOut, costSubcents, wantTokensIn, wantTokensCachedIn, wantTokensOut, wantCostSubcents)
	}

	postReplayEvent := newTestUsageEvent("evt-replay-post", "task-replay", "session-replay")
	if err := repo.CreateTaskUsageEvent(context.Background(), postReplayEvent); err != nil {
		t.Fatalf("CreateTaskUsageEvent after replay: %v", err)
	}
	if got := countTaskUsageEventRows(t, repo); got != wantRows+1 {
		t.Errorf("row count after post-replay write = %d, want %d (the ledger must still accept writes after replay)", got, wantRows+1)
	}
	tokensIn, tokensCachedIn, tokensOut, costSubcents = readTaskSessionRollup(t, repo, "session-replay")
	if tokensIn != wantTokensIn*2 || tokensCachedIn != wantTokensCachedIn*2 || tokensOut != wantTokensOut*2 || costSubcents != wantCostSubcents*2 {
		t.Errorf("rollup after post-replay write = (%d,%d,%d,%d), want (%d,%d,%d,%d) (the post-replay write must still increment the rollup)",
			tokensIn, tokensCachedIn, tokensOut, costSubcents, wantTokensIn*2, wantTokensCachedIn*2, wantTokensOut*2, wantCostSubcents*2)
	}
}

// TestTaskUsageEventsSchema_UniqueIndexRejectsDuplicateUsageEventID proves the
// named unique index actually enforces uniqueness at insert - AC-32's
// unique-violation detector depends on this constraint existing under this
// exact name.
func TestTaskUsageEventsSchema_UniqueIndexRejectsDuplicateUsageEventID(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-1")

	insertUsageEventRow(t, repo, "evt-dup", "task-1", "")

	now := time.Now().UTC()
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_usage_events (
			usage_event_id, task_id, session_id, turn_id,
			agent_profile_id, agent_type, model, provider,
			tokens_in, tokens_cached_read, tokens_cached_write, tokens_out, tokens_thought,
			tokens_total, cost_subcents, cost_source, estimated,
			contract_version, occurred_at, created_at
		) VALUES (?, ?, ?, ?, '', '', '', '', 0, 0, 0, 0, 0, 0, 0, 'unpriced', 0, 1, ?, ?)
	`), "evt-dup", "task-1", nil, nil, now, now)
	if err == nil {
		t.Fatal("expected a unique-constraint error inserting a duplicate usage_event_id, got nil")
	}
}

// TestTaskUsageEventsSchema_TaskDeleteCascades proves task_id CASCADEs: task
// deletion is complete deletion of its spend record.
func TestTaskUsageEventsSchema_TaskDeleteCascades(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-cascade")
	insertUsageEventRow(t, repo, "evt-cascade", "task-cascade", "")

	if err := repo.DeleteTask(context.Background(), "task-cascade"); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}

	var count int
	if err := repo.db.Get(&count, `SELECT COUNT(*) FROM task_usage_events WHERE task_id = ?`, "task-cascade"); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 0 {
		t.Errorf("row count after task deletion = %d, want 0 (task_id must CASCADE)", count)
	}
}

// TestTaskUsageEventsSchema_SessionDeleteSetsNull proves session_id is
// ON DELETE SET NULL: the historical fact that this spend happened must
// survive a session being pruned, mirroring task_step_transitions'
// rationale.
func TestTaskUsageEventsSchema_SessionDeleteSetsNull(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	createUsageEventsTestTask(t, repo, "task-setnull")
	createUsageEventsTestSession(t, repo, "session-setnull", "task-setnull")
	insertUsageEventRow(t, repo, "evt-setnull", "task-setnull", "session-setnull")

	if err := repo.DeleteTaskSession(context.Background(), "session-setnull"); err != nil {
		t.Fatalf("DeleteTaskSession: %v", err)
	}

	var sessionID *string
	if err := repo.db.Get(&sessionID, `SELECT session_id FROM task_usage_events WHERE usage_event_id = ?`, "evt-setnull"); err != nil {
		t.Fatalf("read row: %v", err)
	}
	if sessionID != nil {
		t.Errorf("session_id after session deletion = %v, want nil (row must survive with session_id cleared)", *sessionID)
	}

	var count int
	if err := repo.db.Get(&count, `SELECT COUNT(*) FROM task_usage_events WHERE usage_event_id = ?`, "evt-setnull"); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 1 {
		t.Errorf("row count after session deletion = %d, want 1 (row must be retained)", count)
	}
}

// TestTaskUsageEventsSchema_TaskForeignKeyRejectsUnknownTask proves task_id's
// FK is enforced: an insert naming a task that doesn't exist is rejected
// outright rather than committing, which is what makes AC-32's foreign-key
// classification reachable.
func TestTaskUsageEventsSchema_TaskForeignKeyRejectsUnknownTask(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	now := time.Now().UTC()
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_usage_events (
			usage_event_id, task_id, session_id, turn_id,
			agent_profile_id, agent_type, model, provider,
			tokens_in, tokens_cached_read, tokens_cached_write, tokens_out, tokens_thought,
			tokens_total, cost_subcents, cost_source, estimated,
			contract_version, occurred_at, created_at
		) VALUES (?, ?, ?, ?, '', '', '', '', 0, 0, 0, 0, 0, 0, 0, 'unpriced', 0, 1, ?, ?)
	`), "evt-no-task", "task-does-not-exist", nil, nil, now, now)
	if err == nil {
		t.Fatal("expected a foreign-key error inserting a row against an unknown task_id, got nil")
	}
}
