package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.4
// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.6
func TestAddBranchToTask_InheritedLiveTaskRollsBackDeferredMaterialization(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	taskID := seedInheritedBranchTask(t, repo, string(models.ExecutorTypeWorktree))
	materializer := &stubMaterializer{}
	svc.SetBranchMaterializer(materializer)

	before, err := repo.ListTaskRepositories(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	eventBus.ClearEvents()

	_, err = svc.AddBranchToTask(ctx, AddBranchToTaskRequest{
		TaskID:         taskID,
		RepositoryID:   "repo-inherited",
		BaseBranch:     "main",
		CheckoutBranch: "feature/inherited",
	})
	if err == nil {
		t.Fatal("AddBranchToTask accepted deferred materialization for a live inherited environment")
	}
	after, listErr := repo.ListTaskRepositories(ctx, taskID)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(after) != len(before) {
		t.Fatalf("task repositories after failure = %d, want %d", len(after), len(before))
	}
	for _, event := range eventBus.GetPublishedEvents() {
		if event.Type == events.TaskUpdated {
			t.Fatalf("unexpected task.updated after rollback: %+v", event)
		}
	}
	if materializer.lastTarget.SessionID != "session-child" || materializer.lastTarget.TaskEnvironmentID != "env-parent" {
		t.Fatalf("materializer target = %+v, want inherited child session and parent environment", materializer.lastTarget)
	}
}

// @covers AC-TASKS-ATTACH-WORKSPACE-SOURCES-001.4
func TestAddBranchToTask_RejectsInheritedNonWorktreeEnvironmentBeforeInsert(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	taskID := seedInheritedBranchTask(t, repo, string(models.ExecutorTypeLocalDocker))
	materializer := &stubMaterializer{result: &BranchMaterializationResult{WorktreePath: "/unexpected"}}
	svc.SetBranchMaterializer(materializer)

	before, err := repo.ListTaskRepositories(ctx, taskID)
	if err != nil {
		t.Fatal(err)
	}
	eventBus.ClearEvents()
	_, err = svc.AddBranchToTask(ctx, AddBranchToTaskRequest{
		TaskID:         taskID,
		RepositoryID:   "repo-inherited",
		BaseBranch:     "main",
		CheckoutBranch: "feature/rejected",
	})
	if err == nil || !strings.Contains(err.Error(), "worktree executor") {
		t.Fatalf("AddBranchToTask error = %v, want inherited executor rejection", err)
	}
	after, listErr := repo.ListTaskRepositories(ctx, taskID)
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(after) != len(before) {
		t.Fatalf("task repositories after rejection = %d, want %d", len(after), len(before))
	}
	if materializer.calls != 0 {
		t.Fatalf("materializer calls = %d, want rejection before materialization", materializer.calls)
	}
	for _, event := range eventBus.GetPublishedEvents() {
		if event.Type == events.TaskUpdated {
			t.Fatalf("unexpected task.updated after rejection: %+v", event)
		}
	}
}

func seedInheritedBranchTask(t *testing.T, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
	CreateRepository(context.Context, *models.Repository) error
	CreateTask(context.Context, *models.Task) error
	CreateTaskRepository(context.Context, *models.TaskRepository) error
	CreateTaskEnvironment(context.Context, *models.TaskEnvironment) error
	CreateTaskSession(context.Context, *models.TaskSession) error
}, executorType string,
) string {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	for _, seed := range []struct {
		label string
		err   error
	}{
		{"workspace", repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-inherited", Name: "WS"})},
		{"workflow", repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-inherited", WorkspaceID: "ws-inherited", Name: "WF"})},
		{"repository", repo.CreateRepository(ctx, &models.Repository{ID: "repo-inherited", WorkspaceID: "ws-inherited", Name: "app", DefaultBranch: "main"})},
		{"parent task", repo.CreateTask(ctx, &models.Task{ID: "task-parent", WorkspaceID: "ws-inherited", WorkflowID: "wf-inherited", WorkflowStepID: "step-inherited", Title: "Parent", Priority: "medium", Metadata: map[string]interface{}{}})},
		{"child task", repo.CreateTask(ctx, &models.Task{ID: "task-child", WorkspaceID: "ws-inherited", WorkflowID: "wf-inherited", WorkflowStepID: "step-inherited", Title: "Child", Priority: "medium", ParentID: "task-parent", Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}}})},
		{"parent repository", repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "tr-parent", TaskID: "task-parent", RepositoryID: "repo-inherited", BaseBranch: "main", Metadata: map[string]interface{}{}})},
		{"child repository", repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "tr-child", TaskID: "task-child", RepositoryID: "repo-inherited", BaseBranch: "main", Metadata: map[string]interface{}{}})},
	} {
		if seed.err != nil {
			t.Fatalf("create %s: %v", seed.label, seed.err)
		}
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID:            "env-parent",
		TaskID:        "task-parent",
		ExecutorType:  executorType,
		WorkspacePath: "/tmp/inherited-task",
		TaskDirName:   "inherited-task",
		Status:        models.TaskEnvironmentStatusReady,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "env-parent-primary", RepositoryID: "repo-inherited", BranchSlug: "main", CreatedAt: now,
		}},
	}); err != nil {
		t.Fatalf("create parent environment: %v", err)
	}
	if err := repo.CreateTaskSession(ctx, &models.TaskSession{
		ID:                "session-child",
		TaskID:            "task-child",
		TaskEnvironmentID: "env-parent",
		State:             models.TaskSessionStateWaitingForInput,
		StartedAt:         now,
		UpdatedAt:         now,
	}); err != nil {
		t.Fatalf("create child session: %v", err)
	}
	return "task-child"
}
