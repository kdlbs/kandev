package github

import (
	"context"
	"testing"
)

// recordingReasonDeleter implements both TaskDeleter and TaskDeleterWithReason
// so the cleanup path exercises the reason-threading branch (deleteTaskWithReason
// prefers DeleteTaskWithReason when the wired deleter supports it).
type recordingReasonDeleter struct {
	taskID string
	reason string
}

func (r *recordingReasonDeleter) DeleteTask(_ context.Context, taskID string) error {
	r.taskID = taskID
	return nil
}

func (r *recordingReasonDeleter) DeleteTaskWithReason(_ context.Context, taskID, reason string) error {
	r.taskID = taskID
	r.reason = reason
	return nil
}

// TestCleanupMergedReviewTasks_ThreadsApprovedReason verifies that when a PR is
// approved by the authenticated user, the cleanup path deletes the task with the
// "pr_approved_by_user" reason so the frontend can explain the disappearance.
func TestCleanupMergedReviewTasks_ThreadsApprovedReason(t *testing.T) {
	_, svc, mockClient, store := setupPollerTest(t)
	ctx := context.Background()

	watch := &ReviewWatch{WorkspaceID: "ws-1", Enabled: true}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("CreateReviewWatch: %v", err)
	}

	rpt := &ReviewPRTask{
		ReviewWatchID: watch.ID,
		RepoOwner:     "acme",
		RepoName:      "widget",
		PRNumber:      42,
		PRURL:         "https://github.com/acme/widget/pull/42",
		TaskID:        "task-approved",
	}
	if err := store.CreateReviewPRTask(ctx, rpt); err != nil {
		t.Fatalf("CreateReviewPRTask: %v", err)
	}

	// PR is still open, but the authenticated user already approved it on
	// GitHub, so shouldDeleteReviewTask returns reason pr_approved_by_user.
	mockClient.AddPR(&PR{Number: 42, State: "open", RepoOwner: "acme", RepoName: "widget"})
	mockClient.AddReviews("acme", "widget", 42, []PRReview{
		{State: "APPROVED", Author: mockDefaultUser},
	})

	rec := &recordingReasonDeleter{}
	svc.SetTaskDeleter(rec)

	deleted, err := svc.CleanupMergedReviewTasks(ctx, watch)
	if err != nil {
		t.Fatalf("CleanupMergedReviewTasks: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deleted, got %d", deleted)
	}
	if rec.taskID != "task-approved" {
		t.Errorf("deleted taskID=%q, want task-approved", rec.taskID)
	}
	if rec.reason != "pr_approved_by_user" {
		t.Errorf("reason=%q, want pr_approved_by_user", rec.reason)
	}
}
