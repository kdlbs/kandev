package lifecycle

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// newDetachTestManager builds a Manager wired with a MockExecutor registered
// under executor.NameStandalone, so tests can assert whether
// StopAgentWithReason's survivable-detach branch (AC-EXECUTORS-SURVIVAL kill
// path #4) actually reaches StopInstance.
func newDetachTestManager(t *testing.T, agentSurvivalEnabled bool) (*Manager, *MockExecutor) {
	t.Helper()
	log := newTestLogger()
	mockExecutor := &MockExecutor{name: executor.NameStandalone}
	registry := NewExecutorRegistry(log)
	registry.Register(mockExecutor)
	eventBus := &MockEventBus{}
	mgr := NewManager(newTestRegistry(), eventBus, registry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	mgr.SetAgentSurvivalEnabled(agentSurvivalEnabled)
	return mgr, mockExecutor
}

func newDetachTestExecution(id, sessionID string) *AgentExecution {
	return &AgentExecution{
		ID:          id,
		SessionID:   sessionID,
		TaskID:      "task-" + id,
		RuntimeName: executor.NameStandalone,
		agentctl:    agentctl.NewClient("127.0.0.1", 12345, newTestLogger()),
	}
}

// TestStopAgentWithReasonDetachesOnBackendShutdownWhenSurvivalEnabled pins
// design 01/02's kill-path #4 fix: a graceful backend shutdown with the
// capability enabled must not reach the executor's StopInstance or publish
// events.AgentStopped (which would drive the orchestrator to persist a
// terminal task-session state for a session whose agent is still running) --
// it only releases this backend's own tracking and stream subscription.
func TestStopAgentWithReasonDetachesOnBackendShutdownWhenSurvivalEnabled(t *testing.T) {
	mgr, mockExecutor := newDetachTestManager(t, true)
	execution := newDetachTestExecution("exec-detach", "session-detach")
	require.NoError(t, mgr.executionStore.Add(execution))

	err := mgr.StopAgentWithReason(context.Background(), execution.ID, StopReasonBackendShutdown, false)
	require.NoError(t, err)

	_, exists := mgr.executionStore.Get(execution.ID)
	require.False(t, exists, "detach must remove this backend's own tracking")
	require.Empty(t, mockExecutor.stopInstanceCalls, "detach must never call StopInstance; the instance is left running")

	client, release := execution.AcquireAgentCtlClient()
	release()
	require.Nil(t, client, "detach must release this backend's agentctl stream subscription")

	eventBus := mgr.eventBus.(*MockEventBus)
	for _, evt := range eventBus.PublishedEvents {
		require.NotEqual(t, events.AgentStopped, evt.Type,
			"detach must not publish agent.stopped: it would drive the orchestrator to persist a terminal state for a still-running agent")
	}
}

// TestStopAgentWithReasonTerminatesForNonShutdownReasonEvenWhenSurvivalEnabled
// confirms the detach branch is scoped exactly to StopReasonBackendShutdown:
// every other stop reason -- user stops, rollback cleanup, task deletion --
// keeps today's terminating semantics even when the capability is enabled.
func TestStopAgentWithReasonTerminatesForNonShutdownReasonEvenWhenSurvivalEnabled(t *testing.T) {
	mgr, mockExecutor := newDetachTestManager(t, true)
	execution := newDetachTestExecution("exec-terminate", "session-terminate")
	require.NoError(t, mgr.executionStore.Add(execution))

	err := mgr.StopAgentWithReason(context.Background(), execution.ID, StopReasonTaskDeleted, false)
	require.NoError(t, err)

	require.Len(t, mockExecutor.stopInstanceCalls, 1, "a non-shutdown reason must still stop the runtime instance")

	eventBus := mgr.eventBus.(*MockEventBus)
	require.True(t, hasAgentStoppedEvent(eventBus.PublishedEvents), "a terminating stop must still publish agent.stopped")
}

// TestStopAgentWithReasonTerminatesOnBackendShutdownWhenSurvivalDisabled
// confirms the zero value (capability disabled) preserves today's behavior:
// backend shutdown terminates every agent exactly as it did before this
// capability existed.
func TestStopAgentWithReasonTerminatesOnBackendShutdownWhenSurvivalDisabled(t *testing.T) {
	mgr, mockExecutor := newDetachTestManager(t, false)
	execution := newDetachTestExecution("exec-disabled", "session-disabled")
	require.NoError(t, mgr.executionStore.Add(execution))

	err := mgr.StopAgentWithReason(context.Background(), execution.ID, StopReasonBackendShutdown, false)
	require.NoError(t, err)

	require.Len(t, mockExecutor.stopInstanceCalls, 1, "capability disabled must keep the terminating stop on backend shutdown")

	eventBus := mgr.eventBus.(*MockEventBus)
	require.True(t, hasAgentStoppedEvent(eventBus.PublishedEvents), "a terminating stop must still publish agent.stopped")
}

func hasAgentStoppedEvent(published []*bus.Event) bool {
	for _, evt := range published {
		if evt.Type == events.AgentStopped {
			return true
		}
	}
	return false
}

// TestStopAgentWithReasonTerminatesNonStandaloneRuntimeEvenWhenSurvivalEnabled
// pins the scope boundary the capability is documented to have: the
// survivable-detach branch exists only for the standalone (worktree/local)
// runtime this feature covers. `StopAllAgents` calls StopAgentWithReason with
// StopReasonBackendShutdown for every tracked execution regardless of
// runtime, so a Docker/SSH/Sprites/Kubernetes/remote-docker execution must
// still reach the terminating path -- including the real StopInstance call
// and the agent.stopped publish -- even while the capability is globally
// enabled for the installation's standalone sessions.
func TestStopAgentWithReasonTerminatesNonStandaloneRuntimeEvenWhenSurvivalEnabled(t *testing.T) {
	log := newTestLogger()
	mockExecutor := &MockExecutor{name: executor.NameDocker}
	registry := NewExecutorRegistry(log)
	registry.Register(mockExecutor)
	eventBus := &MockEventBus{}
	mgr := NewManager(newTestRegistry(), eventBus, registry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	mgr.SetAgentSurvivalEnabled(true)

	execution := &AgentExecution{
		ID:          "exec-docker",
		SessionID:   "session-docker",
		TaskID:      "task-exec-docker",
		RuntimeName: executor.NameDocker,
	}
	require.NoError(t, mgr.executionStore.Add(execution))

	err := mgr.StopAgentWithReason(context.Background(), execution.ID, StopReasonBackendShutdown, false)
	require.NoError(t, err)

	require.Len(t, mockExecutor.stopInstanceCalls, 1,
		"a non-standalone runtime must still be stopped on backend shutdown even when the survival capability is enabled")

	eventBus2 := mgr.eventBus.(*MockEventBus)
	require.True(t, hasAgentStoppedEvent(eventBus2.PublishedEvents),
		"a non-standalone runtime's terminating stop must still publish agent.stopped")

	_, exists := mgr.executionStore.Get(execution.ID)
	require.False(t, exists, "a terminated (non-detached) execution must be removed from tracking")
}
