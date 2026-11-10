package lifecycle

import (
	"testing"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// TestPromptSuggestionUsesItsPromptGenerationTurnID verifies a suggestion is
// bound to the turn of its originating prompt generation, not the latest turn.
func TestPromptSuggestionUsesItsPromptGenerationTurnID(t *testing.T) {
	execution := &AgentExecution{}
	execution.setPromptTurnID("turn-a")
	generationA := beginExecutionPrompt(execution)
	execution.setPromptTurnID("turn-b")
	beginExecutionPrompt(execution)

	manager := &Manager{}
	event := manager.handleAgentEventState(execution, agentctl.AgentEvent{
		Type: streams.EventTypePromptSuggestion, PromptGeneration: generationA, Text: "run the tests",
	})
	if event.TurnID != "turn-a" {
		t.Fatalf("suggestion turn ID = %q, want turn-a for prompt generation %d", event.TurnID, generationA)
	}
}

// TestPromptSuggestionWithoutGenerationGetsNoTurn verifies a suggestion with an
// unknown origin is never given the current turn.
func TestPromptSuggestionWithoutGenerationGetsNoTurn(t *testing.T) {
	execution := &AgentExecution{}
	execution.setPromptTurnID("turn-current")
	beginExecutionPrompt(execution)

	manager := &Manager{}
	event := manager.handleAgentEventState(execution, agentctl.AgentEvent{
		Type: streams.EventTypePromptSuggestion, Text: "run the tests",
	})
	if event.TurnID != "" {
		t.Fatalf("suggestion turn ID = %q, want empty without a prompt generation", event.TurnID)
	}
}
