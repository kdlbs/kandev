package service

import (
	"context"
	"errors"
	"testing"

	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// childRelationsFixture wires a real task service, the Office workspace-group
// store, and the canonical workspace-policy attacher, so a child's workspace
// binding is observable through its group membership.
type childRelationsFixture struct {
	svc       *Service
	officeRep *officesqlite.Repository
}

func newChildRelationsFixture(t *testing.T, workspaceID string) childRelationsFixture {
	t.Helper()
	svc, database := createOfficeIntegrationServiceWithDB(t)
	ctx := context.Background()
	officeRepo, err := officesqlite.NewWithDB(database, database, nil)
	if err != nil {
		t.Fatalf("office repository: %v", err)
	}
	taskRepo, ok := svc.tasks.(*sqliterepo.Repository)
	if !ok {
		t.Fatalf("task repository type = %T", svc.tasks)
	}
	svc.SetWorkspacePolicyAttacher(NewHandoffService(taskRepo, nil, nil, officeRepo, officeRepo, nil))
	svc.SetBlockerRepository(officeRepo)
	if err := svc.workspaces.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := svc.workflows.CreateWorkflow(ctx, &models.Workflow{ID: workspaceID + "-wf", WorkspaceID: workspaceID, Name: "Workflow"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	return childRelationsFixture{svc: svc, officeRep: officeRepo}
}

func (f childRelationsFixture) createRoot(t *testing.T, workspaceID, title string, policy *WorkspacePolicy) *models.Task {
	t.Helper()
	result, err := f.svc.CreateTask(context.Background(), &CreateTaskRequest{
		WorkspaceID:     workspaceID,
		WorkflowID:      workspaceID + "-wf",
		Title:           title,
		WorkspacePolicy: policy,
	})
	if err != nil {
		t.Fatalf("create %s: %v", title, err)
	}
	return result.Task
}

func (f childRelationsFixture) sharesParentGroup(t *testing.T, parentID, childID string) bool {
	t.Helper()
	ctx := context.Background()
	childGroup, err := f.officeRep.GetWorkspaceGroupForTask(ctx, childID)
	if err != nil {
		t.Fatalf("GetWorkspaceGroupForTask(child): %v", err)
	}
	if childGroup == nil {
		return false
	}
	parentGroup, err := f.officeRep.GetWorkspaceGroupForTask(ctx, parentID)
	if err != nil {
		t.Fatalf("GetWorkspaceGroupForTask(parent): %v", err)
	}
	return parentGroup != nil && parentGroup.ID == childGroup.ID
}

func childWorkspaceMode(t *testing.T, svc *Service, taskID string) string {
	t.Helper()
	task, err := svc.GetTask(context.Background(), taskID)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	return taskWorkspaceMode(task.Metadata)
}

// @covers AC-OFFICE-AGENT-TASK-RELATIONS-003.1
func TestCreateChildTaskNewWorkspaceModeGetsOwnWorkspace(t *testing.T) {
	f := newChildRelationsFixture(t, "ws-child-new")
	parent := f.createRoot(t, "ws-child-new", "Parent", nil)

	childID, err := f.svc.CreateChildTask(context.Background(), parent, ChildTaskSpec{
		Title:         "Batch child",
		WorkspaceMode: workspaceModeNewWorkspace,
	})
	if err != nil {
		t.Fatalf("CreateChildTask: %v", err)
	}
	if got := childWorkspaceMode(t, f.svc, childID); got != workspaceModeNewWorkspace {
		t.Fatalf("child workspace mode = %q, want %q", got, workspaceModeNewWorkspace)
	}
	if f.sharesParentGroup(t, parent.ID, childID) {
		t.Fatal("new_workspace child joined the parent's workspace group")
	}
}

// @covers AC-OFFICE-AGENT-TASK-RELATIONS-003.2
func TestCreateChildTaskExplicitInheritParentSharesParentWorkspace(t *testing.T) {
	f := newChildRelationsFixture(t, "ws-child-inherit")
	parent := f.createRoot(t, "ws-child-inherit", "Parent", nil)

	childID, err := f.svc.CreateChildTask(context.Background(), parent, ChildTaskSpec{
		Title:         "Child",
		WorkspaceMode: workspaceModeInheritParent,
	})
	if err != nil {
		t.Fatalf("CreateChildTask: %v", err)
	}
	if got := childWorkspaceMode(t, f.svc, childID); got != workspaceModeInheritParent {
		t.Fatalf("child workspace mode = %q, want %q", got, workspaceModeInheritParent)
	}
	if !f.sharesParentGroup(t, parent.ID, childID) {
		t.Fatal("inherit_parent child is not in the parent's workspace group")
	}
}

// An omitted mode keeps today's inherit_parent even when the parent records a
// default_child_workspace; the parent default is not consulted on this path.
//
// @covers AC-OFFICE-AGENT-TASK-RELATIONS-003.5
func TestCreateChildTaskOmittedModeIgnoresParentDefault(t *testing.T) {
	f := newChildRelationsFixture(t, "ws-child-default")
	parent := f.createRoot(t, "ws-child-default", "Parent", &WorkspacePolicy{
		DefaultChildWorkspace: workspaceModeNewWorkspace,
	})

	childID, err := f.svc.CreateChildTask(context.Background(), parent, ChildTaskSpec{Title: "Child"})
	if err != nil {
		t.Fatalf("CreateChildTask: %v", err)
	}
	if got := childWorkspaceMode(t, f.svc, childID); got != workspaceModeInheritParent {
		t.Fatalf("child workspace mode = %q, want %q", got, workspaceModeInheritParent)
	}
	if !f.sharesParentGroup(t, parent.ID, childID) {
		t.Fatal("omitted-mode child is not in the parent's workspace group")
	}
}

// @covers AC-OFFICE-AGENT-TASK-RELATIONS-001.1
// @covers AC-OFFICE-AGENT-TASK-RELATIONS-001.8
func TestCreateChildTaskRecordsBlockerEdges(t *testing.T) {
	f := newChildRelationsFixture(t, "ws-child-blockers")
	ctx := context.Background()
	parent := f.createRoot(t, "ws-child-blockers", "Parent", nil)
	sibling := f.createRoot(t, "ws-child-blockers", "Sibling", nil)

	childID, err := f.svc.CreateChildTask(ctx, parent, ChildTaskSpec{
		Title:     "Join",
		BlockedBy: []string{sibling.ID, parent.ID},
	})
	if err != nil {
		t.Fatalf("CreateChildTask: %v", err)
	}
	blockers, err := f.svc.GetBlockers(ctx, childID)
	if err != nil {
		t.Fatalf("GetBlockers: %v", err)
	}
	got := map[string]bool{}
	for _, id := range blockers {
		got[id] = true
	}
	if len(blockers) != 2 || !got[sibling.ID] || !got[parent.ID] {
		t.Fatalf("blockers = %v, want sibling %s and parent %s", blockers, sibling.ID, parent.ID)
	}
}

// @covers AC-OFFICE-AGENT-TASK-RELATIONS-001.6
func TestCreateChildTaskRejectsMissingBlockerWithoutCreatingTask(t *testing.T) {
	f := newChildRelationsFixture(t, "ws-child-missing")
	ctx := context.Background()
	parent := f.createRoot(t, "ws-child-missing", "Parent", nil)

	_, err := f.svc.CreateChildTask(ctx, parent, ChildTaskSpec{
		Title:     "Join",
		BlockedBy: []string{"missing-task"},
	})
	if !errors.Is(err, ErrTaskReferenceNotFound) {
		t.Fatalf("CreateChildTask error = %v, want ErrTaskReferenceNotFound", err)
	}
	children, err := f.svc.tasks.ListChildren(ctx, parent.ID)
	if err != nil {
		t.Fatalf("ListChildren: %v", err)
	}
	if len(children) != 0 {
		t.Fatalf("children = %d, want 0 after rejected create", len(children))
	}
}
