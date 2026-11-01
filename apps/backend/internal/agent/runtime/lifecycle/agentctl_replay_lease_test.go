package lifecycle

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

// leaseFree reports whether no client lease is held, which is the state a
// terminal stop needs before it can take the write side.
func leaseFree(execution *AgentExecution) bool {
	if !execution.agentctlLifecycleMu.TryLock() {
		return false
	}
	execution.agentctlLifecycleMu.Unlock()
	return true
}

// @covers AC-PLATFORM-AGENTCTL-CLIENT-LEASE-001.4
func TestRecoveredDeliveryReplayReleasesLeaseBeforeLifecycleCallbacks(t *testing.T) {
	ctx := context.Background()
	payload, err := json.Marshal(agentctl.AgentEvent{Type: "complete"})
	require.NoError(t, err)
	repository, _ := runOwnerTestRepository(t)
	client, _, stream, _ := newIdleActivityJournalPeer(t, journal.Event{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2,
		StreamID: "stream", Type: "complete", Payload: payload, CreatedAt: time.Now().UTC(), Terminal: true,
	}, repository)
	require.NoError(t, repository.CreateTask(ctx, &models.Task{ID: "task", Title: "Replay lease"}))
	require.NoError(t, repository.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session", TaskID: "task", QueueIncarnationID: "incarnation",
	}))
	require.NoError(t, repository.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: "session", IncarnationID: "incarnation", Generation: 2, NativeSessionID: "native", CreationReason: "initial",
	}))
	mgr := newTestManager(t)
	e := createTestExecution("execution", "task", "session")
	e.DeliveryMode, e.DeliveryStreamID, e.DeliveryIncarnationID, e.DeliveryHarnessGeneration = DurableDeliveryV1, "stream", "incarnation", 2
	e.DeliveryDescriptor, e.agentctl = &agentctl.DeliveryStatus{Stream: &stream}, client
	e.Status = v1.AgentStatusRunning
	require.NoError(t, mgr.executionStore.Add(e))
	mgr.SetExecutorRunningWriter(repository)
	require.NoError(t, mgr.persistExecutorRunningResult(ctx, e))

	callbacks := 0
	leaseFreeInCallback := true
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{OnAgentEvent: func(execution *AgentExecution, event agentctl.AgentEvent) {
		callbacks++
		leaseFreeInCallback = leaseFreeInCallback && leaseFree(execution)
		mgr.handleAgentEvent(execution, event)
	}}, nil, nil)
	t.Cleanup(sm.Wait)
	sm.setAgentDeliveryRepository(repository)

	require.NoError(t, sm.ReplayRecoveredDelivery(ctx, e))
	require.Positive(t, callbacks, "the replay must reach the lifecycle callback")
	require.True(t, leaseFreeInCallback,
		"replay must not hold the client lease while lifecycle callbacks run: a callback that "+
			"acquires it again deadlocks once a terminal stop is queued")
}

// @covers AC-PLATFORM-AGENTCTL-CLIENT-LEASE-001.5
func TestEscalateStuckCancelReleasesLeaseWhileWaiting(t *testing.T) {
	peer := newMockAgentServer(t)
	t.Cleanup(peer.Close)
	client := createTestClient(t, peer.server.URL)
	t.Cleanup(client.Close)
	mgr := newTestManager(t)
	execution := newLockOrderTestExecution()
	execution.agentctl = client
	require.NoError(t, mgr.executionStore.Add(execution))

	finished := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- mgr.escalateStuckCancel(ctx, execution, finished) }()
	// The synthetic completion is queued just before the wait begins.
	require.Eventually(t, func() bool { return len(execution.promptDoneCh) == 1 },
		2*time.Second, time.Millisecond, "escalation must reach its wait")

	writerDone := make(chan struct{})
	go func() {
		execution.agentctlLifecycleMu.Lock()
		execution.agentctlLifecycleMu.Unlock() //nolint:staticcheck // simulated terminal stop
		close(writerDone)
	}()
	select {
	case <-writerDone:
	case <-time.After(500 * time.Millisecond):
		cancel()
		<-done
		<-writerDone
		t.Fatal("cancel escalation held the client lease while waiting for the prompt to finish")
	}
	cancel()
	require.Error(t, <-done)
}

// @covers AC-PLATFORM-AGENTCTL-CLIENT-LEASE-001.5
func TestEscalateStuckCancelReleasesLeaseBeforePersistence(t *testing.T) {
	peer := newMockAgentServer(t)
	t.Cleanup(peer.Close)
	client := createTestClient(t, peer.server.URL)
	t.Cleanup(client.Close)
	mgr := newTestManager(t)
	execution := newLockOrderTestExecution()
	execution.agentctl = client
	require.NoError(t, mgr.executionStore.Add(execution))
	writer := &recoveryLeaseWriter{execution: execution}
	mgr.SetExecutorRunningWriter(writer)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = mgr.escalateStuckCancel(ctx, execution, make(chan struct{}))
	require.Positive(t, writer.reads, "the ready persistence must run")
	require.True(t, writer.leaseAvailable, "cancel escalation must not hold the lease across persistence")
}

// @covers AC-PLATFORM-AGENTCTL-CLIENT-LEASE-001.5
func TestCancelAgentExecutionReleasesLeaseBeforeEscalation(t *testing.T) {
	peer := newMockAgentServer(t)
	t.Cleanup(peer.Close)
	client := createTestClient(t, peer.server.URL)
	t.Cleanup(client.Close)
	mgr := newTestManager(t)
	execution := newLockOrderTestExecution()
	execution.agentctl = client
	execution.dispatchedPromptPending.Store(true)
	require.NoError(t, mgr.executionStore.Add(execution))
	writer := &recoveryLeaseWriter{execution: execution}
	mgr.SetExecutorRunningWriter(writer)

	_ = mgr.cancelAgentExecution(context.Background(), execution)
	require.Positive(t, writer.reads, "the escalation must persist readiness")
	require.True(t, writer.leaseAvailable, "cancel must not hold the lease across escalation persistence")
}
