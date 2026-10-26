package executor

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/nomad"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/worktree"
	"github.com/stretchr/testify/require"
)

const (
	nomadReadyTaskID      = "task-nomad-ready"
	nomadReadyEnvID       = "environment-nomad-ready"
	nomadReadySessionID   = "session-nomad-ready"
	nomadReadyExecutionID = "execution-nomad-ready"
	nomadReadyGeneration  = int64(2)
)

// nomadReadyFixture is a task environment as the recovery path publishes it:
// status ready, nomad executor, workspace path set, empty materializer, repo
// rows present, retained Nomad inventory present, session still CREATED.
type nomadReadyFixture struct {
	repo          *tasksqlite.Repository
	env           *models.TaskEnvironment
	session       *models.TaskSession
	workspacePath string
}

func newNomadReadyFixture(t *testing.T) *nomadReadyFixture {
	t.Helper()
	db, _ := remoteSelectionSQLiteTemplate.Open(t)
	repo := tasksqlite.NewWithInitializedDB(db, db, nil)
	ctx := context.Background()
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace", Name: "Workspace"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: nomadReadyTaskID, WorkspaceID: "workspace", Title: "Task"}))
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{ID: "repository-one", WorkspaceID: "workspace", Name: "repo-one", DefaultBranch: "main"}))
	require.NoError(t, repo.CreateTaskRepository(ctx, &models.TaskRepository{ID: "attachment", TaskID: nomadReadyTaskID, RepositoryID: "repository-one", BaseBranch: "feature/Topic x"}))

	workspacePath, err := nomad.EnvironmentWorkspacePath("/var/lib/kandev/workspaces/env", nomadReadyEnvID, nomadReadyGeneration)
	require.NoError(t, err)
	env := &models.TaskEnvironment{
		ID: nomadReadyEnvID, TaskID: nomadReadyTaskID, ExecutorType: string(models.ExecutorTypeNomad),
		OwnershipGeneration: nomadReadyGeneration, Status: models.TaskEnvironmentStatusReady,
		WorkspacePath: workspacePath,
		Repos: []*models.TaskEnvironmentRepo{{
			ID: "published-slot", RepositoryID: "repository-one",
			BranchSlug: worktree.SanitizeBranchSlug("feature/Topic x"), WorkspaceRelativePath: "repo-one",
		}},
	}
	require.NoError(t, repo.CreateTaskEnvironment(ctx, env))
	session := &models.TaskSession{
		ID: nomadReadySessionID, TaskID: nomadReadyTaskID, TaskEnvironmentID: env.ID,
		State: models.TaskSessionStateCreated,
	}
	require.NoError(t, repo.CreateTaskSession(ctx, session))
	fx := &nomadReadyFixture{repo: repo, env: env, session: session, workspacePath: workspacePath}
	fx.checkpointInventory(t)
	return fx
}

func (fx *nomadReadyFixture) checkpointInventory(t *testing.T) {
	t.Helper()
	var revision uint64
	for _, phase := range []string{"intent", "registered", "allocated", "ready"} {
		state := nomad.EnvironmentState{
			Version: 1, EnvironmentID: nomadReadyEnvID, Generation: nomadReadyGeneration,
			OperationID: nomadReadyExecutionID, InstallationID: "installation-one", ExecutionID: nomadReadyExecutionID,
			InputDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			JobID:       "kandev-fixture-job", Namespace: "default", Region: "global", Datacenter: "dc1",
			NodeID: "node-one", Platform: "darwin/arm64", SSHHost: "host.internal", SSHUser: "kandev", SSHPort: 22,
			SSHHostFingerprint: "SHA256:abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJK",
			WorkspaceRoot:      "/var/lib/kandev/workspaces/env", WorkspacePath: fx.workspacePath,
			AllocationRoot: "/var/lib/kandev/allocations/env", CPUMHz: 1500, MemoryMB: 4096, Phase: phase,
		}
		if phase == "allocated" || phase == "ready" {
			state.AllocationID, state.AllocationCreateIndex, state.ServerIdentity = "allocation-one", 7, "server-one"
		}
		metadata := map[string]interface{}{}
		require.NoError(t, nomad.PutEnvironmentState(metadata, state))
		running := &models.ExecutorRunning{
			ID: nomadReadySessionID, SessionID: nomadReadySessionID, TaskID: nomadReadyTaskID,
			ExecutorID: "exec-nomad-fixture", Runtime: agentruntime.RuntimeNomad,
			Status: models.ExecutorRunningStatusStarting, AgentExecutionID: nomadReadyExecutionID,
			Metadata: metadata, ExpectedNomadExecutorRevision: revision,
		}
		require.NoError(t, fx.repo.CheckpointNomadExecutorInventory(context.Background(), running))
		revision = running.ExpectedNomadExecutorRevision
	}
}

func (fx *nomadReadyFixture) persist(t *testing.T, req *LaunchAgentRequest) error {
	t.Helper()
	exec := newTestExecutor(t, &mockAgentManager{}, fx.repo)
	existing, err := fx.repo.GetTaskEnvironmentByTaskID(context.Background(), nomadReadyTaskID)
	require.NoError(t, err)
	return exec.persistTaskEnvironment(
		context.Background(), nomadReadyTaskID, fx.session, existing, req,
		&LaunchAgentResponse{WorkspacePath: fx.workspacePath},
		executorConfig{ExecutorID: "exec-nomad-fixture"},
	)
}

func (fx *nomadReadyFixture) launchRequest() *LaunchAgentRequest {
	return &LaunchAgentRequest{
		TaskID: nomadReadyTaskID, SessionID: nomadReadySessionID, TaskEnvironmentID: nomadReadyEnvID,
		ExecutorType: string(models.ExecutorTypeNomad), WorkspaceLayout: "task_dir", TaskDirName: "task-dir-one",
		Repositories: []RepoSpec{{
			RepositoryID: "repository-one", BranchSlug: "feature/Topic x", WorkspaceRelativePath: "repo-one",
		}},
	}
}

func TestPersistTaskEnvironment_PublishedNomadReady_KeepsReadyWithoutReuseRejection(t *testing.T) {
	fx := newNomadReadyFixture(t)
	err := fx.persist(t, fx.launchRequest())
	if errors.Is(err, models.ErrWorkspaceReuseUnsafe) {
		t.Fatalf("published ready Nomad row was rejected as reuse-unsafe: %v", err)
	}
	require.NoError(t, err)

	stored, err := fx.repo.GetTaskEnvironmentByTaskID(context.Background(), nomadReadyTaskID)
	require.NoError(t, err)
	require.Equal(t, models.TaskEnvironmentStatusReady, stored.Status)
	require.Equal(t, string(models.ExecutorTypeNomad), stored.ExecutorType)
	require.Equal(t, fx.workspacePath, stored.WorkspacePath)
	require.Empty(t, stored.MaterializationSessionID)
	require.Equal(t, nomadReadyEnvID, fx.session.TaskEnvironmentID)
}

func TestPersistTaskEnvironment_PublishedNomadReady_FillsLayoutAndTaskDirName(t *testing.T) {
	fx := newNomadReadyFixture(t)
	before, err := fx.repo.GetTaskEnvironmentByTaskID(context.Background(), nomadReadyTaskID)
	require.NoError(t, err)
	require.Empty(t, before.WorkspaceLayout)
	require.Empty(t, before.TaskDirName)

	require.NoError(t, fx.persist(t, fx.launchRequest()))

	stored, err := fx.repo.GetTaskEnvironmentByTaskID(context.Background(), nomadReadyTaskID)
	require.NoError(t, err)
	require.Equal(t, "task_dir", stored.WorkspaceLayout)
	require.Equal(t, "task-dir-one", stored.TaskDirName)
}

func TestPersistTaskEnvironment_PublishedNomadReady_DoesNotDuplicateRepoRows(t *testing.T) {
	fx := newNomadReadyFixture(t)
	require.NoError(t, fx.persist(t, fx.launchRequest()))
	// A second first-prompt style pass must stay idempotent too.
	require.NoError(t, fx.persist(t, fx.launchRequest()))

	rows, err := fx.repo.ListTaskEnvironmentRepos(context.Background(), nomadReadyEnvID)
	require.NoError(t, err)
	seen := map[string]int{}
	for _, row := range rows {
		seen[row.RepositoryID+"\x00"+worktree.SanitizeBranchSlug(row.BranchSlug)]++
	}
	key := "repository-one\x00" + worktree.SanitizeBranchSlug("feature/Topic x")
	require.Equal(t, 1, seen[key], "rows = %+v", rows)
	require.Len(t, rows, 1)
	require.Equal(t, "published-slot", rows[0].ID)
}
