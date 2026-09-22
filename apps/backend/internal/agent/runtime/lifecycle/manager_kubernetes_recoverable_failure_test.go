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
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.1
func TestKubernetesRecoverableFailurePreservesRuntimeAndSecrets(t *testing.T) {
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
		RuntimeName: executor.NameKubernetes, isResumedSession: false,
		agentctl: agentctl.NewClient(host, port, log),
		metadata: map[string]interface{}{
			MetadataKeyAuthTokenSecret:      "runtime-auth",
			MetadataKeyBootstrapNonceSecret: "runtime-bootstrap",
		},
	}))

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := mgr.StopAgentWithReason(ctx, "execution-1", StopReasonRecoverableAgentFailure, true)

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

// @covers AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.1
// @covers AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.4
func TestKubernetesRecoverableFailureStopPolicy(t *testing.T) {
	for _, tc := range []struct {
		name     string
		runtime  executor.Name
		resumed  bool
		reason   string
		preserve bool
	}{
		{"fresh established session", executor.NameKubernetes, false, StopReasonRecoverableAgentFailure, true},
		{"resumed established session", executor.NameKubernetes, true, StopReasonRecoverableAgentFailure, true},
		{"archive remains destructive", executor.NameKubernetes, true, StopReasonTaskArchived, false},
		{"delete remains destructive", executor.NameKubernetes, true, StopReasonTaskDeleted, false},
		{"explicit force remains destructive", executor.NameKubernetes, true, "", false},
		{"fresh bootstrap rollback", executor.NameKubernetes, false, StopReasonAgentBootstrapFailed, false},
		{"standalone unchanged", executor.NameStandalone, false, StopReasonRecoverableAgentFailure, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			log := newTestRegistryLogger()
			registry := NewExecutorRegistry(log)
			backend := &stopTrackingExecutor{MockExecutor: MockExecutor{name: tc.runtime}}
			registry.Register(backend)
			mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
			cleanupManagerStopCh(t, mgr)
			store := newInMemorySecretStore()
			mgr.SetSecretStore(store)
			metadata := map[string]interface{}{
				MetadataKeyAuthTokenSecret: "retained-auth", MetadataKeyBootstrapNonceSecret: "retained-bootstrap",
			}
			for _, id := range []string{"retained-auth", "retained-bootstrap"} {
				require.NoError(t, store.Create(ctx, &secrets.SecretWithValue{Secret: secrets.Secret{ID: id, Name: id}, Value: id + "-value"}))
			}
			execution := &AgentExecution{ID: "execution-1", TaskID: "task-1", SessionID: "session-1",
				RuntimeName: tc.runtime, isResumedSession: tc.resumed, metadata: metadata}
			require.NoError(t, mgr.executionStore.Add(execution))
			require.NoError(t, mgr.StopAgentWithReason(ctx, execution.ID, tc.reason, true))
			require.Equal(t, []bool{!tc.preserve}, backend.forces)
			for _, id := range []string{"retained-auth", "retained-bootstrap"} {
				value, err := store.Reveal(ctx, id)
				if tc.preserve || tc.runtime != executor.NameKubernetes {
					require.NoError(t, err)
					require.Equal(t, id+"-value", value)
				} else {
					require.ErrorIs(t, err, secrets.ErrNotFound)
				}
			}
			require.Equal(t, metadata, execution.MetadataSnapshot())
			_, exists := mgr.executionStore.Get(execution.ID)
			require.False(t, exists)
		})
	}
}

// @covers AC-EXECUTORS-K8S-FAILURE-RECOVERY-001.3
func TestKubernetesRecoverableFailureCleanupCannotStopReplacement(t *testing.T) {
	ctx := context.Background()
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	backend := &stopTrackingExecutor{MockExecutor: MockExecutor{name: executor.NameKubernetes}}
	registry.Register(backend)
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	store := newInMemorySecretStore()
	mgr.SetSecretStore(store)
	require.NoError(t, store.Create(ctx, &secrets.SecretWithValue{
		Secret: secrets.Secret{ID: "shared-runtime-auth", Name: "auth"}, Value: "retained-token",
	}))
	old := &AgentExecution{ID: "old-execution", SessionID: "session", RuntimeName: executor.NameKubernetes,
		metadata: map[string]interface{}{MetadataKeyAuthTokenSecret: "shared-runtime-auth"}}
	require.NoError(t, mgr.executionStore.Add(old))
	require.NoError(t, mgr.StopAgentWithReason(ctx, old.ID, StopReasonRecoverableAgentFailure, true))
	mgr.SetExecutorRunningWriter(&restartKubernetesInventoryStore{rows: []*models.ExecutorRunning{{
		AgentExecutionID: old.ID, SessionID: old.SessionID, Runtime: agentruntime.RuntimeKubernetes,
	}}})
	replacement := &AgentExecution{ID: "replacement", SessionID: "session", RuntimeName: executor.NameKubernetes,
		metadata: old.MetadataSnapshot(), isResumedSession: true}
	require.NoError(t, mgr.executionStore.Add(replacement))
	require.ErrorIs(t, mgr.StopAgentWithReason(ctx, old.ID, StopReasonRecoverableAgentFailure, true), ErrExecutionNotFound)
	current, exists := mgr.executionStore.GetBySessionID("session")
	require.True(t, exists)
	require.Same(t, replacement, current)
	require.Equal(t, 1, backend.stopCalls)
	token, err := mgr.resolveLaunchAuthToken(ctx, &LaunchRequest{ExecutorType: "k8s"}, replacement.MetadataSnapshot())
	require.NoError(t, err)
	require.Equal(t, "retained-token", token)
}
