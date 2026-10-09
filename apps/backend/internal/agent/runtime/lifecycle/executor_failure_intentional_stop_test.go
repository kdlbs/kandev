package lifecycle

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type disconnectDuringStopExecutor struct {
	statusProviderExecutor
	onStop func()
}

func (e *disconnectDuringStopExecutor) StopInstance(ctx context.Context, instance *ExecutorInstance, force bool) error {
	e.onStop()
	return e.MockExecutor.StopInstance(ctx, instance, force)
}

// @covers AC-EXECUTORS-FAILURE-VISIBILITY-001.2
// Intentional teardown must not inspect or reconnect the instance being stopped.
func TestExecutorFailureIntentionalStopSuppressesDisconnectInspection(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		reason string
	}{{"user stop", nil, "user requested"}, {"task cleanup", nil, StopReasonTaskDeleted}, {"failed cleanup", errors.New("stop failed"), StopReasonTaskDeleted}} {
		stopErr := tc.err
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				provider := &disconnectDuringStopExecutor{statusProviderExecutor: statusProviderExecutor{
					MockExecutor: MockExecutor{name: executor.NameDocker, stopInstanceErr: stopErr},
					status:       &RemoteStatus{Observation: &models.ExecutorObservation{Outcome: "healthy", ResourceKey: "container", ObservedAt: time.Now().UTC()}},
				}}
				mgr := newRemoteStatusManager(t, provider)
				mgr.SetExecutorRunningWriter(&failureEnvironmentWriter{})
				mgr.SetExecutorObservationHandler(func(context.Context, models.ExecutorObservationTarget, *models.ExecutorObservation) error {
					t.Error("intentional stop created an executor failure")
					return nil
				})
				execution := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session", TaskEnvironmentID: "env", ContainerID: "container", RuntimeName: agentruntime.RuntimeDocker, promptDoneCh: make(chan PromptCompletionSignal, 1)}
				execution.setSessionInitialized(true)
				require.NoError(t, mgr.executionStore.Add(execution))
				// The same live resource is inspected for an unexpected disconnect.
				mgr.classifyExecutorDisconnect(execution, 0, 0)
				require.Len(t, provider.observed, 1)
				provider.onStop = func() {
					mgr.handleStreamDisconnectWithStartupGeneration(execution, errors.New("stream closed during stop"), 0, 0)
					// Also exercise a classifier already queued before teardown began.
					mgr.classifyExecutorDisconnect(execution, 0, 0)
					synctest.Wait()
					require.Len(t, provider.observed, 1, "teardown re-inspected a deliberately stopped instance")
				}
				err := mgr.StopAgentWithReason(t.Context(), execution.ID, tc.reason, true)
				if stopErr != nil {
					require.ErrorIs(t, err, stopErr)
					mgr.classifyExecutorDisconnect(execution, 0, 0)
					require.Len(t, provider.observed, 2, "failed teardown must release inspection suppression for recovery")
				} else {
					require.NoError(t, err)
					_, tracked := mgr.executionStore.Get(execution.ID)
					require.False(t, tracked)
				}
			})
		})
	}
}
