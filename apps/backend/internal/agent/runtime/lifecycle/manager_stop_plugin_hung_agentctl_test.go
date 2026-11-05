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

func hungAgentctlClient(t *testing.T) *agentctl.Client {
	t.Helper()
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
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
	return agentctl.NewClient(host, port, newTestRegistryLogger())
}

// A frozen agentctl must not consume the whole archive budget: the graceful
// stop is bounded so the archive mutation and the environment cleanup that
// follows still have time to run (drain, then provider destroy).
func TestStopAgentWithReason_PluginRemoteHungAgentctlLeavesBudgetForCleanup(t *testing.T) {
	log := newTestRegistryLogger()
	backend := &MockExecutor{name: executor.NamePluginRemote}
	registry := NewExecutorRegistry(log)
	registry.Register(backend)
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	require.NoError(t, mgr.executionStore.Add(&AgentExecution{
		ID: "exec-plugin-hung", SessionID: "session-plugin-hung", TaskID: "task-plugin-hung",
		RuntimeName: executor.NamePluginRemote, agentctl: hungAgentctlClient(t),
	}))

	const budget = 6 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	start := time.Now()
	err := mgr.StopAgentWithReason(ctx, "exec-plugin-hung", StopReasonTaskTreeArchived, false)

	require.NoError(t, err, "an unconfirmed agentctl stop must not fail the stop")
	require.NoError(t, ctx.Err(), "the stop consumed the entire archive budget")
	require.Less(t, time.Since(start), budget/2, "the graceful stop must be bounded well below the caller budget")
	require.Len(t, backend.stopInstanceCalls, 1)
	require.True(t, backend.stopInstanceCalls[0].AgentStopFailed, "the runtime must learn the agent stop was unconfirmed")
	_, exists := mgr.executionStore.Get("exec-plugin-hung")
	require.False(t, exists)
}

// After the bounded stop the cleanup still reaches the provider: the drain of
// the frozen agentctl misses its deadline, which the destroy reports as
// uncertain so the provider retains and quarantines.
func TestPluginCleanupAfterFrozenAgentctlDestroysWithUncertainDrain(t *testing.T) {
	runtime, operations, _, drainer, _ := archiveDrainFixture(t, nil)
	drainer.err = context.DeadlineExceeded
	_ = destroyClaimEnvironment(runtime)
	require.NotNil(t, operations.destroyRequest, "the provider destroy was never called")
	require.Equal(t, pluginDrainOutcomeUncertain, operations.destroyRequest.GetDrainOutcome())
	require.Nil(t, operations.destroyRequest.GetDrainReceipt())
}

// A read lease held across a call to a frozen agentctl (a git status snapshot
// waits out the 60 s client timeout) must not keep the stop waiting for the
// write side of the lifecycle lock either, before or after the runtime stop.
func TestStopAgentWithReason_PluginRemoteBusyAgentctlLeaseLeavesBudgetForCleanup(t *testing.T) {
	log := newTestRegistryLogger()
	backend := &MockExecutor{name: executor.NamePluginRemote}
	registry := NewExecutorRegistry(log)
	registry.Register(backend)
	mgr := NewManager(newTestRegistry(), &MockEventBus{}, registry, nil, nil, nil, ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	execution := &AgentExecution{
		ID: "exec-plugin-busy", SessionID: "session-plugin-busy", TaskID: "task-plugin-busy",
		RuntimeName: executor.NamePluginRemote, agentctl: hungAgentctlClient(t),
	}
	require.NoError(t, mgr.executionStore.Add(execution))

	_, release := execution.AcquireAgentCtlClient()
	leaseReleased := time.AfterFunc(5*time.Second, release)
	t.Cleanup(func() {
		if leaseReleased.Stop() {
			release()
		}
	})

	const budget = 3 * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()
	err := mgr.StopAgentWithReason(ctx, "exec-plugin-busy", StopReasonTaskTreeArchived, false)

	require.NoError(t, err)
	require.NoError(t, ctx.Err(), "waiting for the busy client consumed the entire archive budget")
	require.Len(t, backend.stopInstanceCalls, 1)
	require.True(t, backend.stopInstanceCalls[0].AgentStopFailed)
	_, exists := mgr.executionStore.Get("exec-plugin-busy")
	require.False(t, exists)

	require.Eventually(t, func() bool {
		return execution.currentAgentCtlClient() == nil
	}, 10*time.Second, 50*time.Millisecond, "the detach must finish once the lease is released")
}
