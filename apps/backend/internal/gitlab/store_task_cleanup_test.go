package gitlab

import (
	"context"
	"testing"
)

func TestCleanupCandidateListsExcludeHistoricalTasks(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	for _, workspaceID := range []string{"ws-1", "ws-2"} {
		seedWorkspace(t, store, workspaceID)
	}

	reviewWatch := &ReviewWatch{ID: "review-cleanup", WorkspaceID: "ws-1"}
	if err := store.CreateReviewWatch(ctx, reviewWatch); err != nil {
		t.Fatalf("create review watch: %v", err)
	}
	otherReviewWatch := &ReviewWatch{ID: "review-other", WorkspaceID: "ws-2"}
	if err := store.CreateReviewWatch(ctx, otherReviewWatch); err != nil {
		t.Fatalf("create other review watch: %v", err)
	}
	issueWatch := &IssueWatch{ID: "issue-cleanup", WorkspaceID: "ws-1"}
	if err := store.CreateIssueWatch(ctx, issueWatch); err != nil {
		t.Fatalf("create issue watch: %v", err)
	}
	otherIssueWatch := &IssueWatch{ID: "issue-other", WorkspaceID: "ws-2"}
	if err := store.CreateIssueWatch(ctx, otherIssueWatch); err != nil {
		t.Fatalf("create other issue watch: %v", err)
	}

	for _, taskID := range []string{"review-active", "review-archived", "issue-active", "issue-archived", "other-review-active", "other-issue-active"} {
		workspaceID := "ws-1"
		if taskID == "other-review-active" || taskID == "other-issue-active" {
			workspaceID = "ws-2"
		}
		seedTask(t, store, taskID, workspaceID)
	}
	for _, taskID := range []string{"review-archived", "issue-archived"} {
		if _, err := store.db.Exec(`UPDATE tasks SET archived_at = CURRENT_TIMESTAMP WHERE id = ?`, taskID); err != nil {
			t.Fatalf("archive task %s: %v", taskID, err)
		}
	}

	reserveReview := func(watchID, project string, iid int, taskID string) {
		t.Helper()
		reserved, err := store.ReserveReviewMRTask(ctx, watchID, project, iid, "https://gitlab.example/mr")
		if err != nil || !reserved {
			t.Fatalf("reserve review %s/%d: reserved=%v err=%v", project, iid, reserved, err)
		}
		if taskID != "" {
			if err := store.AssignReviewMRTaskID(ctx, watchID, project, iid, taskID); err != nil {
				t.Fatalf("assign review %s/%d: %v", project, iid, err)
			}
		}
	}
	reserveIssue := func(watchID, project string, iid int, taskID string) {
		t.Helper()
		reserved, err := store.ReserveIssueWatchTask(ctx, watchID, project, iid, "https://gitlab.example/issue")
		if err != nil || !reserved {
			t.Fatalf("reserve issue %s/%d: reserved=%v err=%v", project, iid, reserved, err)
		}
		if taskID != "" {
			if err := store.AssignIssueWatchTaskID(ctx, watchID, project, iid, taskID); err != nil {
				t.Fatalf("assign issue %s/%d: %v", project, iid, err)
			}
		}
	}
	reserveReview(reviewWatch.ID, "team/review", 1, "review-active")
	reserveReview(reviewWatch.ID, "team/review", 2, "review-archived")
	reserveReview(reviewWatch.ID, "team/review", 3, "review-missing")
	reserveReview(reviewWatch.ID, "team/review", 4, "")
	reserveReview(otherReviewWatch.ID, "team/other-review", 5, "other-review-active")
	reserveIssue(issueWatch.ID, "team/issue", 1, "issue-active")
	reserveIssue(issueWatch.ID, "team/issue", 2, "issue-archived")
	reserveIssue(issueWatch.ID, "team/issue", 3, "issue-missing")
	reserveIssue(issueWatch.ID, "team/issue", 4, "")
	reserveIssue(otherIssueWatch.ID, "team/other-issue", 5, "other-issue-active")

	assertReviewTasks := func(name string, rows []*ReviewMRTask, want map[string]bool) {
		t.Helper()
		got := make(map[string]bool, len(rows))
		for _, row := range rows {
			got[row.TaskID] = true
		}
		if len(rows) != len(want) {
			t.Fatalf("%s rows = %d (%v), want %d (%v)", name, len(rows), got, len(want), want)
		}
		for taskID := range want {
			if !got[taskID] {
				t.Errorf("%s missing task %q in %v", name, taskID, got)
			}
		}
	}
	assertIssueTasks := func(name string, rows []*IssueWatchTask, want map[string]bool) {
		t.Helper()
		got := make(map[string]bool, len(rows))
		for _, row := range rows {
			got[row.TaskID] = true
		}
		if len(rows) != len(want) {
			t.Fatalf("%s rows = %d (%v), want %d (%v)", name, len(rows), got, len(want), want)
		}
		for taskID := range want {
			if !got[taskID] {
				t.Errorf("%s missing task %q in %v", name, taskID, got)
			}
		}
	}

	reviewByWatch, err := store.ListReviewMRTasksByWatch(ctx, reviewWatch.ID)
	if err != nil {
		t.Fatalf("list review rows by watch: %v", err)
	}
	reviewWorkspace, err := store.ListReviewMRTasksForWorkspace(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list review rows for workspace: %v", err)
	}
	reviewAll, err := store.ListAllReviewMRTasks(ctx)
	if err != nil {
		t.Fatalf("list all review rows: %v", err)
	}
	issueByWatch, err := store.ListIssueWatchTasksByWatch(ctx, issueWatch.ID)
	if err != nil {
		t.Fatalf("list issue rows by watch: %v", err)
	}
	issueWorkspace, err := store.ListIssueWatchTasksForWorkspace(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list issue rows for workspace: %v", err)
	}
	issueAll, err := store.ListAllIssueWatchTasks(ctx)
	if err != nil {
		t.Fatalf("list all issue rows: %v", err)
	}
	assertReviewTasks("review by watch", reviewByWatch, map[string]bool{"review-active": true, "": true})
	assertReviewTasks("review workspace", reviewWorkspace, map[string]bool{"review-active": true, "": true})
	assertReviewTasks("review all", reviewAll, map[string]bool{"review-active": true, "": true, "other-review-active": true})
	assertIssueTasks("issue by watch", issueByWatch, map[string]bool{"issue-active": true, "": true})
	assertIssueTasks("issue workspace", issueWorkspace, map[string]bool{"issue-active": true, "": true})
	assertIssueTasks("issue all", issueAll, map[string]bool{"issue-active": true, "": true, "other-issue-active": true})
}

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

// TestHealTaskOwnedOrphansOnStoreInit covers AC5/AC6: store
// initialization deletes gitlab_task_mrs/gitlab_mr_watches rows whose task_id
// has no matching tasks.id, preserves rows for active and archived tasks, and
// a repeat initialization is a no-op.
func TestHealTaskOwnedOrphansOnStoreInit(t *testing.T) {
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
		if taskID != "orphan-task" {
			seedGitLabTaskOwnedAutomationState(t, store, taskID)
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
		for _, table := range testGitLabTaskOwnedTables {
			if got := countGitLabTaskOwnedRows(t, reopened, table, taskID); got != 1 {
				t.Fatalf("%s rows for %s = %d, want 1", table, taskID, got)
			}
		}
	}
	if got, err := reopened.ListTaskMRsByTask(ctx, "orphan-task"); err != nil || len(got) != 0 {
		t.Fatalf("orphan MRs = %d, %v; want 0", len(got), err)
	}
	if got, _ := reopened.GetMRWatchBySession(ctx, "session-orphan-task"); got != nil {
		t.Fatal("orphan watch was not removed")
	}
	for _, table := range testGitLabTaskOwnedTables {
		if got := countGitLabTaskOwnedRows(t, reopened, table, "orphan-task"); got != 0 {
			t.Fatalf("orphan %s rows = %d, want 0", table, got)
		}
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
