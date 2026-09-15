package lifecycle

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/secrets"
)

func TestFailedKubernetesResumePreservesRetainedRuntimeAndSecrets(t *testing.T) {
	log := newTestRegistryLogger()
	executorRegistry := NewExecutorRegistry(log)
	backend := &stopTrackingExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}}
	executorRegistry.Register(backend)
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, executorRegistry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)

	secretStore := newInMemorySecretStore()
	for _, id := range []string{"runtime-auth", "runtime-bootstrap"} {
		require.NoError(t, secretStore.Create(context.Background(), &secrets.SecretWithValue{
			Secret: secrets.Secret{ID: id, Name: id}, Value: "secret",
		}))
	}
	mgr.SetSecretStore(secretStore)
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID: "execution-1", TaskID: "task-1", SessionID: "session-1",
		RuntimeName: executor.NameKubernetes, isResumedSession: true,
		metadata: map[string]interface{}{
			MetadataKeyAuthTokenSecret:      "runtime-auth",
			MetadataKeyBootstrapNonceSecret: "runtime-bootstrap",
		},
	}))

	err := mgr.StopAgentWithReason(
		context.Background(), "execution-1", StopReasonAgentBootstrapFailed, true,
	)

	require.NoError(t, err)
	require.Equal(t, []bool{false}, backend.forces,
		"failed resume cleanup must only close process-local Kubernetes connections")
	require.Len(t, secretStore.store, 2, "retained runtime secrets must remain available for retry")
	_, exists := mgr.executionStore.Get("execution-1")
	require.False(t, exists, "failed execution must release its in-memory session slot")
}

func TestFailedFreshKubernetesLaunchStillCleansItsRuntime(t *testing.T) {
	log := newTestRegistryLogger()
	executorRegistry := NewExecutorRegistry(log)
	backend := &stopTrackingExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}}
	executorRegistry.Register(backend)
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, executorRegistry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID: "execution-1", TaskID: "task-1", SessionID: "session-1",
		RuntimeName: executor.NameKubernetes,
	}))

	err := mgr.StopAgentWithReason(
		context.Background(), "execution-1", StopReasonAgentBootstrapFailed, true,
	)

	require.NoError(t, err)
	require.Equal(t, []bool{true}, backend.forces,
		"a failed fresh launch must reclaim the runtime it created")
}
