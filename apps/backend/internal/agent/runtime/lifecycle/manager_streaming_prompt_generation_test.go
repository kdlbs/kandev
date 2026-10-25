package lifecycle

import (
	"errors"
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
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

// findFlushedMessage returns the PromptGeneration of the first
// "message_streaming" event carrying the given text, or (0, false).
func findFlushedMessage(eventBus *MockEventBusWithTracking, text string) (uint64, bool) {
	for _, event := range eventBus.getStreamEvents() {
		if event.Data == nil || event.Data.Type != "message_streaming" {
			continue
		}
		if event.Data.Text == text {
			return event.Data.PromptGeneration, true
		}
	}
	return 0, false
}

// bufferUnflushedChunk delivers a newline-free message_chunk, which stays in
// execution.messageBuffer (no flush happens on this call) under the active
// generation.
func bufferUnflushedChunk(mgr *Manager, execution *AgentExecution, text string) {
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{Type: "message_chunk", Text: text})
}

// TestFlushMessageBufferOnCompleteUsesClaimedGeneration pins
// handleCompleteEvent's flush call site (manager_events.go's handleCompleteEvent,
// which holds execution.promptLifecycleMu across the flush once a completion is
// claimed). Buffered content that never saw a trailing newline is only ever
// published through this forced flush, so it is the production-dominant path
// for a diagnostic-only or otherwise newline-free turn. Reverting the call to
// pass generation 0 instead of the claimed event.PromptGeneration leaves this
// test as the only failure in the suite.
func TestFlushMessageBufferOnCompleteUsesClaimedGeneration(t *testing.T) {
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

	bufferUnflushedChunk(mgr, execution, "no trailing newline")
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{Type: "complete", PromptGeneration: generation})

	got, ok := findFlushedMessage(eventBus, "no trailing newline")
	if !ok {
		t.Fatalf("no flushed message_streaming event for the buffered text; saw %v", eventBus.getStreamEvents())
	}
	if got != generation {
		t.Errorf("flushed PromptGeneration = %d, want %d (the claimed generation)", got, generation)
	}
}

// TestFlushMessageBufferOnStreamDisconnectUsesClaimedGeneration pins
// handleStreamDisconnect's nonzero-generation branch, which also holds
// execution.promptLifecycleMu across the flush.
func TestFlushMessageBufferOnStreamDisconnectUsesClaimedGeneration(t *testing.T) {
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

	bufferUnflushedChunk(mgr, execution, "disconnected mid-turn")
	mgr.handleStreamDisconnect(execution, errors.New("stream closed"), generation)

	got, ok := findFlushedMessage(eventBus, "disconnected mid-turn")
	if !ok {
		t.Fatalf("no flushed message_streaming event for the buffered text; saw %v", eventBus.getStreamEvents())
	}
	if got != generation {
		t.Errorf("flushed PromptGeneration = %d, want %d (the claimed generation)", got, generation)
	}
}

// TestFlushMessageBufferOnPromptHandoffUsesClaimedGeneration pins
// handlePromptHandoffEvent's flush call site, which also holds
// execution.promptLifecycleMu across the flush once it owns the generation.
func TestFlushMessageBufferOnPromptHandoffUsesClaimedGeneration(t *testing.T) {
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

	bufferUnflushedChunk(mgr, execution, "handed off mid-turn")
	mgr.handleAgentEvent(execution, agentctl.AgentEvent{
		Type:             streams.EventTypeForegroundIdle,
		PromptGeneration: generation,
		Data:             map[string]any{streams.AgentEventDataPromptHandoff: true},
	})

	got, ok := findFlushedMessage(eventBus, "handed off mid-turn")
	if !ok {
		t.Fatalf("no flushed message_streaming event for the buffered text; saw %v", eventBus.getStreamEvents())
	}
	if got != generation {
		t.Errorf("flushed PromptGeneration = %d, want %d (the claimed generation)", got, generation)
	}
}

// TestMessageChunkFlushUsesGenerationActiveAtFlushTime pins that
// handleMessageChunkEvent's newline-triggered flush stamps published content
// with the generation active when that content is actually flushed, not a
// value resolved once when the content was first buffered. A prompt
// generation can advance while content sits in messageBuffer; the flush must
// not carry the stale generation forward.
func TestMessageChunkFlushUsesGenerationActiveAtFlushTime(t *testing.T) {
	mgr, eventBus := createTestManagerWithTracking()
	execution := createTestExecution("exec-1", "task-1", "session-1")
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}
	if _, err := mgr.executionStore.BeginPrompt(execution.ID); err != nil {
		t.Fatalf("begin prompt (generation 1): %v", err)
	}

	bufferUnflushedChunk(mgr, execution, "buffered before the bump")

	generation2, err := mgr.executionStore.BeginPrompt(execution.ID)
	if err != nil {
		t.Fatalf("begin prompt (generation 2): %v", err)
	}

	mgr.handleAgentEvent(execution, agentctl.AgentEvent{Type: "message_chunk", Text: "and after it\n"})

	got, ok := findFlushedMessage(eventBus, "buffered before the bumpand after it\n")
	if !ok {
		t.Fatalf("no flushed message_streaming event for the merged buffer; saw %v", eventBus.getStreamEvents())
	}
	if got != generation2 {
		t.Errorf("flushed PromptGeneration = %d, want %d (the generation active at flush time)", got, generation2)
	}
}
