package executor

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// TestResolveResumeTaskEnvironment_ReusesInheritedEnvironment is a regression
// test for "UNIQUE constraint failed: task_environments.id" on resume. A child
// task created by an office task-handoff (inherit_parent / shared_group) has its
// session.TaskEnvironmentID rewritten to point at the parent's / group's env
// row, which is owned by a *different* task. GetTaskEnvironmentByTaskID(childID)
// therefore returns nil, and without the inherited-env fallback the resume path
// treated the env as absent and re-created a row using the inherited ID, which
// already exists. The resolver must instead return the inherited row so the
// persist path takes its update branch.
func TestResolveResumeTaskEnvironment_ReusesInheritedEnvironment(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	repo.taskEnvironments["env-parent"] = &models.TaskEnvironment{
		ID:           "env-parent",
		TaskID:       "task-parent",
		ExecutorType: string(models.ExecutorTypeLocal),
		Status:       models.TaskEnvironmentStatusReady,
	}
	repo.tasks["task-parent"] = &models.Task{ID: "task-parent"}
	session := &models.TaskSession{
		ID:                "sess-child",
		TaskID:            "task-child",
		TaskEnvironmentID: "env-parent",
	}

	env, err := exec.resolveResumeTaskEnvironment(context.Background(), "task-child", session)
	if err != nil {
		t.Fatalf("resolveResumeTaskEnvironment: %v", err)
	}
	if env == nil || env.ID != "env-parent" {
		t.Fatalf("resolved env = %+v, want inherited env-parent", env)
	}
	// Provenance: the returned row must be the parent's, not a freshly created
	// child-owned row that happened to reuse the inherited ID.
	if env.TaskID != "task-parent" {
		t.Fatalf("env.TaskID = %q, want task-parent (should be the inherited parent row)", env.TaskID)
	}
	if len(repo.createTaskEnvironmentCalls) != 0 {
		t.Fatalf("expected no CreateTaskEnvironment calls, got %d", len(repo.createTaskEnvironmentCalls))
	}
	if session.TaskEnvironmentID != "env-parent" {
		t.Fatalf("session.TaskEnvironmentID = %q, want env-parent", session.TaskEnvironmentID)
	}
}

func TestPrepareResumeRepositorySettings_RejectsInheritedExecutorMismatch(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	repo.tasks["task-parent"] = &models.Task{ID: "task-parent"}
	repo.taskEnvironments["env-parent"] = &models.TaskEnvironment{
		ID:           "env-parent",
		TaskID:       "task-parent",
		ExecutorType: string(models.ExecutorTypeLocal),
		Status:       models.TaskEnvironmentStatusReady,
	}
	task := &v1.Task{
		ID:          "task-child",
		WorkspaceID: "ws-1",
		Metadata: map[string]interface{}{
			"workspace": map[string]interface{}{"mode": "shared_group"},
		},
	}
	session := &models.TaskSession{
		ID:                "sess-child",
		TaskID:            task.ID,
		AgentProfileID:    "profile-123",
		TaskEnvironmentID: "env-parent",
	}
	req := &LaunchAgentRequest{
		TaskID:       task.ID,
		ExecutorType: string(models.ExecutorTypeLocalDocker),
	}

	_, _, _, err := exec.prepareResumeRepositorySettings(context.Background(), task, session, req, ResumeOptions{})
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("prepareResumeRepositorySettings() error = %v, want ErrWorkspaceReuseUnsafe", err)
	}
	if session.TaskEnvironmentID != "env-parent" {
		t.Fatalf("session TaskEnvironmentID = %q, want env-parent", session.TaskEnvironmentID)
	}
}

// TestResolveResumeTaskEnvironment_MissingReferenceFallsThroughToCreate covers a
// session that references an env ID with no matching row (deleted env, or an env
// that was never persisted). The SQLite repo signals this with the production
// ErrTaskEnvironmentNotFound sentinel (not a nil,nil miss), so the resolver must
// treat that sentinel as absent, return nil, and let the caller create a fresh
// row using the free ID — preserving resume rather than aborting it.
func TestResolveResumeTaskEnvironment_MissingReferenceFallsThroughToCreate(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	// Mirror the sqlite repository, which returns ErrTaskEnvironmentNotFound
	// (wrapped) rather than (nil, nil) when the referenced row is absent.
	repo.getTaskEnvironmentFunc = func(_ context.Context, _ string) (*models.TaskEnvironment, error) {
		return nil, repoerrors.ErrTaskEnvironmentNotFound
	}

	session := &models.TaskSession{
		ID:                "sess-child",
		TaskID:            "task-child",
		TaskEnvironmentID: "env-missing",
	}

	env, err := exec.resolveResumeTaskEnvironment(context.Background(), "task-child", session)
	if err != nil {
		t.Fatalf("resolveResumeTaskEnvironment: %v", err)
	}
	if env != nil {
		t.Fatalf("resolved env = %+v, want nil so the create path runs", env)
	}
}

func TestResolveResumeTaskEnvironment_InheritedMissingReferenceFailsClosed(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	repo.getTaskEnvironmentFunc = func(_ context.Context, _ string) (*models.TaskEnvironment, error) {
		return nil, repoerrors.ErrTaskEnvironmentNotFound
	}

	session := &models.TaskSession{
		ID:                "sess-child",
		TaskID:            "task-child",
		TaskEnvironmentID: "env-missing",
	}
	repo.tasks[session.TaskID] = &models.Task{
		ID:       session.TaskID,
		Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}},
	}

	env, err := exec.resolveResumeTaskEnvironment(context.Background(), session.TaskID, session)
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("resolveResumeTaskEnvironmentForTask() error = %v, want ErrWorkspaceReuseUnsafe", err)
	}
	if env != nil {
		t.Fatalf("resolved env = %+v, want nil", env)
	}
}

func TestResolveResumeTaskEnvironment_RejectsArchivedInheritedOwner(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	archivedAt := time.Now()
	repo.tasks["task-parent"] = &models.Task{ID: "task-parent", ArchivedAt: &archivedAt}
	repo.taskEnvironments["env-parent"] = &models.TaskEnvironment{
		ID:     "env-parent",
		TaskID: "task-parent",
		Status: models.TaskEnvironmentStatusReady,
	}

	session := &models.TaskSession{
		ID:                "sess-child",
		TaskID:            "task-child",
		TaskEnvironmentID: "env-parent",
	}
	repo.tasks[session.TaskID] = &models.Task{
		ID:       session.TaskID,
		Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}},
	}

	env, err := exec.resolveResumeTaskEnvironment(context.Background(), session.TaskID, session)
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("resolveResumeTaskEnvironmentForTask() error = %v, want ErrWorkspaceReuseUnsafe", err)
	}
	if env != nil {
		t.Fatalf("resolved env = %+v, want nil", env)
	}
}

func TestResolveResumeTaskEnvironment_RejectsUnverifiableInheritedOwner(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	repo.taskEnvironments["env-parent"] = &models.TaskEnvironment{
		ID:     "env-parent",
		TaskID: "task-parent",
		Status: models.TaskEnvironmentStatusReady,
	}
	repo.getTaskFunc = func(_ context.Context, id string) (*models.Task, error) {
		if id == "task-child" {
			return &models.Task{
				ID:       id,
				Metadata: map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}},
			}, nil
		}
		return nil, errors.New("owner lookup failed")
	}

	session := &models.TaskSession{
		ID:                "sess-child",
		TaskID:            "task-child",
		TaskEnvironmentID: "env-parent",
	}

	env, err := exec.resolveResumeTaskEnvironment(context.Background(), session.TaskID, session)
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("resolveResumeTaskEnvironment() error = %v, want ErrWorkspaceReuseUnsafe", err)
	}
	if env != nil {
		t.Fatalf("resolved env = %+v, want nil", env)
	}
}

// TestResolveResumeTaskEnvironment_OwnedEnvironmentUnchanged locks in the
// pre-existing behavior for a normal (non-inherited) session: the env is found
// by task id and no inherited fallback lookup occurs.
func TestResolveResumeTaskEnvironment_OwnedEnvironmentUnchanged(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	repo.taskEnvironments["env-own"] = &models.TaskEnvironment{
		ID:           "env-own",
		TaskID:       "task-1",
		ExecutorType: string(models.ExecutorTypeLocal),
		Status:       models.TaskEnvironmentStatusReady,
	}
	session := &models.TaskSession{ID: "sess-1", TaskID: "task-1"}

	env, err := exec.resolveResumeTaskEnvironment(context.Background(), "task-1", session)
	if err != nil {
		t.Fatalf("resolveResumeTaskEnvironment: %v", err)
	}
	if env == nil || env.ID != "env-own" {
		t.Fatalf("resolved env = %+v, want env-own", env)
	}
	if session.TaskEnvironmentID != "env-own" {
		t.Fatalf("session.TaskEnvironmentID = %q, want env-own", session.TaskEnvironmentID)
	}
}

// TestResolveResumeTaskEnvironment_NoEnvironmentAndNoReference returns nil so the
// caller can create a fresh environment (the ordinary cold-resume case).
func TestResolveResumeTaskEnvironment_NoEnvironmentAndNoReference(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	session := &models.TaskSession{ID: "sess-1", TaskID: "task-1"}

	env, err := exec.resolveResumeTaskEnvironment(context.Background(), "task-1", session)
	if err != nil {
		t.Fatalf("resolveResumeTaskEnvironment: %v", err)
	}
	if env != nil {
		t.Fatalf("resolved env = %+v, want nil", env)
	}
}

// TestPersistTaskEnvironment_GuestDoesNotMutateOwnerEnvironment is a regression
// test for "persist task environment: ready status requires repository
// inventory" on resume. A guest session (office inherit_parent / shared_group)
// attaches to a canonical environment owned by a *different* task. The guest's
// resume request carries no repo specs and the owner's inventory can be empty,
// so the repo-backed ready-status guard used to fail a resume the guest has no
// authority to fix. The guest must attach without touching the owner's row.
func TestPersistTaskEnvironment_GuestDoesNotMutateOwnerEnvironment(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	// The child is repo-backed, which is what previously tripped the guard.
	repo.taskRepositories["tr-child"] = &models.TaskRepository{
		ID: "tr-child", TaskID: "task-child", RepositoryID: "repo-a",
	}
	owner := &models.TaskEnvironment{
		ID:           "env-parent",
		TaskID:       "task-parent",
		ExecutorType: string(models.ExecutorTypeLocal),
		Status:       models.TaskEnvironmentStatusReady,
	}
	repo.taskEnvironments["env-parent"] = owner
	session := &models.TaskSession{ID: "sess-child", TaskID: "task-child", TaskEnvironmentID: "env-parent"}
	req := &LaunchAgentRequest{TaskID: "task-child", ExecutorType: string(models.ExecutorTypeLocal)}
	resp := &LaunchAgentResponse{}

	if err := exec.persistTaskEnvironment(context.Background(), "task-child", session, owner, req, resp, executorConfig{}); err != nil {
		t.Fatalf("persistTaskEnvironment (guest): %v", err)
	}
	if len(repo.updateTaskEnvironmentCalls) != 0 {
		t.Fatalf("guest must not update the owner env, got %d UpdateTaskEnvironment calls", len(repo.updateTaskEnvironmentCalls))
	}
	if len(repo.createTaskEnvironmentCalls) != 0 {
		t.Fatalf("guest must not create an env, got %d CreateTaskEnvironment calls", len(repo.createTaskEnvironmentCalls))
	}
	if len(repo.finalizeTaskEnvironmentCalls) != 0 {
		t.Fatalf("guest is not the materializer, got %d finalize calls", len(repo.finalizeTaskEnvironmentCalls))
	}
	if len(repo.writeCallLog) != 0 {
		t.Fatalf("guest must not write repo rows, write log = %v", repo.writeCallLog)
	}
	if session.TaskEnvironmentID != "env-parent" {
		t.Fatalf("session.TaskEnvironmentID = %q, want env-parent", session.TaskEnvironmentID)
	}
}

// TestPersistTaskEnvironment_GuestWorktreeStampsSharedTaskDirName covers a
// sessionless inherited subtask that materializes the first physical worktree
// in an environment created by its parent. The environment row is shared, but
// its stable task directory name is still needed for ownership validation when
// the parent is later reset.
func TestPersistTaskEnvironment_GuestWorktreeStampsSharedTaskDirName(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	owner := &models.TaskEnvironment{
		ID:           "env-parent",
		TaskID:       "task-parent",
		ExecutorType: string(models.ExecutorTypeLocal),
		Status:       models.TaskEnvironmentStatusReady,
		Repos: []*models.TaskEnvironmentRepo{{
			TaskEnvironmentID: "env-parent",
			RepositoryID:      "repo-a",
			WorktreeID:        "wt-parent",
		}},
	}
	repo.taskEnvironments[owner.ID] = owner
	session := &models.TaskSession{ID: "sess-child", TaskID: "task-child", TaskEnvironmentID: owner.ID}
	req := &LaunchAgentRequest{
		TaskID:       "task-child",
		ExecutorType: string(models.ExecutorTypeLocal),
		UseWorktree:  true,
		TaskDirName:  "sessionless-child_abc",
		RepositoryID: "repo-a",
	}
	resp := &LaunchAgentResponse{
		WorktreeID:     "wt-child",
		WorktreePath:   "/tasks/sessionless-child_abc/repo-a",
		WorktreeBranch: "feature/sessionless-child",
	}

	if err := exec.persistTaskEnvironment(context.Background(), "task-child", session, owner, req, resp, executorConfig{}); err != nil {
		t.Fatalf("persistTaskEnvironment (guest worktree): %v", err)
	}
	if owner.TaskDirName != "sessionless-child_abc" {
		t.Fatalf("owner TaskDirName = %q, want sessionless-child_abc", owner.TaskDirName)
	}
	if len(repo.updateTaskEnvironmentCalls) != 0 {
		t.Fatalf("guest worktree must not rewrite the shared env, got %d UpdateTaskEnvironment calls", len(repo.updateTaskEnvironmentCalls))
	}
	if len(repo.writeCallLog) != 1 || repo.writeCallLog[0] != "stamp_task_dir" {
		t.Fatalf("guest worktree writes = %v, want only the environment stamp", repo.writeCallLog)
	}
}

func TestClaimSharedTaskEnvironmentTaskDirNameRejectsConflictingWinner(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	repo.taskEnvironments["env-parent"] = &models.TaskEnvironment{
		ID: "env-parent", TaskID: "task-parent", TaskDirName: "winner-root_abc",
	}

	env := *repo.taskEnvironments["env-parent"]
	env.TaskDirName = ""
	err := exec.claimSharedTaskEnvironmentTaskDirName(context.Background(), &env, &LaunchAgentRequest{
		TaskID: "task-child", UseWorktree: true, TaskDirName: "loser-root_def",
	})
	if !errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("claimSharedTaskEnvironmentTaskDirName error = %v, want ErrWorkspaceReuseUnsafe", err)
	}
}

// TestPersistTaskEnvironment_GuestMaterializerRunsFinalizePath proves the guest
// short-circuit does not swallow the shared_group case where a member session is
// elected to materialize a still-CREATING canonical environment. Even though the
// environment is owned by a different task, the elected materializer must run the
// normal finalize path so the group's inventory publishes atomically.
func TestPersistTaskEnvironment_GuestMaterializerRunsFinalizePath(t *testing.T) {
	repo := newMockRepository()
	exec := newTestExecutor(t, &mockAgentManager{}, repo)

	owner := &models.TaskEnvironment{
		ID:                       "env-group",
		TaskID:                   "task-owner",
		ExecutorType:             string(models.ExecutorTypeLocal),
		Status:                   models.TaskEnvironmentStatusCreating,
		MaterializationSessionID: "sess-member",
	}
	repo.taskEnvironments["env-group"] = owner
	session := &models.TaskSession{ID: "sess-member", TaskID: "task-member", TaskEnvironmentID: "env-group"}
	req := &LaunchAgentRequest{TaskID: "task-member", ExecutorType: string(models.ExecutorTypeLocal), RepositoryID: "repo-a"}
	resp := &LaunchAgentResponse{WorktreeID: "wt-a", WorktreePath: "/tasks/group/repo-a", WorktreeBranch: "feat/x"}

	if err := exec.persistTaskEnvironment(context.Background(), "task-member", session, owner, req, resp, executorConfig{}); err != nil {
		t.Fatalf("persistTaskEnvironment (guest materializer): %v", err)
	}
	if len(repo.finalizeTaskEnvironmentCalls) != 1 {
		t.Fatalf("elected materializer must finalize, got %d finalize calls", len(repo.finalizeTaskEnvironmentCalls))
	}
}

type retainedLocalFixture struct {
	executor    *Executor
	repo        *mockRepository
	task        *v1.Task
	session     *models.TaskSession
	request     *LaunchAgentRequest
	environment *models.TaskEnvironment
	source      *models.Repository
}

func newRetainedLocalFixture(t *testing.T, inherited bool, executorType string) *retainedLocalFixture {
	t.Helper()
	repo := newMockRepository()
	e := newTestExecutor(t, &mockAgentManager{}, repo)
	task := &v1.Task{ID: "r3-child", WorkspaceID: "r3-workspace"}
	session := &models.TaskSession{
		ID: "r3-session", TaskID: task.ID, TaskEnvironmentID: "r3-environment",
		ExecutorID:   "r3-local-executor",
		RepositoryID: "r3-source", QueueIncarnationID: "r3-incarnation",
		State: models.TaskSessionStateWaitingForInput, AgentProfileID: "profile-123",
		Metadata: map[string]interface{}{"acp_session_id": "r3-native-history"},
	}
	env := &models.TaskEnvironment{
		ID: session.TaskEnvironmentID, TaskID: task.ID,
		ExecutorID:   session.ExecutorID,
		ExecutorType: executorType, Status: models.TaskEnvironmentStatusReady,
		WorkspacePath: t.TempDir(), OwnershipGeneration: 4,
	}
	source := &models.Repository{
		ID: session.RepositoryID, WorkspaceID: task.WorkspaceID,
		SourceType: "local", LocalPath: t.TempDir(), DefaultBranch: "main",
	}
	session.WorkspacePath = env.WorkspacePath
	repo.executors[session.ExecutorID] = &models.Executor{
		ID: session.ExecutorID, Type: models.ExecutorType(executorType), Status: models.ExecutorStatusActive,
	}
	repo.repositories[source.ID] = source
	repo.tasks[task.ID] = &models.Task{ID: task.ID, WorkspaceID: task.WorkspaceID}
	if inherited {
		task.ParentID = "r3-parent"
		task.Metadata = map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}}
		env.TaskID = task.ParentID
		repo.tasks[task.ID].ParentID = task.ParentID
		repo.tasks[task.ID].Metadata = task.Metadata
		repo.tasks[task.ParentID] = &models.Task{ID: task.ParentID, WorkspaceID: task.WorkspaceID}
		repo.taskRepositories["r3-link"] = &models.TaskRepository{
			ID: "r3-link", TaskID: task.ID, RepositoryID: source.ID, BaseBranch: "main",
		}
		env.Repos = []*models.TaskEnvironmentRepo{{
			ID: "r3-historical", TaskEnvironmentID: env.ID, RepositoryID: source.ID,
			Status: "deleted", CreatedAt: time.Now().UTC(),
		}}
		repo.taskEnvironmentRepos[env.ID] = env.Repos
	}
	repo.taskEnvironments[env.ID] = env
	repo.sessions[session.ID] = session
	request := &LaunchAgentRequest{
		TaskID: task.ID, ExecutorType: executorType, WorkspaceReuseRequired: true,
		Attachments: []v1.MessageAttachment{{Type: "file", AttachmentID: "r3-attachment", Name: "history.txt"}},
	}
	return &retainedLocalFixture{e, repo, task, session, request, env, source}
}

// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.1
func TestRetainedLocalDetachedResumeKeepsTaskWorkspace(t *testing.T) {
	for _, executorType := range []string{string(models.ExecutorTypeLocal), "local_pc"} {
		t.Run(executorType, func(t *testing.T) {
			f := newRetainedLocalFixture(t, false, executorType)
			f.request.Branch = "historical-comparison"
			attachments := append([]v1.MessageAttachment(nil), f.request.Attachments...)
			id, err := f.executor.applyResumeRepoConfig(context.Background(), f.task, f.session, f.request, f.environment, []*repoInfo(nil))
			require.NoError(t, err)
			require.Empty(t, id, "historical detached source must not become a materialization request")
			require.Empty(t, f.request.RepositoryID)
			require.Empty(t, f.request.Branch)
			require.Equal(t, f.environment.WorkspacePath, f.request.WorkspacePath)
			require.Equal(t, "r3-source", f.session.RepositoryID)
			require.Equal(t, "r3-native-history", f.session.Metadata["acp_session_id"])
			require.Equal(t, attachments, f.request.Attachments)
			require.Equal(t, f.task.ID, f.environment.TaskID)
			require.Empty(t, f.repo.createTaskEnvironmentCalls)
		})
	}
}

// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.1
func TestRetainedLocalInheritedResumeKeepsParentWorkspace(t *testing.T) {
	for _, executorType := range []string{string(models.ExecutorTypeLocal), "local_pc"} {
		t.Run(executorType, func(t *testing.T) {
			f := newRetainedLocalFixture(t, true, executorType)
			id, env, repositories, err := f.executor.prepareResumeRepositorySettings(context.Background(), f.task, f.session, f.request, ResumeOptions{})
			require.NoError(t, err)
			require.Empty(t, id)
			require.Empty(t, repositories, "historical child source must not derive materialization or MCP repository context")
			require.Empty(t, f.request.RepositoryID)
			require.Equal(t, f.environment.ID, env.ID)
			require.Equal(t, f.environment.WorkspacePath, f.request.WorkspacePath)
			require.Equal(t, "r3-parent", env.TaskID)
			require.Equal(t, "deleted", env.Repos[0].Status)
			require.Equal(t, "r3-source", f.session.RepositoryID)
			require.Equal(t, "r3-native-history", f.session.Metadata["acp_session_id"])
			require.Len(t, f.request.Attachments, 1)
			require.Empty(t, f.repo.createTaskEnvironmentCalls)
		})
	}
}

// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.1
func TestRetainedLocalDetachedExceptionRejectsOtherContexts(t *testing.T) {
	cases := []struct {
		name   string
		change func(*retainedLocalFixture) []*repoInfo
	}{
		{"foreign_owner", func(f *retainedLocalFixture) []*repoInfo { f.environment.TaskID = "foreign"; return nil }},
		{"session_owner", func(f *retainedLocalFixture) []*repoInfo { f.session.TaskID = "foreign"; return nil }},
		{"worktree", func(f *retainedLocalFixture) []*repoInfo { f.request.UseWorktree = true; return nil }},
		{"not_reused", func(f *retainedLocalFixture) []*repoInfo { f.request.WorkspaceReuseRequired = false; return nil }},
		{"failed_environment", func(f *retainedLocalFixture) []*repoInfo {
			f.environment.Status = models.TaskEnvironmentStatusFailed
			return nil
		}},
		{"attached_source", func(f *retainedLocalFixture) []*repoInfo {
			return []*repoInfo{{RepositoryID: f.source.ID, Repository: f.source, RepositoryPath: f.source.LocalPath}}
		}},
		{"multiple_sources", func(f *retainedLocalFixture) []*repoInfo {
			return []*repoInfo{{RepositoryID: f.source.ID, Repository: f.source, RepositoryPath: f.source.LocalPath}, {RepositoryID: "other", Repository: f.source}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRetainedLocalFixture(t, false, string(models.ExecutorTypeLocal))
			repositories := tc.change(f)
			id, err := f.executor.applyResumeRepoConfig(context.Background(), f.task, f.session, f.request, f.environment, repositories)
			if err == nil {
				require.Equal(t, f.source.ID, id, "disqualified context must keep normal repository resolution")
				require.Equal(t, f.source.ID, f.request.RepositoryID)
			}
			require.Equal(t, "r3-source", f.session.RepositoryID)
			require.Equal(t, "r3-native-history", f.session.Metadata["acp_session_id"])
		})
	}
}

// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.1
func TestRetainedLocalInheritedExceptionRejectsOtherContexts(t *testing.T) {
	cases := []struct {
		name   string
		change func(*retainedLocalFixture)
	}{
		{"foreign_parent", func(f *retainedLocalFixture) { f.environment.TaskID = "foreign" }},
		{"archived_parent", func(f *retainedLocalFixture) { now := time.Now(); f.repo.tasks[f.task.ParentID].ArchivedAt = &now }},
		{"workspace_mismatch", func(f *retainedLocalFixture) { f.repo.tasks[f.task.ParentID].WorkspaceID = "foreign" }},
		{"worktree", func(f *retainedLocalFixture) { f.request.UseWorktree = true }},
		{"multiple_repositories", func(f *retainedLocalFixture) {
			f.repo.taskRepositories["other-link"] = &models.TaskRepository{ID: "other-link", TaskID: f.task.ID, RepositoryID: f.source.ID, BaseBranch: "main", Position: 1}
		}},
		{"parent_attached_source", func(f *retainedLocalFixture) {
			f.repo.taskRepositories["parent-link"] = &models.TaskRepository{ID: "parent-link", TaskID: f.task.ParentID, RepositoryID: f.source.ID}
		}},
		{"active_inventory", func(f *retainedLocalFixture) { f.environment.Repos[0].Status = "ready" }},
		{"physical_worktree_id", func(f *retainedLocalFixture) { f.environment.Repos[0].WorktreeID = "materialized" }},
		{"physical_worktree_path", func(f *retainedLocalFixture) { f.environment.Repos[0].WorktreePath = f.source.LocalPath }},
		{"physical_clone", func(f *retainedLocalFixture) { f.environment.Repos[0].WorktreeSourceClonePath = f.source.LocalPath }},
		{"physical_common_dir", func(f *retainedLocalFixture) { f.environment.Repos[0].WorktreeSourceCommonDir = f.source.LocalPath }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRetainedLocalFixture(t, true, string(models.ExecutorTypeLocal))
			tc.change(f)
			id, _, repositories, err := f.executor.prepareResumeRepositorySettings(context.Background(), f.task, f.session, f.request, ResumeOptions{})
			if err == nil {
				require.False(t, id == "" && len(repositories) == 0 && f.request.RepositoryID == "", "disqualified context received the historical-source exception")
			}
			require.Equal(t, "r3-source", f.session.RepositoryID)
			require.Equal(t, "r3-native-history", f.session.Metadata["acp_session_id"])
		})
	}
}

// @covers AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-002.1
func TestRetainedLocalBuildResumeRequestKeepsWorkspace(t *testing.T) {
	for _, inherited := range []bool{false, true} {
		t.Run(map[bool]string{false: "detached", true: "inherited"}[inherited], func(t *testing.T) {
			for _, executorType := range []string{string(models.ExecutorTypeLocal), "local_pc"} {
				t.Run(executorType, func(t *testing.T) {
					for _, mode := range []struct {
						name       string
						startAgent bool
						native     bool
					}{
						{"workspace_only", false, false},
						{"without_native_conversation", true, false},
						{"native_conversation", true, true},
					} {
						t.Run(mode.name, func(t *testing.T) {
							f := newRetainedLocalFixture(t, inherited, executorType)
							if mode.native {
								f.session.DownstreamACPSessionID = "r3-native-history"
							} else {
								delete(f.session.Metadata, "acp_session_id")
							}
							raw, _ := newResumeLaunchRequest(f.task, f.session, mode.startAgent, ResumeOptions{})
							require.Empty(t, raw.WorkspacePath, "production request construction supplies only the original path")
							req, id, _, env, _, err := f.executor.buildResumeRequest(context.Background(), f.task, f.session, mode.startAgent)
							require.NoError(t, err)
							require.Equal(t, f.environment.WorkspacePath, req.WorkspacePath)
							require.Equal(t, f.session.WorkspacePath, req.OriginalWorkspacePath)
							require.Equal(t, f.environment.ID, env.ID)
							require.Equal(t, f.environment.TaskID, env.TaskID)
							require.Empty(t, id)
							require.Empty(t, req.RepositoryID)
							require.Empty(t, req.RepositoryPath)
							if mode.native {
								require.Equal(t, "r3-native-history", req.ACPSessionID)
							} else {
								require.Empty(t, req.ACPSessionID)
							}
							require.Equal(t, "r3-source", f.session.RepositoryID)
							require.Empty(t, f.repo.createTaskEnvironmentCalls)
						})
					}
				})
			}
		})
	}
}
