package azuredevops

import (
	"context"
	"testing"

	taskservice "github.com/kandev/kandev/internal/task/service"
)

type cleanupClient struct {
	invalidClient
	workItem    *WorkItem
	pullRequest *PullRequest
}

func (c *cleanupClient) GetWorkItem(context.Context, string, int) (*WorkItem, error) {
	if c.workItem == nil {
		return nil, mockNotFound("work item", 0)
	}
	item := *c.workItem
	return &item, nil
}

func (c *cleanupClient) GetPullRequest(context.Context, string, string, int) (*PullRequest, error) {
	if c.pullRequest == nil {
		return nil, mockNotFound("pull request", 0)
	}
	pullRequest := *c.pullRequest
	return &pullRequest, nil
}

type recordingCleanupDeleter struct {
	taskIDs []string
}

func (d *recordingCleanupDeleter) DeleteTaskTree(_ context.Context, taskID string, _ bool) (*taskservice.CascadeOutcome, error) {
	d.taskIDs = append(d.taskIDs, taskID)
	return &taskservice.CascadeOutcome{}, nil
}

func (d *recordingCleanupDeleter) DeleteTask(ctx context.Context, taskID string, cascade bool) (*taskservice.CascadeOutcome, error) {
	d.taskIDs = append(d.taskIDs, taskID)
	return &taskservice.CascadeOutcome{}, nil
}

type recordingTaskSessionChecker struct{ authored bool }

func (c recordingTaskSessionChecker) HasUserAuthoredMessage(context.Context, string) (bool, error) {
	return c.authored, nil
}

func TestCleanupWorkItemWatchHonorsAutoEngagementAndTerminalState(t *testing.T) {
	client := &cleanupClient{workItem: &WorkItem{ID: 101, State: "Closed"}}
	service, store, _ := newTestService(t, func(*Config, string) Client { return client })
	if _, err := service.SetConfigForWorkspace(t.Context(), "ws-1", &SetConfigRequest{OrganizationURL: "https://dev.azure.com/acme", PAT: "pat"}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	watch := &WorkItemWatch{WorkspaceID: "ws-1", ProjectID: "project-1", WIQL: "SELECT", CleanupPolicy: CleanupPolicyAuto}
	if err := store.CreateWorkItemWatch(t.Context(), watch); err != nil {
		t.Fatalf("create watch: %v", err)
	}
	if reserved, err := store.ReserveWorkItemWatchTask(t.Context(), watch.ID, watch.Generation, watch.ProjectID, 101, "https://azure/item/101"); err != nil || !reserved {
		t.Fatalf("reserve: %v %v", reserved, err)
	}
	if err := store.AssignWorkItemWatchTaskID(t.Context(), watch.ID, watch.Generation, watch.ProjectID, 101, "task-101"); err != nil {
		t.Fatalf("attach: %v", err)
	}
	deleter := &recordingCleanupDeleter{}
	service.SetCascadeTaskDeleter(deleter)
	service.SetTaskSessionChecker(recordingTaskSessionChecker{authored: true})
	if deleted, err := service.CleanupWorkItemWatch(t.Context(), "ws-1", watch.ID); err != nil || deleted != 0 {
		t.Fatalf("engaged cleanup = %d, %v; want no deletion", deleted, err)
	}
	service.SetTaskSessionChecker(recordingTaskSessionChecker{})
	deleted, err := service.CleanupWorkItemWatch(t.Context(), "ws-1", watch.ID)
	if err != nil || deleted != 1 {
		t.Fatalf("terminal cleanup = %d, %v; want one deletion", deleted, err)
	}
	if len(deleter.taskIDs) != 1 || deleter.taskIDs[0] != "task-101" {
		t.Fatalf("deleted task IDs = %v", deleter.taskIDs)
	}
	rows, err := store.ListWorkItemWatchTasks(t.Context(), watch.ID, watch.Generation)
	if err != nil || len(rows) != 0 {
		t.Fatalf("cleanup reservations = %v, %v; want empty", rows, err)
	}
}

func TestCleanupPullRequestWatchHonorsNeverPolicy(t *testing.T) {
	client := &cleanupClient{pullRequest: &PullRequest{ID: 42, Status: "completed"}}
	service, store, _ := newTestService(t, func(*Config, string) Client { return client })
	if _, err := service.SetConfigForWorkspace(t.Context(), "ws-1", &SetConfigRequest{OrganizationURL: "https://dev.azure.com/acme", PAT: "pat"}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	watch := &PullRequestWatch{WorkspaceID: "ws-1", ProjectID: "project-1", AzureRepositoryID: "azure-repo-1", CleanupPolicy: CleanupPolicyNever}
	if err := store.CreatePullRequestWatch(t.Context(), watch); err != nil {
		t.Fatalf("create watch: %v", err)
	}
	if reserved, err := store.ReservePullRequestWatchTask(t.Context(), watch.ID, watch.Generation, watch.ProjectID, watch.AzureRepositoryID, 42, "https://azure/pr/42"); err != nil || !reserved {
		t.Fatalf("reserve: %v %v", reserved, err)
	}
	if err := store.AssignPullRequestWatchTaskID(t.Context(), watch.ID, watch.Generation, watch.ProjectID, watch.AzureRepositoryID, 42, "task-42"); err != nil {
		t.Fatalf("attach: %v", err)
	}
	deleter := &recordingCleanupDeleter{}
	service.SetCascadeTaskDeleter(deleter)
	if deleted, err := service.CleanupPullRequestWatch(t.Context(), "ws-1", watch.ID); err != nil || deleted != 0 {
		t.Fatalf("never cleanup = %d, %v; want no deletion", deleted, err)
	}
	if len(deleter.taskIDs) != 0 {
		t.Fatalf("never policy deleted tasks: %v", deleter.taskIDs)
	}
}
