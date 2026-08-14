package gitlab

import (
	"context"
	"testing"
)

func TestDeleteTaskMRsByTaskIDRemovesOnlyTargetRows(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "ws")
	for _, taskID := range []string{"delete-me", "keep-me"} {
		seedTask(t, store, taskID, "ws")
		if err := store.UpsertTaskMR(ctx, newTestMR(taskID, "", "group/project", 1)); err != nil {
			t.Fatalf("create task MR: %v", err)
		}
		if err := store.CreateMRWatch(ctx, &MRWatch{SessionID: "session-" + taskID, TaskID: taskID, ProjectPath: "group/project", Branch: "main"}); err != nil {
			t.Fatalf("create MR watch: %v", err)
		}
	}

	if _, err := store.DeleteMRWatchesByTaskID(ctx, "delete-me"); err != nil {
		t.Fatalf("delete watches: %v", err)
	}
	if _, err := store.DeleteTaskMRsByTaskID(ctx, "delete-me"); err != nil {
		t.Fatalf("delete task MRs: %v", err)
	}

	if got, err := store.ListTaskMRsByTask(ctx, "delete-me"); err != nil || len(got) != 0 {
		t.Fatalf("target MRs = %d, %v; want 0", len(got), err)
	}
	if got, err := store.ListTaskMRsByTask(ctx, "keep-me"); err != nil || len(got) != 1 {
		t.Fatalf("other MRs = %d, %v; want 1", len(got), err)
	}
	if got, _ := store.GetMRWatchBySession(ctx, "session-keep-me"); got == nil {
		t.Fatal("unrelated watch was removed")
	}
}

// TestHealTaskContributionOrphansOnStoreInit covers AC5/AC6: store
// initialization deletes gitlab_task_mrs/gitlab_mr_watches rows whose task_id
// has no matching tasks.id, preserves rows for active and archived tasks, and
// a repeat initialization is a no-op.
func TestHealTaskContributionOrphansOnStoreInit(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	seedWorkspace(t, store, "ws")

	seedTask(t, store, "active-task", "ws")
	seedTask(t, store, "archived-task", "ws")
	if _, err := store.db.Exec(`UPDATE tasks SET archived_at = CURRENT_TIMESTAMP WHERE id = 'archived-task'`); err != nil {
		t.Fatalf("archive task: %v", err)
	}
	// "orphan-task" is deliberately never inserted into tasks.

	for i, taskID := range []string{"active-task", "archived-task", "orphan-task"} {
		if err := store.UpsertTaskMR(ctx, newTestMR(taskID, "", "group/project", i+1)); err != nil {
			t.Fatalf("create task MR for %s: %v", taskID, err)
		}
		if err := store.CreateMRWatch(ctx, &MRWatch{SessionID: "session-" + taskID, TaskID: taskID, ProjectPath: "group/project", Branch: "main"}); err != nil {
			t.Fatalf("create MR watch for %s: %v", taskID, err)
		}
	}

	reopened, err := NewStore(store.db, store.ro)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	for _, taskID := range []string{"active-task", "archived-task"} {
		if got, err := reopened.ListTaskMRsByTask(ctx, taskID); err != nil || len(got) != 1 {
			t.Fatalf("MRs for %s = %d, %v; want 1", taskID, len(got), err)
		}
		if got, _ := reopened.GetMRWatchBySession(ctx, "session-"+taskID); got == nil {
			t.Fatalf("watch for %s was removed, want preserved", taskID)
		}
	}
	if got, err := reopened.ListTaskMRsByTask(ctx, "orphan-task"); err != nil || len(got) != 0 {
		t.Fatalf("orphan MRs = %d, %v; want 0", len(got), err)
	}
	if got, _ := reopened.GetMRWatchBySession(ctx, "session-orphan-task"); got != nil {
		t.Fatal("orphan watch was not removed")
	}

	if _, err := NewStore(reopened.db, reopened.ro); err != nil {
		t.Fatalf("replay store init: %v", err)
	}
	for _, taskID := range []string{"active-task", "archived-task"} {
		if got, err := reopened.ListTaskMRsByTask(ctx, taskID); err != nil || len(got) != 1 {
			t.Fatalf("MRs for %s after replay = %d, %v; want 1", taskID, len(got), err)
		}
	}
}
