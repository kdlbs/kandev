package automation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func setupTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewStore(db, db)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestCreateAndGetAutomation(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a := &Automation{
		WorkspaceID:       "ws-1",
		Name:              "Test Automation",
		Description:       "Runs on cron",
		WorkflowID:        "wf-1",
		WorkflowStepID:    "step-1",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "exec-1",
		Prompt:            "Hello {{trigger.type}}",
		Enabled:           true,
		MaxConcurrentRuns: 1,
	}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	if a.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if a.WebhookSecret == "" {
		t.Fatal("expected non-empty webhook secret")
	}

	got, err := store.GetAutomation(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("expected automation, got nil")
	}
	if got.Name != "Test Automation" {
		t.Errorf("expected name 'Test Automation', got %q", got.Name)
	}
	if !got.Enabled {
		t.Error("expected enabled = true")
	}
}

func TestCreateAndListTriggers(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "A", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}

	cfg, _ := json.Marshal(ScheduledTriggerConfig{CronExpression: "*/5 * * * *"})
	t1 := &AutomationTrigger{AutomationID: a.ID, Type: TriggerTypeScheduled, Config: cfg, Enabled: true}
	if err := store.CreateTrigger(ctx, t1); err != nil {
		t.Fatal(err)
	}

	cfg2, _ := json.Marshal(WebhookTriggerConfig{})
	t2 := &AutomationTrigger{AutomationID: a.ID, Type: TriggerTypeWebhook, Config: cfg2, Enabled: true}
	if err := store.CreateTrigger(ctx, t2); err != nil {
		t.Fatal(err)
	}

	triggers, err := store.ListTriggers(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(triggers) != 2 {
		t.Fatalf("expected 2 triggers, got %d", len(triggers))
	}

	// Verify trigger hydration on GetAutomation.
	got, _ := store.GetAutomation(ctx, a.ID)
	if len(got.Triggers) != 2 {
		t.Fatalf("expected 2 hydrated triggers, got %d", len(got.Triggers))
	}
}

func TestRunDeduplication(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "A", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}

	run := &AutomationRun{
		AutomationID: a.ID,
		TriggerID:    "t-1",
		TriggerType:  TriggerTypeScheduled,
		Status:       RunStatusTaskCreated,
		DedupKey:     "scheduled:t-1:12345",
		TriggerData:  json.RawMessage(`{}`),
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}

	exists, err := store.HasRunWithDedupKey(ctx, a.ID, "scheduled:t-1:12345")
	if err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Error("expected dedup key to exist")
	}

	exists, _ = store.HasRunWithDedupKey(ctx, a.ID, "other-key")
	if exists {
		t.Error("expected other key to not exist")
	}
}

func TestListEnabledTriggersByType(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a1 := &Automation{WorkspaceID: "ws-1", Name: "Enabled", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := store.CreateAutomation(ctx, a1); err != nil {
		t.Fatal(err)
	}
	a2 := &Automation{WorkspaceID: "ws-1", Name: "Disabled", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: false}
	if err := store.CreateAutomation(ctx, a2); err != nil {
		t.Fatal(err)
	}

	cfg, _ := json.Marshal(ScheduledTriggerConfig{CronExpression: "@hourly"})
	if err := store.CreateTrigger(ctx, &AutomationTrigger{AutomationID: a1.ID, Type: TriggerTypeScheduled, Config: cfg, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateTrigger(ctx, &AutomationTrigger{AutomationID: a2.ID, Type: TriggerTypeScheduled, Config: cfg, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	triggers, err := store.ListEnabledTriggersByType(ctx, TriggerTypeScheduled)
	if err != nil {
		t.Fatal(err)
	}
	// Only one — from the enabled automation.
	if len(triggers) != 1 {
		t.Fatalf("expected 1 trigger from enabled automation, got %d", len(triggers))
	}
}

func TestUpdateAutomation(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "Original", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}

	newName := "Updated"
	enabled := false
	if err := store.UpdateAutomation(ctx, a.ID, &UpdateAutomationRequest{Name: &newName, Enabled: &enabled}); err != nil {
		t.Fatal(err)
	}

	got, _ := store.GetAutomation(ctx, a.ID)
	if got.Name != "Updated" {
		t.Errorf("expected name 'Updated', got %q", got.Name)
	}
	if got.Enabled {
		t.Error("expected enabled = false")
	}
}

func TestDeleteAutomation(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "To Delete", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	cfg, _ := json.Marshal(ScheduledTriggerConfig{CronExpression: "@hourly"})
	if err := store.CreateTrigger(ctx, &AutomationTrigger{AutomationID: a.ID, Type: TriggerTypeScheduled, Config: cfg, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	if err := store.DeleteAutomation(ctx, a.ID); err != nil {
		t.Fatal(err)
	}

	got, _ := store.GetAutomation(ctx, a.ID)
	if got != nil {
		t.Error("expected nil after delete")
	}
}

func TestDeleteRun(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "A", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	run := &AutomationRun{
		AutomationID: a.ID,
		TriggerType:  TriggerTypeScheduled,
		Status:       RunStatusSkipped,
		TaskID:       "task-abc",
		TriggerData:  json.RawMessage(`{}`),
	}
	if err := store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}

	// GetRun finds it.
	got, err := store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Error("expected run, got nil")
		return
	}
	if got.TaskID != "task-abc" {
		t.Errorf("expected task_id 'task-abc', got %q", got.TaskID)
	}

	// DeleteRun removes it.
	if err := store.DeleteRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Error("expected nil after delete, got run")
	}
}

func TestDeleteAllRuns(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "B", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := store.CreateRun(ctx, &AutomationRun{
			AutomationID: a.ID,
			TriggerType:  TriggerTypeScheduled,
			Status:       RunStatusSkipped,
			TaskID:       "task-" + string(rune('0'+i)),
			TriggerData:  json.RawMessage(`{}`),
		}); err != nil {
			t.Fatal(err)
		}
	}

	taskIDs, err := store.ListRunTaskIDs(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(taskIDs) != 3 {
		t.Fatalf("expected 3 task IDs, got %d", len(taskIDs))
	}

	if err := store.DeleteAllRuns(ctx, a.ID); err != nil {
		t.Fatal(err)
	}

	runs, err := store.ListRuns(ctx, a.ID, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Errorf("expected 0 runs after delete, got %d", len(runs))
	}
}

// createTasksTable adds a minimal shadow of the task repository's `tasks`
// and `task_sessions` tables (id/archived_at, and task_id/is_primary/state
// respectively) to the store's DB. The automation package never owns
// these tables — apps/backend/internal/task/repository/sqlite is the
// canonical owner — so only tests that exercise the CountActiveRuns/
// ListRuns task-state join create them; production always has the real
// tables already migrated by the task repository before automation
// triggers can fire.
func createTasksTable(t *testing.T, store *Store) {
	t.Helper()
	if _, err := store.db.Exec(`CREATE TABLE tasks (id TEXT PRIMARY KEY, archived_at DATETIME)`); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TABLE task_sessions (id TEXT PRIMARY KEY, task_id TEXT NOT NULL, is_primary INTEGER NOT NULL DEFAULT 0, state TEXT NOT NULL DEFAULT 'CREATED')`); err != nil {
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
