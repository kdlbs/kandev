package lifecycle

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/events"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const (
	cancelStopJoinTimeout  = 5 * time.Second
	cancelStopBarrierWait  = 2 * time.Second
	cancelStopChildTimeout = 20 * time.Second
	cancelStopHelperEnv    = "KANDEV_LIFECYCLE_CANCEL_STOP_HELPER"
)

type cancelPromptBarrier int

const (
	cancelBarrierOpen cancelPromptBarrier = iota
	cancelBarrierClosed
	cancelBarrierNil
)

type cancelStopHarnessOptions struct {
	cancelNotAcknowledged bool
	disconnected          bool
	deliverySubmissionID  string
	promptGeneration      bool
	dispatchedPending     bool
	barrier               cancelPromptBarrier
	transportFailure      string
}

type cancelStopHarness struct {
	manager        *Manager
	mock           *mockAgentServer
	client         *agentctl.Client
	execution      *AgentExecution
	promptDone     chan PromptCompletionSignal
	promptFinished chan struct{}
	generation     uint64
}

// newCancelStopHarness builds one execution backed by the shared mock agent
// server. The cancel response is configurable so one fixture can cover every
// escalation entry: success, unacknowledged, and a disconnected stream.
func newCancelStopHarness(t *testing.T, options cancelStopHarnessOptions) *cancelStopHarness {
	t.Helper()
	mock := newMockAgentServer(t)
	t.Cleanup(mock.Close)
	mock.handler = func(msg ws.Message) *ws.Message {
		if msg.Action == "agent.cancel" {
			payload := map[string]any{"success": true}
			if options.cancelNotAcknowledged {
				payload = map[string]any{
					"success":          false,
					"not_acknowledged": true,
					"error":            "agent did not acknowledge cancellation",
				}
			}
			resp, _ := ws.NewResponse(msg.ID, msg.Action, payload)
			return resp
		}
		return mock.defaultHandler(msg)
	}

	manager := newTestManager(t)
	execution := &AgentExecution{
		ID:            "exec-cancel-stop",
		TaskID:        "task-cancel-stop",
		SessionID:     "session-cancel-stop",
		Status:        v1.AgentStatusRunning,
		WorkspacePath: "/workspace",
		promptDoneCh:  make(chan PromptCompletionSignal, 1),
	}
	fixture := &cancelStopHarness{
		manager:    manager,
		mock:       mock,
		execution:  execution,
		promptDone: execution.promptDoneCh,
	}
	fixture.setBarrier(options.barrier)

	client := createTestClient(t, mock.server.URL)
	t.Cleanup(client.Close)
	if options.disconnected {
		execution.agentctlOverride.Store(agentctl.NewClient("127.0.0.1", 1, newTestLogger()))
	} else {
		fixture.connectStream(t, client)
	}
	if options.deliverySubmissionID != "" {
		execution.DeliveryMode = DurableDeliveryV1
		execution.setDeliverySubmissionID(options.deliverySubmissionID)
	}
	require.NoError(t, manager.executionStore.Add(execution))
	if options.promptGeneration {
		fixture.admitPrompt(t, options)
	}
	return fixture
}

func (f *cancelStopHarness) setBarrier(barrier cancelPromptBarrier) {
	switch barrier {
	case cancelBarrierOpen:
		f.promptFinished = make(chan struct{})
		f.execution.promptFinished = f.promptFinished
	case cancelBarrierClosed:
		closed := make(chan struct{})
		close(closed)
		f.execution.promptFinished = closed
	case cancelBarrierNil:
	}
}

func (f *cancelStopHarness) connectStream(t *testing.T, client *agentctl.Client) {
	t.Helper()
	streamCtx, streamCancel := context.WithCancel(context.Background())
	t.Cleanup(streamCancel)
	require.NoError(t, client.StreamUpdates(streamCtx, func(_ agentctl.AgentEvent) {}, nil, nil))
	waitForWSConnected(t, f.mock)
	f.execution.agentctl = client
	f.client = client
}

func (f *cancelStopHarness) admitPrompt(t *testing.T, options cancelStopHarnessOptions) {
	t.Helper()
	generation, err := f.manager.executionStore.BeginPrompt(f.execution.ID)
	require.NoError(t, err)
	f.manager.executionStore.MarkPromptDispatched(f.execution.ID, generation)
	f.execution.dispatchedPromptPending.Store(options.dispatchedPending)
	f.generation = generation
	if options.transportFailure != "" {
		f.execution.promptDoneCh <- PromptCompletionSignal{
			IsError:          true,
			Error:            options.transportFailure,
			PromptGeneration: generation,
		}
	}
}

// holdPromptSnapshotLock parks cancellation after it acquired its client lease
// but before it captures prompt state or escalates. A stop started behind this
// barrier is guaranteed to be queued for the agentctl writer.
func holdPromptSnapshotLock(t *testing.T, execution *AgentExecution) func() {
	t.Helper()
	execution.promptFinishedMu.Lock()
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(execution.promptFinishedMu.Unlock) }
	t.Cleanup(release)
	return release
}

func (f *cancelStopHarness) startCancel(t *testing.T) <-chan error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), cancelStopJoinTimeout)
	t.Cleanup(cancel)
	result := make(chan error, 1)
	go func() { result <- f.manager.CancelAgent(ctx, f.execution.ID) }()
	require.Eventually(t, func() bool {
		return goroutineParkedIn("captureCancelPromptSnapshot", "sync.(*Mutex).Lock")
	}, cancelStopBarrierWait, time.Millisecond,
		"cancellation did not hold its captured client lease before the prompt snapshot")
	return result
}

func (f *cancelStopHarness) startStop(t *testing.T, reason string, force bool) <-chan error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), cancelStopJoinTimeout)
	t.Cleanup(cancel)
	result := make(chan error, 1)
	go func() {
		result <- f.manager.StopAgentWithReason(ctx, f.execution.ID, reason, force)
	}()
	return result
}

func (f *cancelStopHarness) startForcedStop(t *testing.T) <-chan error {
	t.Helper()
	result := f.startStop(t, "session deleted", true)
	require.Eventually(t, func() bool {
		return goroutineParkedIn("stopExecutionAgentctl", "sync.(*RWMutex).Lock")
	}, cancelStopBarrierWait, time.Millisecond,
		"stop did not queue its agentctl writer behind cancellation's read lease")
	return result
}

func (f *cancelStopHarness) waitForRuntimeLockWaiter(t *testing.T) {
	t.Helper()
	require.Eventually(t, func() bool {
		return goroutineParkedIn(
			"StopAgentWithReason",
			"ctxmutex.(*Mutex).LockContext",
			"semaphore.(*Weighted).Acquire",
		)
	}, cancelStopBarrierWait, time.Millisecond,
		"deletion stop did not queue on the execution's runtime lifecycle lock")
}

func awaitCancelStopResult(t *testing.T, result <-chan error, operation string) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(cancelStopJoinTimeout):
		t.Fatalf("%s did not finish with a queued stop writer; escalation likely reacquired the agentctl read lease", operation)
		return nil
	}
}

func (f *cancelStopHarness) startPromptWaiter(t *testing.T) <-chan PromptCompletionSignal {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	signal := make(chan PromptCompletionSignal, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case completion := <-f.promptDone:
			close(f.promptFinished)
			signal <- completion
		case <-ctx.Done():
		}
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(cancelStopJoinTimeout):
		}
	})
	return signal
}

func awaitPromptCompletion(t *testing.T, signal <-chan PromptCompletionSignal) PromptCompletionSignal {
	t.Helper()
	select {
	case completion := <-signal:
		return completion
	case <-time.After(cancelStopJoinTimeout):
		t.Fatal("prompt completion did not join after cancel escalation")
		return PromptCompletionSignal{}
	}
}

func requirePublishedEvent(t *testing.T, mockBus *MockEventBus, eventType string) {
	t.Helper()
	require.Eventually(t, func() bool {
		mockBus.mu.Lock()
		defer mockBus.mu.Unlock()
		for _, event := range mockBus.PublishedEvents {
			if event.Type == eventType {
				return true
			}
		}
		return false
	}, cancelStopJoinTimeout, time.Millisecond, "missing published %s event", eventType)
}

type cancelEscalationCase struct {
	name         string
	options      cancelStopHarnessOptions
	wantContains string
}

func cancelEscalationCases() []cancelEscalationCase {
	return []cancelEscalationCase{
		{
			name:    "ordinary prompt timeout",
			options: cancelStopHarnessOptions{barrier: cancelBarrierOpen},
		},
		{
			name: "dispatch immediate rejection",
			options: cancelStopHarnessOptions{
				barrier:               cancelBarrierNil,
				promptGeneration:      true,
				dispatchedPending:     true,
				cancelNotAcknowledged: true,
			},
		},
		{
			name:    "disconnected stream",
			options: cancelStopHarnessOptions{barrier: cancelBarrierOpen, disconnected: true},
		},
		{
			name: "dispatch missing completion gate",
			options: cancelStopHarnessOptions{
				barrier:          cancelBarrierNil,
				promptGeneration: true,
			},
		},
		{
			name: "dispatch timeout",
			options: cancelStopHarnessOptions{
				barrier:           cancelBarrierNil,
				promptGeneration:  true,
				dispatchedPending: true,
			},
		},
		{
			name: "dispatch timeout with transport error",
			options: cancelStopHarnessOptions{
				barrier:           cancelBarrierNil,
				promptGeneration:  true,
				dispatchedPending: true,
				transportFailure:  "provider stream failed at completion deadline",
			},
			wantContains: "provider stream failed at completion deadline",
		},
	}
}

func cancelEscalationCaseByName(name string) (cancelEscalationCase, bool) {
	for _, testCase := range cancelEscalationCases() {
		if testCase.name == name {
			return testCase, true
		}
	}
	return cancelEscalationCase{}, false
}

// TestCancelAgentEscalationUsesPinnedClient exercises every escalation entry
// while a forced stop writer is already queued. Each schedule runs in a bounded
// helper process because the pre-fix nested read lease is not cancellable.
func TestCancelAgentEscalationUsesPinnedClient(t *testing.T) {
	for _, testCase := range cancelEscalationCases() {
		t.Run(testCase.name, func(t *testing.T) {
			runCancelStopInSubprocess(t, "escalation:"+testCase.name)
		})
	}
}

// TestCancelStopHelperProcess is the bounded child process that owns one
// potentially irrecoverable cancellation schedule. The parent kills it on
// timeout, so blocked locks and goroutines never enter the main test process.
func TestCancelStopHelperProcess(t *testing.T) {
	schedule := os.Getenv(cancelStopHelperEnv)
	if schedule == "" {
		t.Skip("cancel/stop subprocess helper only")
	}
	switch {
	case strings.HasPrefix(schedule, "escalation:"):
		runCancelEscalationSchedule(t, strings.TrimPrefix(schedule, "escalation:"))
	case schedule == "quiescence":
		runCancelStopQuiescenceSchedule(t)
	case strings.HasPrefix(schedule, "settlement:"):
		runCancelSettlementSchedule(t, strings.TrimPrefix(schedule, "settlement:"))
	default:
		t.Fatalf("unknown cancel/stop helper schedule %q", schedule)
	}
}

func runCancelStopInSubprocess(t *testing.T, schedule string) {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), cancelStopChildTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable,
		"-test.run=^TestCancelStopHelperProcess$",
		"-test.v",
		"-test.count=1",
	)
	cmd.Env = append(os.Environ(), cancelStopHelperEnv+"="+schedule)
	cmd.WaitDelay = 5 * time.Second
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	err = cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("%s: helper subprocess exceeded %s and was killed; the blocked cancellation/stop schedule stayed isolated in the child.\n%s",
			schedule, cancelStopChildTimeout, output.String())
	}
	require.NoError(t, err, "%s helper subprocess failed:\n%s", schedule, output.String())
}

func runCancelEscalationSchedule(t *testing.T, name string) {
	t.Helper()
	testCase, ok := cancelEscalationCaseByName(name)
	require.True(t, ok, "unknown escalation schedule %q", name)
	t.Cleanup(setCancelTimeouts(t, 40*time.Millisecond, 40*time.Millisecond))

	fixture := newCancelStopHarness(t, testCase.options)
	var promptWait <-chan PromptCompletionSignal
	if testCase.options.barrier == cancelBarrierOpen {
		promptWait = fixture.startPromptWaiter(t)
	}
	expectedGeneration := fixture.generation

	releaseSnapshot := holdPromptSnapshotLock(t, fixture.execution)
	cancelResult := fixture.startCancel(t)
	stopResult := fixture.startForcedStop(t)
	releaseSnapshot()

	cancelErr := awaitCancelStopResult(t, cancelResult, "cancellation")
	require.ErrorIs(t, cancelErr, ErrCancelEscalated)
	if testCase.wantContains != "" {
		require.ErrorContains(t, cancelErr, testCase.wantContains)
	}
	require.NoError(t, awaitCancelStopResult(t, stopResult, "stop"))

	if promptWait != nil {
		completion := awaitPromptCompletion(t, promptWait)
		require.True(t, completion.IsError)
		require.Contains(t, completion.Error, "cancel escalated")
	}
	if expectedGeneration != 0 {
		require.Equal(t, expectedGeneration, fixture.execution.promptGeneration,
			"escalation must not advance the captured prompt generation")
	}
	_, exists := fixture.manager.executionStore.Get(fixture.execution.ID)
	require.False(t, exists, "forced stop must remove the cancelled execution")

	mockBus, ok := fixture.manager.eventBus.(*MockEventBus)
	require.True(t, ok)
	requirePublishedEvent(t, mockBus, events.AgentReady)
	requirePublishedEvent(t, mockBus, events.AgentStopped)
}

// TestCancelStopAndDeleteQuiescenceComplete covers the reported three-way
// schedule: cancellation holds the client lease, an ordinary stop owns the
// runtime lifecycle lock while queued for the client writer, and a forced
// deletion stop queues behind that runtime lock.
func TestCancelStopAndDeleteQuiescenceComplete(t *testing.T) {
	runCancelStopInSubprocess(t, "quiescence")
}

func runCancelStopQuiescenceSchedule(t *testing.T) {
	t.Helper()
	t.Cleanup(setCancelTimeouts(t, 40*time.Millisecond, 40*time.Millisecond))

	fixture := newCancelStopHarness(t, cancelStopHarnessOptions{barrier: cancelBarrierOpen})
	promptWait := fixture.startPromptWaiter(t)

	unrelated := &AgentExecution{
		ID:        "exec-unrelated",
		TaskID:    "task-unrelated",
		SessionID: "session-unrelated",
		Status:    v1.AgentStatusRunning,
	}
	unrelated.promptGeneration = 7
	require.NoError(t, fixture.manager.executionStore.Add(unrelated))

	releaseSnapshot := holdPromptSnapshotLock(t, fixture.execution)
	cancelResult := fixture.startCancel(t)
	ordinaryStopResult := fixture.startStop(t, "user stop", false)
	require.Eventually(t, func() bool {
		return goroutineParkedIn("stopExecutionAgentctl", "sync.(*RWMutex).Lock")
	}, cancelStopBarrierWait, time.Millisecond,
		"ordinary stop did not queue its agentctl writer behind cancellation's read lease")
	deletionStopResult := fixture.startStop(t, "session deleted", true)
	fixture.waitForRuntimeLockWaiter(t)
	releaseSnapshot()

	require.ErrorIs(t, awaitCancelStopResult(t, cancelResult, "cancellation"), ErrCancelEscalated)
	completion := awaitPromptCompletion(t, promptWait)
	require.True(t, completion.IsError)
	require.NoError(t, awaitCancelStopResult(t, ordinaryStopResult, "ordinary stop"))
	require.ErrorIs(t, awaitCancelStopResult(t, deletionStopResult, "deletion stop"), ErrExecutionNotFound,
		"the losing deletion stop must report confirmed absence for the same execution")

	_, exists := fixture.manager.executionStore.Get(fixture.execution.ID)
	require.False(t, exists, "the winning stop must remove the cancelled execution")

	secondCtx, cancelSecond := context.WithTimeout(context.Background(), cancelStopJoinTimeout)
	defer cancelSecond()
	secondErr := fixture.manager.StopAgentWithReason(
		secondCtx, fixture.execution.ID, "session deleted", true,
	)
	if secondErr != nil {
		require.True(t, errors.Is(secondErr, ErrExecutionNotFound),
			"repeated deletion stop must accept confirmed absence, got %v", secondErr)
	}

	current, exists := fixture.manager.executionStore.Get(unrelated.ID)
	require.True(t, exists, "unrelated execution must remain registered")
	require.Equal(t, v1.AgentStatusRunning, current.Status)
	require.Equal(t, uint64(7), current.promptGeneration)

	mockBus, ok := fixture.manager.eventBus.(*MockEventBus)
	require.True(t, ok)
	requirePublishedEvent(t, mockBus, events.AgentReady)
	requirePublishedEvent(t, mockBus, events.AgentStopped)
}

// TestCancelEscalationSettlesCapturedDurableClientBeforeStop proves settlement
// still runs through the original client after the stop writer queued, and that
// the client is not closed before settlement resolves. Failures keep the joined
// error and the submission identifier for recovery.
func TestCancelEscalationSettlesCapturedDurableClientBeforeStop(t *testing.T) {
	for _, name := range []string{"successful settlement", "failed settlement"} {
		t.Run(name, func(t *testing.T) {
			runCancelStopInSubprocess(t, "settlement:"+name)
		})
	}
}

func runCancelSettlementSchedule(t *testing.T, name string) {
	t.Helper()
	settlementFail := name == "failed settlement"
	switch name {
	case "successful settlement", "failed settlement":
	default:
		t.Fatalf("unknown settlement schedule %q", name)
	}
	t.Cleanup(setCancelTimeouts(t, 30*time.Millisecond, 30*time.Millisecond))

	submissionID := "submission-" + strings.ReplaceAll(name, " ", "-")
	fixture := newCancelStopHarness(t, cancelStopHarnessOptions{
		barrier:              cancelBarrierOpen,
		deliverySubmissionID: submissionID,
	})
	settlementStarted, releaseSettlement := fixture.stubSettlement(t, settlementFail)
	t.Cleanup(releaseSettlement)
	promptWait := fixture.startPromptWaiter(t)

	releaseSnapshot := holdPromptSnapshotLock(t, fixture.execution)
	cancelResult := fixture.startCancel(t)
	stopResult := fixture.startForcedStop(t)
	releaseSnapshot()

	select {
	case <-settlementStarted:
	case <-time.After(cancelStopJoinTimeout):
		t.Fatal("cancel escalation did not settle the captured durable submission")
	}
	select {
	case err := <-stopResult:
		t.Fatalf("stop closed the client before settlement finished: %v", err)
	default:
	}
	select {
	case err := <-cancelResult:
		t.Fatalf("cancellation returned before settlement finished: %v", err)
	default:
	}

	releaseSettlement()
	cancelErr := awaitCancelStopResult(t, cancelResult, "cancellation")
	require.ErrorIs(t, cancelErr, ErrCancelEscalated)
	if settlementFail {
		require.ErrorContains(t, cancelErr, "submission settlement unavailable")
	}
	require.NoError(t, awaitCancelStopResult(t, stopResult, "stop"))
	awaitPromptCompletion(t, promptWait)

	if settlementFail {
		require.Equal(t, submissionID, fixture.execution.deliverySubmissionIDSnapshot(),
			"failed settlement must retain the submission for retry")
	} else {
		require.Empty(t, fixture.execution.deliverySubmissionIDSnapshot(),
			"successful settlement must clear the captured submission")
	}
	_, exists := fixture.manager.executionStore.Get(fixture.execution.ID)
	require.False(t, exists)
}

// stubSettlement replaces the mock submission route with a blocking handler so
// the test can observe settlement before stop teardown closes the client.
func (f *cancelStopHarness) stubSettlement(t *testing.T, fail bool) (<-chan struct{}, func()) {
	t.Helper()
	mux, ok := f.mock.server.Config.Handler.(*http.ServeMux)
	require.True(t, ok)
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	releaseOnce := func() { once.Do(func() { close(release) }) }
	mux.HandleFunc("/api/v1/agent/submissions/", func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-started:
		default:
			close(started)
		}
		<-release
		if fail {
			http.Error(w, "submission settlement unavailable", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	return started, releaseOnce
}

func setCancelTimeouts(t *testing.T, wait, escalation time.Duration) func() {
	t.Helper()
	previousWait := cancelWaitTimeout
	previousEscalation := cancelEscalationTimeout
	cancelWaitTimeout = wait
	cancelEscalationTimeout = escalation
	return func() {
		cancelWaitTimeout = previousWait
		cancelEscalationTimeout = previousEscalation
	}
}
