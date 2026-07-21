package gitlab

import (
	"context"
	"testing"
)

func TestStoreDisableGitLabWatchesWithError(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	review := &ReviewWatch{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		AgentProfileID: "agent", ExecutorProfileID: "executor", Enabled: true,
	}
	issue := &IssueWatch{
		WorkspaceID: "ws-1", WorkflowID: "wf", WorkflowStepID: "step",
		AgentProfileID: "agent", ExecutorProfileID: "executor", Enabled: true,
	}
	if err := store.CreateReviewWatch(ctx, review); err != nil {
		t.Fatalf("create review: %v", err)
	}
	if err := store.CreateIssueWatch(ctx, issue); err != nil {
		t.Fatalf("create issue: %v", err)
	}
	if err := store.DisableReviewWatchWithError(ctx, review.ID, "profile removed"); err != nil {
		t.Fatalf("disable review: %v", err)
	}
	if err := store.DisableIssueWatchWithError(ctx, issue.ID, "repository removed"); err != nil {
		t.Fatalf("disable issue: %v", err)
	}
	gotReview, _ := store.GetReviewWatch(ctx, review.ID)
	gotIssue, _ := store.GetIssueWatch(ctx, issue.ID)
	if gotReview.Enabled || gotReview.LastError != "profile removed" || gotReview.LastErrorAt == nil {
		t.Fatalf("review self-heal state = %#v", gotReview)
	}
	if gotIssue.Enabled || gotIssue.LastError != "repository removed" || gotIssue.LastErrorAt == nil {
		t.Fatalf("issue self-heal state = %#v", gotIssue)
	}
}
