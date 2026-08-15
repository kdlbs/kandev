package github

import (
	"context"
	"testing"
)

func TestDeleteTaskPRsByTaskIDRemovesOnlyTargetRows(t *testing.T) {
	_, _, _, store := setupPollerTest(t)
	ctx := context.Background()
	for _, taskID := range []string{"delete-me", "keep-me"} {
		seedTask(t, store, taskID, false)
		if err := store.CreateTaskPR(ctx, &TaskPR{TaskID: taskID, WorkspaceID: testWorkspaceID, Owner: "owner", Repo: "repo", PRNumber: 1}); err != nil {
			t.Fatalf("create task PR: %v", err)
		}
		mustCreateWatch(t, store, "session-"+taskID, taskID)
	}

	if _, err := store.DeletePRWatchesByTaskID(ctx, "delete-me"); err != nil {
		t.Fatalf("delete watches: %v", err)
	}
	if _, err := store.DeleteTaskPRsByTaskID(ctx, "delete-me"); err != nil {
		t.Fatalf("delete task PRs: %v", err)
	}

	if got, err := store.ListTaskPRsByTaskIncludingDetached(ctx, "delete-me"); err != nil || len(got) != 0 {
		t.Fatalf("target PRs = %d, %v; want 0", len(got), err)
	}
	if got, err := store.ListTaskPRsByTaskIncludingDetached(ctx, "keep-me"); err != nil || len(got) != 1 {
		t.Fatalf("other PRs = %d, %v; want 1", len(got), err)
	}
	if got, _ := store.GetPRWatchBySession(ctx, "session-keep-me"); got == nil {
		t.Fatal("unrelated watch was removed")
	}
}

// TestHealTaskOwnedOrphansOnStoreInit covers AC5/AC6: store
// initialization deletes github_task_prs/github_pr_watches rows whose task_id
// has no matching tasks.id, preserves rows for active and archived tasks, and
// a repeat initialization is a no-op.
func TestHealTaskOwnedOrphansOnStoreInit(t *testing.T) {
	_, _, _, store := setupPollerTest(t)
	ctx := context.Background()

	seedTask(t, store, "active-task", false)
	seedTask(t, store, "archived-task", true)
	// "orphan-task" is deliberately never inserted into tasks.

	for _, taskID := range []string{"active-task", "archived-task", "orphan-task"} {
		if err := store.CreateTaskPR(ctx, &TaskPR{TaskID: taskID, WorkspaceID: testWorkspaceID, Owner: "owner", Repo: "repo", PRNumber: 1}); err != nil {
			t.Fatalf("create task PR for %s: %v", taskID, err)
		}
		mustCreateWatch(t, store, "session-"+taskID, taskID)
		seedGitHubTaskOwnedAutomationState(t, store, taskID)
	}

	reopened, err := NewStore(store.db, store.ro)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	for _, taskID := range []string{"active-task", "archived-task"} {
		if got, err := reopened.ListTaskPRsByTaskIncludingDetached(ctx, taskID); err != nil || len(got) != 1 {
			t.Fatalf("PRs for %s = %d, %v; want 1", taskID, len(got), err)
		}
		if got, _ := reopened.GetPRWatchBySession(ctx, "session-"+taskID); got == nil {
			t.Fatalf("watch for %s was removed, want preserved", taskID)
		}
		for _, table := range testGitHubTaskOwnedTables {
			if got := countGitHubTaskOwnedRows(t, reopened, table, taskID); got != 1 {
				t.Fatalf("%s rows for %s = %d, want 1", table, taskID, got)
			}
		}
	}
	if got, err := reopened.ListTaskPRsByTaskIncludingDetached(ctx, "orphan-task"); err != nil || len(got) != 0 {
		t.Fatalf("orphan PRs = %d, %v; want 0", len(got), err)
	}
	if got, _ := reopened.GetPRWatchBySession(ctx, "session-orphan-task"); got != nil {
		t.Fatal("orphan watch was not removed")
	}
	for _, table := range testGitHubTaskOwnedTables {
		if got := countGitHubTaskOwnedRows(t, reopened, table, "orphan-task"); got != 0 {
			t.Fatalf("orphan %s rows = %d, want 0", table, got)
		}
	}

	if _, err := NewStore(reopened.db, reopened.ro); err != nil {
		t.Fatalf("replay store init: %v", err)
	}
	for _, taskID := range []string{"active-task", "archived-task"} {
		if got, err := reopened.ListTaskPRsByTaskIncludingDetached(ctx, taskID); err != nil || len(got) != 1 {
			t.Fatalf("PRs for %s after replay = %d, %v; want 1", taskID, len(got), err)
		}
	}
}
