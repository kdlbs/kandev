package gitlab

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

// TestDeleteReviewMRTask_ThreadsMergedReason verifies that when a merged MR's
// review task is swept, the cleanup path deletes it with the
// "pr_merged_or_closed" reason so the frontend can explain the disappearance.
func TestDeleteReviewMRTask_ThreadsMergedReason(t *testing.T) {
	svc := newServiceWithStore(t)
	ctx := context.Background()

	const project = "team/repo"
	mock := NewMockClient(svc.Host())
	mock.SeedMR(project, &MR{IID: 7, State: gitlabStateMerged})

	rec := &recordingReasonDeleter{}
	task := &ReviewMRTask{ID: "rmt-1", ProjectPath: project, MRIID: 7, TaskID: "task-merged"}

	if !svc.deleteReviewMRTaskIfTerminal(ctx, task, CleanupPolicyAlways, mock, rec, nil) {
		t.Fatalf("expected deleteReviewMRTaskIfTerminal to delete the task")
	}
	if rec.taskID != "task-merged" {
		t.Errorf("deleted taskID=%q, want task-merged", rec.taskID)
	}
	if rec.reason != "pr_merged_or_closed" {
		t.Errorf("reason=%q, want pr_merged_or_closed", rec.reason)
	}
}

// TestDeleteIssueWatchTask_ThreadsClosedReason mirrors the review case for the
// issue path: a closed issue's task is deleted with the "issue_closed" reason.
func TestDeleteIssueWatchTask_ThreadsClosedReason(t *testing.T) {
	svc := newServiceWithStore(t)
	ctx := context.Background()

	const project = "team/repo"
	mock := NewMockClient(svc.Host())
	mock.SeedIssue(project, &Issue{IID: 5, State: gitlabStateClosed})

	rec := &recordingReasonDeleter{}
	task := &IssueWatchTask{ID: "iwt-1", ProjectPath: project, IssueIID: 5, TaskID: "task-closed"}

	if !svc.deleteIssueWatchTaskIfTerminal(ctx, task, CleanupPolicyAlways, mock, rec, nil) {
		t.Fatalf("expected deleteIssueWatchTaskIfTerminal to delete the task")
	}
	if rec.taskID != "task-closed" {
		t.Errorf("deleted taskID=%q, want task-closed", rec.taskID)
	}
	if rec.reason != "issue_closed" {
		t.Errorf("reason=%q, want issue_closed", rec.reason)
	}
}
