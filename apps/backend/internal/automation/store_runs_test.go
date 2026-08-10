package automation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func createTasksTable(t *testing.T, store *Store) {
	t.Helper()
	if _, err := store.db.Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY, archived_at DATETIME)`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TABLE task_sessions (id TEXT PRIMARY KEY, task_id TEXT NOT NULL, is_primary INTEGER NOT NULL DEFAULT 0, state TEXT NOT NULL DEFAULT 'CREATED')`); err != nil {
		t.Fatal(err)
	}
	// listRunsWithTaskState reads the agent's last message for the run summary.
	// Without this table the whole query fails as missing-table and ListRuns
	// silently falls back to the raw status, losing the archived/cancelled
	// derivation these tests assert.
	if _, err := store.db.Exec(`CREATE TABLE task_session_messages (id TEXT PRIMARY KEY, task_id TEXT NOT NULL DEFAULT '', author_type TEXT NOT NULL DEFAULT 'user', content TEXT NOT NULL DEFAULT '', type TEXT NOT NULL DEFAULT 'message', created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
	// PrunableRunTaskIDs only offers runs that still hold a checkout, and a
	// checkout is reached task → environment → environment-repository row.
	// Without this table the query fails outright rather than returning
	// nothing.
	if _, err := store.db.Exec(`CREATE TABLE task_environments (id TEXT PRIMARY KEY, task_id TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TABLE task_environment_repos (id TEXT PRIMARY KEY, task_environment_id TEXT NOT NULL, repository_id TEXT NOT NULL DEFAULT '', worktree_id TEXT NOT NULL DEFAULT '', worktree_path TEXT DEFAULT '', status TEXT NOT NULL DEFAULT 'active', deleted_at TIMESTAMP)`); err != nil {
		t.Fatal(err)
	}
}

func insertTask(t *testing.T, store *Store, id string, archived bool) {
	t.Helper()
	var archivedAt interface{}
	if archived {
		archivedAt = time.Now().UTC()
	}
	if _, err := store.db.Exec(`INSERT INTO tasks (id, archived_at) VALUES (?, ?)`, id, archivedAt); err != nil {
		t.Fatal(err)
	}
}

// insertPrimarySession seeds the given task's *current* session (is_primary
// = 1) with an explicit models.TaskSessionState string (e.g. "CANCELLED"),
// so tests can exercise the genuine-cancellation branch of
// listRunsWithTaskState/countActiveRunsWithTaskState independently of
// archived_at. Mirrors production: a task has at most one is_primary = 1
// session at a time (SetPrimarySession unsets the rest on resume).
func insertPrimarySession(t *testing.T, store *Store, taskID, state string) {
	t.Helper()
	if _, err := store.db.Exec(
		`INSERT INTO task_sessions (id, task_id, is_primary, state) VALUES (?, ?, 1, ?)`,
		taskID+"-session", taskID, state,
	); err != nil {
		t.Fatal(err)
	}
}

// insertStaleSession seeds a non-primary (is_primary = 0) session for the
// given task — e.g. a CANCELLED session left over from a stop, before the
// task was resumed and its is_primary flag moved to a fresh session. Used
// to prove the current-session filter isn't fooled by cancellation
// history that no longer reflects the task's live state.
func insertStaleSession(t *testing.T, store *Store, taskID, state string) {
	t.Helper()
	if _, err := store.db.Exec(
		`INSERT INTO task_sessions (id, task_id, is_primary, state) VALUES (?, ?, 0, ?)`,
		taskID+"-stale-session", taskID, state,
	); err != nil {
		t.Fatal(err)
	}
}

// TestCountActiveRuns_ExcludesArchivedCancelledOrMissingTask reproduces the
// reported bug: an automation-generated task that gets archived (manually,
// via auto-archive, via cascade, or by the agent itself) or is explicitly
// cancelled leaves its automation run stuck at task_created forever unless
// CountActiveRuns checks the task's current state. A run whose task is
// archived, cancelled, gone entirely, or never recorded (empty task_id —
// see countActiveRunsWithTaskState's docstring in store.go) no longer
// represents outstanding work and must not count against
// max_concurrent_runs.
func TestCountActiveRuns_ExcludesArchivedCancelledOrMissingTask(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "A", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}

	insertTask(t, store, "task-active", false)
	insertTask(t, store, "task-archived", true)
	insertTask(t, store, "task-cancelled", false)
	insertPrimarySession(t, store, "task-cancelled", "CANCELLED")
	// Resumed after a stop: the stale CANCELLED session is no longer
	// primary, so this task must still count as active.
	insertTask(t, store, "task-resumed", false)
	insertStaleSession(t, store, "task-resumed", "CANCELLED")
	insertPrimarySession(t, store, "task-resumed", "RUNNING")
	// "task-missing" deliberately has no row in tasks at all; "" (the
	// task_id column default) exercises the same no-live-task branch
	// through a different route.

	for _, tid := range []string{"task-active", "task-archived", "task-cancelled", "task-resumed", "task-missing", ""} {
		if err := store.CreateRun(ctx, &AutomationRun{
			AutomationID: a.ID,
			TriggerType:  TriggerTypeScheduled,
			Status:       RunStatusTaskCreated,
			TaskID:       tid,
			TriggerData:  json.RawMessage(`{}`),
		}); err != nil {
			t.Fatal(err)
		}
	}

	count, err := store.CountActiveRuns(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected 2 active runs (task-active and task-resumed are open), got %d", count)
	}
}

// TestCountActiveRuns_FallsBackWhenTasksTableAbsent guards the isolated
// automation-only test DB (and any other DB where the task repository
// hasn't initialised yet): CountActiveRuns must keep counting by status
// alone rather than error when the tasks table doesn't exist.
func TestCountActiveRuns_FallsBackWhenTasksTableAbsent(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "A", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRun(ctx, &AutomationRun{
		AutomationID: a.ID,
		TriggerType:  TriggerTypeScheduled,
		Status:       RunStatusTaskCreated,
		TaskID:       "task-xyz",
		TriggerData:  json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}

	count, err := store.CountActiveRuns(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 active run when tasks table is absent, got %d", count)
	}
}

// TestListRuns_DerivesArchivedCancelledAndActiveStatus ensures the "Recent
// Runs" list the settings UI reads stops labeling a run "Running" once its
// generated task is archived, cancelled, gone, or never recorded (empty
// task_id — see listRunsWithTaskState's docstring in store.go). Archived
// (regardless of whether the UI or the agent itself triggered it) must be
// visually distinct from a genuine user cancellation (task state
// CANCELLED), and archived_at takes precedence when a task is both
// cancelled and archived. Runs that already reached a real terminal
// outcome are left untouched.
func TestListRuns_DerivesArchivedCancelledAndActiveStatus(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "A", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	insertTask(t, store, "task-active", false)
	insertTask(t, store, "task-archived", true)
	insertTask(t, store, "task-cancelled", false)
	insertPrimarySession(t, store, "task-cancelled", "CANCELLED")
	insertTask(t, store, "task-cancelled-and-archived", true)
	insertPrimarySession(t, store, "task-cancelled-and-archived", "CANCELLED")
	// Stopped once (stale, non-primary CANCELLED session), then resumed
	// with a fresh primary session that completed. Must read as its raw
	// stored status, not cancelled — the stale session no longer reflects
	// the task's live state.
	insertTask(t, store, "task-resumed", false)
	insertStaleSession(t, store, "task-resumed", "CANCELLED")
	insertPrimarySession(t, store, "task-resumed", "COMPLETED")

	active := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusTaskCreated, TaskID: "task-active", TriggerData: json.RawMessage(`{}`)}
	archived := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusTaskCreated, TaskID: "task-archived", TriggerData: json.RawMessage(`{}`)}
	cancelled := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusTaskCreated, TaskID: "task-cancelled", TriggerData: json.RawMessage(`{}`)}
	cancelledAndArchived := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusTaskCreated, TaskID: "task-cancelled-and-archived", TriggerData: json.RawMessage(`{}`)}
	resumed := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusTaskCreated, TaskID: "task-resumed", TriggerData: json.RawMessage(`{}`)}
	missing := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusTaskCreated, TaskID: "task-missing", TriggerData: json.RawMessage(`{}`)}
	emptyTaskID := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusTaskCreated, TaskID: "", TriggerData: json.RawMessage(`{}`)}
	succeededOnArchived := &AutomationRun{AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusSucceeded, TaskID: "task-archived", TriggerData: json.RawMessage(`{}`)}
	allRuns := []*AutomationRun{active, archived, cancelled, cancelledAndArchived, resumed, missing, emptyTaskID, succeededOnArchived}
	for _, r := range allRuns {
		if err := store.CreateRun(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	got, err := store.ListRuns(ctx, a.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	statusByID := map[string]RunStatus{}
	for _, r := range got {
		statusByID[r.ID] = r.Status
	}
	if s := statusByID[active.ID]; s != RunStatusTaskCreated {
		t.Errorf("active task run: expected task_created, got %q", s)
	}
	if s := statusByID[archived.ID]; s != RunStatusArchived {
		t.Errorf("archived task run: expected archived, got %q", s)
	}
	if s := statusByID[cancelled.ID]; s != RunStatusCancelled {
		t.Errorf("cancelled task run: expected cancelled, got %q", s)
	}
	if s := statusByID[cancelledAndArchived.ID]; s != RunStatusArchived {
		t.Errorf("cancelled-and-archived task run: expected archived_at to take precedence (archived), got %q", s)
	}
	if s := statusByID[resumed.ID]; s != RunStatusTaskCreated {
		t.Errorf("resumed-after-cancel task run: expected task_created (stale session ignored), got %q", s)
	}
	if s := statusByID[missing.ID]; s != RunStatusCancelled {
		t.Errorf("missing task run: expected cancelled, got %q", s)
	}
	if s := statusByID[emptyTaskID.ID]; s != RunStatusCancelled {
		t.Errorf("empty task_id run: expected cancelled, got %q", s)
	}
	if s := statusByID[succeededOnArchived.ID]; s != RunStatusSucceeded {
		t.Errorf("already-succeeded run on now-archived task: expected succeeded preserved, got %q", s)
	}
}

// TestListRuns_FallsBackWhenTasksTableAbsent mirrors
// TestCountActiveRuns_FallsBackWhenTasksTableAbsent for the display path.
func TestListRuns_FallsBackWhenTasksTableAbsent(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "A", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateRun(ctx, &AutomationRun{
		AutomationID: a.ID,
		TriggerType:  TriggerTypeScheduled,
		Status:       RunStatusTaskCreated,
		TaskID:       "task-xyz",
		TriggerData:  json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}

	got, err := store.ListRuns(ctx, a.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Status != RunStatusTaskCreated {
		t.Fatalf("expected 1 run with status task_created when tasks table is absent, got %+v", got)
	}
}

// A run-mode automation hides its task, so the run row is the only place the
// reader can learn what the agent actually reported. Before this the row could
// say a run succeeded but never what it said.
func TestListRuns_CarriesTheAgentsLastMessageAsSummary(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	createTasksTable(t, store)

	a := &Automation{WorkspaceID: "ws-1", Name: "reporter", WorkflowID: "wf-1", WorkflowStepID: "s-1"}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	insertTask(t, store, "task-report", false)
	if err := store.CreateRun(ctx, &AutomationRun{
		AutomationID: a.ID,
		TriggerType:  TriggerTypeScheduled,
		Status:       RunStatusSucceeded,
		TaskID:       "task-report",
		TriggerData:  json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}

	seed := func(id, author, content string, at time.Time) {
		t.Helper()
		if _, err := store.db.Exec(
			`INSERT INTO task_session_messages (id, task_id, author_type, content, type, created_at) VALUES (?,?,?,?,?,?)`,
			id, "task-report", author, content, "message", at); err != nil {
			t.Fatal(err)
		}
	}
	base := time.Now().UTC()
	seed("m1", "user", "run the sweep", base)
	seed("m2", "agent", "Working on it.", base.Add(time.Minute))
	seed("m3", "agent", "Sweep complete across all 32 specs.", base.Add(2*time.Minute))

	runs, err := store.ListRuns(ctx, a.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if runs[0].Summary != "Sweep complete across all 32 specs." {
		t.Errorf("expected the agent's LAST message, got %q", runs[0].Summary)
	}
}

func TestListRuns_SummaryIsEmptyWhenTheAgentNeverSpoke(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()
	createTasksTable(t, store)

	a := &Automation{WorkspaceID: "ws-1", Name: "quiet", WorkflowID: "wf-1", WorkflowStepID: "s-1"}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	// A skipped run never produces a task, so there is nothing to summarise.
	if err := store.CreateRun(ctx, &AutomationRun{
		AutomationID: a.ID,
		TriggerType:  TriggerTypeScheduled,
		Status:       RunStatusSkipped,
		ErrorMessage: "max_concurrent_runs=1 reached",
		TriggerData:  json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}

	runs, err := store.ListRuns(ctx, a.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	if runs[0].Summary != "" {
		t.Errorf("expected no summary, got %q", runs[0].Summary)
	}
}

// setRunCreatedAt pins a run's created_at so ordering assertions don't
// depend on how many nanoseconds apart two CreateRun calls landed.
func setRunCreatedAt(t *testing.T, store *Store, runID string, at time.Time) {
	t.Helper()
	if _, err := store.db.Exec(`UPDATE automation_runs SET created_at = ? WHERE id = ?`, at, runID); err != nil {
		t.Fatal(err)
	}
}

// The workspace-wide feed is the only view that interleaves runs from
// different automations, so it's the only place a run can be mistaken for
// another automation's — hence the name rides along with each row.
// A run from a second workspace is seeded *newest of all* so exclusion
// can't be mistaken for it simply falling off the end of the ordering.

// The detail view mounts a run's transcript in place, and the chat panel is
// driven by a session id rather than a task id. Resolving it in the run
// projection keeps that one query instead of one lookup per run on the client.
func TestListRuns_CarriesTheRunsPrimarySession(t *testing.T) {
	store := setupTestStore(t)
	createTasksTable(t, store)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "sweep", WorkflowID: "wf-1", WorkflowStepID: "s-1"}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	insertTask(t, store, "task-with-session", false)
	insertPrimarySession(t, store, "task-with-session", "COMPLETED")
	insertTask(t, store, "task-without-session", false)

	withSession := &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeScheduled,
		Status: RunStatusSucceeded, TaskID: "task-with-session", TriggerData: json.RawMessage(`{}`),
	}
	without := &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeScheduled,
		Status: RunStatusSkipped, TaskID: "task-without-session", TriggerData: json.RawMessage(`{}`),
	}
	for _, r := range []*AutomationRun{withSession, without} {
		if err := store.CreateRun(ctx, r); err != nil {
			t.Fatal(err)
		}
	}

	runs, err := store.ListRuns(ctx, a.ID, 50)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]*AutomationRun{}
	for _, r := range runs {
		byID[r.ID] = r
	}
	if got := byID[withSession.ID]; got == nil || got.SessionID == "" {
		t.Fatalf("expected the run to carry its primary session, got %+v", got)
	}
	// A run with no conversation reports none rather than a dangling id the
	// transcript would try to mount.
	if got := byID[without.ID]; got == nil || got.SessionID != "" {
		t.Fatalf("expected no session for a run that never started one, got %+v", got)
	}
}
