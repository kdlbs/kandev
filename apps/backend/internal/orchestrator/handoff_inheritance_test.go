package orchestrator

import (
	"context"
	"errors"
	"strings"
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
	_ = repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-group", TaskID: "parent",
		ExecutorType: string(models.ExecutorTypeLocalDocker), Status: models.TaskEnvironmentStatusReady,
	})

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
	_ = repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-parent", TaskID: "parent",
		ExecutorType: string(models.ExecutorTypeLocalDocker), Status: models.TaskEnvironmentStatusReady,
	})
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
	_ = repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-parent", TaskID: "parent",
		ExecutorType: string(models.ExecutorTypeLocalDocker), Status: models.TaskEnvironmentStatusReady,
	})
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

// REGRESSION: a parent's task_environments row can go missing (deleted by
// a DELETE cascade, or an explicit ResetTaskEnvironment — archive itself
// preserves the row and only tears down its runtime resources) while
// nothing removes or updates the parent's session.TaskEnvironmentID
// pointer. Before this fix, resolveInheritedEnvironment handed that
// dangling id straight back and inheritFromParentEnvironment bound the
// child's session to an environment that no longer exists — the child
// would only discover this much later, at launch time, via an unhelpful
// "workspace reuse is unsafe" error. The fix must fail closed here
// instead, and the error must name the archived parent so the operator
// knows why.
func TestInheritFromParentEnvironment_DanglingEnvironmentFailsClosed(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	ctx := context.Background()
	now := time.Now().UTC()

	ws := &models.Workspace{ID: "ws1", Name: "WS", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkspace(ctx, ws)
	wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkflow(ctx, wf)
	archivedAt := now
	parent := &models.Task{
		ID: "parent", WorkflowID: "wf1", Title: "P", State: v1.TaskStateInProgress,
		ArchivedAt: &archivedAt, CreatedAt: now, UpdatedAt: now,
	}
	_ = repo.CreateTask(ctx, parent)
	child := &models.Task{ID: "child", ParentID: "parent", WorkflowID: "wf1", Title: "C", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateTask(ctx, child)
	// Parent session still points at "env-gone", but (as archive cleanup
	// would leave it) no task_environments row for that id exists.
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "ps1", TaskID: "parent", State: models.TaskSessionStateRunning,
		IsPrimary: true, TaskEnvironmentID: "env-gone", StartedAt: now, UpdatedAt: now,
	})
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "cs1", TaskID: "child", State: models.TaskSessionStateRunning,
		IsPrimary: true, StartedAt: now, UpdatedAt: now,
	})

	err := svc.inheritFromParentEnvironment(ctx, &v1.Task{ID: "child", ParentID: "parent"}, "cs1")
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("inheritFromParentEnvironment error = %v, want workspace reuse unsafe", err)
	}
	if !strings.Contains(err.Error(), "parent") {
		t.Fatalf("error %q does not name the archived parent", err.Error())
	}

	got, getErr := repo.GetTaskSession(ctx, "cs1")
	if getErr != nil || got == nil {
		t.Fatalf("get session: %v", getErr)
	}
	if got.TaskEnvironmentID != "" {
		t.Errorf("child session must not be bound to the dangling environment; got %q", got.TaskEnvironmentID)
	}
}

// REGRESSION (review round on PR #3235): archive deliberately preserves the
// parent's task_environments row and only tears down its runtime resources,
// so taskEnvironmentExists alone cannot detect an archived parent — the row
// genuinely still exists. Before this fix, resolveInheritedEnvironment handed
// that retained id straight back and inheritFromParentEnvironment bound the
// child's session to it, deferring rejection to a much later, more confusing
// failure inside the worktree layer (or, if that layer's checks did not
// trigger, silently launching against a removed workspace). The check must
// reject on the parent's own ArchivedAt state, not merely on row presence.
func TestInheritFromParentEnvironment_RetainedEnvironmentFromArchivedParentFailsClosed(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	ctx := context.Background()
	now := time.Now().UTC()

	ws := &models.Workspace{ID: "ws1", Name: "WS", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkspace(ctx, ws)
	wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateWorkflow(ctx, wf)
	parent := &models.Task{
		ID: "parent", WorkflowID: "wf1", Title: "P", State: v1.TaskStateInProgress,
		CreatedAt: now, UpdatedAt: now,
	}
	_ = repo.CreateTask(ctx, parent)
	// ArchiveTask, not a pre-set struct field: CreateTask's INSERT has no
	// archived_at column, so setting the field before create is silently
	// dropped and the row would come back unarchived.
	if err := repo.ArchiveTask(ctx, "parent"); err != nil {
		t.Fatalf("archive parent: %v", err)
	}
	child := &models.Task{ID: "child", ParentID: "parent", WorkflowID: "wf1", Title: "C", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateTask(ctx, child)
	// Archive preserves the row — it is NOT dangling, unlike the sibling
	// "DanglingEnvironmentFailsClosed" test above.
	_ = repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-parent", TaskID: "parent",
		ExecutorType: string(models.ExecutorTypeLocalDocker), Status: models.TaskEnvironmentStatusReady,
	})
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "ps1", TaskID: "parent", State: models.TaskSessionStateRunning,
		IsPrimary: true, TaskEnvironmentID: "env-parent", StartedAt: now, UpdatedAt: now,
	})
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "cs1", TaskID: "child", State: models.TaskSessionStateRunning,
		IsPrimary: true, StartedAt: now, UpdatedAt: now,
	})

	err := svc.inheritFromParentEnvironment(ctx, &v1.Task{ID: "child", ParentID: "parent"}, "cs1")
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("inheritFromParentEnvironment error = %v, want workspace reuse unsafe", err)
	}
	if !strings.Contains(err.Error(), "parent") || !strings.Contains(err.Error(), "archived") {
		t.Fatalf("error %q does not name the archived parent", err.Error())
	}

	got, getErr := repo.GetTaskSession(ctx, "cs1")
	if getErr != nil || got == nil {
		t.Fatalf("get session: %v", getErr)
	}
	if got.TaskEnvironmentID != "" {
		t.Errorf("child session must not be bound to the archived parent's retained environment; got %q", got.TaskEnvironmentID)
	}
}

func TestInheritFromParentEnvironment_RejectsEnvironmentOwnedByArchivedAncestor(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	ctx := context.Background()
	now := time.Now().UTC()
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-nested", Name: "WS", CreatedAt: now, UpdatedAt: now})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-nested", WorkspaceID: "ws-nested", Name: "WF", CreatedAt: now, UpdatedAt: now})
	ancestor := &models.Task{ID: "ancestor", WorkspaceID: "ws-nested", WorkflowID: "wf-nested", Title: "A", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateTask(ctx, ancestor)
	_ = repo.ArchiveTask(ctx, ancestor.ID)
	parent := &models.Task{ID: "parent", ParentID: ancestor.ID, WorkspaceID: "ws-nested", WorkflowID: "wf-nested", Title: "P", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateTask(ctx, parent)
	child := &models.Task{ID: "child", ParentID: parent.ID, WorkspaceID: "ws-nested", WorkflowID: "wf-nested", Title: "C", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateTask(ctx, child)
	_ = repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{
		ID: "env-ancestor", TaskID: ancestor.ID,
		ExecutorType: string(models.ExecutorTypeLocalDocker), Status: models.TaskEnvironmentStatusReady,
	})
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "parent-session", TaskID: parent.ID, State: models.TaskSessionStateRunning,
		IsPrimary: true, TaskEnvironmentID: "env-ancestor", StartedAt: now, UpdatedAt: now,
	})
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "child-session", TaskID: child.ID, State: models.TaskSessionStateRunning,
		IsPrimary: true, StartedAt: now, UpdatedAt: now,
	})

	err := svc.inheritFromParentEnvironment(ctx, &v1.Task{ID: child.ID, ParentID: parent.ID}, "child-session")
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("inheritFromParentEnvironment error = %v, want workspace reuse unsafe", err)
	}
	got, getErr := repo.GetTaskSession(ctx, "child-session")
	if getErr != nil || got == nil {
		t.Fatalf("get child session: %v", getErr)
	}
	if got.TaskEnvironmentID != "" {
		t.Fatalf("child session bound to ancestor environment %q", got.TaskEnvironmentID)
	}
}

// REGRESSION: the workspace_group fallback branch of resolveInheritedEnvironment
// used to hand back GetSharedGroupEnvironment's id without checking it still
// names a live task_environments row — the exact dangling-id class the
// parent_session branch above was fixed to reject. In the card's primary
// scenario this is not a hypothetical: an inherit_parent child is a member of
// its parent's workspace group, and MarkOwnerSessionMaterialized records the
// parent as that group's owner, so the group fallback normally returns the
// parent's own environment id right back — the same id the parent_session
// branch just rejected one check earlier, whether it is now dangling (the
// row was deleted by something other than archive, which preserves it) or
// merely live-but-useless. Both branches must fail closed the same way.
func TestInheritFromParentEnvironment_DanglingGroupEnvironmentFailsClosed(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.SetWorkspaceMaterializer(&stubMaterializer{
		envByTask: map[string]string{"child": "env-group-gone"},
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

	// Parent has NO sessions, so the parent_session branch is skipped and
	// the group fallback is reached. The materializer names an env id with
	// no matching task_environments row — a dangling reference, regardless
	// of what deleted the row (archive itself never does).
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "cs1", TaskID: "child", State: models.TaskSessionStateRunning,
		IsPrimary: true, StartedAt: now, UpdatedAt: now,
	})

	err := svc.inheritFromParentEnvironment(ctx, &v1.Task{ID: "child", ParentID: "parent"}, "cs1")
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("inheritFromParentEnvironment error = %v, want workspace reuse unsafe", err)
	}

	got, getErr := repo.GetTaskSession(ctx, "cs1")
	if getErr != nil || got == nil {
		t.Fatalf("get session: %v", getErr)
	}
	if got.TaskEnvironmentID != "" {
		t.Errorf("child session must not be bound to the dangling group environment; got %q", got.TaskEnvironmentID)
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
	_ = repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "env-group", TaskID: "member", Status: models.TaskEnvironmentStatusReady})
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

func TestInheritFromSharedGroupEnvironment_RejectsArchivedOwner(t *testing.T) {
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	ctx := context.Background()
	now := time.Now().UTC()
	archivedAt := now
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-shared-archived", Name: "WS", CreatedAt: now, UpdatedAt: now})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-shared-archived", WorkspaceID: "ws-shared-archived", Name: "WF", CreatedAt: now, UpdatedAt: now})
	owner := &models.Task{ID: "owner", WorkspaceID: "ws-shared-archived", WorkflowID: "wf-shared-archived", Title: "owner", State: v1.TaskStateInProgress, ArchivedAt: &archivedAt, CreatedAt: now, UpdatedAt: now}
	_ = repo.CreateTask(ctx, owner)
	_ = repo.ArchiveTask(ctx, owner.ID)
	_ = repo.CreateTask(ctx, &models.Task{ID: "member", WorkspaceID: "ws-shared-archived", WorkflowID: "wf-shared-archived", Title: "member", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now})
	_ = repo.CreateTaskEnvironment(ctx, &models.TaskEnvironment{ID: "env-owner", TaskID: owner.ID, Status: models.TaskEnvironmentStatusReady})
	_ = repo.CreateTaskSession(ctx, &models.TaskSession{ID: "member-session", TaskID: "member", State: models.TaskSessionStateCreated, StartedAt: now, UpdatedAt: now})
	svc.SetWorkspaceMaterializer(&stubMaterializer{envByTask: map[string]string{"member": "env-owner"}})

	err := svc.inheritFromSharedGroup(ctx, &v1.Task{ID: "member"}, "member-session")
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("inheritFromSharedGroup error = %v, want workspace reuse unsafe", err)
	}
	got, getErr := repo.GetTaskSession(ctx, "member-session")
	if getErr != nil || got == nil {
		t.Fatalf("get member session: %v", getErr)
	}
	if got.TaskEnvironmentID != "" {
		t.Fatalf("member session bound to archived owner environment %q", got.TaskEnvironmentID)
	}
}
