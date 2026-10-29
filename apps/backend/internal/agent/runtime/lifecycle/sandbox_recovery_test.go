package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestSandboxRecoveryAdmissionPrecedesTracking(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		t.Run(map[bool]string{false: "ordinary", true: "sandbox listener loss"}[blocked], func(t *testing.T) {
			log, logs := newObservedRecoveryManagerLogger(t)
			backend := &MockExecutor{name: executor.NameStandalone, recoverInstances: []*ExecutorInstance{{InstanceID: "inst-1", TaskID: "task-1", SessionID: "session-1", AgentProfileID: recoveryTestAgentProfileID, RuntimeName: executor.NameStandalone, StandaloneInstanceID: "std-1"}}}
			registry := NewExecutorRegistry(log)
			registry.Register(backend)
			mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, &MockCredentialsManager{}, &MockProfileResolver{}, nil, ExecutorFallbackWarn, "", log)
			cleanupManagerStopCh(t, mgr)
			t.Cleanup(func() { _ = mgr.Stop() })
			registerRecoveryTestAgentProfile(t, mgr)
			mgr.SetExecutorRunningWriter(&listingWriter{rows: []*models.ExecutorRunning{{TaskID: "task-1", SessionID: "session-1", Runtime: agentruntime.RuntimeStandalone, ExecutionProfileID: recoveryTestAgentProfileID}}})
			mgr.SetRecoveryDeadline(5 * time.Second)
			called := false
			mgr.SetSandboxRecoveryAdmission(func(_ context.Context, execution *AgentExecution) error {
				called = true
				require.Equal(t, "session-1", execution.SessionID)
				require.Empty(t, execution.AgentCommand, "reject before command reconstruction or runtime repair")
				if blocked {
					return errors.New("sandbox requires recovery")
				}
				return nil
			})
			require.NoError(t, mgr.Start(t.Context()))
			require.True(t, called)
			_, tracked := mgr.GetExecutionBySessionID("session-1")
			require.Equal(t, !blocked, tracked)
			expected := 0
			if blocked {
				expected = 1
			}
			assertRecoveryCount(t, recoveryOutcomeFields(t, logs), "not_retracked_sandbox", expected)
			if blocked {
				require.Len(t, backend.stopInstanceCalls, 1)
			} else {
				require.Empty(t, backend.stopInstanceCalls)
			}
		})
	}
}
