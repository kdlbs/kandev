package lifecycle

import (
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/pkg/api/v1"
)

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
