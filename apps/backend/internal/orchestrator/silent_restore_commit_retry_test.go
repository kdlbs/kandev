package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type silentRestoreStreamRecoveryManager struct {
	*mockAgentManager
	recoverStream func(context.Context, string, string, string) error
}

func (m *silentRestoreStreamRecoveryManager) RecoverSilentRestoreCandidateStream(ctx context.Context, sessionID, executionID, nativeID string) error {
	return m.recoverStream(ctx, sessionID, executionID, nativeID)
}

func TestSilentRestoreCommitRetryReconnectsCandidate(t *testing.T) {
	f := newSilentRestoreCheckpointFixture(t)
	running := addSilentRestoreCandidateExecution(t, f)
	connected := true
	f.manager.isAgentReadyFn = func(context.Context, string) bool { return connected }
	f.manager.getACPSessionIDForSessionFunc = func(string) (string, bool) { return "provider-session", true }
	f.manager.rowLivenessFn = func(*models.ExecutorRunning) models.ProcessLiveness { return models.ProcessLivenessAlive }
	f.service.agentManager = &silentRestoreStreamRecoveryManager{mockAgentManager: f.manager,
		recoverStream: func(_ context.Context, sessionID, executionID, nativeID string) error {
			require.Equal(t, f.session.ID, sessionID)
			require.Equal(t, f.checkpoint.CandidateExecutionID, executionID)
			require.Equal(t, "provider-session", nativeID)
			connected = true
			return nil
		}}
	_, err := f.repo.DB().Exec(`CREATE TRIGGER reject_restore_commit BEFORE INSERT ON harness_session_generations WHEN NEW.generation = 2 BEGIN SELECT RAISE(ABORT, 'commit unavailable'); END`)
	require.NoError(t, err)
	err = f.service.finishLaunchedSilentRestoreCandidate(f.ctx, f.store, f.attemptID, f.checkpoint, f.inputs, running)
	require.ErrorContains(t, err, "commit unavailable")
	// The failed attempt ends, closing the stream that held uncommitted setup.
	connected = false
	_, err = f.repo.DB().Exec("DROP TRIGGER reject_restore_commit")
	require.NoError(t, err)
	require.NoError(t, f.service.finishLaunchedSilentRestoreCandidate(f.ctx, f.store, f.attemptID, f.checkpoint, f.inputs, running))
	generation, err := f.repo.GetCurrentHarnessSessionGeneration(f.ctx, f.session.ID, f.session.QueueIncarnationID)
	require.NoError(t, err)
	require.EqualValues(t, 2, generation.Generation)
	require.Equal(t, "provider-session", generation.NativeSessionID)
	require.Empty(t, f.manager.stopAgentWithReasonArgs)
}
