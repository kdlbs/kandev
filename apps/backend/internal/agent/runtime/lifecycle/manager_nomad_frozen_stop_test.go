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

// newFrozenAgentctlExecution tracks a Nomad execution whose agentctl accepts
// connections and never answers, like a SIGSTOPped worker leader.
func newFrozenAgentctlExecution(t *testing.T, id string) (*Manager, *AgentExecution) {
	t.Helper()
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	t.Cleanup(func() {
		close(release)
		server.Close()
	})
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
	execution := &AgentExecution{
		ID: id, SessionID: "session-" + id, RuntimeName: executor.NameNomad,
		agentctl: agentctl.NewClient(host, port, log),
	}
	require.NoError(t, mgr.executionStore.Add(execution))

	previous := nomadAgentctlStopTimeout
	nomadAgentctlStopTimeout = 200 * time.Millisecond
	t.Cleanup(func() { nomadAgentctlStopTimeout = previous })
	return mgr, execution
}

func TestNomadHandoffStopReturnsWithinBoundWhenAgentctlIsFrozen(t *testing.T) {
	mgr, _ := newFrozenAgentctlExecution(t, "exec-frozen-handoff")

	done := make(chan error, 1)
	go func() {
		done <- mgr.StopAgentWithReason(context.Background(), "exec-frozen-handoff", StopReasonTaskTreeArchived, false)
	}()
	select {
	case err := <-done:
		require.NoError(t, err, "the transport-only handoff stop leaves the retained job to the cleanup claim")
	case <-time.After(5 * time.Second):
		t.Fatal("handoff stop blocked on a frozen agentctl past its bound")
	}
	_, exists := mgr.executionStore.Get("exec-frozen-handoff")
	require.False(t, exists, "the transport must be detached so the archive can proceed")
}

func TestStopAllAgentsHonorsShutdownDeadlineBehindBlockedStop(t *testing.T) {
	mgr, execution := newFrozenAgentctlExecution(t, "exec-frozen-shutdown")
	// A prior stop holds the execution's lifecycle lock, as a hung archive stop does.
	execution.remoteInstanceLifecycleMu.Lock()
	t.Cleanup(execution.remoteInstanceLifecycleMu.Unlock)

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- mgr.StopAllAgents(ctx) }()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(5 * time.Second):
		t.Fatal("StopAllAgents ignored its shutdown deadline")
	}
}
