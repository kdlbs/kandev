package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

// fakeTurnOutcomeBackend embeds *MockExecutor so it satisfies ExecutorBackend
// via the existing mock, and additionally implements the unexported
// turnOutcomeApplier capability so tests can drive
// Manager.retrieveRecoveredTurnOutcome/applyRecoveredTurnOutcome without a
// real agentctl control server.
type fakeTurnOutcomeBackend struct {
	*MockExecutor
	outcome  *agentctl.TurnOutcome
	fetchErr error
	ackErr   error

	// eventBus, when set by the test, lets ackTurnOutcome record how many
	// events had already been published at ack time -- a monotonic counter
	// proving AC-EXECUTORS-SURVIVAL-004.6's apply-before-ack ordering rather
	// than merely that both happened.
	eventBus *MockEventBus

	fetchCalls           []string
	ackedCalls           []ackedTurnOutcome
	publishedCountsAtAck []int
}

func (f *fakeTurnOutcomeBackend) fetchTurnOutcomeWithRetry(_ context.Context, instanceID string) (*agentctl.TurnOutcome, error) {
	f.fetchCalls = append(f.fetchCalls, instanceID)
	return f.outcome, f.fetchErr
}

func (f *fakeTurnOutcomeBackend) ackTurnOutcome(_ context.Context, instanceID string, turnID int64) error {
	f.ackedCalls = append(f.ackedCalls, ackedTurnOutcome{instanceID: instanceID, turnID: turnID})
	if f.eventBus != nil {
		f.publishedCountsAtAck = append(f.publishedCountsAtAck, len(f.eventBus.PublishedEvents))
	}
	return f.ackErr
}

func newTurnOutcomeTestManager(t *testing.T, backend *fakeTurnOutcomeBackend) (*Manager, *MockEventBus) {
	t.Helper()
	log := newTestLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(backend)
	eventBus := &MockEventBus{}
	mgr := NewManager(newTestRegistry(), eventBus, registry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	return mgr, eventBus
}

func hasEventType(published []*bus.Event, eventType string) bool {
	for _, evt := range published {
		if evt.Type == eventType {
			return true
		}
	}
	return false
}

// TestRetrieveRecoveredTurnOutcomeCases pins AC-EXECUTORS-SURVIVAL-004.5's
// three-way result, plus the out-of-scope backend case (a runtime that never
// implements turn-outcome retention, e.g. Docker/Sprites/SSH/Kubernetes,
// which this capability does not cover at all).
func TestRetrieveRecoveredTurnOutcomeCases(t *testing.T) {
	t.Run("backend does not implement turnOutcomeApplier", func(t *testing.T) {
		mgr, _ := newTurnOutcomeTestManagerPlain(t)
		outcome, result := mgr.retrieveRecoveredTurnOutcome(context.Background(), &ExecutorInstance{RuntimeName: executor.NameStandalone})
		if result != recoveredTurnOutcomeNone || outcome != nil {
			t.Fatalf("result = %v, outcome = %v, want (recoveredTurnOutcomeNone, nil)", result, outcome)
		}
	})

	t.Run("no backend for runtime", func(t *testing.T) {
		mgr, _ := newTurnOutcomeTestManagerPlain(t)
		outcome, result := mgr.retrieveRecoveredTurnOutcome(context.Background(), &ExecutorInstance{RuntimeName: "unknown"})
		if result != recoveredTurnOutcomeNone || outcome != nil {
			t.Fatalf("result = %v, outcome = %v, want (recoveredTurnOutcomeNone, nil)", result, outcome)
		}
	})

	t.Run("nothing retained", func(t *testing.T) {
		backend := &fakeTurnOutcomeBackend{MockExecutor: &MockExecutor{name: executor.NameStandalone}}
		mgr, _ := newTurnOutcomeTestManager(t, backend)
		outcome, result := mgr.retrieveRecoveredTurnOutcome(context.Background(), &ExecutorInstance{
			RuntimeName: executor.NameStandalone, StandaloneInstanceID: "inst-1",
		})
		if result != recoveredTurnOutcomeNone || outcome != nil {
			t.Fatalf("result = %v, outcome = %v, want (recoveredTurnOutcomeNone, nil)", result, outcome)
		}
		if len(backend.fetchCalls) != 1 || backend.fetchCalls[0] != "inst-1" {
			t.Fatalf("fetchCalls = %v, want [inst-1]", backend.fetchCalls)
		}
	})

	t.Run("outcome retained", func(t *testing.T) {
		want := &agentctl.TurnOutcome{TurnID: 9, Event: streams.AgentEvent{Type: streams.EventTypeComplete}}
		backend := &fakeTurnOutcomeBackend{MockExecutor: &MockExecutor{name: executor.NameStandalone}, outcome: want}
		mgr, _ := newTurnOutcomeTestManager(t, backend)
		outcome, result := mgr.retrieveRecoveredTurnOutcome(context.Background(), &ExecutorInstance{
			RuntimeName: executor.NameStandalone, StandaloneInstanceID: "inst-1",
		})
		if result != recoveredTurnOutcomeApplied || outcome != want {
			t.Fatalf("result = %v, outcome = %v, want (recoveredTurnOutcomeApplied, %v)", result, outcome, want)
		}
	})

	t.Run("read failed", func(t *testing.T) {
		backend := &fakeTurnOutcomeBackend{
			MockExecutor: &MockExecutor{name: executor.NameStandalone},
			fetchErr:     errors.New("boom"),
		}
		mgr, _ := newTurnOutcomeTestManager(t, backend)
		outcome, result := mgr.retrieveRecoveredTurnOutcome(context.Background(), &ExecutorInstance{
			RuntimeName: executor.NameStandalone, StandaloneInstanceID: "inst-1",
		})
		if result != recoveredTurnOutcomeReadFailed || outcome != nil {
			t.Fatalf("result = %v, outcome = %v, want (recoveredTurnOutcomeReadFailed, nil)", result, outcome)
		}
	})
}

func newTurnOutcomeTestManagerPlain(t *testing.T) (*Manager, *MockEventBus) {
	t.Helper()
	log := newTestLogger()
	registry := NewExecutorRegistry(log)
	registry.Register(&MockExecutor{name: executor.NameStandalone})
	eventBus := &MockEventBus{}
	mgr := NewManager(newTestRegistry(), eventBus, registry, &MockCredentialsManager{}, &MockProfileResolver{}, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	return mgr, eventBus
}

// TestApplyRecoveredTurnOutcomeAppliesAndAcks pins
// AC-EXECUTORS-SURVIVAL-004.2/004.6: applying a retained outcome publishes
// the state that outcome produces (here, agent.ready from a successful
// completion), seeds the generation guard from the retained event's own
// PromptGeneration, and only then acknowledges it.
func TestApplyRecoveredTurnOutcomeAppliesAndAcks(t *testing.T) {
	outcome := &agentctl.TurnOutcome{
		TurnID: 9,
		Event: streams.AgentEvent{
			Type:             streams.EventTypeComplete,
			PromptGeneration: 5,
		},
	}
	backend := &fakeTurnOutcomeBackend{MockExecutor: &MockExecutor{name: executor.NameStandalone}, outcome: outcome}
	mgr, eventBus := newTurnOutcomeTestManager(t, backend)
	backend.eventBus = eventBus
	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	ri := &ExecutorInstance{RuntimeName: executor.NameStandalone, StandaloneInstanceID: "inst-1"}

	mgr.applyRecoveredTurnOutcome(context.Background(), execution, ri, outcome)

	if !hasEventType(eventBus.PublishedEvents, events.AgentReady) {
		t.Fatal("expected agent.ready to be published for a successful completion outcome")
	}
	if len(backend.ackedCalls) != 1 || backend.ackedCalls[0] != (ackedTurnOutcome{instanceID: "inst-1", turnID: 9}) {
		t.Fatalf("ackedCalls = %v, want [{inst-1 9}]", backend.ackedCalls)
	}
	if got := execution.promptGenerationSnapshot(); got != 5 {
		t.Fatalf("promptGeneration = %d, want 5 (seeded from the retained event)", got)
	}
	if !execution.isRecoveryDuplicateEvent(&streams.AgentEvent{ControlTurnID: 9}) {
		t.Fatal("expected the applied turn ID (9) to be recorded so a live redelivery is recognized")
	}
	// AC-EXECUTORS-SURVIVAL-004.6: the outcome must be applied (here, the
	// agent.ready publish above) before it is acknowledged -- not merely
	// that both eventually happen. The published-event count observed at
	// ack time is a monotonic witness of that ordering: it can only be
	// nonzero if the apply's publish already ran.
	if len(backend.publishedCountsAtAck) != 1 {
		t.Fatalf("publishedCountsAtAck = %v, want exactly 1 ack observed", backend.publishedCountsAtAck)
	}
	if backend.publishedCountsAtAck[0] < 1 {
		t.Fatalf("published event count at ack time = %d, want >= 1 (apply's publish must precede ack)",
			backend.publishedCountsAtAck[0])
	}
}

// TestApplyRecoveredTurnOutcomeDedupesLiveRedelivery pins
// AC-EXECUTORS-SURVIVAL-004.4: once an outcome is applied, a subsequent live
// event carrying the same ControlTurnID (agentctl's parked send, finally
// unblocked by the stream reconnecting) must not be re-applied -- no second
// agent.ready.
func TestApplyRecoveredTurnOutcomeDedupesLiveRedelivery(t *testing.T) {
	outcome := &agentctl.TurnOutcome{
		TurnID: 9,
		Event: streams.AgentEvent{
			Type:             streams.EventTypeComplete,
			PromptGeneration: 5,
		},
	}
	backend := &fakeTurnOutcomeBackend{MockExecutor: &MockExecutor{name: executor.NameStandalone}, outcome: outcome}
	mgr, eventBus := newTurnOutcomeTestManager(t, backend)
	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	ri := &ExecutorInstance{RuntimeName: executor.NameStandalone, StandaloneInstanceID: "inst-1"}
	mgr.applyRecoveredTurnOutcome(context.Background(), execution, ri, outcome)
	readyCountAfterApply := countEventType(eventBus.PublishedEvents, events.AgentReady)

	// The live redelivery: the same event, stamped with the control-server
	// turn identifier, as it would arrive once the stream reconnects.
	redelivered := outcome.Event
	redelivered.ControlTurnID = outcome.TurnID
	mgr.handleAgentEvent(execution, redelivered)

	if got := countEventType(eventBus.PublishedEvents, events.AgentReady); got != readyCountAfterApply {
		t.Fatalf("agent.ready published %d times after redelivery, want %d (redelivery must be a no-op)",
			got, readyCountAfterApply)
	}
}

func countEventType(published []*bus.Event, eventType string) int {
	count := 0
	for _, evt := range published {
		if evt.Type == eventType {
			count++
		}
	}
	return count
}

// TestPublishRecoveredExecutionRunningPublishesAgentRunning pins
// AC-EXECUTORS-SURVIVAL-004.5's "nothing retained" case.
func TestPublishRecoveredExecutionRunningPublishesAgentRunning(t *testing.T) {
	mgr, eventBus := newTurnOutcomeTestManagerPlain(t)
	execution := createTestExecution("exec-1", "task-1", "session-1")

	mgr.publishRecoveredExecutionRunning(context.Background(), execution)

	if !hasEventType(eventBus.PublishedEvents, events.AgentRunning) {
		t.Fatal("expected agent.running to be published")
	}
}

// newRecoveryTurnOutcomeExecutorInstance builds the ExecutorInstance a
// Manager.Start recovery pass would hand to the turn-outcome step, wired with
// a resolvable agent profile so it clears the earlier
// reDeriveRecoveredAgentIdentity gate.
func newRecoveryTurnOutcomeExecutorInstance() *ExecutorInstance {
	return &ExecutorInstance{
		InstanceID:           "exec-recovered",
		TaskID:               "task-1",
		SessionID:            "session-1",
		AgentProfileID:       recoveryTestAgentProfileID,
		RuntimeName:          executor.NameStandalone,
		StandaloneInstanceID: "std-inst-1",
	}
}

// TestManagerStartAppliesRetainedTurnOutcome pins the full Manager.Start
// wiring for AC-EXECUTORS-SURVIVAL-004.2: a retained outcome is retrieved,
// applied (publishing whatever state it produces), and acknowledged.
func TestManagerStartAppliesRetainedTurnOutcome(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	backend := &fakeTurnOutcomeBackend{
		MockExecutor: &MockExecutor{
			name:             executor.NameStandalone,
			recoverInstances: []*ExecutorInstance{newRecoveryTurnOutcomeExecutorInstance()},
		},
		outcome: &agentctl.TurnOutcome{
			TurnID: 3,
			Event:  streams.AgentEvent{Type: streams.EventTypeComplete, PromptGeneration: 11},
		},
	}
	registry.Register(backend)
	eventBus := &MockEventBus{}
	mgr := NewManager(newTestRegistry(), eventBus, registry, nil, nil, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	t.Cleanup(func() { _ = mgr.Stop() })
	registerRecoveryTestAgentProfile(t, mgr)

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	execution, ok := mgr.GetExecutionBySessionID("session-1")
	if !ok {
		t.Fatal("expected the recovered session to be tracked")
	}
	// The final published state is agent.ready, matching "whatever state
	// that outcome produces" (AC-EXECUTORS-SURVIVAL-004.2). An agent.running
	// may also be published ahead of it: this execution's firstActivityOnce
	// has never fired, so claimPromptCompletion treats this first-ever
	// completion exactly as it would for a live first turn, publishing
	// Running before Ready -- that is ordinary completion semantics, not the
	// recovery-specific "nothing retained" case this test is not exercising.
	if !hasEventType(eventBus.PublishedEvents, events.AgentReady) {
		t.Fatal("expected agent.ready to be published for the retained completion outcome")
	}
	if got := execution.promptGenerationSnapshot(); got != 11 {
		t.Fatalf("promptGeneration = %d, want 11", got)
	}
	if len(backend.ackedCalls) != 1 || backend.ackedCalls[0] != (ackedTurnOutcome{instanceID: "std-inst-1", turnID: 3}) {
		t.Fatalf("ackedCalls = %v, want [{std-inst-1 3}]", backend.ackedCalls)
	}
}

// TestManagerStartPublishesRunningWhenNoTurnOutcomeRetained pins
// AC-EXECUTORS-SURVIVAL-004.5's "nothing retained" case through the full
// Manager.Start wiring.
func TestManagerStartPublishesRunningWhenNoTurnOutcomeRetained(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	backend := &fakeTurnOutcomeBackend{
		MockExecutor: &MockExecutor{
			name:             executor.NameStandalone,
			recoverInstances: []*ExecutorInstance{newRecoveryTurnOutcomeExecutorInstance()},
		},
	}
	registry.Register(backend)
	eventBus := &MockEventBus{}
	mgr := NewManager(newTestRegistry(), eventBus, registry, nil, nil, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	t.Cleanup(func() { _ = mgr.Stop() })
	registerRecoveryTestAgentProfile(t, mgr)

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if _, ok := mgr.GetExecutionBySessionID("session-1"); !ok {
		t.Fatal("expected the recovered session to be tracked")
	}
	if !hasEventType(eventBus.PublishedEvents, events.AgentRunning) {
		t.Fatal("expected agent.running to be published when nothing was retained")
	}
	if len(backend.ackedCalls) != 0 {
		t.Fatalf("ackedCalls = %v, want none: nothing was retained, so nothing to acknowledge", backend.ackedCalls)
	}
}

// TestManagerStartRefusesAndStopsRecoveredExecutionWhenTurnOutcomeCannotBeRetrieved
// pins AC-EXECUTORS-SURVIVAL-004.5's read-failed case: the instance must not
// be re-tracked or published as running -- it takes the same not-re-tracked
// stop path as an unreconstructable agent identity.
func TestManagerStartRefusesAndStopsRecoveredExecutionWhenTurnOutcomeCannotBeRetrieved(t *testing.T) {
	log := newTestRegistryLogger()
	registry := NewExecutorRegistry(log)
	backend := &fakeTurnOutcomeBackend{
		MockExecutor: &MockExecutor{
			name:             executor.NameStandalone,
			recoverInstances: []*ExecutorInstance{newRecoveryTurnOutcomeExecutorInstance()},
		},
		fetchErr: errors.New("control server unreachable"),
	}
	registry.Register(backend)
	eventBus := &MockEventBus{}
	mgr := NewManager(newTestRegistry(), eventBus, registry, nil, nil, nil,
		ExecutorFallbackWarn, "", log)
	cleanupManagerStopCh(t, mgr)
	t.Cleanup(func() { _ = mgr.Stop() })
	registerRecoveryTestAgentProfile(t, mgr)

	if err := mgr.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if _, ok := mgr.GetExecutionBySessionID("session-1"); ok {
		t.Fatal("expected the session NOT to be tracked when its turn outcome could not be retrieved")
	}
	if hasEventType(eventBus.PublishedEvents, events.AgentRunning) {
		t.Fatal("expected agent.running NOT to be published: a read failure is not 'nothing retained'")
	}
	if len(backend.stopInstanceCalls) != 1 || backend.stopInstanceCalls[0].InstanceID != "exec-recovered" {
		t.Fatalf("stopInstanceCalls = %v, want exactly one call for exec-recovered", backend.stopInstanceCalls)
	}
}
