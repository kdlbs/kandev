package gitlab

import (
	"testing"
)

func TestServiceIssueWatchLifecycle(t *testing.T) {
	store := newTestStore(t)
	client := NewMockClient(DefaultHost)
	client.SeedIssue("group/project", &Issue{IID: 3, Title: "Bug", State: "opened", ProjectPath: "group/project"})
	service := NewService(DefaultHost, client, "mock", nil, newTestLogger(t))
	service.SetStore(store)
	service.workspaceClients["workspace"] = client

	watch, err := service.CreateIssueWatch(t.Context(), &CreateIssueWatchRequest{
		WorkspaceID: "workspace", WorkflowID: "workflow", WorkflowStepID: "step",
		AgentProfileID: "agent", ExecutorProfileID: "executor", Projects: []ProjectFilter{{Path: " group/project "}},
		Labels: []string{"bug"}, PollIntervalSeconds: 1,
	})
	if err != nil {
		t.Fatalf("CreateIssueWatch() error = %v", err)
	}
	if watch.PollIntervalSeconds != minWatchPollIntervalSec || len(watch.Projects) != 1 || watch.Projects[0].Path != "group/project" {
		t.Fatalf("normalized watch = %#v", watch)
	}
	listed, err := service.ListIssueWatches(t.Context(), "workspace")
	if err != nil || len(listed) != 1 || listed[0].ID != watch.ID {
		t.Fatalf("ListIssueWatches() = (%#v, %v)", listed, err)
	}
	all, err := service.ListAllIssueWatches(t.Context())
	if err != nil || len(all) != 1 {
		t.Fatalf("ListAllIssueWatches() = (%#v, %v)", all, err)
	}

	prompt, enabled, max := "Investigate", true, 2
	if err := service.UpdateIssueWatch(t.Context(), watch.ID, &UpdateIssueWatchRequest{Prompt: &prompt, Enabled: &enabled, MaxInflightTasks: &max}); err != nil {
		t.Fatalf("UpdateIssueWatch() error = %v", err)
	}
	updated, err := service.GetIssueWatch(t.Context(), watch.ID)
	if err != nil || updated.Prompt != prompt || updated.MaxInflightTasks == nil || *updated.MaxInflightTasks != 2 {
		t.Fatalf("updated watch = (%#v, %v)", updated, err)
	}
	found, err := service.TriggerIssueWatch(t.Context(), watch.ID)
	if err != nil || len(found) != 1 || found[0].IID != 3 {
		t.Fatalf("TriggerIssueWatch() = (%#v, %v)", found, err)
	}
	if ok, err := store.ReserveIssueWatchTask(t.Context(), watch.ID, "group/project", 3, "url"); err != nil || !ok {
		t.Fatalf("reserve issue = (%v, %v)", ok, err)
	}
	found, err = service.CheckIssueWatch(t.Context(), watch)
	if err != nil || len(found) != 0 {
		t.Fatalf("deduplicated CheckIssueWatch() = (%#v, %v)", found, err)
	}
	if err := service.DeleteIssueWatch(t.Context(), watch.ID); err != nil {
		t.Fatalf("DeleteIssueWatch() error = %v", err)
	}
	if deleted, err := service.GetIssueWatch(t.Context(), watch.ID); err != nil || deleted != nil {
		t.Fatalf("deleted watch = (%#v, %v)", deleted, err)
	}
}

func TestServiceReviewWatchLifecycle(t *testing.T) {
	store := newTestStore(t)
	client := NewMockClient(DefaultHost)
	client.SeedMR("group/project", &MR{IID: 5, Title: "Feature", State: mrStateOpen, ProjectPath: "group/project"})
	service := NewService(DefaultHost, client, "mock", nil, newTestLogger(t))
	service.SetStore(store)
	service.workspaceClients["workspace"] = client

	watch, err := service.CreateReviewWatch(t.Context(), &CreateReviewWatchRequest{
		WorkspaceID: "workspace", WorkflowID: "workflow", WorkflowStepID: "step",
		AgentProfileID: "agent", ExecutorProfileID: "executor", Projects: []ProjectFilter{{Path: "group/project"}},
		PollIntervalSeconds: 1,
	})
	if err != nil {
		t.Fatalf("CreateReviewWatch() error = %v", err)
	}
	if watch.ReviewScope != ReviewScopeUserAndTeams || watch.PollIntervalSeconds != minWatchPollIntervalSec {
		t.Fatalf("normalized watch = %#v", watch)
	}
	listed, err := service.ListReviewWatches(t.Context(), "workspace")
	if err != nil || len(listed) != 1 || listed[0].ID != watch.ID {
		t.Fatalf("ListReviewWatches() = (%#v, %v)", listed, err)
	}
	all, err := service.ListAllReviewWatches(t.Context())
	if err != nil || len(all) != 1 {
		t.Fatalf("ListAllReviewWatches() = (%#v, %v)", all, err)
	}

	prompt, scope := "Review carefully", ReviewScopeUser
	if err := service.UpdateReviewWatch(t.Context(), watch.ID, &UpdateReviewWatchRequest{Prompt: &prompt, ReviewScope: &scope}); err != nil {
		t.Fatalf("UpdateReviewWatch() error = %v", err)
	}
	updated, err := service.GetReviewWatch(t.Context(), watch.ID)
	if err != nil || updated.Prompt != prompt || updated.ReviewScope != scope {
		t.Fatalf("updated watch = (%#v, %v)", updated, err)
	}
	found, err := service.TriggerReviewWatch(t.Context(), watch.ID)
	if err != nil || len(found) != 1 || found[0].IID != 5 {
		t.Fatalf("TriggerReviewWatch() = (%#v, %v)", found, err)
	}
	if ok, err := store.ReserveReviewMRTask(t.Context(), watch.ID, "group/project", 5, "url"); err != nil || !ok {
		t.Fatalf("reserve MR = (%v, %v)", ok, err)
	}
	found, err = service.CheckReviewWatch(t.Context(), watch)
	if err != nil || len(found) != 0 {
		t.Fatalf("deduplicated CheckReviewWatch() = (%#v, %v)", found, err)
	}
	if err := service.DeleteReviewWatch(t.Context(), watch.ID); err != nil {
		t.Fatalf("DeleteReviewWatch() error = %v", err)
	}
	if deleted, err := service.GetReviewWatch(t.Context(), watch.ID); err != nil || deleted != nil {
		t.Fatalf("deleted watch = (%#v, %v)", deleted, err)
	}
}

func TestServiceWatchValidationErrors(t *testing.T) {
	service := NewService(DefaultHost, NewMockClient(DefaultHost), "mock", nil, newTestLogger(t))
	if _, err := service.CreateIssueWatch(t.Context(), nil); err == nil {
		t.Fatal("CreateIssueWatch(nil) error = nil")
	}
	if _, err := service.CreateReviewWatch(t.Context(), nil); err == nil {
		t.Fatal("CreateReviewWatch(nil) error = nil")
	}
	if _, err := service.CheckIssueWatch(t.Context(), nil); err == nil {
		t.Fatal("CheckIssueWatch(nil) error = nil")
	}
	if _, err := service.CheckReviewWatch(t.Context(), nil); err == nil {
		t.Fatal("CheckReviewWatch(nil) error = nil")
	}
	disabledIssue := &IssueWatch{Enabled: false}
	if found, err := service.CheckIssueWatch(t.Context(), disabledIssue); err != nil || found != nil {
		t.Fatalf("disabled issue watch = (%#v, %v)", found, err)
	}
	disabledReview := &ReviewWatch{Enabled: false}
	if found, err := service.CheckReviewWatch(t.Context(), disabledReview); err != nil || found != nil {
		t.Fatalf("disabled review watch = (%#v, %v)", found, err)
	}
}
