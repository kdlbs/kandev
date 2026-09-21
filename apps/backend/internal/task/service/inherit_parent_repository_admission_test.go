package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-TASKS-MCP-WORKSPACE-MODE-004.5
func TestValidateInheritedWorkspaceRepositorySelectionRejectsMissingBranchSlot(t *testing.T) {
	svc, repo := setupOfficeTest(t)
	ctx := context.Background()
	workspace, err := repo.GetWorkspace(ctx, "ws-1")
	if err != nil {
		t.Fatalf("get workspace: %v", err)
	}
	if err := repo.CreateRepository(ctx, &models.Repository{
		ID: "repo-parent", WorkspaceID: "ws-1", Name: "Parent", DefaultBranch: "main",
	}); err != nil {
		t.Fatalf("create repository: %v", err)
	}
	parentResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  workspace.OfficeWorkflowID,
		Title:       "Parent",
		Repositories: []TaskRepositoryInput{{
			RepositoryID: "repo-parent",
			BaseBranch:   "main",
		}},
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}
	env := &models.TaskEnvironment{
		ID:            "env-parent",
		TaskID:        parentResult.Task.ID,
		ExecutorType:  string(models.ExecutorTypeWorktree),
		Status:        models.TaskEnvironmentStatusCreating,
		WorkspacePath: "/tmp/parent-workspace",
	}
	if err := repo.CreateTaskEnvironment(ctx, env); err != nil {
		t.Fatalf("create environment: %v", err)
	}
	if err := repo.CreateTaskEnvironmentRepo(ctx, &models.TaskEnvironmentRepo{
		TaskEnvironmentID: env.ID,
		RepositoryID:      "repo-parent",
		BranchSlug:        "feature-parent",
		Status:            "active",
	}); err != nil {
		t.Fatalf("create environment repository: %v", err)
	}
	env.Status = models.TaskEnvironmentStatusReady
	if err := repo.UpdateTaskEnvironment(ctx, env); err != nil {
		t.Fatalf("ready environment: %v", err)
	}

	err = svc.ValidateInheritedWorkspaceRepositorySelection(ctx, parentResult.Task.ID, []TaskRepositoryInput{{
		RepositoryID: "repo-parent",
		BaseBranch:   "main",
	}})
	if err == nil {
		t.Fatal("validation succeeded for a missing canonical branch slot")
	}
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("validation error = %v, want ErrWorkspaceReuseUnsafe", err)
	}
}
