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
			if len(published) != 2 {
				t.Fatalf("stream events = %d, want original evidence and one projected chunk", len(published))
			}
			evidence := published[0].Data
			if evidence == nil || evidence.Type != event.Type || evidence.MessageID != "" || evidence.CanonicalProjection {
				t.Fatalf("original evidence = %+v, want unprojected source identity", evidence)
			}
			if evidence.PromptGeneration != generation || evidence.ProviderDiagnosticCandidate != event.ProviderDiagnosticCandidate {
				t.Fatalf("original evidence lost generation or diagnostic provenance: %+v", evidence)
			}
			wantText := event.Text
			if event.Type == "reasoning" {
				wantText = event.ReasoningText
			}
			if evidence.Text != wantText {
				t.Fatalf("original evidence text = %q, want %q", evidence.Text, wantText)
			}
			data := published[1].Data
			if data == nil || !data.CanonicalProjection || data.MessageID != event.CanonicalMessageID {
				t.Fatalf("projected chunk = %+v, want canonical message identity", data)
			}
			if data.PromptGeneration != generation {
				t.Errorf("prompt generation = %d, want %d", data.PromptGeneration, generation)
			}
			if data.ProviderDiagnosticCandidate {
				t.Error("visible canonical projection retained diagnostic evidence marker")
			}
		})
	}
}
