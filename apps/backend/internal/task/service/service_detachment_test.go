package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type reparentAfterDetachRepository struct {
	repository.TaskRepository
	detacher taskDetachmentRepository
	parentID string
}

func (r *reparentAfterDetachRepository) DetachTask(ctx context.Context, taskID string) (bool, error) {
	changed, err := r.detacher.DetachTask(ctx, taskID)
	if err != nil {
		return false, err
	}
	task, err := r.GetTask(ctx, taskID)
	if err != nil {
		return false, err
	}
	task.ParentID = r.parentID
	if err := r.UpdateTask(ctx, task); err != nil {
		return false, err
	}
	return changed, nil
}

func TestDetachTaskNormalizesInheritedWorkspaceAndPreservesTaskState(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createDetachmentFixture(t, ctx, repo)
	before, err := repo.GetTask(ctx, "child")
	if err != nil {
		t.Fatalf("GetTask before detach: %v", err)
	}
	eventBus.ClearEvents()

	detached, err := svc.DetachTask(ctx, "child")
	if err != nil {
		t.Fatalf("DetachTask: %v", err)
	}

	if detached.ParentID != "" {
		t.Fatalf("ParentID = %q, want empty", detached.ParentID)
	}
	if detached.WorkflowID != "workflow" || detached.WorkflowStepID != "step" || detached.State != v1.TaskStateInProgress {
		t.Fatalf("placement/state changed: %#v", detached)
	}
	if !detached.UpdatedAt.After(before.UpdatedAt) {
		t.Fatalf("UpdatedAt = %s, want after %s", detached.UpdatedAt, before.UpdatedAt)
	}
	workspace, ok := detached.Metadata["workspace"].(map[string]interface{})
	if !ok {
		t.Fatalf("workspace metadata = %#v", detached.Metadata["workspace"])
	}
	if workspace["mode"] != "shared_group" {
		t.Fatalf("workspace mode = %#v, want shared_group", workspace["mode"])
	}
	if workspace["group_id"] != "group-1" || detached.Metadata["unrelated"] != "keep" {
		t.Fatalf("unrelated metadata changed: %#v", detached.Metadata)
	}

	persisted, err := repo.GetTask(ctx, "child")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if persisted.ParentID != "" {
		t.Fatalf("persisted ParentID = %q, want empty", persisted.ParentID)
	}
	persistedWorkspace := persisted.Metadata["workspace"].(map[string]interface{})
	if persistedWorkspace["mode"] != "shared_group" {
		t.Fatalf("persisted workspace mode = %#v", persistedWorkspace["mode"])
	}

	eventData := singleDetachmentEventData(t, eventBus)
	if parentID, ok := eventData["parent_id"]; !ok || parentID != nil {
		t.Fatalf("parent_id event field = %#v (present=%v), want explicit nil", parentID, ok)
	}
	eventMetadata := eventData["metadata"].(map[string]interface{})
	eventWorkspace := eventMetadata["workspace"].(map[string]interface{})
	if eventWorkspace["mode"] != "shared_group" {
		t.Fatalf("event workspace mode = %#v, want shared_group", eventWorkspace["mode"])
	}
}

func TestDetachTaskIsIdempotentForRootTask(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createDetachmentFixture(t, ctx, repo)
	if _, err := svc.DetachTask(ctx, "child"); err != nil {
		t.Fatalf("first DetachTask: %v", err)
	}
	before, err := repo.GetTask(ctx, "child")
	if err != nil {
		t.Fatalf("GetTask before retry: %v", err)
	}
	eventBus.ClearEvents()

	detached, err := svc.DetachTask(ctx, "child")
	if err != nil {
		t.Fatalf("second DetachTask: %v", err)
	}

	if !detached.UpdatedAt.Equal(before.UpdatedAt) {
		t.Fatalf("UpdatedAt changed on idempotent retry: %s -> %s", before.UpdatedAt, detached.UpdatedAt)
	}
	eventData := singleDetachmentEventData(t, eventBus)
	if parentID, ok := eventData["parent_id"]; !ok || parentID != nil {
		t.Fatalf("parent_id event field = %#v (present=%v), want explicit nil", parentID, ok)
	}
}

func TestDetachTaskPreservesNonInheritedWorkspaceModes(t *testing.T) {
	for _, mode := range []string{"shared_group", "new_workspace"} {
		t.Run(mode, func(t *testing.T) {
			svc, _, repo := createTestService(t)
			ctx := context.Background()
			createDetachmentFixture(t, ctx, repo)
			task, err := repo.GetTask(ctx, "child")
			if err != nil {
				t.Fatalf("GetTask: %v", err)
			}
			workspace := task.Metadata["workspace"].(map[string]interface{})
			workspace["mode"] = mode
			if err := repo.UpdateTask(ctx, task); err != nil {
				t.Fatalf("UpdateTask fixture: %v", err)
			}

			detached, err := svc.DetachTask(ctx, "child")
			if err != nil {
				t.Fatalf("DetachTask: %v", err)
			}

			detachedWorkspace := detached.Metadata["workspace"].(map[string]interface{})
			if detachedWorkspace["mode"] != mode {
				t.Fatalf("workspace mode = %#v, want %s", detachedWorkspace["mode"], mode)
			}
			if detachedWorkspace["group_id"] != "group-1" {
				t.Fatalf("workspace group_id = %#v, want group-1", detachedWorkspace["group_id"])
			}
		})
	}
}

func TestDetachTaskPreservesDescendantsAndWorkspaceGroupMembership(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	createDetachmentFixture(t, ctx, repo)
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "grandchild", WorkspaceID: "workspace", WorkflowID: "workflow", WorkflowStepID: "step",
		Title: "Grandchild", Priority: "medium", State: v1.TaskStateTODO, ParentID: "child",
	}); err != nil {
		t.Fatalf("CreateTask grandchild: %v", err)
	}
	now := time.Now().UTC()
	if _, err := repo.DB().ExecContext(ctx, `
		INSERT INTO task_workspace_groups
			(id, workspace_id, owner_task_id, materialized_kind, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, "group-1", "workspace", "parent", "single_repo", now, now); err != nil {
		t.Fatalf("insert workspace group: %v", err)
	}
	if _, err := repo.DB().ExecContext(ctx, `
		INSERT INTO task_workspace_group_members (workspace_group_id, task_id, role, created_at)
		VALUES (?, ?, ?, ?)
	`, "group-1", "child", "member", now); err != nil {
		t.Fatalf("insert workspace group member: %v", err)
	}

	if _, err := svc.DetachTask(ctx, "child"); err != nil {
		t.Fatalf("DetachTask: %v", err)
	}

	grandchild, err := repo.GetTask(ctx, "grandchild")
	if err != nil {
		t.Fatalf("GetTask grandchild: %v", err)
	}
	if grandchild.ParentID != "child" {
		t.Fatalf("grandchild ParentID = %q, want child", grandchild.ParentID)
	}
	var releasedAt *time.Time
	if err := repo.DB().QueryRowContext(ctx, `
		SELECT released_at FROM task_workspace_group_members
		WHERE workspace_group_id = ? AND task_id = ?
	`, "group-1", "child").Scan(&releasedAt); err != nil {
		t.Fatalf("query workspace group member: %v", err)
	}
	if releasedAt != nil {
		t.Fatalf("workspace group membership released at %s", releasedAt)
	}
}

func TestDetachTaskEventReflectsConcurrentReparent(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createDetachmentFixture(t, ctx, repo)
	eventBus.ClearEvents()
	svc.tasks = &reparentAfterDetachRepository{
		TaskRepository: repo,
		detacher:       repo,
		parentID:       "replacement-parent",
	}

	task, err := svc.DetachTask(ctx, "child")
	if err != nil {
		t.Fatalf("DetachTask: %v", err)
	}
	if task.ParentID != "replacement-parent" {
		t.Fatalf("ParentID = %q, want replacement-parent", task.ParentID)
	}
	eventData := singleDetachmentEventData(t, eventBus)
	if eventData["parent_id"] != "replacement-parent" {
		t.Fatalf("parent_id event field = %#v, want replacement-parent", eventData["parent_id"])
	}
}

func createDetachmentFixture(t *testing.T, ctx context.Context, repo taskEventTestRepository) {
	t.Helper()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace", Name: "Workspace"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "workflow", WorkspaceID: "workspace", Name: "Workflow"}); err != nil {
		t.Fatalf("CreateWorkflow: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "parent", WorkspaceID: "workspace", WorkflowID: "workflow", WorkflowStepID: "step",
		Title: "Parent", Priority: "medium", State: v1.TaskStateTODO,
	}); err != nil {
		t.Fatalf("CreateTask parent: %v", err)
	}
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "child", WorkspaceID: "workspace", WorkflowID: "workflow", WorkflowStepID: "step",
		Title: "Child", Priority: "high", State: v1.TaskStateInProgress, ParentID: "parent",
		Metadata: map[string]interface{}{
			"workspace": map[string]interface{}{
				"mode":     "inherit_parent",
				"group_id": "group-1",
			},
			"unrelated": "keep",
		},
		CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateTask child: %v", err)
	}
}

func singleDetachmentEventData(t *testing.T, eventBus *MockEventBus) map[string]interface{} {
	t.Helper()
	published := eventBus.GetPublishedEvents()
	if len(published) != 1 || published[0].Type != events.TaskUpdated {
		t.Fatalf("published events = %#v, want one task.updated", published)
	}
	data, ok := published[0].Data.(map[string]interface{})
	if !ok {
		t.Fatalf("event data type = %T, want map", published[0].Data)
	}
	return data
}
