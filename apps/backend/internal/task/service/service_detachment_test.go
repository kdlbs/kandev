package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
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

// @covers AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.1
func TestDetachTaskTransfersSharedWorkspaceStewardship(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	createDetachmentFixture(t, ctx, repo)
	if _, err := repo.DB().ExecContext(ctx, `
		UPDATE task_workspace_groups SET materialized_environment_id = ? WHERE id = ?
	`, "env-1", "group-1"); err != nil {
		t.Fatalf("update workspace group environment: %v", err)
	}
	if err := repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-1", TaskID: "parent", ExecutorType: string(models.ExecutorTypeWorktree),
		WorkspacePath: t.TempDir(), Status: models.TaskEnvironmentStatusReady,
	}); err != nil {
		t.Fatalf("CreateTaskEnvironment: %v", err)
	}

	if _, err := svc.DetachTask(ctx, "child"); err != nil {
		t.Fatalf("DetachTask: %v", err)
	}

	var groupOwner, environmentOwner string
	var groupGeneration, environmentGeneration int64
	if err := repo.DB().QueryRowContext(ctx,
		`SELECT owner_task_id, ownership_generation FROM task_workspace_groups WHERE id = ?`, "group-1",
	).Scan(&groupOwner, &groupGeneration); err != nil {
		t.Fatalf("query workspace group owner: %v", err)
	}
	if err := repo.DB().QueryRowContext(ctx,
		`SELECT task_id, ownership_generation FROM task_environments WHERE id = ?`, "env-1",
	).Scan(&environmentOwner, &environmentGeneration); err != nil {
		t.Fatalf("query task environment owner: %v", err)
	}
	if groupOwner != "child" || environmentOwner != "child" {
		t.Fatalf("owners after detach = group %q, environment %q; want child, child", groupOwner, environmentOwner)
	}
	var parentRole, childRole string
	if err := repo.DB().QueryRowContext(ctx, `
		SELECT role FROM task_workspace_group_members WHERE workspace_group_id = ? AND task_id = ?
	`, "group-1", "parent").Scan(&parentRole); err != nil {
		t.Fatalf("query parent membership role: %v", err)
	}
	if err := repo.DB().QueryRowContext(ctx, `
		SELECT role FROM task_workspace_group_members WHERE workspace_group_id = ? AND task_id = ?
	`, "group-1", "child").Scan(&childRole); err != nil {
		t.Fatalf("query child membership role: %v", err)
	}
	if parentRole != "member" || childRole != "owner" {
		t.Fatalf("membership roles after detach = parent %q, child %q; want member, owner", parentRole, childRole)
	}
	if groupGeneration != 2 || environmentGeneration != 2 {
		t.Fatalf("generations after detach = group %d, environment %d; want 2, 2", groupGeneration, environmentGeneration)
	}

	if _, err := svc.DetachTask(ctx, "child"); err != nil {
		t.Fatalf("idempotent DetachTask: %v", err)
	}
	if err := repo.DB().QueryRowContext(ctx,
		`SELECT ownership_generation FROM task_workspace_groups WHERE id = ?`, "group-1",
	).Scan(&groupGeneration); err != nil {
		t.Fatalf("query workspace group generation after retry: %v", err)
	}
	if err := repo.DB().QueryRowContext(ctx,
		`SELECT ownership_generation FROM task_environments WHERE id = ?`, "env-1",
	).Scan(&environmentGeneration); err != nil {
		t.Fatalf("query task environment generation after retry: %v", err)
	}
	if groupGeneration != 2 || environmentGeneration != 2 {
		t.Fatalf("generations after retry = group %d, environment %d; want 2, 2", groupGeneration, environmentGeneration)
	}
}

// @covers AC-TASKS-DETACHED-WORKSPACE-CONTINUITY-001.2
func TestDetachTaskRollsBackHierarchyWhenWorkspaceTransferFails(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	createDetachmentFixture(t, ctx, repo)
	if _, err := repo.DB().ExecContext(ctx, `
		UPDATE task_workspace_groups SET materialized_environment_id = ? WHERE id = ?
	`, "missing-environment", "group-1"); err != nil {
		t.Fatalf("update workspace group environment: %v", err)
	}

	if _, err := svc.DetachTask(ctx, "child"); err == nil {
		t.Fatal("DetachTask succeeded with a missing canonical environment")
	}
	child, err := repo.GetTask(ctx, "child")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if child.ParentID != "parent" {
		t.Fatalf("child parent after rollback = %q, want parent", child.ParentID)
	}
	var owner string
	var generation int64
	if err := repo.DB().QueryRowContext(ctx, `
		SELECT owner_task_id, ownership_generation FROM task_workspace_groups WHERE id = ?
	`, "group-1").Scan(&owner, &generation); err != nil {
		t.Fatalf("query workspace group after rollback: %v", err)
	}
	if owner != "parent" || generation != 1 {
		t.Fatalf("workspace group after rollback = owner %q generation %d; want parent, 1", owner, generation)
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

func TestDetachTaskPublishesOfficeTaskUpdated(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createDetachmentFixture(t, ctx, repo)
	eventBus.ClearEvents()

	if _, err := svc.DetachTask(ctx, "child"); err != nil {
		t.Fatalf("DetachTask: %v", err)
	}

	for _, event := range eventBus.GetPublishedEvents() {
		if event.Type != events.OfficeTaskUpdated {
			continue
		}
		data, ok := event.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("office event data type = %T, want map", event.Data)
		}
		if data["task_id"] != "child" || data["workspace_id"] != "workspace" {
			t.Fatalf("office event identity = %#v", data)
		}
		fields, ok := data["fields"].([]string)
		if !ok || len(fields) != 2 || fields[0] != "parent_id" || fields[1] != "metadata" {
			t.Fatalf("office event fields = %#v, want parent_id and metadata", data["fields"])
		}
		return
	}
	t.Fatal("office.task.updated event was not published")
}

func createDetachmentFixture(t *testing.T, ctx context.Context, repo *sqliterepo.Repository) {
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
	now := time.Now().UTC()
	if _, err := repo.DB().ExecContext(ctx, `
		INSERT INTO task_workspace_groups
			(id, workspace_id, owner_task_id, materialized_kind, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, "group-1", "workspace", "parent", "single_repo", now, now); err != nil {
		t.Fatalf("insert workspace group: %v", err)
	}
	for _, member := range []struct {
		taskID string
		role   string
	}{{"parent", "owner"}, {"child", "member"}} {
		if _, err := repo.DB().ExecContext(ctx, `
			INSERT INTO task_workspace_group_members (workspace_group_id, task_id, role, created_at)
			VALUES (?, ?, ?, ?)
		`, "group-1", member.taskID, member.role, now); err != nil {
			t.Fatalf("insert workspace group member %s: %v", member.taskID, err)
		}
	}
}

func singleDetachmentEventData(t *testing.T, eventBus *MockEventBus) map[string]interface{} {
	t.Helper()
	published := eventBus.GetPublishedEvents()
	for _, event := range published {
		if event.Type != events.TaskUpdated {
			continue
		}
		data, ok := event.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("event data type = %T, want map", event.Data)
		}
		return data
	}
	t.Fatalf("published events = %#v, want task.updated", published)
	return nil
}
