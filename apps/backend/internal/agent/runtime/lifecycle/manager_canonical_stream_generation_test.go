package lifecycle

import (
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

func TestCanonicalStreamingEventsRetainPromptEvidence(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		event agentctl.AgentEvent
	}{
		{name: "message", event: agentctl.AgentEvent{Type: "message_chunk", Text: "partial output"}},
		{name: "reasoning", event: agentctl.AgentEvent{Type: "reasoning", ReasoningText: "partial reasoning"}},
		{name: "provider diagnostic", event: agentctl.AgentEvent{Type: "message_chunk", Text: "provider diagnostic", ProviderDiagnosticCandidate: true}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			mgr, eventBus := createTestManagerWithTracking()
			execution := createTestExecution("exec-1", "task-1", "session-1")
			if err := mgr.executionStore.Add(execution); err != nil {
				t.Fatalf("add execution: %v", err)
			}
			generation, err := mgr.executionStore.BeginPrompt(execution.ID)
			if err != nil {
				t.Fatalf("begin prompt: %v", err)
			}
			event := testCase.event
			event.CanonicalProjection = true
			event.CanonicalMessageID = "projected-message"
			mgr.handleAgentEvent(execution, event)
			mgr.flushStreamCoalescer(execution)
			published := eventBus.getStreamEvents()
			if len(published) != 1 {
				t.Fatalf("stream events = %d, want one projected chunk", len(published))
			}
			data := published[0].Data
			if data == nil || !data.CanonicalProjection || data.MessageID != event.CanonicalMessageID {
				t.Fatalf("projected chunk = %+v, want canonical message identity", data)
			}
			if data.PromptGeneration != generation {
				t.Errorf("prompt generation = %d, want %d", data.PromptGeneration, generation)
			}
			if data.ProviderDiagnosticCandidate != event.ProviderDiagnosticCandidate {
				t.Errorf("diagnostic candidate = %t, want %t", data.ProviderDiagnosticCandidate, event.ProviderDiagnosticCandidate)
			}
		})
	}
}
