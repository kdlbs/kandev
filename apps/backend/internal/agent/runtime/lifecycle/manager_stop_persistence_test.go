package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestStopTaskExecutionPersistsStoppedRuntimeAndResumeToken(t *testing.T) {
	ctx := context.Background()
	repo, _ := runOwnerTestRepository(t)
	mgr := newTestManager(t)
	mgr.SetExecutorRunningWriter(repo)
	backend := &runOwnerRecoveryExecutor{MockExecutor: MockExecutor{name: executor.NameStandalone}}
	mgr.executorRegistry = NewExecutorRegistry(mgr.logger)
	mgr.executorRegistry.Register(backend)
	execution := &AgentExecution{
		ID: "execution-task-stop", TaskID: "task-stop-persisted-runtime", SessionID: "session-stop-persisted-runtime",
		Owner:  ExecutionOwner{Kind: ExecutionOwnerTask, TaskID: "task-stop-persisted-runtime", SessionID: "session-stop-persisted-runtime"},
		Status: v1.AgentStatusReady, RuntimeName: executor.NameStandalone,
	}
	require.NoError(t, mgr.executionStore.Add(execution))
	require.NoError(t, mgr.persistExecutorRunningResult(ctx, execution))
	running, err := repo.GetExecutorRunningBySessionID(ctx, execution.SessionID)
	require.NoError(t, err)
	running.ResumeToken = "saved-conversation-token"
	running.Resumable = true
	require.NoError(t, repo.UpsertExecutorRunning(ctx, running))

	require.NoError(t, mgr.StopAgentWithReason(ctx, execution.ID, "user stop", true))
	running, err = repo.GetExecutorRunningBySessionID(ctx, execution.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.ExecutorRunningStatusStopped, running.Status)
	require.Equal(t, "saved-conversation-token", running.ResumeToken)
	require.True(t, running.Resumable)
}
