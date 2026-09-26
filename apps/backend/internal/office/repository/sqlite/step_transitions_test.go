package sqlite_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// newTestRepoWithTaskSchema builds an office repository on a database that
// also carries the task_step_transitions ledger table — office's own
// initSchema does not create it (it is owned by internal/task/repository/
// sqlite), so GetStepTransitionActor's tests need both schemas present on
// the same connection, exactly as production wires them. Returns the raw
// db handle too, so the test can seed ledger rows directly.
func newTestRepoWithTaskSchema(t *testing.T) (*sqlite.Repository, *sqlx.DB) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	if _, err := tasksqlite.NewWithDB(db, db, nil); err != nil {
		t.Fatalf("task schema init: %v", err)
	}
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new office repo: %v", err)
	}
	return repo, db
}

// insertStepTransition writes a raw task_step_transitions row, mirroring
// the precedent in internal/task/repository/sqlite/step_transition_reads_test.go.
// task_step_transitions.task_id is a foreign key onto tasks(id) and FK
// enforcement is on for this connection, so it seeds a minimal 'task-1' row
// (idempotent — INSERT OR IGNORE) the first time it is called.
func insertStepTransition(
	t *testing.T, db *sqlx.DB, id int64, actorKind, actorID, sessionID string, occurredAt time.Time,
) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := db.Exec(
		`INSERT OR IGNORE INTO tasks (id, title, created_at, updated_at) VALUES ('task-1', 'Task 1', ?, ?)`,
		now, now,
	); err != nil {
		t.Fatalf("seed task-1: %v", err)
	}
	var actorIDArg, sessionIDArg any
	if actorID != "" {
		actorIDArg = actorID
	}
	if sessionID != "" {
		sessionIDArg = sessionID
		if _, err := db.Exec(
			`INSERT OR IGNORE INTO task_sessions (id, task_id, started_at, updated_at) VALUES (?, 'task-1', ?, ?)`,
			sessionID, now, now,
		); err != nil {
			t.Fatalf("seed task session %q: %v", sessionID, err)
		}
	}
	_, err := db.Exec(db.Rebind(`
		INSERT INTO task_step_transitions
			(id, task_id, session_id, from_workflow_id, from_workflow_step_id,
			 to_workflow_id, to_workflow_step_id, trigger, actor_kind, actor_id,
			 contract_version, occurred_at)
		VALUES (?, 'task-1', ?, 'wf-1', 'from-step', 'wf-1', 'to-step', 'test', ?, ?, 1, ?)
	`), id, sessionIDArg, actorKind, actorIDArg, occurredAt)
	if err != nil {
		t.Fatalf("insert step transition %d: %v", id, err)
	}
}

// TestGetStepTransitionActor_ReadsAgentAttribution covers the agent half of
// AC-OFFICE-RUN-CAUSATION-001.25: a step-entry wake caused by a ledger row
// whose actor_kind is "agent" must resolve that row's own session_id and
// occurred_at, so the caller can find the run that executed the turn which
// produced the transition.
func TestGetStepTransitionActor_ReadsAgentAttribution(t *testing.T) {
	repo, db := newTestRepoWithTaskSchema(t)
	occurredAt := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	insertStepTransition(t, db, 1, "agent", "", "sess-1", occurredAt)

	got, err := repo.GetStepTransitionActor(context.Background(), 1)
	if err != nil {
		t.Fatalf("get step transition actor: %v", err)
	}
	if got == nil {
		t.Fatal("got nil, want a resolved actor row")
	}
	if string(got.ActorKind) != "agent" {
		t.Errorf("actor_kind = %q, want %q", got.ActorKind, "agent")
	}
	if got.SessionID != "sess-1" {
		t.Errorf("session_id = %q, want %q", got.SessionID, "sess-1")
	}
	if !got.OccurredAt.Equal(occurredAt) {
		t.Errorf("occurred_at = %v, want %v", got.OccurredAt, occurredAt)
	}
}

// TestGetStepTransitionActor_ReadsHumanAttribution covers the human half:
// actor_id is populated and session_id is absent for a human-triggered move.
func TestGetStepTransitionActor_ReadsHumanAttribution(t *testing.T) {
	repo, db := newTestRepoWithTaskSchema(t)
	occurredAt := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	insertStepTransition(t, db, 2, "human", "user-42", "", occurredAt)

	got, err := repo.GetStepTransitionActor(context.Background(), 2)
	if err != nil {
		t.Fatalf("get step transition actor: %v", err)
	}
	if string(got.ActorKind) != "human" {
		t.Errorf("actor_kind = %q, want %q", got.ActorKind, "human")
	}
	if got.ActorID != "user-42" {
		t.Errorf("actor_id = %q, want %q", got.ActorID, "user-42")
	}
	if got.SessionID != "" {
		t.Errorf("session_id = %q, want empty", got.SessionID)
	}
}

// TestGetStepTransitionActor_UnknownIDReturnsNoRows pins the sentinel a
// resolver falls back to the system/task-boundary carrier on.
func TestGetStepTransitionActor_UnknownIDReturnsNoRows(t *testing.T) {
	repo, _ := newTestRepoWithTaskSchema(t)
	if _, err := repo.GetStepTransitionActor(context.Background(), 999); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("err = %v, want sql.ErrNoRows", err)
	}
}
