package lifecycle

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

func newNomadForceStopManager(t *testing.T, id string, handler http.HandlerFunc) *Manager {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)
	host, portString, err := net.SplitHostPort(parsed.Host)
	require.NoError(t, err)
	port, err := strconv.Atoi(portString)
	require.NoError(t, err)

	log := newTestRegistryLogger()
	execRegistry := NewExecutorRegistry(log)
	execRegistry.Register(&retryableStopBackend{MockExecutor: MockExecutor{name: executor.NameNomad}})
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, execRegistry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID: id, SessionID: "session-" + id, RuntimeName: executor.NameNomad,
		agentctl: agentctl.NewClient(host, port, log),
	}))
	previous := nomadAgentctlStopTimeout
	nomadAgentctlStopTimeout = 200 * time.Millisecond
	t.Cleanup(func() { nomadAgentctlStopTimeout = previous })
	return mgr
}

func TestNomadForceStopKeepsExecutionTrackedWhenAgentStopFails(t *testing.T) {
	mgr := newNomadForceStopManager(t, "exec-force-fail", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "stop failed", http.StatusInternalServerError)
	})

	err := mgr.StopAgentWithReason(context.Background(), "exec-force-fail", "", true)

	require.Error(t, err, "an unconfirmed agent stop must not report stopped")
	_, exists := mgr.executionStore.Get("exec-force-fail")
	require.True(t, exists, "the execution stays tracked so the stop can be retried")
}

func TestNomadForceStopReleasesExecutionWhenAgentStopSucceeds(t *testing.T) {
	var stopCalls int
	mgr := newNomadForceStopManager(t, "exec-force-ok", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/stop" {
			stopCalls++
		}
		_, _ = w.Write([]byte(`{"success":true}`))
	})

	require.NoError(t, mgr.StopAgentWithReason(context.Background(), "exec-force-ok", "", true))

	require.Equal(t, 1, stopCalls, "a forced Nomad stop must still ask agentctl to stop the agent")
	_, exists := mgr.executionStore.Get("exec-force-ok")
	require.False(t, exists)
}
