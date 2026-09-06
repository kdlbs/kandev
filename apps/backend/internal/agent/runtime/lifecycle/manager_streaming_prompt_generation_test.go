package lifecycle

import (
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// TestStreamingEventsCarryActivePromptGeneration pins that the identity the
// recovery-evidence layer fences on (AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.20/
// .21/.23) is actually present on the events that layer observes. Without it,
// every ordinary message/thinking/tool event mismatches the nonzero generation
// an interactive turn is dispatched under, and the orchestrator's diagnostic
// correlation (observeProviderDiagnostic/observePromptAttempt) silently
// no-ops for the whole turn.
func TestStreamingEventsCarryActivePromptGeneration(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	generation, err := mgr.executionStore.BeginPrompt(execution.ID)
	if err != nil {
		t.Fatalf("begin prompt: %v", err)
	}
	if generation == 0 {
		t.Fatal("test setup: expected a nonzero generation")
	}

	mgr.handleAgentEvent(execution, agentctl.AgentEvent{Type: "message_chunk", Text: "hello\n"})
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{Type: "reasoning", ReasoningText: "thinking\n"})
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "tool_call", ToolCallID: "tool-1", ToolName: "bash",
	})
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type: "tool_update", ToolCallID: "tool-1", ToolStatus: toolStatusComplete,
	})

	seen := map[string]uint64{}
	for _, event := range eventBus.getStreamEvents() {
		if event.Data == nil {
			continue
		}
		seen[event.Data.Type] = event.Data.PromptGeneration
	}

	for _, eventType := range []string{"message_streaming", "thinking_streaming", "tool_call", "tool_update"} {
		got, ok := seen[eventType]
		if !ok {
			t.Fatalf("no %q event published; saw %v", eventType, seen)
		}
		if got != generation {
			t.Errorf("%q PromptGeneration = %d, want %d (the active generation)", eventType, got, generation)
		}
	}
}
