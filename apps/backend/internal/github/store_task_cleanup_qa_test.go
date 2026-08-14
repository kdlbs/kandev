package github

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// countTaskPRRows counts github_task_prs rows directly, bypassing the
// task-scoped read paths so a test can observe rows for task IDs those paths
// would refuse (empty string, injection payloads).
func countTaskPRRows(t *testing.T, store *Store) int {
	t.Helper()
	var n int
	if err := store.db.Get(&n, `SELECT COUNT(*) FROM github_task_prs`); err != nil {
		t.Fatalf("count github_task_prs: %v", err)
	}
	return n
}

// TestDeleteTaskPRsByTaskIDIsParameterized proves the task-scoped delete binds
// its task ID rather than interpolating it. A task_id carrying SQL
// metacharacters must delete exactly its own row and leave the table intact.
func TestDeleteTaskPRsByTaskIDIsParameterized(t *testing.T) {
	_, _, _, store := setupPollerTest(t)
	ctx := context.Background()

	const injection = `x'; DROP TABLE github_task_prs; --`
	for i, taskID := range []string{injection, "innocent"} {
		seedTask(t, store, taskID, false)
		if err := store.CreateTaskPR(ctx, &TaskPR{
			TaskID: taskID, WorkspaceID: testWorkspaceID, Owner: "owner", Repo: "repo", PRNumber: i + 1,
		}); err != nil {
			t.Fatalf("create task PR for %q: %v", taskID, err)
		}
	}

	n, err := store.DeleteTaskPRsByTaskID(ctx, injection)
	if err != nil {
		t.Fatalf("delete by injection task id: %v", err)
	}
	if n != 1 {
		t.Fatalf("deleted = %d, want 1", n)
	}
	// The table must still exist and still hold the unrelated row.
	if got := countTaskPRRows(t, store); got != 1 {
		t.Fatalf("remaining rows = %d, want 1 (table intact, innocent row kept)", got)
	}
	if got, _ := store.ListTaskPRsByTaskIncludingDetached(ctx, "innocent"); len(got) != 1 {
		t.Fatalf("innocent task PRs = %d, want 1", len(got))
	}
}

// TestHandleTaskDeletedConcurrentDeliveryIsIdempotent fires the same deletion
// event from many goroutines at once, the shape a redelivered or duplicated
// bus message takes. It exercises credential revocation, watch pruning and
// task-PR pruning together, and under -race guards the service state they share.
func TestHandleTaskDeletedConcurrentDeliveryIsIdempotent(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()

	for _, taskID := range []string{"doomed", "bystander"} {
		seedTask(t, store, taskID, false)
		if err := store.CreateTaskPR(ctx, &TaskPR{
			TaskID: taskID, WorkspaceID: testWorkspaceID, Owner: "owner", Repo: "repo", PRNumber: 1,
		}); err != nil {
			t.Fatalf("create task PR for %s: %v", taskID, err)
		}
		mustCreateWatch(t, store, "session-"+taskID, taskID)
	}

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

	if got, _ := store.ListTaskPRsByTaskIncludingDetached(ctx, "doomed"); len(got) != 0 {
		t.Fatalf("doomed task PRs = %d, want 0", len(got))
	}
	if got, _ := store.ListTaskPRsByTaskIncludingDetached(ctx, "bystander"); len(got) != 1 {
		t.Fatalf("bystander task PRs = %d, want 1", len(got))
	}
	if got, _ := store.GetPRWatchBySession(ctx, "session-doomed"); got != nil {
		t.Fatal("doomed watch survived the delete")
	}
	if got, _ := store.GetPRWatchBySession(ctx, "session-bystander"); got == nil {
		t.Fatal("bystander watch was removed by a concurrent delete of another task")
	}
}

// TestHealTaskContributionOrphansHandlesUnusualTaskIDs pins how the startup
// sweep treats rows whose task_id can never match a tasks row: the empty
// string, and a very long unicode identifier. Both are ownerless by
// definition, so both must be swept, while a real task's rows survive.
func TestHealTaskContributionOrphansHandlesUnusualTaskIDs(t *testing.T) {
	_, _, _, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "real-task", false)

	longUnicode := strings.Repeat("🛠️任務", 200)
	for i, taskID := range []string{"real-task", "", longUnicode} {
		if err := store.CreateTaskPR(ctx, &TaskPR{
			TaskID: taskID, WorkspaceID: testWorkspaceID, Owner: "owner", Repo: "repo", PRNumber: i + 1,
		}); err != nil {
			t.Fatalf("create task PR %d: %v", i, err)
		}
	}
	if got := countTaskPRRows(t, store); got != 3 {
		t.Fatalf("seeded rows = %d, want 3", got)
	}

	if _, err := NewStore(store.db, store.ro); err != nil {
		t.Fatalf("reopen store: %v", err)
	}

	if got := countTaskPRRows(t, store); got != 1 {
		t.Fatalf("rows after sweep = %d, want 1 (only the owned row survives)", got)
	}
	if got, _ := store.ListTaskPRsByTaskIncludingDetached(ctx, "real-task"); len(got) != 1 {
		t.Fatalf("owned task PRs = %d, want 1", len(got))
	}
}

// TestHandleTaskDeletedRemovesDetachedTaskPRTombstones covers a row shape the
// happy-path tests miss: a PR the user explicitly unlinked leaves a detached
// tombstone behind. Hard-deleting the owning task must remove the tombstone
// too, otherwise the "permanent delete" contract leaks rows no surface reads.
func TestHandleTaskDeletedRemovesDetachedTaskPRTombstones(t *testing.T) {
	_, svc, _, store := setupPollerTest(t)
	ctx := context.Background()
	seedTask(t, store, "doomed", false)

	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID: "doomed", WorkspaceID: testWorkspaceID, Owner: "owner", Repo: "repo", PRNumber: 1,
	}); err != nil {
		t.Fatalf("create task PR: %v", err)
	}
	if _, err := store.db.Exec(
		`UPDATE github_task_prs SET detached_at = CURRENT_TIMESTAMP WHERE task_id = 'doomed'`,
	); err != nil {
		t.Fatalf("detach task PR: %v", err)
	}
	if got, _ := store.ListTaskPRsByTaskIncludingDetached(ctx, "doomed"); len(got) != 1 {
		t.Fatalf("detached tombstone missing before delete, got %d rows", len(got))
	}

	event := bus.NewEvent(events.TaskDeleted, "task-service", map[string]interface{}{"task_id": "doomed"})
	if err := svc.handleTaskDeleted(ctx, event); err != nil {
		t.Fatalf("handle task deleted: %v", err)
	}

	if got, _ := store.ListTaskPRsByTaskIncludingDetached(ctx, "doomed"); len(got) != 0 {
		t.Fatalf("detached tombstone survived task deletion, got %d rows", len(got))
	}
	if got := countTaskPRRows(t, store); got != 0 {
		t.Fatalf("github_task_prs rows = %d, want 0", got)
	}
}
