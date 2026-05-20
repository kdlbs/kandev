package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// stubMaterializer is an in-test WorkspaceMaterializer that returns a
// canned env id for the configured task and records Mark calls.
type stubMaterializer struct {
	envByTask map[string]string
	marked    []string
}

func (m *stubMaterializer) MarkOwnerSessionMaterialized(_ context.Context, taskID string) {
	m.marked = append(m.marked, taskID)
}

func (m *stubMaterializer) GetSharedGroupEnvironment(_ context.Context, taskID string) string {
	return m.envByTask[taskID]
}

// REGRESSION (post-review #2): inherit_parent must fall back to the
// workspace group's MaterializedEnvironmentID when the parent task has
// no live primary session. Without this fallback, a child re-launching
// after the parent's session was cleared would silently get a fresh env
// and the workspace-inheritance contract would break.
func TestInheritFromParentEnvironment_FallsBackToGroupEnv(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetWorkspaceMaterializer(&stubMaterializer{
		envByTask: map[string]string{"child": "env-group"},
	})
	ctx := context.Background()
	now := time.Now().UTC()

	ws := &models.Workspace{ID: "ws1", Name: "WS", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkspace(ctx, ws)
	wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkflow(ctx, wf)
	parent := &models.Task{ID: "parent", WorkflowID: "wf1", Title: "P", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateTask(ctx, parent)
	child := &models.Task{ID: "child", ParentID: "parent", WorkflowID: "wf1", Title: "C", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateTask(ctx, child)

	// Parent intentionally has NO sessions — the fallback path must
	// kick in and consult the materializer for the child's group env.
	childSession := &models.TaskSession{
		ID: "cs1", TaskID: "child", State: models.TaskSessionStateRunning,
		IsPrimary: true, StartedAt: now, UpdatedAt: now,
	}
	_ = repo.CreateTaskSession(ctx, childSession)

	task := &v1.Task{ID: "child", ParentID: "parent"}
	svc.inheritFromParentEnvironment(ctx, task, "cs1")

	got, err := repo.GetTaskSession(ctx, "cs1")
	if err != nil || got == nil {
		t.Fatalf("get session: %v", err)
	}
	if got.TaskEnvironmentID != "env-group" {
		t.Errorf("TaskEnvironmentID = %q, want env-group (group fallback)", got.TaskEnvironmentID)
	}
}

// When the parent has a primary session with an env, that takes
// precedence over the group fallback.
func TestInheritFromParentEnvironment_ParentSessionWins(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetWorkspaceMaterializer(&stubMaterializer{
		envByTask: map[string]string{"child": "env-group"},
	})
	ctx := context.Background()
	now := time.Now().UTC()

	ws := &models.Workspace{ID: "ws1", Name: "WS", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkspace(ctx, ws)
	wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkflow(ctx, wf)
	parent := &models.Task{ID: "parent", WorkflowID: "wf1", Title: "P", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateTask(ctx, parent)
	child := &models.Task{ID: "child", ParentID: "parent", WorkflowID: "wf1", Title: "C", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateTask(ctx, child)
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "ps1", TaskID: "parent", State: models.TaskSessionStateRunning,
		IsPrimary: true, TaskEnvironmentID: "env-parent",
		StartedAt: now, UpdatedAt: now,
	})
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "cs1", TaskID: "child", State: models.TaskSessionStateRunning,
		IsPrimary: true, StartedAt: now, UpdatedAt: now,
	})

	svc.inheritFromParentEnvironment(ctx, &v1.Task{ID: "child", ParentID: "parent"}, "cs1")

	got, _ := repo.GetTaskSession(ctx, "cs1")
	if got.TaskEnvironmentID != "env-parent" {
		t.Errorf("parent session env should win; got %q", got.TaskEnvironmentID)
	}
}
