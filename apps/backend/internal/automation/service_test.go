package automation

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
)

// fakeTaskDeleter records deletions and can inject errors per task ID.
type fakeTaskDeleter struct {
	deleted []string
	errors  map[string]error
}

func (f *fakeTaskDeleter) DeleteTask(_ context.Context, id string) error {
	f.deleted = append(f.deleted, id)
	if f.errors != nil {
		if err, ok := f.errors[id]; ok {
			return err
		}
	}
	return nil
}

// fakeRepositoryLookup implements RepositoryLookup for tests: repos maps
// repository ID -> workspace ID it belongs to.
type fakeRepositoryLookup struct {
	repos map[string]string
}

func (f *fakeRepositoryLookup) GetRepository(_ context.Context, id string) (string, string, bool) {
	ws, ok := f.repos[id]
	if !ok {
		return "", "", false
	}
	return ws, "main", true
}

func TestCreateAutomation_RepositoryIDsRequireLookup(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	_, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		Name: "x", WorkspaceID: "ws-a", WorkflowID: "wf", WorkflowStepID: "s",
		RepositoryIDs: []string{"repo-1"},
	})
	if !errors.Is(err, ErrRepositoryLookupUnavailable) {
		t.Fatalf("expected ErrRepositoryLookupUnavailable, got %v", err)
	}

	// No repository_ids at all still succeeds without a lookup wired.
	if _, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		Name: "y", WorkspaceID: "ws-a", WorkflowID: "wf", WorkflowStepID: "s",
	}); err != nil {
		t.Fatalf("expected success with no repository_ids, got %v", err)
	}
}

func TestCreateAutomation_RejectsForeignRepositoryID(t *testing.T) {
	svc := newTestService(t)
	svc.SetRepositoryLookup(&fakeRepositoryLookup{repos: map[string]string{
		"repo-a": "ws-a",
		"repo-b": "ws-other",
	}})
	ctx := context.Background()

	_, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		Name: "x", WorkspaceID: "ws-a", WorkflowID: "wf", WorkflowStepID: "s",
		RepositoryIDs: []string{"repo-a", "repo-b"},
	})
	if !errors.Is(err, ErrRepositoryNotInWorkspace) {
		t.Fatalf("expected ErrRepositoryNotInWorkspace, got %v", err)
	}

	_, err = svc.CreateAutomation(ctx, &CreateAutomationRequest{
		Name: "y", WorkspaceID: "ws-a", WorkflowID: "wf", WorkflowStepID: "s",
		RepositoryIDs: []string{"repo-does-not-exist"},
	})
	if !errors.Is(err, ErrRepositoryNotInWorkspace) {
		t.Fatalf("expected ErrRepositoryNotInWorkspace for unknown repo, got %v", err)
	}
}

func TestCreateAutomation_RejectsDuplicateRepositoryID(t *testing.T) {
	svc := newTestService(t)
	svc.SetRepositoryLookup(&fakeRepositoryLookup{repos: map[string]string{"repo-a": "ws-a"}})
	ctx := context.Background()

	_, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		Name: "x", WorkspaceID: "ws-a", WorkflowID: "wf", WorkflowStepID: "s",
		RepositoryIDs: []string{"repo-a", "repo-a"},
	})
	if !errors.Is(err, ErrDuplicateRepositoryID) {
		t.Fatalf("expected ErrDuplicateRepositoryID, got %v", err)
	}
}

func TestCreateAutomation_AcceptsValidRepositoryIDs(t *testing.T) {
	svc := newTestService(t)
	svc.SetRepositoryLookup(&fakeRepositoryLookup{repos: map[string]string{
		"repo-a": "ws-a",
		"repo-b": "ws-a",
	}})
	ctx := context.Background()

	a, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		Name: "x", WorkspaceID: "ws-a", WorkflowID: "wf", WorkflowStepID: "s",
		RepositoryIDs: []string{"repo-a", "repo-b"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(a.RepositoryIDs) != 2 || a.RepositoryIDs[0] != "repo-a" || a.RepositoryIDs[1] != "repo-b" {
		t.Fatalf("expected repository_ids [repo-a repo-b], got %v", a.RepositoryIDs)
	}
	if len(a.Repositories) != 2 || a.Repositories[0].BaseBranch != "main" || a.Repositories[1].BaseBranch != "main" {
		t.Fatalf("expected legacy IDs to resolve repository default branches, got %#v", a.Repositories)
	}
}

func TestCreateAutomation_PreservesExplicitRepositoryBaseBranch(t *testing.T) {
	svc := newTestService(t)
	svc.SetRepositoryLookup(&fakeRepositoryLookup{repos: map[string]string{"repo-a": "ws-a"}})

	a, err := svc.CreateAutomation(context.Background(), &CreateAutomationRequest{
		Name: "x", WorkspaceID: "ws-a",
		Repositories: []AutomationRepository{{RepositoryID: "repo-a", BaseBranch: "release/2"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := a.Repositories[0].BaseBranch; got != "release/2" {
		t.Fatalf("base branch = %q, want release/2", got)
	}
}

func TestUpdateAutomation_RejectsForeignRepositoryID(t *testing.T) {
	svc := newTestService(t)
	svc.SetRepositoryLookup(&fakeRepositoryLookup{repos: map[string]string{
		"repo-a": "ws-a",
		"repo-b": "ws-other",
	}})
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-a", Name: "A", WorkflowID: "wf", WorkflowStepID: "s", Enabled: true}
	if err := svc.store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}

	_, err := svc.UpdateAutomation(ctx, a.ID, &UpdateAutomationRequest{RepositoryIDs: []string{"repo-b"}})
	if !errors.Is(err, ErrRepositoryNotInWorkspace) {
		t.Fatalf("expected ErrRepositoryNotInWorkspace, got %v", err)
	}

	// A valid repo from the automation's own workspace succeeds.
	if _, err := svc.UpdateAutomation(ctx, a.ID, &UpdateAutomationRequest{RepositoryIDs: []string{"repo-a"}}); err != nil {
		t.Fatalf("unexpected error updating with a valid repo: %v", err)
	}

	// Updates that don't touch repository_ids never consult the lookup.
	newName := "Renamed"
	if _, err := svc.UpdateAutomation(ctx, a.ID, &UpdateAutomationRequest{Name: &newName}); err != nil {
		t.Fatalf("unexpected error on unrelated update: %v", err)
	}
}

func TestService_DeleteRun_CallsTaskDeleter(t *testing.T) {
	svc := newTestService(t)
	deleter := &fakeTaskDeleter{}
	svc.SetTaskDeleter(deleter)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "A", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := svc.store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	run := &AutomationRun{
		AutomationID: a.ID,
		TriggerType:  TriggerTypeScheduled,
		Status:       RunStatusTaskCreated,
		TaskID:       "task-xyz",
		TriggerData:  json.RawMessage(`{}`),
	}
	if err := svc.store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}

	if err := svc.DeleteRun(ctx, run.ID); err != nil {
		t.Fatalf("DeleteRun: %v", err)
	}

	// Task deleter must have been called.
	if len(deleter.deleted) != 1 || deleter.deleted[0] != "task-xyz" {
		t.Errorf("expected DeleteTask(task-xyz), got %v", deleter.deleted)
	}
	// Run row must be gone.
	got, _ := svc.store.GetRun(ctx, run.ID)
	if got != nil {
		t.Error("expected run row to be removed")
	}
}

func TestService_DeleteRun_TaskNotFound_StillDeletesRun(t *testing.T) {
	svc := newTestService(t)
	deleter := &fakeTaskDeleter{
		errors: map[string]error{"task-gone": ErrTaskNotFound},
	}
	svc.SetTaskDeleter(deleter)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "B", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := svc.store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	run := &AutomationRun{
		AutomationID: a.ID,
		TriggerType:  TriggerTypeScheduled,
		Status:       RunStatusSkipped,
		TaskID:       "task-gone",
		TriggerData:  json.RawMessage(`{}`),
	}
	if err := svc.store.CreateRun(ctx, run); err != nil {
		t.Fatal(err)
	}

	// Must succeed even though the task is not found.
	if err := svc.DeleteRun(ctx, run.ID); err != nil {
		t.Fatalf("DeleteRun with not-found task: %v", err)
	}

	// Run row must still be gone.
	got, _ := svc.store.GetRun(ctx, run.ID)
	if got != nil {
		t.Error("expected run row to be removed despite task-not-found")
	}
}

func TestService_DeleteRun_PreservesVisibleAutomationTask(t *testing.T) {
	svc := newTestService(t)
	deleter := &fakeTaskDeleter{}
	svc.SetTaskDeleter(deleter)
	svc.SetTaskOriginLookup(&fakeTaskOriginLookup{results: map[string]fakeOriginResult{
		"visible-task": {workspaceID: "ws-1", isAutomationRun: false, ok: true},
	}})
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "visible", Enabled: true}
	require.NoError(t, svc.store.CreateAutomation(ctx, a))
	run := &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusTaskCreated,
		TaskID: "visible-task", TriggerData: json.RawMessage(`{}`),
	}
	require.NoError(t, svc.store.CreateRun(ctx, run))

	require.NoError(t, svc.DeleteRun(ctx, run.ID))
	require.Empty(t, deleter.deleted)
}

func TestService_DeleteAutomation_PreservesVisibleAutomationTasks(t *testing.T) {
	svc := newTestService(t)
	deleter := &fakeTaskDeleter{}
	svc.SetTaskDeleter(deleter)
	svc.SetTaskOriginLookup(&fakeTaskOriginLookup{results: map[string]fakeOriginResult{
		"visible-task": {workspaceID: "ws-1", isAutomationRun: false, ok: true},
	}})
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "visible", Enabled: true}
	require.NoError(t, svc.store.CreateAutomation(ctx, a))
	require.NoError(t, svc.store.CreateRun(ctx, &AutomationRun{
		AutomationID: a.ID, TriggerType: TriggerTypeScheduled, Status: RunStatusSucceeded,
		TaskID: "visible-task", TriggerData: json.RawMessage(`{}`),
	}))

	require.NoError(t, svc.DeleteAutomation(ctx, a.ID))
	require.Empty(t, deleter.deleted)
	jobs, err := svc.store.ListCleanupJobs(ctx)
	require.NoError(t, err)
	require.Empty(t, jobs)
}

func TestService_DeleteAllRuns_CallsTaskDeleterForEach(t *testing.T) {
	svc := newTestService(t)
	deleter := &fakeTaskDeleter{}
	svc.SetTaskDeleter(deleter)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "C", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := svc.store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	taskIDs := []string{"task-1", "task-2", "task-3"}
	for _, tid := range taskIDs {
		if err := svc.store.CreateRun(ctx, &AutomationRun{
			AutomationID: a.ID,
			TriggerType:  TriggerTypeScheduled,
			Status:       RunStatusTaskCreated,
			TaskID:       tid,
			TriggerData:  json.RawMessage(`{}`),
		}); err != nil {
			t.Fatal(err)
		}
	}
	// Also one run with no task_id (fire-and-forget / skipped).
	if err := svc.store.CreateRun(ctx, &AutomationRun{
		AutomationID: a.ID,
		TriggerType:  TriggerTypeScheduled,
		Status:       RunStatusSkipped,
		TriggerData:  json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}

	if err := svc.DeleteAllRuns(ctx, a.ID); err != nil {
		t.Fatalf("DeleteAllRuns: %v", err)
	}

	// All three task IDs must have been passed to DeleteTask.
	if len(deleter.deleted) != 3 {
		t.Errorf("expected 3 task deletions, got %d: %v", len(deleter.deleted), deleter.deleted)
	}
	// All run rows gone.
	runs, _ := svc.store.ListRuns(ctx, a.ID, 50)
	if len(runs) != 0 {
		t.Errorf("expected 0 runs, got %d", len(runs))
	}
}

func TestService_DeleteAllRuns_PreservesVisibleAutomationTasks(t *testing.T) {
	svc := newTestService(t)
	deleter := &fakeTaskDeleter{}
	svc.SetTaskDeleter(deleter)
	svc.SetTaskOriginLookup(&fakeTaskOriginLookup{results: map[string]fakeOriginResult{
		"hidden-task":  {workspaceID: "ws-1", isAutomationRun: true, ok: true},
		"visible-task": {workspaceID: "ws-1", isAutomationRun: false, ok: true},
	}})
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "visible delete-all", Enabled: true}
	require.NoError(t, svc.store.CreateAutomation(ctx, a))
	for _, taskID := range []string{"hidden-task", "visible-task"} {
		require.NoError(t, svc.store.CreateRun(ctx, &AutomationRun{
			AutomationID: a.ID,
			TriggerType:  TriggerTypeScheduled,
			Status:       RunStatusSucceeded,
			TaskID:       taskID,
			TriggerData:  json.RawMessage(`{}`),
		}))
	}

	require.NoError(t, svc.DeleteAllRuns(ctx, a.ID))
	require.Equal(t, []string{"hidden-task"}, deleter.deleted)
}

func TestService_DeleteAllRuns_TaskNotFound_StillClearsRuns(t *testing.T) {
	svc := newTestService(t)
	deleter := &fakeTaskDeleter{
		errors: map[string]error{"task-stale": ErrTaskNotFound},
	}
	svc.SetTaskDeleter(deleter)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "D", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := svc.store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	for _, tid := range []string{"task-stale", "task-ok"} {
		if err := svc.store.CreateRun(ctx, &AutomationRun{
			AutomationID: a.ID,
			TriggerType:  TriggerTypeScheduled,
			Status:       RunStatusTaskCreated,
			TaskID:       tid,
			TriggerData:  json.RawMessage(`{}`),
		}); err != nil {
			t.Fatal(err)
		}
	}

	if err := svc.DeleteAllRuns(ctx, a.ID); err != nil {
		t.Fatalf("DeleteAllRuns with not-found task: %v", err)
	}

	runs, _ := svc.store.ListRuns(ctx, a.ID, 50)
	if len(runs) != 0 {
		t.Errorf("expected 0 runs after delete-all, got %d", len(runs))
	}
}

// TestService_DeleteAllRuns_SerializesAgainstConcurrentRecordRun is a
// regression guard for the orphaned-task race: DeleteAllRuns snapshots task
// IDs via ListRunTaskIDs and then issues a broad DELETE by automation_id. If
// a run were recorded in between, its row would be purged by the broad
// DELETE without its task ever reaching the TaskDeleter. The per-automation
// run lock must make RecordRun block for the full duration of DeleteAllRuns.
func TestService_DeleteAllRuns_SerializesAgainstConcurrentRecordRun(t *testing.T) {
	svc := newTestService(t)
	svc.SetTaskDeleter(&fakeTaskDeleter{})
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-1", Name: "Race", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := svc.store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}
	if err := svc.store.CreateRun(ctx, &AutomationRun{
		AutomationID: a.ID,
		TriggerType:  TriggerTypeScheduled,
		Status:       RunStatusTaskCreated,
		TaskID:       "task-existing",
		TriggerData:  json.RawMessage(`{}`),
	}); err != nil {
		t.Fatal(err)
	}

	// Simulate DeleteAllRuns being mid-flight by holding its lock directly.
	// Register cleanup immediately, before any t.Fatal path, so a failure in
	// the select below (RecordRun completing too early) can't leak the lock
	// for the rest of the test binary's life. sync.Once makes the explicit
	// unlock() call below and this cleanup mutually safe against a double
	// Unlock() panic.
	unlock := svc.automationRunLock(a.ID)
	var unlockOnce sync.Once
	safeUnlock := func() { unlockOnce.Do(unlock) }
	t.Cleanup(safeUnlock)

	done := make(chan error, 1)
	go func() {
		done <- svc.RecordRun(ctx, &AutomationRun{
			AutomationID: a.ID,
			TriggerType:  TriggerTypeScheduled,
			Status:       RunStatusTaskCreated,
			TaskID:       "task-new",
			TriggerData:  json.RawMessage(`{}`),
		})
	}()

	select {
	case <-done:
		t.Fatal("RecordRun completed while the automation run lock was held — DeleteAllRuns is not serialized against run creation")
	case <-time.After(50 * time.Millisecond):
		// Expected: RecordRun is still blocked on the lock.
	}

	safeUnlock()

	if err := <-done; err != nil {
		t.Fatalf("RecordRun: %v", err)
	}

	runs, _ := svc.store.ListRuns(ctx, a.ID, 50)
	if len(runs) != 2 {
		t.Errorf("expected 2 runs (pre-existing + the one recorded after unlock), got %d", len(runs))
	}
}

// TestDeleteAllRuns_AutomationSurvives is a regression guard: deleting all run
// rows — including issuing real DELETE SQL against task rows in the shared
// in-memory DB — must never delete the parent automation row. A real DB-level
// deleter catches SQL trigger / ON DELETE CASCADE regressions. Note: event
// handler side-effects are not covered here (no orchestrator runs in this test).
func TestDeleteAllRuns_AutomationSurvives(t *testing.T) {
	store := setupTestStore(t)
	ctx := context.Background()

	// Create a minimal tasks table in the same in-memory DB so the
	// real-deleter can insert and then DELETE task rows.
	if _, err := store.db.ExecContext(ctx,
		`CREATE TABLE IF NOT EXISTS tasks (id TEXT PRIMARY KEY, title TEXT, state TEXT)`); err != nil {
		t.Fatal("create tasks table:", err)
	}

	// sqliteTaskDeleter deletes from the real tasks table in the same DB —
	// any SQL cascade or trigger that touched automations would fire here.
	realDeleter := &sqliteTaskDeleter{db: store.db}

	log, _ := logger.NewFromZap(zap.NewNop())
	eb := bus.NewMemoryEventBus(log)
	svc := NewService(store, eb, log)
	svc.SetTaskDeleter(realDeleter)

	a := &Automation{WorkspaceID: "ws-1", Name: "Survives", WorkflowID: "wf-1", WorkflowStepID: "s-1", Enabled: true}
	if err := store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}

	// Insert real task rows and create runs referencing them.
	taskIDs := []string{"task-a", "task-b", "task-c"}
	for _, tid := range taskIDs {
		if _, err := store.db.ExecContext(ctx,
			`INSERT INTO tasks (id, title, state) VALUES (?, 'Test task', 'running')`, tid); err != nil {
			t.Fatal("insert task:", err)
		}
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
	// Skipped runs without task IDs.
	for range 3 {
		if err := store.CreateRun(ctx, &AutomationRun{
			AutomationID: a.ID, TriggerType: TriggerTypeScheduled,
			Status: RunStatusSkipped, TriggerData: json.RawMessage(`{}`),
		}); err != nil {
			t.Fatal(err)
		}
	}

	if err := svc.DeleteAllRuns(ctx, a.ID); err != nil {
		t.Fatalf("DeleteAllRuns: %v", err)
	}

	// Automation row must still exist after real task DELETEs fired.
	got, err := store.GetAutomation(ctx, a.ID)
	if err != nil {
		t.Fatalf("GetAutomation after DeleteAllRuns: %v", err)
	}
	if got == nil {
		t.Error("automation was deleted by DeleteAllRuns — regression")
		return
	}
	if got.Name != "Survives" {
		t.Errorf("unexpected automation name %q", got.Name)
	}

	// Runs must be gone.
	runs, _ := store.ListRuns(ctx, a.ID, 50)
	if len(runs) != 0 {
		t.Errorf("expected 0 runs, got %d", len(runs))
	}
	// Task rows should have been deleted.
	if len(realDeleter.deleted) != 3 {
		t.Errorf("expected 3 task deletions, got %d: %v", len(realDeleter.deleted), realDeleter.deleted)
	}
}

// sqliteTaskDeleter deletes from the real tasks table in the same in-memory
// DB, so any SQL trigger or ON DELETE CASCADE that touches automations fires.
type sqliteTaskDeleter struct {
	db      *sqlx.DB
	deleted []string
}

func (d *sqliteTaskDeleter) DeleteTask(ctx context.Context, id string) error {
	d.deleted = append(d.deleted, id)
	_, err := d.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id)
	return err
}

// TestWorkspaceAuthorizerGatesAccess verifies the opt-in-auth workspace
// authorizer is enforced across the automation surface: a caller denied for a
// workspace cannot read, list, mutate, trigger, or reveal secrets of
// automations in it. A nil authorizer (auth disabled / internal) is unscoped.
func TestWorkspaceAuthorizerGatesAccess(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	a := &Automation{WorkspaceID: "ws-a", Name: "A", WorkflowID: "wf", WorkflowStepID: "s", Enabled: true, WebhookSecret: "shh"}
	if err := svc.store.CreateAutomation(ctx, a); err != nil {
		t.Fatal(err)
	}

	denied := errors.New("denied")
	svc.SetWorkspaceAuthorizer(func(_ context.Context, ws string) error {
		if ws == "ws-a" {
			return denied
		}
		return nil
	})

	if _, err := svc.GetAutomation(ctx, a.ID); !errors.Is(err, denied) {
		t.Fatalf("GetAutomation: %v", err)
	}
	if _, err := svc.ListAutomations(ctx, "ws-a"); !errors.Is(err, denied) {
		t.Fatalf("ListAutomations: %v", err)
	}
	if _, err := svc.UpdateAutomation(ctx, a.ID, &UpdateAutomationRequest{}); !errors.Is(err, denied) {
		t.Fatalf("UpdateAutomation: %v", err)
	}
	if err := svc.DeleteAutomation(ctx, a.ID); !errors.Is(err, denied) {
		t.Fatalf("DeleteAutomation: %v", err)
	}
	if _, err := svc.GetWebhookSecret(ctx, a.ID); !errors.Is(err, denied) {
		t.Fatalf("GetWebhookSecret: %v", err)
	}
	if _, err := svc.ListRuns(ctx, a.ID, 10); !errors.Is(err, denied) {
		t.Fatalf("ListRuns: %v", err)
	}
	if _, err := svc.ListWorkspaceRuns(ctx, "ws-a", 10); !errors.Is(err, denied) {
		t.Fatalf("ListWorkspaceRuns: %v", err)
	}
	if _, err := svc.ListAutomationSummaries(ctx, "ws-a"); !errors.Is(err, denied) {
		t.Fatalf("ListAutomationSummaries: %v", err)
	}
	if _, err := svc.GetAutomationSummary(ctx, a.ID); !errors.Is(err, denied) {
		t.Fatalf("GetAutomationSummary: %v", err)
	}
	if _, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{Name: "x", WorkspaceID: "ws-a", WorkflowID: "wf", WorkflowStepID: "s"}); !errors.Is(err, denied) {
		t.Fatalf("CreateAutomation: %v", err)
	}

	// A workspace the caller is allowed for still works.
	if _, err := svc.ListAutomations(ctx, "ws-owned"); err != nil {
		t.Fatalf("allowed workspace list: %v", err)
	}
}

// Workflow and workflow step are optional for every automation: no automation
// run is placed on a board, so no automation needs a starting column. Creation
// used to reject this outright for the (now withdrawn) task execution mode.
func TestCreateAutomation_SucceedsWithoutWorkflowOrStep(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	a, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		WorkspaceID:       "ws-1",
		Name:              "nightly report",
		Prompt:            "summarise yesterday",
		AgentProfileID:    "agent-1",
		ExecutorProfileID: "exec-1",
	})
	if err != nil {
		t.Fatalf("CreateAutomation without workflow: %v", err)
	}
	if a.WorkflowID != "" || a.WorkflowStepID != "" {
		t.Fatalf("expected no workflow placement, got %q/%q", a.WorkflowID, a.WorkflowStepID)
	}
	if !a.Enabled || a.MaxConcurrentRuns != 1 {
		t.Fatalf("expected an enabled automation with the default concurrency, got %+v", a)
	}

	// It must round-trip through the store too — the read path no longer
	// projects the withdrawn execution_mode column.
	got, err := svc.GetAutomation(ctx, a.ID)
	if err != nil {
		t.Fatalf("GetAutomation: %v", err)
	}
	if got == nil || got.Name != "nightly report" {
		t.Fatalf("expected the stored automation back, got %+v", got)
	}
}

// execution_mode is accepted and ignored on input — an old client that still
// sends it must not break, and the value must not come back out.
func TestCreateAutomation_IgnoresExecutionModeOnTheWire(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	var req CreateAutomationRequest
	raw := `{"workspace_id":"ws-1","name":"legacy client","execution_mode":"run"}`
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		t.Fatalf("decode legacy payload: %v", err)
	}
	a, err := svc.CreateAutomation(ctx, &req)
	if err != nil {
		t.Fatalf("CreateAutomation with legacy execution_mode: %v", err)
	}

	encoded, err := json.Marshal(a)
	if err != nil {
		t.Fatalf("marshal automation: %v", err)
	}
	if strings.Contains(string(encoded), "execution_mode") {
		t.Fatalf("execution_mode must be omitted from responses, got %s", string(encoded))
	}
}

// GetAutomation reports a missing row as (nil, nil). A stale id — a bookmarked
// page for a deleted automation, a client retrying after a delete — used to
// dereference that nil and panic the backend on an ordinary not-found.
func TestAuthorizeAutomation_MissingAutomationIsNotFoundNotAPanic(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return nil })

	for name, call := range map[string]func() error{
		"ListRuns":             func() error { _, err := svc.ListRuns(ctx, "gone", 10); return err },
		"GetAutomationSummary": func() error { _, err := svc.GetAutomationSummary(ctx, "gone"); return err },
		"DeleteAllRuns":        func() error { return svc.DeleteAllRuns(ctx, "gone") },
	} {
		t.Run(name, func(t *testing.T) {
			err := call()
			if !errors.Is(err, ErrAutomationNotFound) {
				t.Fatalf("expected ErrAutomationNotFound, got %v", err)
			}
		})
	}
}

// The destructive run operations authorize themselves rather than relying on
// the WS handler to remember: a new caller must not be able to skip the check.
func TestDeleteAllRuns_RefusesAForeignWorkspace(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	a, err := svc.CreateAutomation(ctx, &CreateAutomationRequest{
		Name: "sweep", WorkspaceID: "ws-owned", WorkflowID: "wf", WorkflowStepID: "s",
	})
	if err != nil {
		t.Fatal(err)
	}
	denied := errors.New("denied")
	svc.SetWorkspaceAuthorizer(func(_ context.Context, workspaceID string) error {
		if workspaceID == "ws-owned" {
			return denied
		}
		return nil
	})

	if err := svc.DeleteAllRuns(ctx, a.ID); !errors.Is(err, denied) {
		t.Fatalf("expected the workspace check to refuse, got %v", err)
	}
}
