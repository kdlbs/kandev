package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentruntime"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/startup"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type builtinRecoveryBlockFailingWriter struct {
	*tasksqlite.Repository
	attempted bool
}

func (w *builtinRecoveryBlockFailingWriter) UpsertSessionRecoveryBlock(context.Context, *models.SessionRecoveryBlock) error {
	w.attempted = true
	return context.DeadlineExceeded
}

func TestBuiltinRecoveryProgressRequiresPersistedRefusal(t *testing.T) {
	for _, persisted := range []bool{false, true} {
		name := "block unavailable"
		if persisted {
			name = "block persisted"
		}
		t.Run(name, func(t *testing.T) {
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
			writer := &builtinRecoveryBlockFailingWriter{Repository: repo}
			if !persisted {
				mgr.SetExecutorRunningWriter(writer)
			}
			t.Cleanup(func() { require.NoError(t, mgr.Stop()) })
			reporter, logs := newRecoveryProgressReporter(t)
			require.NoError(t, mgr.Start(startup.WithReporter(ctx, reporter)))
			_, tracked := mgr.GetExecutionBySessionID(sessionID)
			require.False(t, tracked, "a refused binding must never resume its streams")
			fields := completedStepFields(t, logs, startup.StepSessionsRecovery)
			wantDone := 0
			if persisted {
				wantDone = 1
			} else {
				require.True(t, writer.attempted, "exercise the real recovery-block write")
			}
			assertRecoveryCount(t, fields, "done", wantDone)
			assertRecoveryCount(t, fields, "total", 1)
		})
	}
}
