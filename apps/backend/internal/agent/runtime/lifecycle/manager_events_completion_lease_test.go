package lifecycle

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	commonlogger "github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const completionLeaseChildEnv = "KANDEV_COMPLETION_STARTUP_LEASE_CHILD"

func TestCompletionStartupLease_PendingResumeWriter(t *testing.T) {
	if os.Getenv(completionLeaseChildEnv) == "1" {
		runCompletionStartupLeasePendingWriterChild(t)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCompletionStartupLease_PendingResumeWriter$", "-test.v")
	cmd.Env = append(os.Environ(), completionLeaseChildEnv+"=1")
	output, err := cmd.CombinedOutput()
	outputText := string(output)
	if !strings.Contains(outputText, "completion startup lease: resume writer pending") {
		t.Fatalf("child did not prove resume-writer contention (err=%v):\n%s", err, outputText)
	}
	if ctx.Err() != nil {
		t.Fatalf("completion child did not finish after proving resume-writer contention: %s", outputText)
	}
	if err != nil {
		t.Fatalf("completion child failed after proving resume-writer contention: %v\n%s", err, outputText)
	}
}

func runCompletionStartupLeasePendingWriterChild(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-completion-lease", "task-completion-lease", "session-completion-lease")
	execution.Status = v1.AgentStatusRunning
	execution.setSessionInitialized(true)
	startupGeneration := execution.beginStartupAttemptWithID("attempt-origin")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	callbackPaused, releaseCallback := pauseCompletionBeforeReadiness(t, mgr)
	callbackDone := make(chan struct{})
	go func() {
		defer close(callbackDone)
		mgr.handleAgentEventWithStartupGeneration(execution, agentctl.AgentEvent{
			Type:      "complete",
			SessionID: execution.SessionID,
			Data:      map[string]any{"stop_reason": "end_turn"},
		}, startupGeneration)
	}()
	waitForCompletionBarrier(t, callbackPaused)

	writerDone := make(chan error, 1)
	go func() {
		writerDone <- mgr.BindResumeAttempt(context.Background(), execution.SessionID, "attempt-resume")
	}()
	proveResumeWriterPending(t, execution, releaseCallback)
	if _, err := fmt.Fprintln(os.Stdout, "completion startup lease: resume writer pending"); err != nil {
		t.Fatalf("write contention marker: %v", err)
	}
	close(releaseCallback)

	<-callbackDone
	if err := <-writerDone; err != nil {
		t.Fatalf("BindResumeAttempt: %v", err)
	}

	var readyCount int
	for _, published := range eventBus.PublishedEvents {
		if published.Subject != events.AgentReady {
			continue
		}
		readyCount++
		payload, ok := published.Event.Data.(AgentEventPayload)
		if !ok {
			t.Fatalf("ready payload has type %T, want AgentEventPayload", published.Event.Data)
		}
		if payload.AttemptID != "attempt-origin" {
			t.Fatalf("ready attempt ID = %q, want originating attempt", payload.AttemptID)
		}
	}
	if readyCount != 1 {
		t.Fatalf("AgentReady publications = %d, want 1", readyCount)
	}
	if execution.Status != v1.AgentStatusReady {
		t.Fatalf("execution status = %q, want Ready", execution.Status)
	}
	if _, err := mgr.executionStore.BeginPrompt(execution.ID); err != nil {
		t.Fatalf("BeginPrompt after completion: %v", err)
	}
}

// Covers AC-TASKS-SESSION-TURN-SETTLEMENT-001.2 and 001.3.
func TestCompletionStartupLease_SyntheticStates(t *testing.T) {
	tests := []struct {
		name               string
		initialStatus      v1.AgentStatus
		completeDirectly   bool
		includeTurnContent bool
	}{
		{
			name:               "running turn with content through dispatcher",
			initialStatus:      v1.AgentStatusRunning,
			includeTurnContent: true,
		},
		{
			name:             "ready empty turn through direct completion",
			initialStatus:    v1.AgentStatusReady,
			completeDirectly: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			execution := createTestExecution("exec-completion-state", "task-completion-state", "session-completion-state")
			execution.Status = test.initialStatus
			execution.setSessionInitialized(true)
			startupGeneration := execution.beginStartupAttemptWithID("attempt-origin")
			mgr, eventBus := createCompletionSignalTestManager(execution)
			if err := mgr.executionStore.Add(execution); err != nil {
				t.Fatalf("add execution: %v", err)
			}

			if test.includeTurnContent {
				mgr.handleAgentEventWithStartupGeneration(execution, agentctl.AgentEvent{
					Type: "message_chunk",
					Text: "turn output\n",
				}, startupGeneration)
			}
			completeEvent := &agentctl.AgentEvent{
				Type:      streams.EventTypeComplete,
				SessionID: execution.SessionID,
				Data:      map[string]any{"stop_reason": "end_turn"},
			}
			if test.completeDirectly {
				if !mgr.handleCompleteEvent(execution, completeEvent) {
					t.Fatal("direct completion was rejected")
				}
			} else {
				mgr.handleAgentEventWithStartupGeneration(execution, *completeEvent, startupGeneration)
			}

			if execution.Status != v1.AgentStatusReady {
				t.Fatalf("execution status = %q, want Ready", execution.Status)
			}
			assertCompletionReadyPublication(t, eventBus)
			if test.includeTurnContent {
				assertCompletionTurnContent(t, eventBus)
			}
			if test.initialStatus == v1.AgentStatusReady {
				assertRunningBeforeReady(t, eventBus)
			}
			if _, err := mgr.executionStore.BeginPrompt(execution.ID); err != nil {
				t.Fatalf("BeginPrompt after completion: %v", err)
			}
		})
	}
}

// Covers AC-TASKS-SESSION-TURN-SETTLEMENT-001.4.
func TestCompletionStartupLease_StaleAndPending(t *testing.T) {
	t.Run("stale startup callback", func(t *testing.T) {
		execution := createTestExecution("exec-stale-startup", "task-stale-startup", "session-stale-startup")
		execution.setSessionInitialized(true)
		staleGeneration := execution.beginStartupAttemptWithID("attempt-old")
		execution.beginStartupAttemptWithID("attempt-current")
		mgr, eventBus := createCompletionSignalTestManager(execution)
		if err := mgr.executionStore.Add(execution); err != nil {
			t.Fatalf("add execution: %v", err)
		}

		mgr.handleAgentEventWithStartupGeneration(execution, agentctl.AgentEvent{
			Type: streams.EventTypeComplete,
		}, staleGeneration)
		assertCompletionNotSettled(t, execution, eventBus)
	})

	t.Run("stale prompt completion", func(t *testing.T) {
		execution := createTestExecution("exec-stale-prompt", "task-stale-prompt", "session-stale-prompt")
		execution.setSessionInitialized(true)
		execution.beginStartupAttemptWithID("attempt-current")
		mgr, eventBus := createCompletionSignalTestManager(execution)
		if err := mgr.executionStore.Add(execution); err != nil {
			t.Fatalf("add execution: %v", err)
		}
		staleGeneration, err := mgr.executionStore.BeginPrompt(execution.ID)
		if err != nil {
			t.Fatalf("begin stale prompt: %v", err)
		}
		if _, err := mgr.executionStore.BeginPrompt(execution.ID); err != nil {
			t.Fatalf("begin successor prompt: %v", err)
		}
		mgr.executionStore.MarkPromptDispatched(execution.ID, execution.promptGenerationSnapshot())
		execution.dispatchedPromptPending.Store(true)

		if mgr.handleCompleteEvent(execution, &agentctl.AgentEvent{
			Type:             streams.EventTypeComplete,
			PromptGeneration: staleGeneration,
		}) {
			t.Fatal("stale prompt completion was accepted")
		}
		assertCompletionNotSettled(t, execution, eventBus)
	})

	t.Run("unnumbered completion with pending dispatch", func(t *testing.T) {
		execution := createTestExecution("exec-pending-prompt", "task-pending-prompt", "session-pending-prompt")
		execution.setSessionInitialized(true)
		execution.beginStartupAttemptWithID("attempt-current")
		mgr, eventBus := createCompletionSignalTestManager(execution)
		if err := mgr.executionStore.Add(execution); err != nil {
			t.Fatalf("add execution: %v", err)
		}
		generation, err := mgr.executionStore.BeginPrompt(execution.ID)
		if err != nil {
			t.Fatalf("begin prompt: %v", err)
		}
		mgr.executionStore.MarkPromptDispatched(execution.ID, generation)
		execution.dispatchedPromptPending.Store(true)

		if mgr.handleCompleteEvent(execution, &agentctl.AgentEvent{Type: streams.EventTypeComplete}) {
			t.Fatal("unnumbered completion was accepted while a dispatched prompt was pending")
		}
		assertCompletionNotSettled(t, execution, eventBus)
		if !execution.dispatchedPromptPending.Load() {
			t.Fatal("pending-dispatch marker was cleared by rejected completion")
		}
	})
}

type completionSignalEventBus struct {
	*MockEventBusWithTracking
	execution      *AgentExecution
	readySawSignal bool
	readySignal    PromptCompletionSignal
}

func (b *completionSignalEventBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	if subject == events.AgentReady {
		select {
		case b.readySignal = <-b.execution.promptDoneCh:
			b.readySawSignal = true
		default:
		}
	}
	return b.MockEventBusWithTracking.Publish(ctx, subject, event)
}

func createCompletionSignalTestManager(execution *AgentExecution) (*Manager, *completionSignalEventBus) {
	mgr, _ := createTestManagerWithTracking()
	eventBus := &completionSignalEventBus{
		MockEventBusWithTracking: &MockEventBusWithTracking{},
		execution:                execution,
	}
	mgr.eventBus = eventBus
	mgr.eventPublisher = NewEventPublisher(eventBus, mgr.logger)
	return mgr, eventBus
}

func callCompletionStateWithStartupLease(
	t *testing.T,
	mgr *Manager,
	execution *AgentExecution,
	event *agentctl.AgentEvent,
	isError bool,
	failureEvidence *PromptAttemptEvidence,
) {
	t.Helper()
	generation := execution.startupAttemptSnapshot()
	accepted := execution.withStartupAttempt(generation, func(attemptID string) {
		event.AttemptID = attemptID
		mgr.handleCompleteEventMarkState(execution, event, isError, failureEvidence)
	})
	if !accepted {
		t.Fatalf("startup generation %d was unexpectedly stale", generation)
	}
}

func pauseCompletionBeforeReadiness(t *testing.T, mgr *Manager) (<-chan struct{}, chan struct{}) {
	t.Helper()
	callbackPaused := make(chan struct{})
	releaseCallback := make(chan struct{})
	core, _ := observer.New(zapcore.InfoLevel)
	hookedCore := zapcore.RegisterHooks(core, func(entry zapcore.Entry) error {
		if entry.Message == "complete event processed" {
			close(callbackPaused)
			<-releaseCallback
		}
		return nil
	})
	log, err := commonlogger.NewFromZap(zap.New(hookedCore))
	if err != nil {
		t.Fatalf("create hook logger: %v", err)
	}
	mgr.logger = log
	return callbackPaused, releaseCallback
}

func waitForCompletionBarrier(t *testing.T, callbackPaused <-chan struct{}) {
	t.Helper()
	select {
	case <-callbackPaused:
	case <-time.After(2 * time.Second):
		t.Fatal("completion callback did not reach its pre-readiness barrier")
	}
}

func proveResumeWriterPending(t *testing.T, execution *AgentExecution, releaseCallback chan struct{}) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if !execution.startupCallbackMu.TryRLock() {
			return
		}
		execution.startupCallbackMu.RUnlock()
		runtime.Gosched()
	}
	close(releaseCallback)
	t.Fatal("BindResumeAttempt never blocked new callback readers")
}

func assertCompletionReadyPublication(t *testing.T, eventBus *completionSignalEventBus) {
	t.Helper()
	if !eventBus.readySawSignal {
		t.Fatal("prompt completion was not signaled before AgentReady publication")
	}
	if eventBus.readySignal.PromptGeneration != 0 || eventBus.readySignal.IsError {
		t.Fatalf("completion signal at AgentReady = %+v, want successful synthetic completion", eventBus.readySignal)
	}
	var readyCount int
	for _, published := range eventBus.PublishedEvents {
		if published.Subject != events.AgentReady {
			continue
		}
		readyCount++
		payload, ok := published.Event.Data.(AgentEventPayload)
		if !ok {
			t.Fatalf("ready payload has type %T, want AgentEventPayload", published.Event.Data)
		}
		if payload.AttemptID != "attempt-origin" {
			t.Fatalf("ready attempt ID = %q, want originating attempt", payload.AttemptID)
		}
	}
	if readyCount != 1 {
		t.Fatalf("AgentReady publications = %d, want 1", readyCount)
	}
}

func assertCompletionTurnContent(t *testing.T, eventBus *completionSignalEventBus) {
	t.Helper()
	for _, event := range eventBus.getStreamEvents() {
		if event.Data != nil && event.Data.Type == "message_streaming" && event.Data.Text == "turn output\n" {
			return
		}
	}
	t.Fatal("completion did not preserve its preceding turn content")
}

func assertRunningBeforeReady(t *testing.T, eventBus *completionSignalEventBus) {
	t.Helper()
	runningIndex, readyIndex := -1, -1
	for index, published := range eventBus.PublishedEvents {
		switch published.Subject {
		case events.AgentRunning:
			if runningIndex == -1 {
				runningIndex = index
			}
		case events.AgentReady:
			if readyIndex == -1 {
				readyIndex = index
			}
		}
	}
	if runningIndex < 0 || readyIndex < 0 || runningIndex >= readyIndex {
		t.Fatalf("empty-turn event order has Running=%d and Ready=%d, want Running before Ready", runningIndex, readyIndex)
	}
}

func assertCompletionNotSettled(t *testing.T, execution *AgentExecution, eventBus *completionSignalEventBus) {
	t.Helper()
	if execution.Status != v1.AgentStatusRunning {
		t.Fatalf("execution status = %q, want Running after rejected completion", execution.Status)
	}
	if eventBus.readySawSignal {
		t.Fatal("rejected completion published AgentReady")
	}
	for _, published := range eventBus.PublishedEvents {
		if published.Subject == events.AgentReady {
			t.Fatal("rejected completion published AgentReady")
		}
	}
	select {
	case signal := <-execution.promptDoneCh:
		t.Fatalf("rejected completion signaled prompt completion: %+v", signal)
	default:
	}
}
