package lifecycle

import (
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/events"
)

func TestAgentFailedPayloadCarriesTerminalPromptEvidence(t *testing.T) {
	for _, tc := range []struct {
		name          string
		priorActivity bool
		priorEvents   []agentctl.AgentEvent
		useErrorEvent bool
		wantOutput    bool
		wantEffect    bool
	}{
		{
			name: "process exit with no activity",
		},
		{
			name:          "process exit after activity",
			priorActivity: true,
			wantOutput:    true,
			wantEffect:    true,
		},
		{
			name:          "error completion with no activity",
			useErrorEvent: true,
		},
		{
			name:          "error completion after activity",
			priorActivity: true,
			useErrorEvent: true,
			wantOutput:    true,
			wantEffect:    true,
		},
		{
			name:          "matching ACP provider diagnostic is not model output",
			useErrorEvent: true,
			priorEvents: []agentctl.AgentEvent{{
				Type:                        "message_chunk",
				Text:                        "API Error: Repeated 529 Overloaded errors. The API is at capacity.",
				ProviderDiagnosticCandidate: true,
				PromptGeneration:            7,
			}},
		},
		{
			name:          "provider diagnostic followed by assistant output remains unsafe",
			useErrorEvent: true,
			wantOutput:    true,
			wantEffect:    true,
			priorEvents: []agentctl.AgentEvent{
				{
					Type:                        "message_chunk",
					Text:                        "API Error: Repeated 529 Overloaded errors. The API is at capacity.",
					ProviderDiagnosticCandidate: true,
					PromptGeneration:            7,
				},
				{Type: "message_chunk", Text: "I resumed normal work.", PromptGeneration: 7},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mgr, eventBus := createTestManagerWithTracking()
			execution := createTestExecution("exec-1", "task-1", "session-1")
			execution.promptGeneration = 7
			if err := mgr.executionStore.Add(execution); err != nil {
				t.Fatalf("add execution: %v", err)
			}
			if tc.priorActivity {
				mgr.recordActivity(execution, agentctl.AgentEvent{Type: "message_chunk"})
			}
			for _, event := range tc.priorEvents {
				mgr.handleAgentEvent(execution, event)
			}

			if tc.useErrorEvent {
				mgr.handleAgentEvent(execution, agentctl.AgentEvent{
					Type:             "error",
					Error:            "agent failed",
					PromptGeneration: 7,
				})
			} else if err := mgr.MarkCompleted(execution.ID, 1, "agent failed"); err != nil {
				t.Fatalf("mark completed: %v", err)
			}

			var payload AgentEventPayload
			found := false
			eventBus.mu.Lock()
			for _, published := range eventBus.PublishedEvents {
				if published.Subject != events.AgentFailed {
					continue
				}
				payload, found = published.Event.Data.(AgentEventPayload)
				break
			}
			eventBus.mu.Unlock()
			if !found {
				t.Fatal("agent.failed payload was not published")
			}
			if !payload.EvidenceKnown {
				t.Fatal("agent.failed payload omitted known prompt evidence")
			}
			if payload.OutputObserved != tc.wantOutput || payload.EffectObserved != tc.wantEffect {
				t.Fatalf("prompt evidence = output:%v effect:%v, want output:%v effect:%v", payload.OutputObserved, payload.EffectObserved, tc.wantOutput, tc.wantEffect)
			}
		})
	}
}
