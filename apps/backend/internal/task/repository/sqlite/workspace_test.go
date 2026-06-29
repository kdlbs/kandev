package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func TestDeleteWorkspaceCascadeWithNameDeletesWorkspaceChildren(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForHealTests(t)

	seedWorkspaceCascadeRows(t, repo, "ws-delete")

	if err := repo.DeleteWorkspaceCascadeWithName(ctx, "ws-delete", "Delete Me"); err != nil {
		t.Fatalf("DeleteWorkspaceCascadeWithName: %v", err)
	}
	if _, err := repo.GetWorkspace(ctx, "ws-delete"); err == nil {
		t.Fatalf("workspace should be deleted")
	}
	if _, err := repo.GetTask(ctx, "task-delete"); err == nil {
		t.Fatalf("workspace task should be deleted")
	}
	workflows, err := repo.ListWorkflows(ctx, "ws-delete", true)
	if err != nil {
		t.Fatalf("ListWorkflows: %v", err)
	}
	if len(workflows) != 0 {
		t.Fatalf("workspace workflows should be deleted, got %d", len(workflows))
	}
}

func TestDeleteWorkspaceCascadeWithNameRejectsMismatchedName(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForHealTests(t)

	seedWorkspaceCascadeRows(t, repo, "ws-delete")

	err := repo.DeleteWorkspaceCascadeWithName(ctx, "ws-delete", "Wrong")
	if !errors.Is(err, repoerrors.ErrWorkspaceNameMismatch) {
		t.Fatalf("expected ErrWorkspaceNameMismatch, got %v", err)
	}
	if _, err := repo.GetWorkspace(ctx, "ws-delete"); err != nil {
		t.Fatalf("workspace should remain: %v", err)
	}
	if _, err := repo.GetTask(ctx, "task-delete"); err != nil {
		t.Fatalf("workspace task should remain: %v", err)
	}
	if _, err := repo.GetWorkflow(ctx, "wf-delete"); err != nil {
		t.Fatalf("workspace workflow should remain: %v", err)
	}
}

func TestDeleteWorkspaceCascadeWithNameRollsBackWhenChildDeleteFails(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForHealTests(t)

	seedWorkspaceCascadeRows(t, repo, "ws-delete")
	if _, err := repo.db.Exec(`
		CREATE TRIGGER fail_workspace_task_delete
		BEFORE DELETE ON tasks
		WHEN OLD.workspace_id = 'ws-delete'
		BEGIN
			SELECT RAISE(ABORT, 'task delete blocked');
		END
	`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	if err := repo.DeleteWorkspaceCascadeWithName(ctx, "ws-delete", "Delete Me"); err == nil {
		t.Fatalf("expected child delete failure")
	}
	if _, err := repo.GetWorkspace(ctx, "ws-delete"); err != nil {
		t.Fatalf("workspace should roll back: %v", err)
	}
	if _, err := repo.GetTask(ctx, "task-delete"); err != nil {
		t.Fatalf("workspace task should roll back: %v", err)
	}
	if _, err := repo.GetWorkflow(ctx, "wf-delete"); err != nil {
		t.Fatalf("workspace workflow should roll back: %v", err)
	}
}

func seedWorkspaceCascadeRows(t *testing.T, repo *Repository, workspaceID string) {
	t.Helper()
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Delete Me"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{
		ID:          "wf-delete",
		WorkspaceID: workspaceID,
		Name:        "Doomed",
	}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID:             "task-delete",
		WorkspaceID:    workspaceID,
		WorkflowID:     "wf-delete",
		WorkflowStepID: "step-delete",
		Title:          "Delete task",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
}
