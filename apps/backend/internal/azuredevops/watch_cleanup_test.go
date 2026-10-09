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

type countingCleanupClient struct {
	*cleanupClient
	workItemCalls    int
	pullRequestCalls int
}

func (c *countingCleanupClient) GetWorkItem(ctx context.Context, projectID string, id int) (*WorkItem, error) {
	c.workItemCalls++
	return c.cleanupClient.GetWorkItem(ctx, projectID, id)
}

func (c *countingCleanupClient) GetPullRequest(ctx context.Context, projectID, repositoryID string, id int) (*PullRequest, error) {
	c.pullRequestCalls++
	return c.cleanupClient.GetPullRequest(ctx, projectID, repositoryID, id)
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
	seedAzureTask(t, store, "task-101", "ws-1")
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

func TestCleanupSkipsHistoricalTasks(t *testing.T) {
	client := &countingCleanupClient{cleanupClient: &cleanupClient{
		workItem:    &WorkItem{ID: 101, State: "Closed"},
		pullRequest: &PullRequest{ID: 201, Status: "completed"},
	}}
	service, store, _ := newTestService(t, func(*Config, string) Client { return client })
	if _, err := service.SetConfigForWorkspace(t.Context(), "ws-1", &SetConfigRequest{OrganizationURL: "https://dev.azure.com/acme", PAT: "pat"}); err != nil {
		t.Fatalf("set config: %v", err)
	}
	if _, err := store.db.Exec(`CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL DEFAULT '',
		archived_at DATETIME
	)`); err != nil {
		t.Fatalf("create tasks table: %v", err)
	}
	for _, taskID := range []string{"work-active", "work-archived", "pr-active", "pr-archived"} {
		if _, err := store.db.Exec(`INSERT INTO tasks (id, workspace_id) VALUES (?, ?)`, taskID, "ws-1"); err != nil {
			t.Fatalf("seed task %s: %v", taskID, err)
		}
	}
	for _, taskID := range []string{"work-archived", "pr-archived"} {
		if _, err := store.db.Exec(`UPDATE tasks SET archived_at = CURRENT_TIMESTAMP WHERE id = ?`, taskID); err != nil {
			t.Fatalf("archive task %s: %v", taskID, err)
		}
	}

	workWatch := &WorkItemWatch{ID: "work-cleanup-service", WorkspaceID: "ws-1", ProjectID: "project-1", WIQL: "SELECT", CleanupPolicy: CleanupPolicyAuto}
	if err := store.CreateWorkItemWatch(t.Context(), workWatch); err != nil {
		t.Fatalf("create work-item watch: %v", err)
	}
	pullRequestWatch := &PullRequestWatch{ID: "pr-cleanup-service", WorkspaceID: "ws-1", ProjectID: "project-1", AzureRepositoryID: "repo-1", CleanupPolicy: CleanupPolicyAuto}
	if err := store.CreatePullRequestWatch(t.Context(), pullRequestWatch); err != nil {
		t.Fatalf("create pull-request watch: %v", err)
	}
	reserveWork := func(itemID int, taskID string) {
		t.Helper()
		reserved, err := store.ReserveWorkItemWatchTask(t.Context(), workWatch.ID, workWatch.Generation, workWatch.ProjectID, itemID, "https://azure/item")
		if err != nil || !reserved {
			t.Fatalf("reserve work item %d: reserved=%v err=%v", itemID, reserved, err)
		}
		if taskID != "" {
			if err := store.AssignWorkItemWatchTaskID(t.Context(), workWatch.ID, workWatch.Generation, workWatch.ProjectID, itemID, taskID); err != nil {
				t.Fatalf("assign work item %d: %v", itemID, err)
			}
		}
	}
	reservePullRequest := func(id int, taskID string) {
		t.Helper()
		reserved, err := store.ReservePullRequestWatchTask(t.Context(), pullRequestWatch.ID, pullRequestWatch.Generation, pullRequestWatch.ProjectID, pullRequestWatch.AzureRepositoryID, id, "https://azure/pr")
		if err != nil || !reserved {
			t.Fatalf("reserve pull request %d: reserved=%v err=%v", id, reserved, err)
		}
		if taskID != "" {
			if err := store.AssignPullRequestWatchTaskID(t.Context(), pullRequestWatch.ID, pullRequestWatch.Generation, pullRequestWatch.ProjectID, pullRequestWatch.AzureRepositoryID, id, taskID); err != nil {
				t.Fatalf("assign pull request %d: %v", id, err)
			}
		}
	}
	reserveWork(101, "work-active")
	reserveWork(102, "work-archived")
	reserveWork(103, "work-missing")
	reserveWork(104, "")
	reservePullRequest(201, "pr-active")
	reservePullRequest(202, "pr-archived")
	reservePullRequest(203, "pr-missing")
	reservePullRequest(204, "")

	deleter := &recordingCleanupDeleter{}
	service.SetCascadeTaskDeleter(deleter)
	if deleted, err := service.CleanupWorkItemWatch(t.Context(), "ws-1", workWatch.ID); err != nil || deleted != 1 {
		t.Fatalf("work-item cleanup = %d, %v; want one deletion", deleted, err)
	}
	if deleted, err := service.CleanupPullRequestWatch(t.Context(), "ws-1", pullRequestWatch.ID); err != nil || deleted != 1 {
		t.Fatalf("pull-request cleanup = %d, %v; want one deletion", deleted, err)
	}
	if client.workItemCalls != 1 {
		t.Fatalf("work-item status calls = %d, want 1 for the active task", client.workItemCalls)
	}
	if client.pullRequestCalls != 1 {
		t.Fatalf("pull-request status calls = %d, want 1 for the active task", client.pullRequestCalls)
	}
	if len(deleter.taskIDs) != 2 || deleter.taskIDs[0] != "work-active" || deleter.taskIDs[1] != "pr-active" {
		t.Fatalf("deleted task IDs = %v, want active tasks only", deleter.taskIDs)
	}
}
