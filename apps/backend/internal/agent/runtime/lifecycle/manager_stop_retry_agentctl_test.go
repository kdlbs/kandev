package lifecycle

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// This reviewer-requested contract test documents that Client.Close drains
// streams and idle connections without disabling later HTTP stop retries.
func TestStopAgentWithReason_BackendRetryReusesClosedAgentctlClient(t *testing.T) {
	var stopRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/stop" {
			http.NotFound(w, r)
			return
		}
		stopRequests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	t.Cleanup(server.Close)

	log := newTestRegistryLogger()
	parsedURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	host, portString, err := net.SplitHostPort(parsedURL.Host)
	require.NoError(t, err)
	port, err := strconv.Atoi(portString)
	require.NoError(t, err)

	execRegistry := NewExecutorRegistry(log)
	backend := &retryableStopBackend{
		MockExecutor: MockExecutor{name: executor.NameStandalone},
		stopErr:      errors.New("runtime stop failed"),
	}
	execRegistry.Register(backend)
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, execRegistry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)

	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID:          "exec-agentctl-retry",
		SessionID:   "session-agentctl-retry",
		RuntimeName: executor.NameStandalone,
		agentctl:    agentctl.NewClient(host, port, log),
	}))

	err = mgr.StopAgentWithReason(context.Background(), "exec-agentctl-retry", "first attempt", false)
	require.ErrorIs(t, err, backend.stopErr)

	backend.stopErr = nil
	require.NoError(t, mgr.StopAgentWithReason(context.Background(), "exec-agentctl-retry", "retry", false))
	require.Equal(t, int32(2), stopRequests.Load(), "the retry must reach agentctl after the first Close")
	_, exists := mgr.executionStore.Get("exec-agentctl-retry")
	require.False(t, exists)
}
