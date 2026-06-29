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

// plainTaskDeleter implements only TaskDeleter (not TaskDeleterWithReason) so
// deleteTaskWithReason must take its documented fallback to DeleteTask.
type plainTaskDeleter struct{ taskID string }

func (p *plainTaskDeleter) DeleteTask(_ context.Context, taskID string) error {
	p.taskID = taskID
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
	if rec.reason != reasonMRMergedOrClosed {
		t.Errorf("reason=%q, want %q", rec.reason, reasonMRMergedOrClosed)
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
	if rec.reason != reasonIssueClosed {
		t.Errorf("reason=%q, want %q", rec.reason, reasonIssueClosed)
	}
}

// TestDeleteTaskWithReason_FallsBackWhenUnsupported covers the documented
// fallback: a deleter implementing only TaskDeleter is still invoked via
// DeleteTask (the reason is dropped) rather than panicking on the assertion.
func TestDeleteTaskWithReason_FallsBackWhenUnsupported(t *testing.T) {
	rec := &plainTaskDeleter{}
	if err := deleteTaskWithReason(context.Background(), rec, "task-x", reasonIssueClosed); err != nil {
		t.Fatalf("deleteTaskWithReason: %v", err)
	}
	if rec.taskID != "task-x" {
		t.Errorf("taskID=%q, want task-x (fallback DeleteTask must be called)", rec.taskID)
	}
}

// TestCleanupAllReviewTasks_ThreadsReason exercises the public wiring
// (SetTaskDeleter → CleanupAllReviewTasks → sweep) end-to-end, asserting the
// reason survives all the way to the deleter.
func TestCleanupAllReviewTasks_ThreadsReason(t *testing.T) {
	svc := newServiceWithStore(t)
	ctx := context.Background()
	store := svc.store

	const project = "team/repo"
	const iid = 7
	watch := &ReviewWatch{WorkspaceID: "ws-1", Enabled: true, CleanupPolicy: CleanupPolicyAlways}
	if err := store.CreateReviewWatch(ctx, watch); err != nil {
		t.Fatalf("CreateReviewWatch: %v", err)
	}
	if _, err := store.ReserveReviewMRTask(ctx, watch.ID, project, iid, "url"); err != nil {
		t.Fatalf("ReserveReviewMRTask: %v", err)
	}
	if err := store.AssignReviewMRTaskID(ctx, watch.ID, project, iid, "task-merged"); err != nil {
		t.Fatalf("AssignReviewMRTaskID: %v", err)
	}

	mock := swapToMock(t, svc)
	mock.SeedMR(project, &MR{IID: iid, State: gitlabStateMerged})

	rec := &recordingReasonDeleter{}
	svc.SetTaskDeleter(rec)

	deleted, err := svc.CleanupAllReviewTasks(ctx)
	if err != nil {
		t.Fatalf("CleanupAllReviewTasks: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted=%d, want 1", deleted)
	}
	if rec.reason != reasonMRMergedOrClosed {
		t.Errorf("reason=%q, want %q", rec.reason, reasonMRMergedOrClosed)
	}
}

// TestCleanupAllIssueTasks_ThreadsReason is the issue-path mirror of the
// review public-path test above.
func TestCleanupAllIssueTasks_ThreadsReason(t *testing.T) {
	svc := newServiceWithStore(t)
	ctx := context.Background()
	store := svc.store

	const project = "team/repo"
	const iid = 5
	watch := &IssueWatch{WorkspaceID: "ws-1", Enabled: true, CleanupPolicy: CleanupPolicyAlways}
	if err := store.CreateIssueWatch(ctx, watch); err != nil {
		t.Fatalf("CreateIssueWatch: %v", err)
	}
	if _, err := store.ReserveIssueWatchTask(ctx, watch.ID, project, iid, "url"); err != nil {
		t.Fatalf("ReserveIssueWatchTask: %v", err)
	}
	if err := store.AssignIssueWatchTaskID(ctx, watch.ID, project, iid, "task-closed"); err != nil {
		t.Fatalf("AssignIssueWatchTaskID: %v", err)
	}

	mock := swapToMock(t, svc)
	mock.SeedIssue(project, &Issue{IID: iid, State: gitlabStateClosed})

	rec := &recordingReasonDeleter{}
	svc.SetTaskDeleter(rec)

	deleted, err := svc.CleanupAllIssueTasks(ctx)
	if err != nil {
		t.Fatalf("CleanupAllIssueTasks: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted=%d, want 1", deleted)
	}
	if rec.reason != reasonIssueClosed {
		t.Errorf("reason=%q, want %q", rec.reason, reasonIssueClosed)
	}
}
