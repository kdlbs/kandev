package orchestrator

import (
	"context"
	"errors"
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

// RC1 regression: inherited-environment propagation must run on the direct
// start path (prepareSessionForStart), not only PrepareTaskSession. MCP-created
// subtasks auto-start through startTask, which prepares the session via
// prepareSessionForStart. Without propagation there, an inherit_parent subtask
// would provision a fresh worktree instead of reusing the parent's.
func TestPrepareSessionForStart_PropagatesInheritedEnvironment(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{repoForExecutionLookup: repo})
	ctx := context.Background()
	now := time.Now().UTC()

	ws := &models.Workspace{ID: "ws1", Name: "WS", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkspace(ctx, ws)
	wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkflow(ctx, wf)
	parent := &models.Task{ID: "parent", WorkspaceID: "ws1", WorkflowID: "wf1", Title: "P", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateTask(ctx, parent)
	child := &models.Task{ID: "child", ParentID: "parent", WorkspaceID: "ws1", WorkflowID: "wf1", Title: "C", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateTask(ctx, child)
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "ps1", TaskID: "parent", State: models.TaskSessionStateRunning,
		IsPrimary: true, TaskEnvironmentID: "env-parent", StartedAt: now, UpdatedAt: now,
	})

	childTask := &v1.Task{
		ID:       "child",
		ParentID: "parent",
		Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}},
	}
	sessionID, _, err := svc.prepareSessionForStart(ctx, childTask, "profile-1", "profile-1", "exec-1", "", "")
	if err != nil {
		t.Fatalf("prepareSessionForStart: %v", err)
	}

	got, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil || got == nil {
		t.Fatalf("get session: %v", err)
	}
	if got.TaskEnvironmentID != "env-parent" {
		t.Errorf("TaskEnvironmentID = %q, want env-parent (inherited via start path)", got.TaskEnvironmentID)
	}
}

func TestPrepareSessionForStart_InheritedWorkspaceMissingCompensatesSession(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), newMockTaskRepo(), &mockAgentManager{repoForExecutionLookup: repo})
	ctx := context.Background()
	now := time.Now().UTC()
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-missing", Name: "WS", CreatedAt: now, UpdatedAt: now})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-missing", WorkspaceID: "ws-missing", Name: "WF", CreatedAt: now, UpdatedAt: now})
	_ = repo.CreateTask(ctx, &models.Task{ID: "parent-missing", WorkspaceID: "ws-missing", WorkflowID: "wf-missing", Title: "P", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now})
	_ = repo.CreateTask(ctx, &models.Task{ID: "child-missing", ParentID: "parent-missing", WorkspaceID: "ws-missing", WorkflowID: "wf-missing", Title: "C", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now})

	_, created, err := svc.prepareSessionForStart(ctx, &v1.Task{
		ID:       "child-missing",
		ParentID: "parent-missing",
		Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}},
	}, "profile-1", "profile-1", "exec-1", "", "")
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("prepareSessionForStart error = %v, want workspace reuse unsafe", err)
	}
	if created {
		t.Fatal("failed inherited launch must not report a created session")
	}
	sessions, listErr := repo.ListTaskSessions(ctx, "child-missing")
	if listErr != nil || len(sessions) != 0 {
		t.Fatalf("sessions after rejected inherited launch = %+v, %v", sessions, listErr)
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

func TestInheritFromSharedGroupEnvironment_BindsCanonicalEnvironment(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetWorkspaceMaterializer(&stubMaterializer{envByTask: map[string]string{"member": "env-group"}})
	ctx := context.Background()
	now := time.Now().UTC()
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-shared", Name: "WS", CreatedAt: now, UpdatedAt: now})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-shared", WorkspaceID: "ws-shared", Name: "WF", CreatedAt: now, UpdatedAt: now})
	_ = repo.CreateTask(ctx, &models.Task{ID: "member", WorkspaceID: "ws-shared", WorkflowID: "wf-shared", Title: "member", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now})
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{ID: "member-session", TaskID: "member", State: models.TaskSessionStateCreated, StartedAt: now, UpdatedAt: now})

	if err := svc.inheritFromSharedGroup(ctx, &v1.Task{ID: "member"}, "member-session"); err != nil {
		t.Fatalf("inherit shared group: %v", err)
	}
	got, err := repo.GetTaskSession(ctx, "member-session")
	if err != nil || got.TaskEnvironmentID != "env-group" {
		t.Fatalf("shared environment binding = %+v, %v", got, err)
	}
}

func TestInheritFromSharedGroupEnvironment_MissingResolverFailsClosed(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	err := svc.inheritFromSharedGroup(context.Background(), &v1.Task{ID: "member"}, "session")
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("shared group error = %v, want workspace reuse unsafe", err)
	}
}
