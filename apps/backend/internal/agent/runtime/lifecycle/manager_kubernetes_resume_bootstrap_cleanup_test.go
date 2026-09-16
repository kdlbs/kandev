package lifecycle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/secrets"
)

func TestFailedKubernetesResumePreservesRetainedRuntimeAndSecrets(t *testing.T) {
	log := newTestRegistryLogger()
	stopRequests := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stopRequests <- r.Method + " " + r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	t.Cleanup(server.Close)
	host, port := parseHTTPURL(t, server.URL)

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
		agentctl: agentctl.NewClient(host, port, log),
		metadata: map[string]interface{}{
			MetadataKeyAuthTokenSecret:      "runtime-auth",
			MetadataKeyBootstrapNonceSecret: "runtime-bootstrap",
		},
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := mgr.StopAgentWithReason(ctx, "execution-1", StopReasonAgentBootstrapFailed, true)

	require.NoError(t, err)
	select {
	case request := <-stopRequests:
		require.Equal(t, "POST /api/v1/stop", request)
	case <-time.After(time.Second):
		t.Fatal("the failed process was not stopped before its Kubernetes runtime was retained")
	}
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
