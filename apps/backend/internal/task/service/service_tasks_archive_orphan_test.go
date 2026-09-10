package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
)

// seedArchiveOrphanParentAndChild creates a workspace/workflow plus a parent
// task and returns everything needed to attach children under it.
func seedArchiveOrphanParentAndChild(t *testing.T, repo interface {
	CreateWorkspace(context.Context, *models.Workspace) error
	CreateWorkflow(context.Context, *models.Workflow) error
	CreateTask(context.Context, *models.Task) error
}, parentID string) (workspaceID, workflowID string) {
	t.Helper()
	ctx := context.Background()
	workspaceID = "ws-" + parentID
	workflowID = "wf-" + parentID
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: workspaceID, Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: workflowID, WorkspaceID: workspaceID, Name: "Workflow"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: parentID, WorkspaceID: workspaceID, WorkflowID: workflowID, WorkflowStepID: "step",
		Title: "Parent", Priority: "medium",
	}); err != nil {
		t.Fatalf("CreateTask(parent): %v", err)
	}
	return workspaceID, workflowID
}

func inheritParentChildTask(id, parentID, workspaceID, workflowID string) *models.Task {
	return &models.Task{
		ID: id, ParentID: parentID, WorkspaceID: workspaceID, WorkflowID: workflowID, WorkflowStepID: "step",
		Title: "Child", Priority: "medium",
		Metadata: map[string]interface{}{
			"workspace": map[string]interface{}{"mode": "inherit_parent"},
		},
	}
}

// REGRESSION: archiving a parent task tears down its runtime resources
// (worktree, container/sandbox) but preserves its own task_environments
// row, so a not-yet-launched inherit_parent child's session.TaskEnvironmentID
// (there is no foreign key) is left pointing at a workspace whose worktree
// is gone even though the row itself still exists. Before this fix the
// child kept rendering as an ordinary launchable CREATED card, and only
// discovered the problem days later at launch time. ArchiveTask must stamp
// an orphan marker on the child immediately, and publish task.updated so
// any listening UI picks it up.
func TestArchiveTask_MarksUnmaterializedInheritParentChildOrphaned(t *testing.T) {
	svc, bus, repo := createTestService(t)
	ctx := context.Background()
	const parentID = "task-archive-orphan-parent"
	const childID = "task-archive-orphan-child"

	workspaceID, workflowID := seedArchiveOrphanParentAndChild(t, repo, parentID)
	if err := repo.CreateTask(ctx, inheritParentChildTask(childID, parentID, workspaceID, workflowID)); err != nil {
		t.Fatalf("CreateTask(child): %v", err)
	}

	if err := svc.ArchiveTask(ctx, parentID); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}

	child, err := repo.GetTask(ctx, childID)
	if err != nil {
		t.Fatalf("GetTask(child): %v", err)
	}
	workspace, _ := child.Metadata["workspace"].(map[string]interface{})
	if workspace == nil {
		t.Fatal("child metadata.workspace missing after archive")
	}
	if orphaned, _ := workspace["orphaned"].(bool); !orphaned {
		t.Fatalf("child metadata.workspace.orphaned = %v, want true", workspace["orphaned"])
	}
	if reason, _ := workspace["orphaned_reason"].(string); reason != "parent_archived" {
		t.Fatalf("child metadata.workspace.orphaned_reason = %q, want %q", reason, "parent_archived")
	}
	if gotParentID, _ := workspace["orphaned_parent_id"].(string); gotParentID != parentID {
		t.Fatalf("child metadata.workspace.orphaned_parent_id = %q, want %q", gotParentID, parentID)
	}
	if _, ok := workspace["orphaned_at"].(string); !ok {
		t.Fatal("child metadata.workspace.orphaned_at missing after archive")
	}
	if workspace["mode"] != "inherit_parent" {
		t.Fatalf("child metadata.workspace.mode changed to %v, want unchanged %q", workspace["mode"], "inherit_parent")
	}

	found := false
	for _, evt := range bus.GetPublishedEvents() {
		if evt.Type != events.TaskUpdated {
			continue
		}
		data, ok := evt.Data.(map[string]interface{})
		if ok && data["task_id"] == childID {
			found = true
		}
	}
	if !found {
		t.Fatal("no task.updated event published for orphaned child")
	}
}

// A child that already materialized its own task_environments row is not
// orphaned by the parent's archive: the executor's by-task-id environment
// lookup finds that row first and never falls through to the (now-gone)
// inherited one, so marking it would be a false positive.
func TestArchiveTask_DoesNotMarkChildWithOwnEnvironmentOrphaned(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	const parentID = "task-archive-orphan-owned-parent"
	const childID = "task-archive-orphan-owned-child"

	workspaceID, workflowID := seedArchiveOrphanParentAndChild(t, repo, parentID)
	if err := repo.CreateTask(ctx, inheritParentChildTask(childID, parentID, workspaceID, workflowID)); err != nil {
		t.Fatalf("CreateTask(child): %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-owned-by-child", TaskID: childID, ExecutorType: "local_docker",
		Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}

	if err := svc.ArchiveTask(ctx, parentID); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}

	child, err := repo.GetTask(ctx, childID)
	if err != nil {
		t.Fatalf("GetTask(child): %v", err)
	}
	workspace, _ := child.Metadata["workspace"].(map[string]interface{})
	if orphaned, _ := workspace["orphaned"].(bool); orphaned {
		t.Fatal("child with its own task environment was marked orphaned")
	}
}

// A child that is not workspace_mode=inherit_parent is unaffected by a
// parent archive; only inherited-workspace children can be stranded this way.
func TestArchiveTask_DoesNotMarkNonInheritParentChildOrphaned(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	const parentID = "task-archive-orphan-newws-parent"
	const childID = "task-archive-orphan-newws-child"

	workspaceID, workflowID := seedArchiveOrphanParentAndChild(t, repo, parentID)
	child := inheritParentChildTask(childID, parentID, workspaceID, workflowID)
	child.Metadata["workspace"].(map[string]interface{})["mode"] = "new_workspace"
	if err := repo.CreateTask(ctx, child); err != nil {
		t.Fatalf("CreateTask(child): %v", err)
	}

	if err := svc.ArchiveTask(ctx, parentID); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}

	got, err := repo.GetTask(ctx, childID)
	if err != nil {
		t.Fatalf("GetTask(child): %v", err)
	}
	workspace, _ := got.Metadata["workspace"].(map[string]interface{})
	if orphaned, _ := workspace["orphaned"].(bool); orphaned {
		t.Fatal("new_workspace child was marked orphaned by parent archive")
	}
}
