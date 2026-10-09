package lifecycle

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
)

type reconciliationTestClock struct {
	mu    sync.Mutex
	now   time.Time
	waits []time.Duration
}

func newReconciliationTestClock() *reconciliationTestClock {
	return &reconciliationTestClock{now: time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)}
}

func (c *reconciliationTestClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *reconciliationTestClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func (c *reconciliationTestClock) Wait(_ context.Context, d time.Duration) error {
	c.mu.Lock()
	c.waits = append(c.waits, d)
	c.now = c.now.Add(d)
	c.mu.Unlock()
	return nil
}

func (c *reconciliationTestClock) Waits() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]time.Duration(nil), c.waits...)
}

type fakeDeliveryReconciliationPeer struct {
	mu              sync.Mutex
	status          func(context.Context, int) (*agentctl.StatusResponse, error)
	descriptor      func(context.Context, int) (*agentctl.DeliveryStatus, error)
	submission      func(context.Context, int) (*journal.Submission, error)
	attach          func(context.Context) error
	statusCalls     int
	descriptorCalls int
	submissionCalls int
	attachCalls     int
	streamOpen      bool
}

func (p *fakeDeliveryReconciliationPeer) GetStatus(ctx context.Context) (*agentctl.StatusResponse, error) {
	p.mu.Lock()
	p.statusCalls++
	n := p.statusCalls
	fn := p.status
	p.mu.Unlock()
	return fn(ctx, n)
}

func (p *fakeDeliveryReconciliationPeer) GetDeliveryStatus(ctx context.Context, _ string) (*agentctl.DeliveryStatus, error) {
	p.mu.Lock()
	p.descriptorCalls++
	n := p.descriptorCalls
	fn := p.descriptor
	p.mu.Unlock()
	return fn(ctx, n)
}

func (p *fakeDeliveryReconciliationPeer) GetDeliverySubmission(ctx context.Context, _ string) (*journal.Submission, error) {
	p.mu.Lock()
	p.submissionCalls++
	n := p.submissionCalls
	fn := p.submission
	p.mu.Unlock()
	return fn(ctx, n)
}

func (p *fakeDeliveryReconciliationPeer) HasAgentStream() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.streamOpen
}

func (p *fakeDeliveryReconciliationPeer) AttachAgentStream(ctx context.Context) error {
	p.mu.Lock()
	p.attachCalls++
	fn := p.attach
	p.mu.Unlock()
	if err := fn(ctx); err != nil {
		return err
	}
	p.mu.Lock()
	p.streamOpen = true
	p.mu.Unlock()
	return nil
}

func (p *fakeDeliveryReconciliationPeer) Counts() (status, descriptor, submission, attach int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.statusCalls, p.descriptorCalls, p.submissionCalls, p.attachCalls
}

func newDeliveryReconciliationTestPeer() *fakeDeliveryReconciliationPeer {
	return &fakeDeliveryReconciliationPeer{
		status: func(context.Context, int) (*agentctl.StatusResponse, error) {
			return &agentctl.StatusResponse{AgentStatus: "running"}, nil
		},
		descriptor: func(context.Context, int) (*agentctl.DeliveryStatus, error) {
			return &agentctl.DeliveryStatus{
				StorageCapability: journal.StorageCapability{Version: 1, Durable: true},
				SessionID:         "session-1", IncarnationID: "incarnation-1", HarnessGeneration: 3,
				StreamID: "stream-1",
			}, nil
		},
		submission: func(context.Context, int) (*journal.Submission, error) {
			return &journal.Submission{
				ID: "submission-1", StreamID: "stream-1", SessionID: "session-1",
				IncarnationID: "incarnation-1", HarnessGeneration: 3,
				State: journal.SubmissionDispatching,
			}, nil
		},
		attach: func(context.Context) error { return nil },
	}
}

func newDeliveryReconciliationTestExecution() *AgentExecution {
	execution := &AgentExecution{
		ID: "execution-1", SessionID: "session-1", Owner: ExecutionOwner{
			Kind: ExecutionOwnerTask, TaskID: "task-1", SessionID: "session-1", WorkspaceID: "workspace-1",
		},
		DeliveryMode: DurableDeliveryV1, DeliveryStreamID: "stream-1",
		DeliveryIncarnationID: "incarnation-1", DeliveryHarnessGeneration: 3,
		Status: "running", runtimeEpoch: 0,
		promptGeneration: 1, dispatchedPromptGeneration: 1,
		promptDoneCh: make(chan PromptCompletionSignal, 1),
	}
	execution.setDeliverySubmissionID("submission-1")
	return execution
}

func newDeliveryReconciliationTestManager(clock *reconciliationTestClock) *StreamManager {
	sm := NewStreamManager(newTestLogger(), StreamCallbacks{}, nil, nil)
	sm.reconciliationNow = clock.Now
	sm.reconciliationWait = clock.Wait
	sm.isExecutionCurrent = func(*AgentExecution) bool { return true }
	return sm
}

func TestDeliveryReconcileBoundedWindow(t *testing.T) {
	clock := newReconciliationTestClock()
	sm := newDeliveryReconciliationTestManager(clock)
	peer := newDeliveryReconciliationTestPeer()
	peer.status = func(context.Context, int) (*agentctl.StatusResponse, error) {
		clock.Advance(2 * time.Second)
		return nil, errors.New("peer unavailable")
	}
	result := sm.reconcileDeliveryWithPeer(context.Background(), newDeliveryReconciliationTestExecution(), peer)
	if result.Outcome != DeliveryReconciliationUncertain {
		t.Fatalf("outcome = %q, want uncertain", result.Outcome)
	}
	if result.Attempts != 3 {
		t.Fatalf("attempts = %d, want 3", result.Attempts)
	}
	if elapsed := clock.Now().Sub(time.Date(2026, 9, 27, 0, 0, 0, 0, time.UTC)); elapsed > deliveryReconciliationWindow {
		t.Fatalf("reconciliation elapsed %s, exceeds %s", elapsed, deliveryReconciliationWindow)
	}
	if got, want := clock.Waits(), []time.Duration{time.Second, 2 * time.Second}; !equalDurations(got, want) {
		t.Fatalf("waits = %v, want %v", got, want)
	}
	statusCalls, descriptorCalls, submissionCalls, attachCalls := peer.Counts()
	if statusCalls != 3 || descriptorCalls != 0 || submissionCalls != 0 || attachCalls != 0 {
		t.Fatalf("peer calls = status:%d descriptor:%d submission:%d attach:%d", statusCalls, descriptorCalls, submissionCalls, attachCalls)
	}
	sm.Wait()
}

func TestDeliveryReconcileRearmedAfterOutage(t *testing.T) {
	clock := newReconciliationTestClock()
	sm := newDeliveryReconciliationTestManager(clock)
	peer := newDeliveryReconciliationTestPeer()
	peer.status = func(_ context.Context, call int) (*agentctl.StatusResponse, error) {
		if call <= deliveryReconciliationAttemptLimit {
			return nil, errors.New("outage")
		}
		return &agentctl.StatusResponse{AgentStatus: "running"}, nil
	}
	execution := newDeliveryReconciliationTestExecution()
	first := sm.reconcileDeliveryWithPeer(context.Background(), execution, peer)
	if first.Outcome != DeliveryReconciliationUncertain {
		t.Fatalf("first outcome = %q, want uncertain", first.Outcome)
	}
	second := sm.reconcileDeliveryWithPeer(context.Background(), execution, peer)
	if second.Outcome != DeliveryReconciliationRunningAttached {
		t.Fatalf("second outcome = %q, want running_attached", second.Outcome)
	}
	if second.Attempts != 1 {
		t.Fatalf("second attempts = %d, want one fresh attempt", second.Attempts)
	}
	sm.Wait()
}

func TestDeliveryReconcileSingleFlight(t *testing.T) {
	clock := newReconciliationTestClock()
	sm := newDeliveryReconciliationTestManager(clock)
	peer := newDeliveryReconciliationTestPeer()
	entered := make(chan struct{})
	release := make(chan struct{})
	peer.status = func(ctx context.Context, call int) (*agentctl.StatusResponse, error) {
		if call == 1 {
			close(entered)
			select {
			case <-release:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return &agentctl.StatusResponse{AgentStatus: "running"}, nil
	}
	execution := newDeliveryReconciliationTestExecution()
	results := make(chan DeliveryReconciliationResult, 2)
	go func() { results <- sm.reconcileDeliveryWithPeer(context.Background(), execution, peer) }()
	<-entered
	go func() { results <- sm.reconcileDeliveryWithPeer(context.Background(), execution, peer) }()
	waitForReconciliationJoin(t, sm, execution.ID)
	close(release)
	first, second := <-results, <-results
	if first.Outcome != DeliveryReconciliationRunningAttached || second.Outcome != first.Outcome {
		t.Fatalf("outcomes = %q, %q; want both running_attached", first.Outcome, second.Outcome)
	}
	statusCalls, _, _, attachCalls := peer.Counts()
	if statusCalls != 1 || attachCalls != 1 {
		t.Fatalf("single-flight duplicated work: status calls=%d attach calls=%d", statusCalls, attachCalls)
	}
	sm.Wait()
}

func TestDeliveryReconcileRejectsStaleOwner(t *testing.T) {
	clock := newReconciliationTestClock()
	sm := newDeliveryReconciliationTestManager(clock)
	peer := newDeliveryReconciliationTestPeer()
	entered := make(chan struct{})
	release := make(chan struct{})
	var current atomic.Bool
	current.Store(true)
	sm.isExecutionCurrent = func(*AgentExecution) bool { return current.Load() }
	peer.status = func(ctx context.Context, _ int) (*agentctl.StatusResponse, error) {
		close(entered)
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return &agentctl.StatusResponse{AgentStatus: "running"}, nil
	}
	var phases []DeliveryReconciliationPhase
	sm.onDeliveryReconciliationPhase = func(_ *AgentExecution, _ DeliveryReconciliationIdentity, phase DeliveryReconciliationPhase) {
		phases = append(phases, phase)
	}
	resultCh := make(chan DeliveryReconciliationResult, 1)
	go func() {
		resultCh <- sm.reconcileDeliveryWithPeer(context.Background(), newDeliveryReconciliationTestExecution(), peer)
	}()
	<-entered
	current.Store(false)
	close(release)
	result := <-resultCh
	if result.Outcome != DeliveryReconciliationOwnerMismatch {
		t.Fatalf("outcome = %q, want owner_mismatch", result.Outcome)
	}
	if len(phases) != 1 || phases[0] != DeliveryReconciliationPhaseReconnecting {
		t.Fatalf("stale owner phases = %v, want only initial reconnecting", phases)
	}
	sm.Wait()
}

func TestDeliveryReconcileStopCancelsCycle(t *testing.T) {
	clock := newReconciliationTestClock()
	sm := newDeliveryReconciliationTestManager(clock)
	peer := newDeliveryReconciliationTestPeer()
	entered := make(chan struct{})
	peer.status = func(ctx context.Context, _ int) (*agentctl.StatusResponse, error) {
		close(entered)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	execution := newDeliveryReconciliationTestExecution()
	resultCh := make(chan DeliveryReconciliationResult, 1)
	go func() { resultCh <- sm.reconcileDeliveryWithPeer(context.Background(), execution, peer) }()
	<-entered
	sm.cancelDeliveryReconciliation(execution.ID)
	result := <-resultCh
	if result.Outcome != DeliveryReconciliationTransportUnavailable || !errors.Is(result.Err, context.Canceled) {
		t.Fatalf("stop result = (%q, %v), want transport_unavailable with cancellation", result.Outcome, result.Err)
	}
	retry := sm.reconcileDeliveryWithPeer(context.Background(), execution, peer)
	if retry.Outcome != DeliveryReconciliationTransportUnavailable || !errors.Is(retry.Err, context.Canceled) {
		t.Fatalf("retry after Stop = (%q, %v), want canceled", retry.Outcome, retry.Err)
	}
	sm.Wait()
}

func TestDeliveryReconcileMissingExecutorTunnel(t *testing.T) {
	clock := newReconciliationTestClock()
	sm := newDeliveryReconciliationTestManager(clock)
	result := sm.ReconcileAgentDelivery(context.Background(), newDeliveryReconciliationTestExecution())
	if result.Outcome != DeliveryReconciliationTransportUnavailable || !errors.Is(result.Err, ErrDeliveryTransportUnavailable) {
		t.Fatalf("missing tunnel result = (%q, %v), want transport_unavailable", result.Outcome, result.Err)
	}
	sm.Wait()
}

func TestDeliveryReconcileLivePeer(t *testing.T) {
	clock := newReconciliationTestClock()
	sm := newDeliveryReconciliationTestManager(clock)
	peer := newDeliveryReconciliationTestPeer()
	execution := newDeliveryReconciliationTestExecution()
	var phases []DeliveryReconciliationPhase
	sm.onDeliveryReconciliationPhase = func(_ *AgentExecution, _ DeliveryReconciliationIdentity, phase DeliveryReconciliationPhase) {
		phases = append(phases, phase)
	}
	result := sm.reconcileDeliveryWithPeer(context.Background(), execution, peer)
	if result.Outcome != DeliveryReconciliationRunningAttached {
		t.Fatalf("outcome = %q, want running_attached", result.Outcome)
	}
	if execution.promptCompletionGeneration == execution.promptGeneration || execution.dispatchedPromptGeneration != execution.promptGeneration {
		t.Fatal("attaching a live peer changed the active-turn admission fence")
	}
	if !peer.HasAgentStream() {
		t.Fatal("live peer did not reattach its updates stream")
	}
	if len(phases) != 2 || phases[0] != DeliveryReconciliationPhaseReconnecting ||
		phases[1] != DeliveryReconciliationPhaseRecovered {
		t.Fatalf("live peer phases = %v, want reconnecting then recovered after owner verification", phases)
	}
	_, _, _, attachCalls := peer.Counts()
	if attachCalls != 1 {
		t.Fatalf("attach calls = %d, want one", attachCalls)
	}
	sm.Wait()
}

func equalDurations(got, want []time.Duration) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func waitForReconciliationJoin(t *testing.T, sm *StreamManager, executionID string) {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		sm.reconciliationMu.Lock()
		cycle := sm.deliveryReconciliations[executionID]
		joined := cycle != nil && cycle.waiters > 0
		sm.reconciliationMu.Unlock()
		if joined {
			return
		}
		select {
		case <-deadline:
			t.Fatal("second caller did not join the active reconciliation")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}
