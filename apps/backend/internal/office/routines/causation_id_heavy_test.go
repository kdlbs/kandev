package routines_test

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routines"
)

// newTestRoutineServiceWithTaskTables mirrors newTestRoutineService but
// also creates minimal tasks/task_sessions tables on the same database,
// so a heavy-path test can walk the real
// office_routine_runs.linked_task_id -> tasks.id -> task_sessions.task_id
// chain instead of only asserting the id fields in isolation.
func newTestRoutineServiceWithTaskTables(t *testing.T) (*routines.RoutineService, *sqlx.DB) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT '',
		title TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS task_sessions (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create task_sessions table: %v", err)
	}

	log := logger.Default()
	return routines.NewRoutineService(repo, log, &noopActivity{}), db
}

// dbBackedTaskCreator is CreateOfficeTaskInWorkflow's fake for this
// file only: unlike fakeTaskCreator (a synthetic id with nothing
// behind it), it inserts a real tasks row plus a task_sessions row
// naming it, so the produced id is actually reachable by a join —
// standing in for auto_start_agent launching the routine's task.
type dbBackedTaskCreator struct {
	db       *sqlx.DB
	taskID   string
	sessID   string
	nextSeq  int
	captured struct{ workflowID string }
}

func (f *dbBackedTaskCreator) CreateOfficeTaskInWorkflow(
	_ context.Context, workspaceID, _, _, workflowID, title, _ string,
) (string, error) {
	f.nextSeq++
	f.captured.workflowID = workflowID
	f.taskID = "task-heavy-causation"
	f.sessID = "session-heavy-causation"
	if _, err := f.db.Exec(`INSERT INTO tasks (id, workspace_id, title) VALUES (?, ?, ?)`,
		f.taskID, workspaceID, title); err != nil {
		return "", err
	}
	if _, err := f.db.Exec(`INSERT INTO task_sessions (id, task_id) VALUES (?, ?)`,
		f.sessID, f.taskID); err != nil {
		return "", err
	}
	return f.taskID, nil
}

// AC-OFFICE-LOOP-LIVENESS-002.9: a heavy routine fire's run row carries
// both the minted causation id and the linked task id, and that task id
// is a real, reachable row — walkable forward to the session the task
// launched, the correlation chain a loop-liveness diagnosis depends on.
func TestDispatchRoutineRun_HeavyPathCausationIDAndLinkedTaskReachSession(t *testing.T) {
	svc, db := newTestRoutineServiceWithTaskTables(t)
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID:            "ws-1",
		Name:                   "Heavy Causation",
		TaskTemplate:           `{"title":"Heavy {{name}}","description":"D"}`,
		AssigneeAgentProfileID: "agent-heavy",
		Status:                 "active",
		ConcurrencyPolicy:      "always_create",
		Variables:              `{"name":{"default":"routine"}}`,
	}
	if err := svc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}

	wf := &fakeWorkflowEnsurer{}
	tc := &dbBackedTaskCreator{db: db}
	svc.SetWorkflowEnsurer(wf)
	svc.SetTaskCreator(tc)

	run, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("fire manual: %v", err)
	}
	if run.CausationID == "" {
		t.Fatal("expected non-empty causation id on the heavy-path run")
	}
	if run.LinkedTaskID != tc.taskID {
		t.Fatalf("LinkedTaskID = %q, want %q", run.LinkedTaskID, tc.taskID)
	}

	var sessionID string
	err = db.QueryRow(`
		SELECT task_sessions.id
		FROM office_routine_runs
		JOIN tasks ON tasks.id = office_routine_runs.linked_task_id
		JOIN task_sessions ON task_sessions.task_id = tasks.id
		WHERE office_routine_runs.id = ?
	`, run.ID).Scan(&sessionID)
	if err != nil {
		t.Fatalf("two-hop join from routine run to session: %v", err)
	}
	if sessionID != tc.sessID {
		t.Fatalf("joined session id = %q, want %q", sessionID, tc.sessID)
	}
}
