package gitlab

import (
	"context"
	"testing"
)

// TestDeleteReviewMRTaskIfTerminal_RetainsWhenLifecyclePromptsEnabled is
// AC24: under cleanup_policy=auto, a review task whose MR is merged is
// retained while any lifecycle switch is enabled — mirroring GitHub's
// HasEnabledTaskPRAgentPrompts retention gate.
func TestDeleteReviewMRTaskIfTerminal_RetainsWhenLifecyclePromptsEnabled(t *testing.T) {
	svc := newServiceWithStore(t)
	ctx := context.Background()

	const project = "team/repo"
	mock := NewMockClient(svc.Host())
	mock.SeedMR(project, &MR{IID: 7, State: gitlabStateMerged})
	seedTask(t, svc.store, "task-subscribed", "")

	setMRSwitches(t, svc.store, "task-subscribed", mrIdentity(project, 7), TaskMRAutomationSwitchPatch{
		PromptOnMerged: boolPtr(true),
	})

	rec := &recordingReasonDeleter{}
	task := &ReviewMRTask{ID: "rmt-1", ProjectPath: project, MRIID: 7, TaskID: "task-subscribed"}

	if svc.deleteReviewMRTaskIfTerminal(ctx, task, CleanupPolicyAuto, mock, rec, nil) {
		t.Fatalf("expected retention: task has an enabled MR lifecycle prompt switch")
	}
	if rec.taskID != "" {
		t.Fatalf("deleter should not have been called, got taskID=%q", rec.taskID)
	}
}

// TestDeleteReviewMRTaskIfTerminal_AlwaysPolicyIgnoresLifecyclePrompts
// confirms cleanup_policy=always still deletes regardless of lifecycle
// switches (AC24's second half).
func TestDeleteReviewMRTaskIfTerminal_AlwaysPolicyIgnoresLifecyclePrompts(t *testing.T) {
	svc := newServiceWithStore(t)
	ctx := context.Background()

	const project = "team/repo"
	mock := NewMockClient(svc.Host())
	mock.SeedMR(project, &MR{IID: 7, State: gitlabStateMerged})
	seedTask(t, svc.store, "task-subscribed", "")

	setMRSwitches(t, svc.store, "task-subscribed", mrIdentity(project, 7), TaskMRAutomationSwitchPatch{
		PromptOnMerged: boolPtr(true),
	})

	rec := &recordingReasonDeleter{}
	task := &ReviewMRTask{ID: "rmt-1", ProjectPath: project, MRIID: 7, TaskID: "task-subscribed"}

	if !svc.deleteReviewMRTaskIfTerminal(ctx, task, CleanupPolicyAlways, mock, rec, nil) {
		t.Fatalf("expected deletion under cleanup_policy=always despite the enabled switch")
	}
	if rec.taskID != "task-subscribed" {
		t.Fatalf("deleted taskID=%q, want task-subscribed", rec.taskID)
	}
}

// TestDeleteReviewMRTaskIfTerminal_LifecycleLookupErrorRetains is the AC31
// counterpart for cleanup: a transient error checking the lifecycle switches
// must retain the task rather than risk deleting a subscribed one.
func TestDeleteReviewMRTaskIfTerminal_LifecycleLookupErrorRetains(t *testing.T) {
	svc := NewService(DefaultHost, NewNoopClient(DefaultHost), AuthMethodNone, nil, newTestLogger(t))
	// Deliberately no store wired: HasEnabledTaskMRAgentPrompts returns
	// errStoreUnavailable, which must retain rather than delete.
	ctx := context.Background()

	const project = "team/repo"
	mock := NewMockClient(svc.Host())
	mock.SeedMR(project, &MR{IID: 7, State: gitlabStateMerged})

	rec := &recordingReasonDeleter{}
	task := &ReviewMRTask{ID: "rmt-1", ProjectPath: project, MRIID: 7, TaskID: "task-x"}

	if svc.deleteReviewMRTaskIfTerminal(ctx, task, CleanupPolicyAuto, mock, rec, nil) {
		t.Fatalf("expected retention on a lifecycle-prompt lookup error")
	}
	if rec.taskID != "" {
		t.Fatalf("deleter should not have been called, got taskID=%q", rec.taskID)
	}
}
