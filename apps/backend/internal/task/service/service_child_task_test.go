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

	parentResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Parent",
		ProjectID:   "proj-1",
	})
	parent := parentResult.Task
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

// Regression: CreateChildTask must keep inheriting the parent's project so
// office cost events (which copy tasks.project_id verbatim) attribute to the
// same project as the rest of the task tree.
func TestCreateChildTask_InheritsParentProject(t *testing.T) {
	svc, repo := setupOfficeTest(t)
	ctx := context.Background()

	parentResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Parent",
		ProjectID:   "proj-1",
	})
	parent := parentResult.Task
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}

	childID, err := svc.CreateChildTask(ctx, parent, ChildTaskSpec{Title: "Child"})
	if err != nil {
		t.Fatalf("CreateChildTask: %v", err)
	}

	got, err := repo.GetTask(ctx, childID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.ProjectID != "proj-1" {
		t.Errorf("project_id = %q, want inherited proj-1", got.ProjectID)
	}
}

func TestCreateChildTask_OverridesWorkflow(t *testing.T) {
	svc, repo := setupOfficeTest(t)
	ctx := context.Background()

	parentResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Parent",
		ProjectID:   "proj-1",
	})
	parent := parentResult.Task
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}

	// Override with another workflow in the parent's workspace.
	otherWorkflowID := parent.WorkflowID + "-override"
	if err := repo.CreateWorkflow(ctx, &models.Workflow{
		ID: otherWorkflowID, WorkspaceID: parent.WorkspaceID, Name: "Override",
	}); err != nil {
		t.Fatalf("create override workflow: %v", err)
	}
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

	parentResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Parent",
		ProjectID:   "proj-1",
		Repositories: []TaskRepositoryInput{{
			RepositoryID:   "repo-x",
			BaseBranch:     "main",
			CheckoutBranch: "feature/parent",
		}},
	})
	parent := parentResult.Task
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}

	// Child created without explicit repositories — mirrors the UI inherit_parent
	// flow, which omits repositories expecting the backend to inherit them.
	childResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Child",
		ParentID:    parent.ID,
		ProjectID:   "proj-1",
	})
	child := childResult.Task
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

// TestCreateTask_Subtask_InheritsParentProject covers the generic create
// path (mirrors MCP create_task_kandev / REST create with parent_id set): a
// subtask created with a parent but no explicit project_id must inherit the
// parent's project and, since that earns it office recognition, an
// identifier — otherwise its office cost events leak (no project_id to roll
// up to).
func TestCreateTask_Subtask_InheritsParentProject(t *testing.T) {
	svc, repo := setupOfficeTest(t)
	ctx := context.Background()

	parentResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Parent",
		ProjectID:   "proj-1",
	})
	parent := parentResult.Task
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}

	childResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Child",
		ParentID:    parent.ID,
		Origin:      models.TaskOriginAgentCreated,
	})
	child := childResult.Task
	if err != nil {
		t.Fatalf("create child: %v", err)
	}

	got, err := repo.GetTask(ctx, child.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.ProjectID != "proj-1" {
		t.Errorf("project_id = %q, want inherited proj-1", got.ProjectID)
	}
	if got.Identifier == "" {
		t.Error("expected identifier once the task is recognized as an office task")
	}
}

func TestCreateTask_Subtask_ExplicitProjectNotOverridden(t *testing.T) {
	svc, repo := setupOfficeTest(t)
	ctx := context.Background()

	parentResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Parent",
		ProjectID:   "proj-1",
	})
	parent := parentResult.Task
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}

	childResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Child",
		ParentID:    parent.ID,
		ProjectID:   "proj-2",
	})
	child := childResult.Task
	if err != nil {
		t.Fatalf("create child: %v", err)
	}

	got, err := repo.GetTask(ctx, child.ID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.ProjectID != "proj-2" {
		t.Errorf("project_id = %q, want explicit proj-2 kept", got.ProjectID)
	}
}

// TestCreateTask_Subtask_AutoTitleRejectedRegardlessOfProjectSource is the F3
// regression: prepareTaskForCreation must classify office-ness once, from the
// final req.ProjectID, so an inherited project reaches the same
// ErrAutoTitleUnsupportedForOffice rejection an explicit project_id does.
// Before the fix, inheritParentProject ran after prepareAutoTitle and
// validateCreateTaskRequest, so a request with no explicit project_id sailed
// past both — isOfficeRequest saw ProjectID still empty — then got
// classified as an office task once the project was inherited, silently
// creating a task with agent_title_pending set: exactly what the guard
// exists to prevent.
func TestCreateTask_Subtask_AutoTitleRejectedRegardlessOfProjectSource(t *testing.T) {
	svc, repo := setupOfficeTest(t)
	ctx := context.Background()

	ws, err := repo.GetWorkspace(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	officeWorkflowID := ws.OfficeWorkflowID

	parentResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Parent",
		ProjectID:   "proj-1",
	})
	parent := parentResult.Task
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}

	_, err = svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		ParentID:    parent.ID,
		WorkflowID:  officeWorkflowID,
		ProjectID:   "proj-1",
		AutoTitle:   true,
		Description: "please do the thing now",
	})
	if !errors.Is(err, ErrAutoTitleUnsupportedForOffice) {
		t.Fatalf("explicit project: error = %v, want ErrAutoTitleUnsupportedForOffice", err)
	}

	_, err = svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		ParentID:    parent.ID,
		WorkflowID:  officeWorkflowID,
		AutoTitle:   true,
		Description: "please do the thing now",
	})
	if !errors.Is(err, ErrAutoTitleUnsupportedForOffice) {
		t.Fatalf("inherited project: error = %v, want the SAME ErrAutoTitleUnsupportedForOffice the explicit "+
			"case gets, not a silently-created office task carrying agent_title_pending", err)
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

	parentResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:  "ws-1",
		Title:        "Parent",
		ProjectID:    "proj-1",
		Repositories: []TaskRepositoryInput{{RepositoryID: "repo-a", BaseBranch: "main"}},
	})
	parent := parentResult.Task
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}

	childResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID:  "ws-1",
		Title:        "Child",
		ParentID:     parent.ID,
		ProjectID:    "proj-1",
		Repositories: []TaskRepositoryInput{{RepositoryID: "repo-b", BaseBranch: "dev"}},
	})
	child := childResult.Task
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
	parentResult, _ := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Parent",
		ProjectID:   "proj-1",
	})
	parent := parentResult.Task
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

	rootResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		Title:       "Root",
	})
	root := rootResult.Task
	if err != nil {
		t.Fatalf("create root: %v", err)
	}

	childResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  "wf-1",
		ParentID:    root.ID,
		Title:       "Child",
	})
	child := childResult.Task
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

	rootResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		Title:       "Root office",
		ProjectID:   "proj-1",
	})
	root := rootResult.Task
	if err != nil {
		t.Fatalf("create root: %v", err)
	}

	childResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		ParentID:    root.ID,
		Title:       "Child office",
		ProjectID:   "proj-1",
	})
	child := childResult.Task
	if err != nil {
		t.Fatalf("create child: %v", err)
	}

	grandchildResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		ParentID:    child.ID,
		Title:       "Grandchild office",
		ProjectID:   "proj-1",
	})
	grandchild := grandchildResult.Task
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

// Regression: a caller authorized only for req.WorkspaceID must not have a
// foreign workspace's project silently attributed to a subtask it creates.
// Explicit cross-workspace subtask creation is itself a supported MCP flow
// (TestHandleCreateTask_SubtaskHonorsExplicitWorkspaceAndWorkflow), so this
// must not reject the create outright — only project inheritance is at
// stake. Without this guard, POST /tasks with a foreign parent_id and no
// project_id landed a task in the caller's workspace carrying another
// workspace's project id: cost attribution leaking across a workspace
// boundary, the same class of bug this card exists to close, pointed
// sideways instead of down the tree.
func TestCreateTask_Subtask_CrossWorkspaceParentDoesNotInheritForeignProject(t *testing.T) {
	svc, repo := setupOfficeTest(t)
	ctx := context.Background()

	ws, err := repo.GetWorkspace(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspace: %v", err)
	}
	officeWorkflowID := ws.OfficeWorkflowID

	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-2", Name: "Other Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace ws-2: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-2", WorkspaceID: "ws-2", Name: "Board"}); err != nil {
		t.Fatalf("CreateWorkflow wf-2: %v", err)
	}

	foreignParentResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-2",
		WorkflowID:  "wf-2",
		Title:       "Foreign parent",
		ProjectID:   "proj-ws2",
	})
	if err != nil {
		t.Fatalf("create foreign parent: %v", err)
	}
	foreignParent := foreignParentResult.Task

	childResult, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "ws-1",
		WorkflowID:  officeWorkflowID,
		ParentID:    foreignParent.ID,
		Title:       "Child",
		Origin:      models.TaskOriginAgentCreated,
	})
	if err != nil {
		t.Fatalf("create child with cross-workspace parent: %v", err)
	}
	child := childResult.Task
	if child.WorkspaceID != "ws-1" {
		t.Errorf("workspace_id = %q, want ws-1 (the requested, authorized workspace)", child.WorkspaceID)
	}
	if child.ProjectID == "proj-ws2" {
		t.Fatalf("project_id = %q: leaked workspace ws-2's project into a ws-1 task", child.ProjectID)
	}
	if child.ProjectID != "" {
		t.Errorf("project_id = %q, want empty (no same-workspace project to inherit)", child.ProjectID)
	}
}
