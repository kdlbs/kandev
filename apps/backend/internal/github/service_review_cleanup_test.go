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

	// Stub: DeleteTask returns the sentinel-wrapped not-found error as the
	// real adapter (see cmd/kandev/turn_adapters.go's taskDeleterAdapter) does.
	svc.SetTaskDeleter(&stubTaskDeleter{
		err: fmt.Errorf("%w: %s", ErrTaskNotFound, taskID),
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

// Regression: when a task already has a TaskPR row pointing to PR #1 and
// AssociatePRWithTask is called with a different PR #2 (e.g. first PR
// closed, new PR opened — or a multi-branch task gaining a second PR on a
// different branch), the new row must be inserted as a sibling without
// touching the existing #1 row. Multi-branch tasks rely on this so two
// PRs on the same (task, repo) coexist; the old "delete-then-insert"
// behavior collapsed multi-branch tasks down to one PR row.
func TestAssociatePRWithTask_AddsSecondPRAsSibling(t *testing.T) {
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

	// Associate a new PR #2 on a different branch.
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
	tp, err := svc.AssociatePRWithTask(ctx, "t1", "", newPR)
	if err != nil {
		t.Fatalf("AssociatePRWithTask: %v", err)
	}
	if tp.PRNumber != 2 {
		t.Errorf("returned TaskPR.PRNumber=%d, want 2", tp.PRNumber)
	}

	all, err := store.ListTaskPRsByTask(ctx, "t1")
	if err != nil {
		t.Fatalf("ListTaskPRsByTask: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 PR rows after associating second PR, got %d", len(all))
	}
	nums := map[int]bool{}
	for _, r := range all {
		nums[r.PRNumber] = true
	}
	if !nums[1] || !nums[2] {
		t.Errorf("missing expected PR numbers: %v", nums)
	}
}
