package lifecycle

import (
	"context"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/events"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// setShuttingDown flips the manager into graceful-shutdown state without running
// the teardown loop. StopAllAgents sets shuttingDown before it lists executions
// and returns early when the store is empty, so calling it before any execution
// is added yields IsShuttingDown() == true with no side effects.
func setShuttingDown(t *testing.T, mgr *Manager) {
	t.Helper()
	if err := mgr.StopAllAgents(context.Background()); err != nil {
		t.Fatalf("StopAllAgents: %v", err)
	}
	if !mgr.IsShuttingDown() {
		t.Fatal("StopAllAgents did not set IsShuttingDown() = true")
	}
}

// setShuttingDownFlag flips only the shutdown flag, leaving tracked executions
// in place. Unlike setShuttingDown (which runs StopAllAgents and therefore stops
// and removes every execution), this is for scenarios that need an execution
// still present in the store while IsShuttingDown() is true.
func setShuttingDownFlag(mgr *Manager) {
	mgr.shuttingDown.Store(true)
}

func publishedSubjects(bus *MockEventBusWithTracking) []string {
	bus.mu.Lock()
	defer bus.mu.Unlock()
	subjects := make([]string, 0, len(bus.PublishedEvents))
	for _, ev := range bus.PublishedEvents {
		subjects = append(subjects, ev.Subject)
	}
	return subjects
}

func containsSubject(subjects []string, want string) bool {
	return countSubject(subjects, want) > 0
}

func countSubject(subjects []string, want string) int {
	n := 0
	for _, s := range subjects {
		if s == want {
			n++
		}
	}
	return n
}

// TestHandleCompleteEventMarkState_ShutdownRaceMarksStopped verifies that an
// error completion arriving while the backend is shutting down marks the
// execution STOPPED and publishes AgentStopped instead of failing it.
func TestHandleCompleteEventMarkState_ShutdownRaceMarksStopped(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	setShuttingDown(t, mgr)

	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	errorEvent := &agentctl.AgentEvent{
		Type:  "complete",
		Error: "-32603 peer disconnected before response",
		Data:  map[string]any{"is_error": true},
	}
	mgr.handleCompleteEventMarkState(execution, errorEvent, true, nil)

	got, _ := mgr.GetExecution("exec-1")
	if got.Status != v1.AgentStatusStopped {
		t.Errorf("status = %v, want %v", got.Status, v1.AgentStatusStopped)
	}
	subjects := publishedSubjects(eventBus)
	if containsSubject(subjects, events.AgentFailed) {
		t.Errorf("AgentFailed was published during shutdown; subjects=%v", subjects)
	}
	if !containsSubject(subjects, events.AgentStopped) {
		t.Errorf("AgentStopped was not published during shutdown; subjects=%v", subjects)
	}
}

// TestHandleCompleteEventMarkState_ErrorFailsWhenNotShuttingDown verifies the
// unchanged failure path: without shutdown, an error completion marks the
// execution FAILED and publishes AgentFailed.
func TestHandleCompleteEventMarkState_ErrorFailsWhenNotShuttingDown(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()

	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	errorEvent := &agentctl.AgentEvent{
		Type:  "complete",
		Error: "boom",
		Data:  map[string]any{"is_error": true},
	}
	mgr.handleCompleteEventMarkState(execution, errorEvent, true, nil)

	got, _ := mgr.GetExecution("exec-1")
	if got.Status != v1.AgentStatusFailed {
		t.Errorf("status = %v, want %v", got.Status, v1.AgentStatusFailed)
	}
	subjects := publishedSubjects(eventBus)
	if !containsSubject(subjects, events.AgentFailed) {
		t.Errorf("AgentFailed was not published; subjects=%v", subjects)
	}
	if containsSubject(subjects, events.AgentStopped) {
		t.Errorf("AgentStopped should not be published outside shutdown; subjects=%v", subjects)
	}
}

// TestMarkCompleted_ShutdownRaceMarksStopped verifies the process-exit chokepoint
// honors shutdown: a failing MarkCompleted during shutdown becomes STOPPED.
func TestMarkCompleted_ShutdownRaceMarksStopped(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	setShuttingDown(t, mgr)

	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	if err := mgr.MarkCompleted("exec-1", 1, "boom"); err != nil {
		t.Fatalf("MarkCompleted: %v", err)
	}

	got, _ := mgr.GetExecution("exec-1")
	if got.Status != v1.AgentStatusStopped {
		t.Errorf("status = %v, want %v", got.Status, v1.AgentStatusStopped)
	}
	subjects := publishedSubjects(eventBus)
	if containsSubject(subjects, events.AgentFailed) {
		t.Errorf("AgentFailed was published during shutdown; subjects=%v", subjects)
	}
	if !containsSubject(subjects, events.AgentStopped) {
		t.Errorf("AgentStopped was not published during shutdown; subjects=%v", subjects)
	}
}

// TestMarkCompleted_SuccessUnaffectedByShutdown verifies a clean (exit 0)
// completion during shutdown still completes normally, not stopped.
func TestMarkCompleted_SuccessUnaffectedByShutdown(t *testing.T) {
	mgr, _ := createTestManagerWithTracking()
	setShuttingDown(t, mgr)

	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	if err := mgr.MarkCompleted("exec-1", 0, ""); err != nil {
		t.Fatalf("MarkCompleted: %v", err)
	}

	got, _ := mgr.GetExecution("exec-1")
	if got.Status != v1.AgentStatusCompleted {
		t.Errorf("status = %v, want %v", got.Status, v1.AgentStatusCompleted)
	}
}

// TestMarkStoppedDuringShutdown_PreservesTerminalFailure verifies that a genuine
// FAILED outcome recorded before shutdown is not downgraded to STOPPED, and that
// a duplicate shutdown completion does not re-publish AgentStopped. This guards
// the duplicate-terminal race where an ACP error marks FAILED first and a later
// process-exit completion arrives after shutdown begins.
func TestMarkStoppedDuringShutdown_PreservesTerminalFailure(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()

	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	// First: a genuine failure while not shutting down -> FAILED + AgentFailed.
	if err := mgr.MarkCompleted("exec-1", 1, "boom"); err != nil {
		t.Fatalf("MarkCompleted: %v", err)
	}

	// Then shutdown begins and a duplicate error completion arrives. Set only the
	// flag so exec-1 stays in the store (StopAllAgents would remove it).
	setShuttingDownFlag(mgr)
	if err := mgr.markStoppedDuringShutdown(execution, 1, "boom"); err != nil {
		t.Fatalf("markStoppedDuringShutdown: %v", err)
	}

	got, _ := mgr.GetExecution("exec-1")
	if got.Status != v1.AgentStatusFailed {
		t.Errorf("status = %v, want %v (terminal failure must not downgrade to stopped)",
			got.Status, v1.AgentStatusFailed)
	}
	subjects := publishedSubjects(eventBus)
	if n := countSubject(subjects, events.AgentStopped); n != 0 {
		t.Errorf("AgentStopped published %d times for already-terminal execution; want 0; subjects=%v", n, subjects)
	}
	if n := countSubject(subjects, events.AgentFailed); n != 1 {
		t.Errorf("AgentFailed published %d times; want exactly 1; subjects=%v", n, subjects)
	}
}

// TestMarkStoppedDuringShutdown_DuplicatePublishesOnce verifies that when both
// terminal chokepoints reach markStoppedDuringShutdown during shutdown for the
// same execution (ACP error event + process exit), only the first transition
// applies and AgentStopped is published exactly once.
func TestMarkStoppedDuringShutdown_DuplicatePublishesOnce(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	setShuttingDown(t, mgr)

	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	if err := mgr.markStoppedDuringShutdown(execution, 1, "boom"); err != nil {
		t.Fatalf("markStoppedDuringShutdown (first): %v", err)
	}
	if err := mgr.markStoppedDuringShutdown(execution, 1, "boom"); err != nil {
		t.Fatalf("markStoppedDuringShutdown (second): %v", err)
	}

	got, _ := mgr.GetExecution("exec-1")
	if got.Status != v1.AgentStatusStopped {
		t.Errorf("status = %v, want %v", got.Status, v1.AgentStatusStopped)
	}
	subjects := publishedSubjects(eventBus)
	if n := countSubject(subjects, events.AgentStopped); n != 1 {
		t.Errorf("AgentStopped published %d times; want exactly 1; subjects=%v", n, subjects)
	}
}

// TestHandleCompleteEvent_ShutdownRaceStillSignalsPromptDone verifies the
// invariant that a shutdown-race error completion, driven through the full
// handleCompleteEvent -> finishPromptCompletion path, still signals promptDoneCh
// so an in-flight SendPrompt waiter unblocks instead of hanging. The signal is
// emitted by handleCompleteEventSignal before the shutdown guard is reached; this
// guards against a future refactor reordering the signal below the guard.
func TestHandleCompleteEvent_ShutdownRaceStillSignalsPromptDone(t *testing.T) {
	mgr, _ := createTestManagerWithTracking()
	setShuttingDown(t, mgr)

	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	errorEvent := &agentctl.AgentEvent{
		Type:  "complete",
		Error: "-32603 peer disconnected before response",
		Data:  map[string]any{"is_error": true},
	}
	mgr.handleCompleteEvent(execution, errorEvent)

	select {
	case signal := <-execution.promptDoneCh:
		if !signal.IsError {
			t.Error("expected error signal for shutdown-race abort")
		}
		if signal.StopReason != "error" {
			t.Errorf("stop_reason = %q, want %q", signal.StopReason, "error")
		}
	default:
		t.Fatal("no signal on promptDoneCh; an in-flight prompt waiter would hang")
	}

	got, _ := mgr.GetExecution("exec-1")
	if got.Status != v1.AgentStatusStopped {
		t.Errorf("status = %v, want %v", got.Status, v1.AgentStatusStopped)
	}
}
