package github

import (
	"context"
	"fmt"
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

func TestReviewCleanupInventoryRetainsHistoricalTasks(t *testing.T) {
	_, _, _, store := setupPollerTest(t)
	ctx := context.Background()

	reviewWatch := &ReviewWatch{WorkspaceID: testWorkspaceID, Enabled: true}
	if err := store.CreateReviewWatch(ctx, reviewWatch); err != nil {
		t.Fatalf("CreateReviewWatch: %v", err)
	}
	issueWatch := &IssueWatch{WorkspaceID: testWorkspaceID, Enabled: true}
	if err := store.CreateIssueWatch(ctx, issueWatch); err != nil {
		t.Fatalf("CreateIssueWatch: %v", err)
	}

	seedTask(t, store, "active-review", false)
	seedTask(t, store, "archived-review", true)
	seedTask(t, store, "active-issue", false)
	seedTask(t, store, "archived-issue", true)

	for _, taskID := range []string{"active-review", "archived-review", "missing-review", ""} {
		if err := store.CreateReviewPRTask(ctx, &ReviewPRTask{
			ReviewWatchID: reviewWatch.ID,
			RepoOwner:     "acme",
			RepoName:      "widget",
			PRNumber:      len(taskID) + 1,
			TaskID:        taskID,
		}); err != nil {
			t.Fatalf("CreateReviewPRTask(%q): %v", taskID, err)
		}
	}
	for _, taskID := range []string{"active-issue", "archived-issue", "missing-issue", ""} {
		issueNumber := len(taskID) + 1
		reserved, err := store.ReserveIssueWatchTask(
			ctx, issueWatch.ID, "acme", "widget", issueNumber,
			fmt.Sprintf("https://github.com/acme/widget/issues/%d", issueNumber),
		)
		if err != nil || !reserved {
			t.Fatalf("ReserveIssueWatchTask(%q): reserved=%v err=%v", taskID, reserved, err)
		}
		if taskID != "" {
			if err := store.AssignIssueWatchTaskID(ctx, issueWatch.ID, "acme", "widget", issueNumber, taskID); err != nil {
				t.Fatalf("AssignIssueWatchTaskID(%q): %v", taskID, err)
			}
		}
	}

	assertReviewCandidates := func(name string, candidates []*ReviewPRTask) {
		t.Helper()
		got := make(map[string]bool, len(candidates))
		for _, candidate := range candidates {
			got[candidate.TaskID] = true
		}
		if len(got) != 4 || !got["active-review"] || !got["archived-review"] ||
			!got["missing-review"] || !got[""] {
			t.Errorf("%s task IDs = %#v, want active, archived, missing, and empty reservation rows", name, got)
		}
	}
	assertIssueCandidates := func(name string, candidates []*IssueWatchTask) {
		t.Helper()
		got := make(map[string]bool, len(candidates))
		for _, candidate := range candidates {
			got[candidate.TaskID] = true
		}
		if len(got) != 2 || !got["active-issue"] || !got[""] {
			t.Errorf("%s task IDs = %#v, want active-issue and empty reservation", name, got)
		}
	}

	byWatchReview, err := store.ListReviewPRTasksByWatch(ctx, reviewWatch.ID)
	if err != nil {
		t.Fatalf("ListReviewPRTasksByWatch: %v", err)
	}
	assertReviewCandidates("review by-watch", byWatchReview)
	allReview, err := store.ListAllReviewPRTasks(ctx)
	if err != nil {
		t.Fatalf("ListAllReviewPRTasks: %v", err)
	}
	assertReviewCandidates("review global", allReview)

	byWatchIssue, err := store.ListIssueWatchTasksByWatch(ctx, issueWatch.ID)
	if err != nil {
		t.Fatalf("ListIssueWatchTasksByWatch: %v", err)
	}
	assertIssueCandidates("issue by-watch", byWatchIssue)
	allIssue, err := store.ListAllIssueWatchTasks(ctx)
	if err != nil {
		t.Fatalf("ListAllIssueWatchTasks: %v", err)
	}
	assertIssueCandidates("issue global", allIssue)
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
