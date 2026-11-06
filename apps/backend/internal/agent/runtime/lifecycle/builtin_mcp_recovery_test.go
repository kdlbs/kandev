package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentruntime"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestBuiltinRecoveryCorruptionParksBeforeRetracking(t *testing.T) {
	const taskID, sessionID, executionID = "task-a", "session-a", "execution"
	ctx := context.Background()
	repo := newSettingsSourceRecoveryRepository(t, taskID, sessionID)
	metadata := map[string]any{p.BuiltinBindingMetadataKey: "{}"}
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		TaskID: taskID, SessionID: sessionID, Runtime: agentruntime.RuntimeStandalone,
		Status: models.ExecutorRunningStatusRunning, Resumable: true, AgentExecutionID: executionID,
		ExecutionProfileID: recoveryTestAgentProfileID, ResumeToken: "native-token", Metadata: metadata,
	}))
	mgr := newSettingsSourceRecoveryManager(t, &MockEventBus{}, repo, executionID, sessionID, taskID)
	mgr.SetBuiltinActionResolver(builtinTestActions)
	mgr.streamManager.mcpPrincipalScoper = builtinTestPrincipal
	backend, err := mgr.executorRegistry.GetBackend(executor.NameStandalone)
	require.NoError(t, err)
	backend.(*MockExecutor).recoverInstances[0].Metadata = metadata
	t.Cleanup(func() { require.NoError(t, mgr.Stop()) })
	require.NoError(t, mgr.Start(ctx))
	_, tracked := mgr.GetExecutionBySessionID(sessionID)
	require.False(t, tracked, "an invalid package binding must never resume its streams")
	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	incarnation := session.QueueIncarnationID
	if incarnation == "" {
		incarnation = sessionID
	}
	block, err := repo.GetOpenSessionRecoveryBlock(ctx, sessionID, incarnation, 0)
	require.NoError(t, err)
	require.Equal(t, "builtin_mcp_binding", block.Reason)
	running, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, "native-token", running.ResumeToken)
}
