package lifecycle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

func newAgentctlStatusServer(t *testing.T, status int) (host string, port int) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	t.Cleanup(server.Close)
	return parseHTTPURL(t, server.URL)
}

// @covers AC-EXECUTORS-REMOTE-DOCKER-001.12
func TestRemoteDockerRecoverableFailurePreservesRuntime(t *testing.T) {
	for _, tc := range []struct {
		name     string
		reason   string
		resumed  bool
		attached bool
		force    bool
		want     bool
	}{
		{"established recoverable failure", StopReasonRecoverableAgentFailure, false, false, true, false},
		{"resumed recoverable failure", StopReasonRecoverableAgentFailure, true, true, true, false},
		{"reused bootstrap failure", StopReasonAgentBootstrapFailed, false, true, true, false},
		{"fresh bootstrap rollback", StopReasonAgentBootstrapFailed, false, false, true, true},
		{"conversation resume on fresh compute", StopReasonAgentBootstrapFailed, true, false, true, true},
		{"archive remains destructive", StopReasonTaskArchived, true, true, true, true},
		{"delete remains destructive", StopReasonTaskDeleted, true, true, true, true},
		{"explicit force remains destructive", "", true, true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := newTestRegistryLogger()
			registry := NewExecutorRegistry(log)
			backend := &stopTrackingExecutor{MockExecutor: MockExecutor{name: executor.NameRemoteDocker}}
			registry.Register(backend)
			mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
			cleanupManagerStopCh(t, mgr)
			host, port := newAgentctlStatusServer(t, http.StatusOK)
			execution := &AgentExecution{
				ID: "execution-1", TaskID: "task-1", SessionID: "session-1",
				RuntimeName: executor.NameRemoteDocker, isResumedSession: tc.resumed,
				attachedExistingCompute: tc.attached,
				agentctl:                agentctl.NewClient(host, port, log),
				ContainerID:             "container-1",
			}
			require.NoError(t, mgr.executionStore.Add(execution))

			require.NoError(t, mgr.StopAgentWithReason(context.Background(), execution.ID, tc.reason, tc.force))
			require.Equal(t, []bool{tc.want}, backend.forces)
			_, stillTracked := mgr.executionStore.Get(execution.ID)
			require.False(t, stillTracked, "a successful stop releases the in-memory execution slot")
		})
	}
}

// @covers AC-EXECUTORS-REMOTE-DOCKER-001.12
func TestRemoteDockerFailedResumePreservesRuntime(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	backend := &stopTrackingExecutor{MockExecutor: MockExecutor{name: executor.NameRemoteDocker}}
	registry.Register(backend)
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	host, port := newAgentctlStatusServer(t, http.StatusOK)
	execution := &AgentExecution{
		ID: "execution-1", TaskID: "task-1", SessionID: "session-1",
		RuntimeName: executor.NameRemoteDocker, isResumedSession: true,
		attachedExistingCompute: true,
		agentctl:                agentctl.NewClient(host, port, log),
		ContainerID:             "container-1",
		metadata:                map[string]interface{}{"retained": "evidence"},
	}
	require.NoError(t, mgr.executionStore.Add(execution))

	require.NoError(t, mgr.StopAgentWithReason(context.Background(), execution.ID, StopReasonAgentBootstrapFailed, true))
	require.Equal(t, []bool{false}, backend.forces,
		"a resumed attempt that attached to retained compute must not force-remove it")
	require.Equal(t, map[string]interface{}{"retained": "evidence"}, execution.MetadataSnapshot())
}

// @covers AC-EXECUTORS-REMOTE-DOCKER-001.12
func TestRemoteDockerRecoverableStopFailureRetainsOwner(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	backend := &stopTrackingExecutor{MockExecutor: MockExecutor{name: executor.NameRemoteDocker}}
	registry.Register(backend)
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	host, port := newAgentctlStatusServer(t, http.StatusInternalServerError)
	execution := &AgentExecution{
		ID: "execution-1", TaskID: "task-1", SessionID: "session-1",
		RuntimeName: executor.NameRemoteDocker,
		agentctl:    agentctl.NewClient(host, port, log),
		ContainerID: "container-1",
	}
	require.NoError(t, mgr.executionStore.Add(execution))

	err := mgr.StopAgentWithReason(context.Background(), execution.ID, StopReasonRecoverableAgentFailure, true)
	require.ErrorIs(t, err, errRecoverableStopUnconfirmed)
	require.Empty(t, backend.forces, "an unconfirmed stop must not reach destructive runtime cleanup")
	current, exists := mgr.executionStore.Get(execution.ID)
	require.True(t, exists, "the failed stop keeps its exact cleanup ownership for retry")
	require.Same(t, execution, current)
}

// @covers AC-EXECUTORS-REMOTE-DOCKER-001.12
func TestRemoteDockerResumeRollbackPreservesRuntime(t *testing.T) {
	for _, tc := range []struct {
		name    string
		resumed bool
	}{
		{"resumed session with previous execution", true},
		{"workspace-only attach without previous execution", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := newTestRegistryLogger()
			registry := NewExecutorRegistry(log)
			backend := &stopTrackingExecutor{MockExecutor: MockExecutor{name: executor.NameRemoteDocker}}
			registry.Register(backend)
			mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
			cleanupManagerStopCh(t, mgr)
			execution := &AgentExecution{
				ID: "execution-1", TaskID: "task-1", SessionID: "session-1",
				RuntimeName: executor.NameRemoteDocker, isResumedSession: tc.resumed,
				attachedExistingCompute: true,
				ContainerID:             "container-1",
			}
			require.NoError(t, mgr.executionStore.Add(execution))

			execInstance := &ExecutorInstance{InstanceID: execution.ID, ContainerID: "container-1"}
			require.NoError(t, mgr.stopRegisteredLaunchRuntime(backend, execInstance, execution))
			require.Equal(t, []bool{false}, backend.forces)
			require.Equal(t, StopReasonAgentBootstrapFailed, execInstance.StopReason,
				"a retained rollback must keep a preserving stop reason")
			mgr.finishRegisteredLaunchRollback(execution, false, true)
			_, exists := mgr.executionStore.Get(execution.ID)
			require.False(t, exists, "the failed attempt releases its in-memory slot")
		})
	}
}
