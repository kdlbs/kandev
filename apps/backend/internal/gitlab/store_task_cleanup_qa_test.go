package gitlab

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// countTaskMRRows counts gitlab_task_mrs rows directly, bypassing
// ListTaskMRsByTask so a test can observe rows for task IDs the normal read
// path would refuse (empty string, injection payloads).
func countTaskMRRows(t *testing.T, store *Store) int {
	t.Helper()
	var n int
	if err := store.db.Get(&n, `SELECT COUNT(*) FROM gitlab_task_mrs`); err != nil {
		t.Fatalf("count gitlab_task_mrs: %v", err)
	}
	return n
}

// TestDeleteTaskMRsByTaskIDIsParameterized proves the task-scoped delete binds
// its task ID rather than interpolating it. A task_id carrying SQL
// metacharacters must delete exactly its own row and leave the table intact.
func TestDeleteTaskMRsByTaskIDIsParameterized(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "ws")

	const injection = `x'; DROP TABLE gitlab_task_mrs; --`
	seedTask(t, store, injection, "ws")
	seedTask(t, store, "innocent", "ws")
	if err := store.UpsertTaskMR(ctx, newTestMR(injection, "", "group/project", 1)); err != nil {
		t.Fatalf("create injection task MR: %v", err)
	}
	if err := store.UpsertTaskMR(ctx, newTestMR("innocent", "", "group/project", 2)); err != nil {
		t.Fatalf("create innocent task MR: %v", err)
	}

	n, err := store.DeleteTaskMRsByTaskID(ctx, injection)
	if err != nil {
		t.Fatalf("delete by injection task id: %v", err)
	}
	if n != 1 {
		t.Fatalf("deleted = %d, want 1", n)
	}
	// The table must still exist and still hold the unrelated row.
	if got := countTaskMRRows(t, store); got != 1 {
		t.Fatalf("remaining rows = %d, want 1 (table intact, innocent row kept)", got)
	}
	if got, _ := store.ListTaskMRsByTask(ctx, "innocent"); len(got) != 1 {
		t.Fatalf("innocent task MRs = %d, want 1", len(got))
	}
}

// TestHandleTaskDeletedSurvivesStoreFailure covers the dependency-failure path:
// when the underlying database is unusable the handler must log and return nil
// rather than panicking or propagating an error that would poison the shared
// event bus for other subscribers.
func TestHandleTaskDeletedSurvivesStoreFailure(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "ws")
	seedTask(t, store, "t1", "ws")
	if err := store.UpsertTaskMR(ctx, newTestMR("t1", "", "group/project", 1)); err != nil {
		t.Fatalf("create task MR: %v", err)
	}

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	svc := NewService(DefaultHost, NewMockClient(DefaultHost), "", nil, log)
	svc.SetStore(store)

	// Simulate the dependency failing underneath a live subscription.
	if err := store.db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	event := bus.NewEvent(events.TaskDeleted, "task-service", map[string]interface{}{"task_id": "t1"})
	if err := svc.handleTaskDeleted(ctx, event); err != nil {
		t.Fatalf("handler must swallow store failures, got: %v", err)
	}
}

// TestHandleTaskDeletedConcurrentDeliveryIsIdempotent fires the same deletion
// event from many goroutines at once, the shape a redelivered or duplicated
// bus message takes. Run under -race it also guards the store/service locking
// around s.store.
func TestHandleTaskDeletedConcurrentDeliveryIsIdempotent(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "ws")
	seedTask(t, store, "doomed", "ws")
	seedTask(t, store, "bystander", "ws")
	for _, taskID := range []string{"doomed", "bystander"} {
		if err := store.UpsertTaskMR(ctx, newTestMR(taskID, "", "group/project", 1)); err != nil {
			t.Fatalf("create task MR for %s: %v", taskID, err)
		}
		if err := store.CreateMRWatch(ctx, &MRWatch{
			SessionID: "session-" + taskID, TaskID: taskID, ProjectPath: "group/project", Branch: "main",
		}); err != nil {
			t.Fatalf("create MR watch for %s: %v", taskID, err)
		}
	}

	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	svc := NewService(DefaultHost, NewMockClient(DefaultHost), "", nil, log)
	svc.SetStore(store)

	event := bus.NewEvent(events.TaskDeleted, "task-service", map[string]interface{}{"task_id": "doomed"})
	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := svc.handleTaskDeleted(ctx, event); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent delivery returned an error: %v", err)
	}

	if got, _ := store.ListTaskMRsByTask(ctx, "doomed"); len(got) != 0 {
		t.Fatalf("doomed task MRs = %d, want 0", len(got))
	}
	if got, _ := store.ListTaskMRsByTask(ctx, "bystander"); len(got) != 1 {
		t.Fatalf("bystander task MRs = %d, want 1", len(got))
	}
	if got, _ := store.GetMRWatchBySession(ctx, "session-doomed"); got != nil {
		t.Fatal("doomed watch survived the delete")
	}
	if got, _ := store.GetMRWatchBySession(ctx, "session-bystander"); got == nil {
		t.Fatal("bystander watch was removed by a concurrent delete of another task")
	}
}

// TestHealTaskContributionOrphansHandlesUnusualTaskIDs pins how the startup
// sweep treats rows whose task_id can never match a tasks row: the empty
// string, and a very long unicode identifier. Both are ownerless by
// definition, so both must be swept, while a real task's rows survive.
func TestHealTaskContributionOrphansHandlesUnusualTaskIDs(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "ws")
	seedTask(t, store, "real-task", "ws")

	longUnicode := strings.Repeat("🛠️任務", 200)
	for i, taskID := range []string{"real-task", "", longUnicode} {
		if err := store.UpsertTaskMR(ctx, newTestMR(taskID, "", "group/project", i+1)); err != nil {
			t.Fatalf("create task MR %d: %v", i, err)
		}
	}
	if got := countTaskMRRows(t, store); got != 3 {
		t.Fatalf("seeded rows = %d, want 3", got)
	}

	if _, err := NewStore(store.db, store.ro); err != nil {
		t.Fatalf("reopen store: %v", err)
	}

	if got := countTaskMRRows(t, store); got != 1 {
		t.Fatalf("rows after sweep = %d, want 1 (only the owned row survives)", got)
	}
	if got, _ := store.ListTaskMRsByTask(ctx, "real-task"); len(got) != 1 {
		t.Fatalf("owned task MRs = %d, want 1", len(got))
	}
}
