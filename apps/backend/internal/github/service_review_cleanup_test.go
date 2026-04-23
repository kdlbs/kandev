package github

import (
	"context"
	"fmt"
	"testing"
)

// stubTaskDeleter implements TaskDeleter for testing.
type stubTaskDeleter struct {
	err error
}

func (s *stubTaskDeleter) DeleteTask(_ context.Context, _ string) error {
	return s.err
}

// TestCleanupMergedReviewTasks_TaskAlreadyDeleted verifies that when DeleteTask
// returns "not found" the orphaned dedup record is still removed, preventing the
// 5-minute poller from logging the same warning indefinitely.
func TestCleanupMergedReviewTasks_TaskAlreadyDeleted(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	// Create a review watch.
	watch := &ReviewWatch{WorkspaceID: "ws-1", Enabled: true}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("CreateReviewWatch: %v", err)
	}

	// Create a dedup record pointing to an already-deleted task.
	taskID := "task-already-gone"
	rpt := &ReviewPRTask{
		ReviewWatchID: watch.ID,
		RepoOwner:     "acme",
		RepoName:      "widget",
		PRNumber:      42,
		PRURL:         "https://github.com/acme/widget/pull/42",
		TaskID:        taskID,
	}
	if err := store.CreateReviewPRTask(ctx, rpt); err != nil {
		t.Fatalf("CreateReviewPRTask: %v", err)
	}

	// Mock: PR is merged so shouldDeleteReviewTask returns true.
	mockClient.AddPR(&PR{
		Number:    42,
		State:     prStateMerged,
		RepoOwner: "acme",
		RepoName:  "widget",
	})

	// Stub: DeleteTask returns "not found" as the real service does.
	svc.SetTaskDeleter(&stubTaskDeleter{
		err: fmt.Errorf("task not found: %s", taskID),
	})

	deleted, err := svc.CleanupMergedReviewTasks(ctx, watch)
	if err != nil {
		t.Fatalf("CleanupMergedReviewTasks returned error: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	// The orphaned dedup record must be gone.
	remaining, err := store.ListReviewPRTasksByWatch(ctx, watch.ID)
	if err != nil {
		t.Fatalf("ListReviewPRTasksByWatch: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("expected 0 remaining dedup records, got %d", len(remaining))
	}
}

// Regression: when a task already has a TaskPR row pointing to an old PR
// (e.g. the first PR was closed and a second one opened on the same or a new
// branch), AssociatePRWithTask must replace the stale row so downstream
// consumers (UI, GetTaskPR) observe the current PR rather than the old one.
func TestAssociatePRWithTask_ReplacesStaleAssociation(t *testing.T) {
	svc, store, _ := setupSyncTest(t)
	ctx := context.Background()

	// Seed an existing association for PR #1.
	if err := store.CreateTaskPR(ctx, &TaskPR{
		TaskID:     "t1",
		Owner:      "owner",
		Repo:       "repo",
		PRNumber:   1,
		PRURL:      "https://github.com/owner/repo/pull/1",
		PRTitle:    "First",
		HeadBranch: "feat-a",
		BaseBranch: "main",
		State:      "closed",
	}); err != nil {
		t.Fatalf("seed TaskPR: %v", err)
	}

	// Associate a new PR #2 (could be on same or different branch).
	newPR := &PR{
		Number:      2,
		Title:       "Second",
		HTMLURL:     "https://github.com/owner/repo/pull/2",
		HeadBranch:  "feat-b",
		BaseBranch:  "main",
		State:       "open",
		RepoOwner:   "owner",
		RepoName:    "repo",
		AuthorLogin: "alice",
	}
	tp, err := svc.AssociatePRWithTask(ctx, "t1", newPR)
	if err != nil {
		t.Fatalf("AssociatePRWithTask: %v", err)
	}
	if tp.PRNumber != 2 {
		t.Errorf("returned TaskPR.PRNumber=%d, want 2", tp.PRNumber)
	}

	got, err := store.GetTaskPR(ctx, "t1")
	if err != nil {
		t.Fatalf("GetTaskPR: %v", err)
	}
	if got == nil || got.PRNumber != 2 {
		t.Errorf("GetTaskPR after replace = %+v, want PRNumber=2", got)
	}
}
