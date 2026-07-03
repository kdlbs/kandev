package automation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
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
	unlock := svc.automationRunLock(a.ID)

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

	unlock()

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
