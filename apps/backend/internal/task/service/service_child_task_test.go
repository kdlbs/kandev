package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestCreateChildTask_HappyPath_InheritsWorkflow(t *testing.T) {
	svc, repo := setupOfficeTest(t)
	ctx := context.Background()

	parent, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Parent",
		ProjectID:   "proj-1",
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}

	childID, err := svc.CreateChildTask(ctx, parent, ChildTaskSpec{
		Title:       "Child",
		Description: "Investigate",
	})
	if err != nil {
		t.Fatalf("CreateChildTask: %v", err)
	}
	if childID == "" {
		t.Fatalf("expected non-empty child id")
	}

	got, err := repo.GetTask(ctx, childID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.ParentID != parent.ID {
		t.Errorf("parent_id = %q, want %q", got.ParentID, parent.ID)
	}
	if got.WorkflowID != parent.WorkflowID {
		t.Errorf("workflow_id = %q, want inherited %q", got.WorkflowID, parent.WorkflowID)
	}
	if got.WorkspaceID != parent.WorkspaceID {
		t.Errorf("workspace_id = %q, want %q", got.WorkspaceID, parent.WorkspaceID)
	}
	if got.Origin != models.TaskOriginAgentCreated {
		t.Errorf("origin = %q, want agent_created", got.Origin)
	}
}

func TestCreateChildTask_OverridesWorkflow(t *testing.T) {
	svc, _ := setupOfficeTest(t)
	ctx := context.Background()

	parent, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Parent",
		ProjectID:   "proj-1",
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}

	// Override with a workflow id different from parent's. The repository
	// stores whatever workflow id we pass — even if there's no rows for
	// it in workflow_steps, we just want to confirm the request shape.
	otherWorkflowID := parent.WorkflowID + "-override"
	childID, err := svc.CreateChildTask(ctx, parent, ChildTaskSpec{
		Title:      "Switch flow",
		WorkflowID: otherWorkflowID,
	})
	if err != nil {
		t.Fatalf("CreateChildTask: %v", err)
	}
	if childID == "" {
		t.Fatalf("expected non-empty child id")
	}
}

func TestCreateTask_Subtask_InheritsParentRepositories(t *testing.T) {
	svc, repo := setupOfficeTest(t)
	ctx := context.Background()

	if err := repo.CreateRepository(ctx, &models.Repository{ID: "repo-x", WorkspaceID: "ws-1", Name: "Repo"}); err != nil {
		t.Fatalf("CreateRepository: %v", err)
	}

	parent, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Parent",
		ProjectID:   "proj-1",
		Repositories: []TaskRepositoryInput{{
			RepositoryID:   "repo-x",
			BaseBranch:     "main",
			CheckoutBranch: "feature/parent",
		}},
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}

	// Child created without explicit repositories — mirrors the UI inherit_parent
	// flow, which omits repositories expecting the backend to inherit them.
	child, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Child",
		ParentID:    parent.ID,
		ProjectID:   "proj-1",
	})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}

	childRepos, err := repo.ListTaskRepositories(ctx, child.ID)
	if err != nil {
		t.Fatalf("ListTaskRepositories: %v", err)
	}
	if len(childRepos) != 1 {
		t.Fatalf("child repos = %d, want 1 inherited from parent", len(childRepos))
	}
	if childRepos[0].RepositoryID != "repo-x" {
		t.Errorf("repository_id = %q, want repo-x", childRepos[0].RepositoryID)
	}
	if childRepos[0].BaseBranch != "main" {
		t.Errorf("base_branch = %q, want inherited main", childRepos[0].BaseBranch)
	}
	// CheckoutBranch is dropped: two worktrees can't share a working branch.
	if childRepos[0].CheckoutBranch != "" {
		t.Errorf("checkout_branch = %q, want empty (dropped on inherit)", childRepos[0].CheckoutBranch)
	}
}

func TestCreateTask_Subtask_ExplicitRepositoriesNotOverridden(t *testing.T) {
	svc, repo := setupOfficeTest(t)
	ctx := context.Background()

	for _, id := range []string{"repo-a", "repo-b"} {
		if err := repo.CreateRepository(ctx, &models.Repository{ID: id, WorkspaceID: "ws-1", Name: id}); err != nil {
			t.Fatalf("CreateRepository %s: %v", id, err)
		}
	}

	parent, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:  "ws-1",
		Title:        "Parent",
		ProjectID:    "proj-1",
		Repositories: []TaskRepositoryInput{{RepositoryID: "repo-a", BaseBranch: "main"}},
	})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}

	child, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:  "ws-1",
		Title:        "Child",
		ParentID:     parent.ID,
		ProjectID:    "proj-1",
		Repositories: []TaskRepositoryInput{{RepositoryID: "repo-b", BaseBranch: "dev"}},
	})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}

	childRepos, err := repo.ListTaskRepositories(ctx, child.ID)
	if err != nil {
		t.Fatalf("ListTaskRepositories: %v", err)
	}
	if len(childRepos) != 1 || childRepos[0].RepositoryID != "repo-b" {
		t.Fatalf("child repos = %+v, want only explicit repo-b", childRepos)
	}
}

func TestCreateChildTask_RequiresParent(t *testing.T) {
	svc, _ := setupOfficeTest(t)
	_, err := svc.CreateChildTask(context.Background(), nil, ChildTaskSpec{Title: "X"})
	if err == nil || !strings.Contains(err.Error(), "parent") {
		t.Fatalf("expected parent error, got: %v", err)
	}
}

func TestCreateChildTask_RequiresTitle(t *testing.T) {
	svc, _ := setupOfficeTest(t)
	ctx := context.Background()
	parent, _ := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Parent",
		ProjectID:   "proj-1",
	})
	_, err := svc.CreateChildTask(ctx, parent, ChildTaskSpec{})
	if err == nil || !strings.Contains(err.Error(), "title") {
		t.Fatalf("expected title error, got: %v", err)
	}
}

func TestCreateTask_SubtaskOfSubtask_Kanban_Rejected(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Workspace"})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-1", WorkspaceID: "ws-1", Name: "Board"})

	root, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Root",
	})
	if err != nil {
		t.Fatalf("create root: %v", err)
	}

	child, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		ParentID:    root.ID,
		Title:       "Child",
	})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}

	_, err = svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		ParentID:    child.ID,
		Title:       "Grandchild",
	})
	if err == nil || !errors.Is(err, ErrSubtaskDepthExceeded) {
		t.Fatalf("expected ErrSubtaskDepthExceeded, got: %v", err)
	}
}

func TestCreateTask_SubtaskOfSubtask_Office_Allowed(t *testing.T) {
	svc, repo := setupOfficeTest(t)
	ctx := context.Background()

	root, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Root office",
		ProjectID:   "proj-1",
	})
	if err != nil {
		t.Fatalf("create root: %v", err)
	}

	child, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		ParentID:    root.ID,
		Title:       "Child office",
		ProjectID:   "proj-1",
	})
	if err != nil {
		t.Fatalf("create child: %v", err)
	}

	grandchild, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		ParentID:    child.ID,
		Title:       "Grandchild office",
		ProjectID:   "proj-1",
	})
	if err != nil {
		t.Fatalf("create grandchild: %v", err)
	}

	got, err := repo.GetTask(ctx, grandchild.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.ParentID != child.ID {
		t.Errorf("parent_id = %q, want %q", got.ParentID, child.ID)
	}
}
