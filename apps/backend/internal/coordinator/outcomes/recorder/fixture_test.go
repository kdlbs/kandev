package recorder

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/db"
)

const externalTablesSQL = `
	CREATE TABLE tasks (id TEXT PRIMARY KEY, workspace_id TEXT, workflow_id TEXT, workflow_step_id TEXT, state TEXT, archived_at DATETIME);
	CREATE TABLE workflow_steps (id TEXT PRIMARY KEY, workflow_id TEXT, position INTEGER NOT NULL DEFAULT 0, complete_task_on_enter INTEGER NOT NULL DEFAULT 0);
	CREATE TABLE task_sessions (id TEXT PRIMARY KEY, task_id TEXT, state TEXT, started_at DATETIME);
	CREATE TABLE task_usage_events (id INTEGER PRIMARY KEY AUTOINCREMENT, task_id TEXT, session_id TEXT, turn_id TEXT, cost_subcents INTEGER NOT NULL DEFAULT 0, cost_source TEXT NOT NULL DEFAULT 'priced');
	CREATE TABLE github_task_prs (id TEXT PRIMARY KEY, task_id TEXT, state TEXT, merged_at DATETIME);
	CREATE TABLE session_step_history (id INTEGER PRIMARY KEY AUTOINCREMENT, session_id TEXT NOT NULL, from_step_id TEXT, to_step_id TEXT NOT NULL, trigger TEXT NOT NULL, actor_id TEXT, metadata TEXT, created_at DATETIME NOT NULL);
`

type fixture struct {
	t     *testing.T
	db    *sqlx.DB
	store *coordinator.Store
	coord *coordinator.Coordinator
	now   time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "outcomes.db")
	wc, err := db.OpenSQLite(path)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	t.Cleanup(func() { _ = wc.Close() })
	rc, err := db.OpenSQLiteReader(path)
	if err != nil {
		t.Fatalf("open reader: %v", err)
	}
	t.Cleanup(func() { _ = rc.Close() })
	writer, reader := sqlx.NewDb(wc, "sqlite3"), sqlx.NewDb(rc, "sqlite3")
	store, err := coordinator.NewStore(writer, reader)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	if _, err := writer.Exec(externalTablesSQL); err != nil {
		t.Fatalf("external tables: %v", err)
	}
	c := &coordinator.Coordinator{WorkspaceID: "ws-1", Name: "Coord", AgentProfileID: "agent-1", ExecutorProfileID: "exec-1"}
	if err := store.CreateCoordinator(context.Background(), c); err != nil {
		t.Fatalf("create coordinator: %v", err)
	}
	return &fixture{t: t, db: writer, store: store, coord: c, now: time.Now().UTC().Truncate(time.Millisecond)}
}

func (f *fixture) exec(query string, args ...any) {
	f.t.Helper()
	if _, err := f.db.Exec(f.db.Rebind(query), args...); err != nil {
		f.t.Fatalf("exec %q: %v", query, err)
	}
}

func (f *fixture) at(offset time.Duration) time.Time { return f.now.Add(offset) }

// proposal inserts a proposal of the fixture coordinator in status with an
// optional created task and returns its id.
func (f *fixture) proposal(id, status, kind, taskID string, updatedAt time.Time) string {
	f.t.Helper()
	var task any
	if taskID != "" {
		task = taskID
	}
	f.exec(`INSERT INTO coordinator_proposals (id, coordinator_id, workspace_id, status, spec_json, task_id, kind, created_at, updated_at)
		VALUES (?, ?, ?, ?, '{"title":"a","step_id":"s1"}', ?, ?, ?, ?)`,
		id, f.coord.ID, f.coord.WorkspaceID, status, task, kind, updatedAt.Add(-time.Minute), updatedAt)
	return id
}

// approvedRow inserts the approved created or moved activity row of a proposal.
func (f *fixture) approvedRow(id, proposalID, class, taskID string, at time.Time) {
	f.t.Helper()
	f.exec(`INSERT INTO coordinator_activity (id, coordinator_id, workspace_id, action_class, outcome, "authorization", target_task_id, proposal_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'approved', 'requires_approval', ?, ?, ?, ?)`, id, f.coord.ID, f.coord.WorkspaceID, class, taskID, proposalID, at, at)
}

func (f *fixture) task(id, stepID string, archived bool) {
	f.t.Helper()
	var archivedAt any
	if archived {
		archivedAt = f.now
	}
	f.exec(`INSERT INTO tasks (id, workspace_id, workflow_id, workflow_step_id, state, archived_at) VALUES (?, 'ws-1', 'wf', ?, 'TODO', ?)`, id, stepID, archivedAt)
}

func (f *fixture) step(id string, position int, completes bool) {
	f.t.Helper()
	f.exec(`INSERT INTO workflow_steps (id, workflow_id, position, complete_task_on_enter) VALUES (?, 'wf', ?, ?)`, id, position, completes)
}

type outcomeRow struct {
	ProposalID    string     `db:"proposal_id"`
	CoordinatorID string     `db:"coordinator_id"`
	Kind          string     `db:"kind"`
	TurnID        *string    `db:"turn_id"`
	Decision      string     `db:"decision"`
	Automatic     bool       `db:"automatic"`
	DecidedAt     time.Time  `db:"decided_at"`
	EditedJSON    string     `db:"edited_fields"`
	ReasonCode    string     `db:"reason_code"`
	TaskID        *string    `db:"task_id"`
	TaskResult    *string    `db:"task_result"`
	Cost          *int64     `db:"cost_subcents"`
	ReopenCount   int        `db:"reopen_count"`
	ApprovedAt    *time.Time `db:"approved_at"`
	MergedAt      *time.Time `db:"merged_at"`
	LastStepID    string     `db:"last_step_id"`
	Final         bool       `db:"final"`
	GradedAt      time.Time  `db:"graded_at"`
}

func (f *fixture) outcome(proposalID string) *outcomeRow {
	f.t.Helper()
	var rows []outcomeRow
	if err := f.db.Select(&rows, f.db.Rebind(`SELECT * FROM coordinator_outcomes WHERE proposal_id = ?`), proposalID); err != nil {
		f.t.Fatalf("read outcome: %v", err)
	}
	if len(rows) == 0 {
		return nil
	}
	return &rows[0]
}

func (f *fixture) outcomeCount() int {
	f.t.Helper()
	var n int
	if err := f.db.Get(&n, `SELECT COUNT(*) FROM coordinator_outcomes`); err != nil {
		f.t.Fatal(err)
	}
	return n
}
