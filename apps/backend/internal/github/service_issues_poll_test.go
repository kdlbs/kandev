package github

import (
	"context"
	"testing"
)

func TestCheckIssueWatch_PollTimestampPreservesConcurrentSettings(t *testing.T) {
	svc, store := setupWatchServiceTest(t)
	ctx := context.Background()
	watch := &IssueWatch{
		WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
		Repos:  []RepoFilter{{Owner: "acme", Name: "widget"}},
		Prompt: "fix it", CustomQuery: "is:open", Enabled: true, PollIntervalSeconds: 300,
	}
	if err := store.CreateIssueWatch(ctx, watch); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.GetIssueWatch(ctx, watch.ID)
	if err != nil || snapshot == nil {
		t.Fatalf("load poll snapshot: watch=%v err=%v", snapshot, err)
	}
	prompt := "handle it"
	disabled := false
	if err := svc.UpdateIssueWatch(ctx, watch.ID, &UpdateIssueWatchRequest{
		Prompt: &prompt, Enabled: &disabled,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CheckIssueWatch(ctx, snapshot); err != nil {
		t.Fatal(err)
	}
	updated, err := store.GetIssueWatch(ctx, watch.ID)
	if err != nil || updated == nil {
		t.Fatalf("read updated watch: watch=%v err=%v", updated, err)
	}
	if updated.Prompt != prompt || updated.Enabled {
		t.Errorf("poll overwrote settings: prompt=%q enabled=%v", updated.Prompt, updated.Enabled)
	}
	if updated.CustomQuery != "is:open" {
		t.Errorf("poll changed query: %q", updated.CustomQuery)
	}
	if updated.LastPolledAt == nil {
		t.Error("completed poll did not persist its timestamp")
	}
}
