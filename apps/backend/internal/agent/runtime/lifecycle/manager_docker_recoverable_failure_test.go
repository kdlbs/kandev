package lifecycle

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/stretchr/testify/require"
)

func TestDockerRecoverableFailureRetainsEstablishedContainer(t *testing.T) {
	for _, tc := range []struct {
		reason string
		force  bool
	}{
		{StopReasonRecoverableAgentFailure, false},
		{StopReasonAgentBootstrapFailed, true},
		{StopReasonTaskDeleted, true},
		{"", true},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			log := newTestRegistryLogger()
			registry := NewExecutorRegistry(log)
			backend := &stopTrackingExecutor{MockExecutor: MockExecutor{name: executor.NameDocker}}
			registry.Register(backend)
			mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
			cleanupManagerStopCh(t, mgr)
			execution := &AgentExecution{ID: "execution", TaskID: "task", SessionID: "session", RuntimeName: executor.NameDocker, ContainerID: "container"}
			require.NoError(t, mgr.executionStore.Add(execution))
			require.NoError(t, mgr.StopAgentWithReason(context.Background(), execution.ID, tc.reason, true))
			require.Equal(t, []bool{tc.force}, backend.forces)
		})
	}
}
