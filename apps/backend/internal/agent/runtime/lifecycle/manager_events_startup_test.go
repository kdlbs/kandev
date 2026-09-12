package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/pkg/api/v1"
)

type blockingStartupEventBus struct {
	*MockEventBusWithTracking
	entered chan struct{}
	release chan struct{}
}

func (b *blockingStartupEventBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	if strings.HasPrefix(subject, events.AgentStream+".") || subject == events.AgentctlError {
		select {
		case <-b.entered:
		default:
			close(b.entered)
		}
		<-b.release
	}
	return b.MockEventBusWithTracking.Publish(ctx, subject, event)
}

func TestHandleCompleteEventMarkState_DefersUninitializedStartupFailure(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	execution.Status = v1.AgentStatusStarting
	execution.beginStartupAttempt()
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	mgr.handleCompleteEventMarkState(execution, &agentctl.AgentEvent{
		Type:  streams.EventTypeComplete,
		Error: "Agent process exited with code 1",
		Data:  map[string]any{"is_error": true},
	}, true, nil)

	got, _ := mgr.GetExecution(execution.ID)
	if got.Status != v1.AgentStatusStarting {
		t.Fatalf("status = %q, want %q while startup owns failure classification", got.Status, v1.AgentStatusStarting)
	}
	if containsSubject(publishedSubjects(eventBus), events.AgentFailed) {
		t.Fatal("startup process failure published agent.failed before lifecycle recovery classified it")
	}
}

func TestMarkCompleted_DefersUninitializedStartupFailure(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	execution.Status = v1.AgentStatusStarting
	execution.beginStartupAttempt()
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	if err := mgr.MarkCompleted(execution.ID, 1, "Agent process exited with code 1"); err != nil {
		t.Fatalf("MarkCompleted: %v", err)
	}

	got, _ := mgr.GetExecution(execution.ID)
	if got.Status != v1.AgentStatusStarting {
		t.Fatalf("status = %q, want %q while startup owns failure classification", got.Status, v1.AgentStatusStarting)
	}
	if containsSubject(publishedSubjects(eventBus), events.AgentFailed) {
		t.Fatal("startup process failure published agent.failed before lifecycle recovery classified it")
	}
}

func TestHandleAgentEvent_DefersUninitializedStartupFailure(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	execution.Status = v1.AgentStatusStarting
	execution.beginStartupAttempt()
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:  "error",
		Error: "Agent process exited with code 1",
		Data:  map[string]any{"is_error": true},
	})

	select {
	case <-execution.promptDoneCh:
		t.Fatal("startup process failure signaled prompt completion before startup recovery classified it")
	default:
	}
	if containsSubject(publishedSubjects(eventBus), events.AgentFailed) {
		t.Fatal("startup process failure published agent.failed before lifecycle recovery classified it")
	}
}

func TestStartupGenerationPublishesOriginatingAttemptIDOnReusedExecution(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-reused", "task-1", "session-1")
	execution.ResumeAttemptID = "legacy-execution-label"
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	oldGeneration := execution.beginStartupAttemptWithID("attempt-old")
	newGeneration := execution.beginStartupAttemptWithID("attempt-new")

	// The old stream callback is delivered after the same execution has been
	// reused for a replacement startup. Generation fencing drops it before it
	// can publish or mutate lifecycle state.
	mgr.handleAgentEventWithStartupGeneration(execution, agentctl.AgentEvent{
		Type:             "plan",
		PromptGeneration: 1,
	}, oldGeneration)
	if got := len(eventBus.getStreamEvents()); got != 0 {
		t.Fatalf("stale startup callback published %d stream events", got)
	}

	mgr.handleAgentEventWithStartupGeneration(execution, agentctl.AgentEvent{
		Type:             "plan",
		PromptGeneration: 1,
	}, newGeneration)
	streamEvents := eventBus.getStreamEvents()
	if len(streamEvents) != 1 {
		t.Fatalf("replacement startup published %d stream events, want 1", len(streamEvents))
	}
	if streamEvents[0].AttemptID != "attempt-new" {
		t.Fatalf("replacement stream attempt ID = %q, want attempt-new", streamEvents[0].AttemptID)
	}
}

func TestBindResumeAttemptCarriesIdentityAcrossAdoptedStartupCallbacks(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-adopted", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	startupGeneration := execution.beginStartupAttempt()
	if err := mgr.BindResumeAttempt(context.Background(), execution.SessionID, "attempt-adopted"); err != nil {
		t.Fatalf("bind resume attempt: %v", err)
	}

	// The existing stream keeps its captured generation, but its events must be
	// attributed to the recovery attempt that adopted that generation.
	mgr.handleAgentEventWithStartupGeneration(execution, agentctl.AgentEvent{
		Type:             "plan",
		PromptGeneration: 1,
	}, startupGeneration)
	streamEvents := eventBus.getStreamEvents()
	if len(streamEvents) != 1 {
		t.Fatalf("adopted startup published %d stream events, want 1", len(streamEvents))
	}
	if streamEvents[0].AttemptID != "attempt-adopted" {
		t.Fatalf("adopted stream attempt ID = %q, want attempt-adopted", streamEvents[0].AttemptID)
	}

	mgr.eventPublisher.PublishAgentEvent(context.Background(), events.AgentBootReady, execution)
	var lifecycleEvent AgentEventPayload
	found := false
	eventBus.mu.Lock()
	for _, published := range eventBus.PublishedEvents {
		if published.Event.Type != events.AgentBootReady {
			continue
		}
		lifecycleEvent, found = published.Event.Data.(AgentEventPayload)
		break
	}
	eventBus.mu.Unlock()
	if !found {
		t.Fatal("adopted startup did not publish an agent.boot_ready payload")
	}
	if lifecycleEvent.AttemptID != "attempt-adopted" {
		t.Fatalf("adopted lifecycle attempt ID = %q, want attempt-adopted", lifecycleEvent.AttemptID)
	}
}

func TestMarkBootReadyRejectsStaleStartupGeneration(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-boot-stale", "task-1", "session-1")
	execution.Status = v1.AgentStatusStarting
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	oldGeneration := execution.beginStartupAttemptWithID("attempt-old")
	execution.beginStartupAttemptWithID("attempt-new")

	err := mgr.markBootReadyForStartup(context.Background(), execution.ID, oldGeneration)
	if err == nil {
		t.Fatal("stale boot-ready callback succeeded")
	}
	got, _ := mgr.GetExecution(execution.ID)
	if got.Status != v1.AgentStatusStarting {
		t.Fatalf("status after stale boot-ready = %q, want %q", got.Status, v1.AgentStatusStarting)
	}
	if containsSubject(publishedSubjects(eventBus), events.AgentBootReady) {
		t.Fatal("stale boot-ready callback published an event")
	}
}

func TestStartupCallbackLeasePreventsReplacementDuringMutation(t *testing.T) {
	mgr, tracked := createTestManagerWithTracking()
	blocking := &blockingStartupEventBus{
		MockEventBusWithTracking: tracked,
		entered:                  make(chan struct{}),
		release:                  make(chan struct{}),
	}
	mgr.eventPublisher = NewEventPublisher(blocking, newTestLogger())
	execution := createTestExecution("exec-lease", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	oldGeneration := execution.beginStartupAttemptWithID("attempt-old")

	eventDone := make(chan struct{})
	go func() {
		mgr.handleAgentEventWithStartupGeneration(execution, agentctl.AgentEvent{Type: "plan"}, oldGeneration)
		close(eventDone)
	}()
	select {
	case <-blocking.entered:
	case <-time.After(time.Second):
		t.Fatal("startup callback did not reach its publication barrier")
	}

	replacementDone := make(chan struct{})
	go func() {
		execution.beginStartupAttemptWithID("attempt-new")
		close(replacementDone)
	}()
	select {
	case <-replacementDone:
		t.Fatal("replacement advanced startup generation while the old callback still held its lease")
	case <-time.After(50 * time.Millisecond):
	}

	close(blocking.release)
	select {
	case <-eventDone:
	case <-time.After(time.Second):
		t.Fatal("startup callback did not finish after its barrier was released")
	}
	select {
	case <-replacementDone:
	case <-time.After(time.Second):
		t.Fatal("replacement did not advance after the old callback completed")
	}
}

func TestStartupDisconnectLeasePreventsReplacementDuringMutation(t *testing.T) {
	mgr, tracked := createTestManagerWithTracking()
	blocking := &blockingStartupEventBus{
		MockEventBusWithTracking: tracked,
		entered:                  make(chan struct{}),
		release:                  make(chan struct{}),
	}
	mgr.eventPublisher = NewEventPublisher(blocking, newTestLogger())
	execution := createTestExecution("exec-disconnect-lease", "task-1", "session-1")
	execution.setSessionInitialized(true)
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	oldGeneration := execution.beginStartupAttemptWithID("attempt-old")

	disconnectDone := make(chan struct{})
	go func() {
		mgr.handleStreamDisconnectWithStartupGeneration(
			execution, errors.New("stream lost"), 0, oldGeneration,
		)
		close(disconnectDone)
	}()
	select {
	case <-blocking.entered:
	case <-time.After(time.Second):
		t.Fatal("startup disconnect did not reach its publication barrier")
	}

	replacementDone := make(chan struct{})
	go func() {
		execution.beginStartupAttemptWithID("attempt-new")
		close(replacementDone)
	}()
	select {
	case <-replacementDone:
		t.Fatal("replacement advanced startup generation while disconnect mutation was leased")
	case <-time.After(50 * time.Millisecond):
	}

	close(blocking.release)
	select {
	case <-disconnectDone:
	case <-time.After(time.Second):
		t.Fatal("startup disconnect did not finish after its barrier was released")
	}
	select {
	case <-replacementDone:
	case <-time.After(time.Second):
		t.Fatal("replacement did not advance after disconnect mutation completed")
	}
}

func TestStartupGenerationCapturesAttemptIDForReusedExecutionMessageChunk(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-message-reused", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	oldGeneration := execution.beginStartupAttemptWithID("attempt-old")
	newGeneration := execution.beginStartupAttemptWithID("attempt-new")

	mgr.handleAgentEventWithStartupGeneration(execution, agentctl.AgentEvent{
		Type: streams.EventTypeMessageChunk,
		Text: "old\n",
	}, oldGeneration)
	if got := len(eventBus.getStreamEvents()); got != 0 {
		t.Fatalf("stale message callback published %d stream events", got)
	}

	mgr.handleAgentEventWithStartupGeneration(execution, agentctl.AgentEvent{
		Type: streams.EventTypeMessageChunk,
		Text: "replacement\n",
	}, newGeneration)
	streamEvents := eventBus.getStreamEvents()
	if len(streamEvents) != 1 {
		t.Fatalf("replacement message callback published %d stream events, want 1", len(streamEvents))
	}
	if streamEvents[0].AttemptID != "attempt-new" {
		t.Fatalf("message stream attempt ID = %q, want attempt-new", streamEvents[0].AttemptID)
	}
}
