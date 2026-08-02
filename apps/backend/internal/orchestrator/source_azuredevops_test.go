package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/azuredevops"
)

type azureWatchServiceFake struct {
	reservedWorkItem bool
	reservedPR       bool
	assignedTaskID   string
	workWatchID      string
	prWatchID        string
}

func (f *azureWatchServiceFake) ReserveWorkItemWatchTask(context.Context, string, int64, string, int, string) (bool, error) {
	return f.reservedWorkItem, nil
}
func (f *azureWatchServiceFake) AssignWorkItemWatchTaskID(_ context.Context, watchID string, _ int64, _ string, _ int, taskID string) error {
	f.workWatchID, f.assignedTaskID = watchID, taskID
	return nil
}
func (f *azureWatchServiceFake) ReleaseWorkItemWatchTask(context.Context, string, int64, string, int) error {
	return nil
}
func (f *azureWatchServiceFake) DisableWorkItemWatchWithError(context.Context, string, string) error {
	return nil
}
func (f *azureWatchServiceFake) ReservePullRequestWatchTask(context.Context, string, int64, string, string, int, string) (bool, error) {
	return f.reservedPR, nil
}
func (f *azureWatchServiceFake) AssignPullRequestWatchTaskID(_ context.Context, watchID string, _ int64, _ string, _ string, _ int, taskID string) error {
	f.prWatchID, f.assignedTaskID = watchID, taskID
	return nil
}
func (f *azureWatchServiceFake) ReleasePullRequestWatchTask(context.Context, string, int64, string, string, int) error {
	return nil
}
func (f *azureWatchServiceFake) DisablePullRequestWatchWithError(context.Context, string, string) error {
	return nil
}

func TestAzureWorkItemWatcherSourceBuildsIssueTask(t *testing.T) {
	service := &azureWatchServiceFake{reservedWorkItem: true}
	source := NewAzureDevOpsWorkItemWatcherSource(service, nil)
	event := &azuredevops.WorkItemWatchEvent{
		WatchID: "watch-1", WatchGeneration: 3, WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
		RepositoryID: "repo-1", BaseBranch: "main", AgentProfileID: "agent-1", ExecutorProfileID: "executor-1",
		Prompt: "Implement {{work_item.title}} ({{work_item.url}})", ProjectID: "project-1",
		WorkItem: azuredevops.WorkItem{ID: 101, Title: "Fix build", State: "Active", Type: "Bug", WebURL: "https://dev.azure.com/acme/wi/101"},
	}
	req, err := source.BuildTaskRequest(event)
	if err != nil {
		t.Fatalf("BuildTaskRequest: %v", err)
	}
	if req.Title != "[project-1#101] Fix build" || req.Description != "Implement Fix build (https://dev.azure.com/acme/wi/101)" {
		t.Fatalf("request = %+v", req)
	}
	if req.Metadata[azureDevOpsWorkItemWatchMetadataKey] != "watch-1" || len(req.Repositories) != 1 {
		t.Fatalf("metadata/repositories = %#v/%+v", req.Metadata, req.Repositories)
	}
	reserved, err := source.Reserve(context.Background(), event)
	if err != nil || !reserved {
		t.Fatalf("Reserve = %v, %v", reserved, err)
	}
	if err := source.AttachTaskID(context.Background(), event, "task-1"); err != nil || service.assignedTaskID != "task-1" {
		t.Fatalf("AttachTaskID = %v, assigned=%q", err, service.assignedTaskID)
	}
}

func TestAzurePullRequestWatcherSourceBuildsReviewTask(t *testing.T) {
	service := &azureWatchServiceFake{reservedPR: true}
	source := NewAzureDevOpsPullRequestWatcherSource(service, nil)
	event := &azuredevops.PullRequestWatchEvent{
		WatchID: "watch-pr", WatchGeneration: 2, WorkspaceID: "ws-1", WorkflowID: "wf-1", WorkflowStepID: "step-1",
		RepositoryID: "repo-1", BaseBranch: "main", AgentProfileID: "agent-1", ExecutorProfileID: "executor-1",
		Prompt: "Review {{pull_request.title}} at {{pull_request.url}}", ProjectID: "project-1", AzureRepositoryID: "azure-repo-1",
		PullRequest: azuredevops.PullRequest{ID: 42, Title: "Ship it", WebURL: "https://dev.azure.com/acme/pr/42", RepositoryID: "azure-repo-1", ProjectName: "Platform"},
	}
	req, err := source.BuildTaskRequest(event)
	if err != nil {
		t.Fatalf("BuildTaskRequest: %v", err)
	}
	if req.Title != "[project-1!42] Ship it" || req.Description != "Review Ship it at https://dev.azure.com/acme/pr/42" {
		t.Fatalf("request = %+v", req)
	}
	if req.Metadata[azureDevOpsPullRequestWatchMetadataKey] != "watch-pr" || len(req.Repositories) != 1 {
		t.Fatalf("metadata/repositories = %#v/%+v", req.Metadata, req.Repositories)
	}
	reserved, err := source.Reserve(context.Background(), event)
	if err != nil || !reserved {
		t.Fatalf("Reserve = %v, %v", reserved, err)
	}
	if err := source.AttachTaskID(context.Background(), event, "task-pr-1"); err != nil || service.assignedTaskID != "task-pr-1" {
		t.Fatalf("AttachTaskID = %v, assigned=%q", err, service.assignedTaskID)
	}
}
