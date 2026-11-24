package lifecycle

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

const dispatchedStallThreshold = 5 * time.Second

// dispatchedStallFixture returns a manager and an accepted dispatch-only
// prompt generation tracked in its execution store.
func dispatchedStallFixture(t *testing.T) (*Manager, *MockEventBusWithTracking, *AgentExecution, uint64) {
	t.Helper()
	bus := &MockEventBusWithTracking{}
	sm := NewSessionManager(newSessionTestLogger(), make(chan struct{}))
	sm.eventPublisher = NewEventPublisher(bus, newSessionTestLogger())
	store := NewExecutionStore()
	sm.executionStore = store
	execution := &AgentExecution{
		ID: "dispatched-execution", SessionID: "dispatched-session",
		promptDoneCh: make(chan PromptCompletionSignal, 1),
	}
	if err := store.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	generation := beginExecutionPrompt(execution)
	sm.markPromptDispatched(execution, generation)
	result, err := sm.finishAcceptedPrompt(context.Background(), execution, true, nil, generation)
	if err != nil || result.StopReason != PromptStopReasonDispatched {
		t.Fatalf("dispatch-only prompt = %+v, %v; want dispatched", result, err)
	}
	if !execution.dispatchedPromptPending.Load() {
		t.Fatal("dispatch-only prompt must be pending until it settles")
	}
	return &Manager{executionStore: store, sessionManager: sm}, bus, execution, generation
}

func checkStallsAfter(m *Manager, wait time.Duration, reported map[string]uint64) {
	time.Sleep(wait)
	m.checkDispatchedPromptStalls(context.Background(), dispatchedStallThreshold, reported)
}

// @covers AC-AGENTS-AGENT-STALL-RECOVERY-001.1
// @covers AC-AGENTS-AGENT-STALL-RECOVERY-001.2
func TestDispatchOnlyPromptReportsOneStall(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m, bus, _, _ := dispatchedStallFixture(t)
		reported := map[string]uint64{}
		checkStallsAfter(m, 4*time.Second, reported)
		if got := countPublishedEvents(bus, "agent.stalled"); got != 0 {
			t.Fatalf("stall events before the threshold = %d, want 0", got)
		}
		checkStallsAfter(m, 2*time.Second, reported)
		if got := countPublishedEvents(bus, "agent.stalled"); got != 1 {
			t.Fatalf("stall events after the threshold = %d, want 1", got)
		}
		checkStallsAfter(m, time.Minute, reported)
		if got := countPublishedEvents(bus, "agent.stalled"); got != 1 {
			t.Fatalf("stall events for one generation = %d, want 1", got)
		}
	})
}

func TestDispatchOnlyStallCheckSkipsSettledPrompt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m, bus, execution, _ := dispatchedStallFixture(t)
		reported := map[string]uint64{"dispatched-execution": 0, "gone-execution": 3}
		execution.dispatchedPromptPending.Store(false)
		checkStallsAfter(m, 10*time.Second, reported)
		if got := countPublishedEvents(bus, "agent.stalled"); got != 0 {
			t.Fatalf("stall events after settlement = %d, want 0", got)
		}
		if len(reported) != 0 {
			t.Fatalf("reported generations = %v, want entries dropped for settled executions", reported)
		}
	})
}

func TestDispatchedPromptStallLoopEndsAtStop(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		m, _, _, _ := dispatchedStallFixture(t)
		m.stopCh = make(chan struct{})
		m.wg.Add(1)
		go m.dispatchedPromptStallLoop(context.Background())
		time.Sleep(3 * time.Minute)
		close(m.stopCh)
		m.wg.Wait()
	})
}
