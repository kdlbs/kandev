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
	for _, s := range subjects {
		if s == want {
			return true
		}
	}
	return false
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
	mgr.handleCompleteEventMarkState(execution, errorEvent, true)

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
	mgr.handleCompleteEventMarkState(execution, errorEvent, true)

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
