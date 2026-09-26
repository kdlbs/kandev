package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestWorkspaceCRUDDefaultsOwnershipAndMissingErrors(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	workspace := &models.Workspace{Name: "Workspace", Description: "before"}
	if err := repo.CreateWorkspace(ctx, workspace); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if workspace.ID == "" || workspace.TaskPrefix != "KAN" || workspace.CreatedAt.IsZero() || !workspace.CreatedAt.Equal(workspace.UpdatedAt) {
		t.Fatalf("workspace defaults = %+v", workspace)
	}
	got, err := repo.GetWorkspace(ctx, workspace.ID)
	if err != nil || got.Name != "Workspace" || got.Description != "before" {
		t.Fatalf("GetWorkspace = %+v, %v", got, err)
	}
	got.Name, got.Description = "Updated", "after"
	if err := repo.UpdateWorkspace(ctx, got); err != nil {
		t.Fatalf("UpdateWorkspace: %v", err)
	}
	if err := repo.ClaimUnownedWorkspaces(ctx, "owner-one"); err != nil {
		t.Fatalf("ClaimUnownedWorkspaces: %v", err)
	}
	listed, err := repo.ListWorkspaces(ctx)
	if err != nil {
		t.Fatalf("ListWorkspaces: %v", err)
	}
	var found *models.Workspace
	for _, item := range listed {
		if item.ID == workspace.ID {
			found = item
		}
	}
	if found == nil || found.Name != "Updated" || found.Description != "after" || found.OwnerID != "owner-one" {
		t.Fatalf("updated workspace = %+v", found)
	}
	if _, err := repo.GetWorkspace(ctx, "missing"); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("GetWorkspace missing error = %v", err)
	}
	if err := repo.UpdateWorkspace(ctx, &models.Workspace{ID: "missing"}); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("UpdateWorkspace missing error = %v", err)
	}
	if err := repo.DeleteWorkspace(ctx, workspace.ID); err != nil {
		t.Fatalf("DeleteWorkspace: %v", err)
	}
	if err := repo.DeleteWorkspace(ctx, workspace.ID); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("second DeleteWorkspace error = %v", err)
	}
}

func TestWorkspaceCascadeConfirmationAndCleanupAreAtomic(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "workspace-cascade")
	if err := repo.UpdateWorkspace(ctx, &models.Workspace{ID: "workspace-cascade", Name: "Confirmed", Description: "kept on rollback"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow-cascade", WorkspaceID: "workspace-cascade", Name: "Workflow"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateTask(ctx, &models.Task{ID: "task-cascade", WorkspaceID: "workspace-cascade", WorkflowID: "workflow-cascade", Title: "Task"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := repo.DeleteWorkspaceCascadeWithName(ctx, "workspace-cascade", "Wrong"); !errors.Is(err, repoerrors.ErrWorkspaceNameMismatch) {
		t.Fatalf("name mismatch error = %v", err)
	}
	cleanupErr := errors.New("cleanup failed")
	cleanupCalled := false
	_, _, err := repo.DeleteWorkspaceCascadeWithNameAndSecretCleanup(ctx, "workspace-cascade", "Confirmed", func(context.Context, *sqlx.Tx) error {
		cleanupCalled = true
		return cleanupErr
	})
	if !cleanupCalled || !errors.Is(err, cleanupErr) {
		t.Fatalf("cleanup rollback = called %v, error %v", cleanupCalled, err)
	}
	if _, err := repo.GetWorkspace(ctx, "workspace-cascade"); err != nil {
		t.Fatalf("workspace missing after cleanup rollback: %v", err)
	}
	if _, err := repo.GetTask(ctx, "task-cascade"); err != nil {
		t.Fatalf("task missing after cleanup rollback: %v", err)
	}

	tasks, workflows, err := repo.DeleteWorkspaceCascadeWithNameAndSecretCleanup(ctx, "workspace-cascade", "Confirmed", func(context.Context, *sqlx.Tx) error { return nil })
	if err != nil {
		t.Fatalf("DeleteWorkspaceCascadeWithNameAndSecretCleanup: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != "task-cascade" || len(workflows) != 1 || workflows[0].ID != "workflow-cascade" {
		t.Fatalf("cascade snapshots = tasks %+v, workflows %+v", tasks, workflows)
	}
	if _, err := repo.GetWorkspace(ctx, "workspace-cascade"); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("workspace remains after cascade: %v", err)
	}
	if _, _, err := repo.DeleteWorkspaceCascade(ctx, "workspace-cascade"); !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("second cascade error = %v", err)
	}
}
