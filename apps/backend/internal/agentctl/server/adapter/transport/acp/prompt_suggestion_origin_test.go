package acp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// startPromptForSuggestion makes turn the in-flight prompt the way sendPrompt
// does once it owns the gate and sends session/prompt.
func startPromptForSuggestion(a *Adapter, generation uint64) *promptTurnState {
	_, turn := newPromptTurnState(context.Background(), generation, false)
	a.promptTurnMu.Lock()
	a.promptTurn = turn
	a.promptTurnMu.Unlock()
	a.markPromptSentForSuggestion(turn)
	return turn
}

func suggest(a *Adapter, text string) []AgentEvent {
	a.handleClaudeSDKMessage(claudeSDKMessage("session-1", "prompt_suggestion", text))
	return drainEvents(a)
}

func requireSuggestionEvent(t *testing.T, events []AgentEvent, text string, generation uint64) {
	t.Helper()
	if len(events) != 1 {
		t.Fatalf("events = %+v, want one prompt_suggestion", events)
	}
	got := events[0]
	if got.Type != streams.EventTypePromptSuggestion || got.Text != text || got.PromptGeneration != generation {
		t.Fatalf("event = {type:%q text:%q generation:%d}, want {prompt_suggestion %q %d}",
			got.Type, got.Text, got.PromptGeneration, text, generation)
	}
}

// TestPromptSuggestionCarriesOriginatingGeneration verifies a suggestion that
// arrives after its turn finishes is bound to that turn's prompt generation.
func TestPromptSuggestionCarriesOriginatingGeneration(t *testing.T) {
	a := newPromptSuggestionAdapter(true, true)
	a.finishPromptTurn(startPromptForSuggestion(a, 7))

	requireSuggestionEvent(t, suggest(a, "run the tests"), "run the tests", 7)
}

// TestPromptSuggestionDuringNewerPromptIsDropped verifies that turn A's
// suggestion arriving while turn B is in flight is dropped, and that B keeps a
// trustworthy origin for its own suggestion.
func TestPromptSuggestionDuringNewerPromptIsDropped(t *testing.T) {
	a := newPromptSuggestionAdapter(true, true)
	a.finishPromptTurn(startPromptForSuggestion(a, 1))
	requireSuggestionEvent(t, suggest(a, "a"), "a", 1)

	b := startPromptForSuggestion(a, 2)
	if events := suggest(a, "late a"); len(events) != 0 {
		t.Fatalf("events during turn B = %+v, want none", events)
	}
	a.finishPromptTurn(b)
	requireSuggestionEvent(t, suggest(a, "b"), "b", 2)
}

// TestDelayedSuggestionAfterNewerTurnCompletesIsDropped verifies the delayed
// A-after-B case: turn B started while A's suggestion could still arrive, so a
// suggestion after B completes has no trustworthy origin and is discarded.
func TestDelayedSuggestionAfterNewerTurnCompletesIsDropped(t *testing.T) {
	a := newPromptSuggestionAdapter(true, true)
	a.finishPromptTurn(startPromptForSuggestion(a, 1))
	a.finishPromptTurn(startPromptForSuggestion(a, 2))

	if events := suggest(a, "delayed a"); len(events) != 0 {
		t.Fatalf("events = %+v, want the ambiguous suggestion dropped", events)
	}
	// Once that window is consumed, the next turn starts clean again.
	a.finishPromptTurn(startPromptForSuggestion(a, 3))
	requireSuggestionEvent(t, suggest(a, "c"), "c", 3)
}

// TestPromptSuggestionWindowAcceptsOneSuggestion verifies a finished turn
// yields at most one suggestion; a second one is not attributed to it.
func TestPromptSuggestionWindowAcceptsOneSuggestion(t *testing.T) {
	a := newPromptSuggestionAdapter(true, true)
	a.finishPromptTurn(startPromptForSuggestion(a, 4))
	requireSuggestionEvent(t, suggest(a, "first"), "first", 4)
	if events := suggest(a, "second"); len(events) != 0 {
		t.Fatalf("events = %+v, want the second suggestion dropped", events)
	}
}

// TestPromptSuggestionWithoutKnownOriginIsDropped verifies suggestions with no
// finished turn, or from a turn without a lifecycle generation, are dropped.
func TestPromptSuggestionWithoutKnownOriginIsDropped(t *testing.T) {
	a := newPromptSuggestionAdapter(true, true)
	if events := suggest(a, "no turn yet"); len(events) != 0 {
		t.Fatalf("events = %+v, want none before any turn", events)
	}
	a.finishPromptTurn(startPromptForSuggestion(a, 0))
	if events := suggest(a, "unknown generation"); len(events) != 0 {
		t.Fatalf("events = %+v, want none for generation 0", events)
	}
}

// TestHandedOffPromptMakesSuccessorOriginAmbiguous verifies that when a
// predecessor finishes while its successor is already in flight, the
// successor's suggestion is discarded.
func TestHandedOffPromptMakesSuccessorOriginAmbiguous(t *testing.T) {
	a := newPromptSuggestionAdapter(true, true)
	first := startPromptForSuggestion(a, 1)
	second := startPromptForSuggestion(a, 2)
	a.finishPromptTurn(first)
	a.finishPromptTurn(second)

	if events := suggest(a, "ambiguous"); len(events) != 0 {
		t.Fatalf("events = %+v, want the overlapped successor's suggestion dropped", events)
	}
}
