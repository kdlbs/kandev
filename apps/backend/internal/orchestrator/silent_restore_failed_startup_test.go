package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-002.5
func TestSilentRestoreFailedStartupConverges(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(fmt.Sprint(cancelled), func(t *testing.T) {
			f := newSilentRestoreCheckpointFixtureWithLaunching(t, false)
			f.service.sessionCeiling = newSessionCeilingController(10, nil, nil)
			startupFailure := errors.New("native load failed")
			ctx, cancel := context.WithCancel(f.ctx)
			defer cancel()
			if cancelled {
				startupFailure = context.Canceled
			}
			stopped := false
			f.manager.rowLivenessFn = func(row *models.ExecutorRunning) models.ProcessLiveness {
				if row.AgentExecutionID != f.checkpoint.CandidateExecutionID || stopped {
					return models.ProcessLivenessDead
				}
				return models.ProcessLivenessAlive
			}
			f.manager.launchAgentFunc = func(ctx context.Context, req *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				require.Equal(t, f.checkpoint.CandidateExecutionID, req.CandidateExecutionID)
				require.Empty(t, req.TaskDescription)
				require.NoError(t, req.OnExecutionAllocated(ctx, req.CandidateExecutionID))
				addSilentRestoreCandidateExecution(t, f)
				return &executor.LaunchAgentResponse{AgentExecutionID: req.CandidateExecutionID}, nil
			}
			f.manager.startAgentProcessFunc = func(context.Context, string) error {
				if cancelled {
					cancel()
				}
				return startupFailure
			}
			f.service.agentManager = &failedRestoreCleanupManager{mockAgentManager: f.manager, cleanup: func(ctx context.Context, sessionID, executionID string, updatedAt time.Time, checkpoint func(context.Context) error) error {
				require.Equal(t, f.checkpoint.CandidateExecutionID, executionID)
				require.NoError(t, ctx.Err())
				stopped = true
				if err := checkpoint(ctx); err != nil {
					return err
				}
				return f.repo.DeleteExecutorRunningIfCurrent(ctx, sessionID, executionID, updatedAt)
			}}

			running, err := f.repo.GetExecutorRunningBySessionID(f.ctx, f.session.ID)
			require.NoError(t, err)
			err = f.service.startSilentRestoreCandidate(ctx, f.store, f.attemptID, f.checkpoint, f.inputs, running)
			if cancelled {
				require.ErrorIs(t, err, ErrResumeAttemptCancelled)
			} else {
				require.ErrorIs(t, err, startupFailure)
			}
			stored, err := f.store.GetRestoreAttempt(f.ctx, f.attemptID)
			require.NoError(t, err)
			require.Equal(t, models.SilentRestoreStageCandidateDead, stored.Checkpoint.Stage)
			require.True(t, stopped)
			require.Empty(t, f.manager.stopAgentWithReasonArgs, "ordinary cancellation must not delete recovery evidence")
			// Simulate restart after checkpointing and inventory cleanup.
			rotated, err := f.service.rotateDeadSilentRestoreCandidate(f.ctx, f.store, f.attemptID, *stored.Checkpoint, f.inputs)
			require.NoError(t, err)
			require.Equal(t, models.SilentRestoreStageCandidateAllocated, rotated.Stage)
			require.NotEqual(t, f.checkpoint.CandidateExecutionID, rotated.CandidateExecutionID)
		})
	}
}

type failedRestoreCleanupManager struct {
	*mockAgentManager
	cleanup func(context.Context, string, string, time.Time, func(context.Context) error) error
}

func (m *failedRestoreCleanupManager) CleanupFailedRestoreIfCurrent(ctx context.Context, sessionID, executionID string, updatedAt time.Time, checkpoint func(context.Context) error) error {
	return m.cleanup(ctx, sessionID, executionID, updatedAt, checkpoint)
}

func TestSilentRestoreFailedStartupPreservesReadyCandidate(t *testing.T) {
	f := newSilentRestoreCheckpointFixture(t)
	addSilentRestoreCandidateExecution(t, f)
	f.manager.isAgentReadyFn = func(context.Context, string) bool { return true }
	f.manager.getACPSessionIDForSessionFunc = func(string) (string, bool) { return "provider-session", true }
	f.manager.rowLivenessFn = func(*models.ExecutorRunning) models.ProcessLiveness { return models.ProcessLivenessAlive }
	cleanups := 0
	f.service.agentManager = &failedRestoreCleanupManager{mockAgentManager: f.manager, cleanup: func(context.Context, string, string, time.Time, func(context.Context) error) error {
		cleanups++
		return errors.New("ready candidate was stopped")
	}}
	require.NoError(t, f.service.settleFailedSilentRestoreCandidate(f.ctx, f.store, f.attemptID, f.checkpoint, f.inputs))
	require.Zero(t, cleanups)
	stored, err := f.store.GetRestoreAttempt(f.ctx, f.attemptID)
	require.NoError(t, err)
	require.Equal(t, models.SilentRestoreStageCandidateLaunching, stored.Checkpoint.Stage)
}
