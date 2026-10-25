package lifecycle

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/db"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestRunOwnerRegistrationPersistsWithoutTaskSession(t *testing.T) {
	ctx := context.Background()
	repo, database := runOwnerTestRepository(t)
	mgr := newTestManager(t)
	mgr.SetExecutorRunningWriter(repo)
	owner := ExecutionOwner{Kind: ExecutionOwnerRun, WorkspaceID: "ws", RunID: "run", RunSessionID: "run-session", Attempt: 1, AgentProfileID: "office-agent"}
	execution := &AgentExecution{ID: "execution", WorkspaceID: "ws", RunID: "run", RunSessionID: "run-session", RunAttempt: 1, Owner: owner, OwnerAdmission: allowRunOwner{}, OfficeAgentProfileID: "office-agent", Status: v1.AgentStatusRunning, RuntimeName: executor.NameStandalone}
	backend := &runOwnerRecoveryExecutor{MockExecutor: MockExecutor{name: executor.NameStandalone}}
	mgr.executorRegistry = NewExecutorRegistry(mgr.logger)
	mgr.executorRegistry.Register(backend)
	require.NoError(t, mgr.registerAndPublishExecution(ctx, execution, backend, &ExecutorInstance{InstanceID: execution.ID}, ""))
	row, err := repo.GetExecutorRunningBySessionID(ctx, "run-session")
	require.NoError(t, err)
	require.Equal(t, "execution", row.AgentExecutionID)
	require.Empty(t, row.TaskID)
	require.Empty(t, execution.SessionID)
	var count int
	require.NoError(t, database.Get(&count, "SELECT COUNT(*) FROM task_sessions"))
	require.Zero(t, count)
	recovery, ok := any(mgr).(interface {
		StopRunOwnerForRecovery(context.Context, ExecutionOwner) error
	})
	require.True(t, ok, "runtime must reconcile the durable run-owned inventory")
	mgr.RemoveExecution(execution.ID)
	backend.stopErr = errors.New("runtime unavailable")
	require.Error(t, recovery.StopRunOwnerForRecovery(ctx, owner))
	row, err = repo.GetExecutorRunningBySessionID(ctx, "run-session")
	require.NoError(t, err)
	require.NotEqual(t, "stopped", row.Status)
	backend.stopErr = nil
	wrongOwner := owner
	wrongOwner.Attempt++
	require.Error(t, recovery.StopRunOwnerForRecovery(ctx, wrongOwner))
	require.NoError(t, recovery.StopRunOwnerForRecovery(ctx, owner))
	require.Equal(t, "execution", backend.stoppedID)
	row, err = repo.GetExecutorRunningBySessionID(ctx, "run-session")
	require.NoError(t, err)
	require.Equal(t, "stopped", row.Status)
	execution.Status = v1.AgentStatusRunning
	require.NoError(t, mgr.executionStore.Add(execution))
	require.NoError(t, mgr.persistExecutorRunningResult(ctx, execution))
	require.NoError(t, mgr.StopAgentWithReason(ctx, execution.ID, "office_turn_complete", false))
	row, err = repo.GetExecutorRunningBySessionID(ctx, "run-session")
	require.NoError(t, err)
	require.Equal(t, "stopped", row.Status, "normal stop must settle the durable run inventory")
}

type allowRunOwner struct{}

func (allowRunOwner) AdmitExecution(context.Context, ExecutionOwner) error { return nil }

type runOwnerRecoveryExecutor struct {
	MockExecutor
	stopErr   error
	stoppedID string
}

func (b *runOwnerRecoveryExecutor) StopInstance(_ context.Context, instance *ExecutorInstance, _ bool) error {
	b.stoppedID = instance.StandaloneInstanceID
	return b.stopErr
}

func TestRunOwnerLaunchCreatesWorkspaceWithoutTaskMarker(t *testing.T) {
	mgr := newTestManager(t)
	mgr.dataDir = t.TempDir()
	repo, _ := runOwnerTestRepository(t)
	mgr.SetExecutorRunningWriter(repo)
	backend := &createInstanceExecutor{MockExecutor: MockExecutor{name: executor.NameStandalone}}
	mgr.executorRegistry = NewExecutorRegistry(mgr.logger)
	mgr.executorRegistry.Register(backend)
	req := &LaunchRequest{WorkspaceID: "ws", AgentProfileID: "profile-1", ExecutorType: "local_pc",
		Owner: ExecutionOwner{Kind: ExecutionOwnerRun, WorkspaceID: "ws", RunID: "run", RunSessionID: "run-session", Attempt: 1, AgentProfileID: "office-agent"}, OwnerAdmission: allowRunOwner{},
	}
	execution, err := mgr.Launch(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, filepath.Join(mgr.dataDir, "office-runs", "ws", "run-session"), execution.WorkspacePath)
	require.DirExists(t, execution.WorkspacePath)
	require.Empty(t, execution.SessionID)
	require.Equal(t, "run-session", backend.lastRequest.SessionID)
}

func runOwnerTestRepository(t *testing.T) (*tasksqlite.Repository, *sqlx.DB) {
	t.Helper()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "runtime.db"))
	require.NoError(t, err)
	database := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	repo, err := tasksqlite.NewWithDB(database, database, nil)
	require.NoError(t, err)

	return repo, database
}
