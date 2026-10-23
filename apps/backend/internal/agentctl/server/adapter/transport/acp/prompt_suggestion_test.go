package acp

import (
	"encoding/json"
	"reflect"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func newPromptSuggestionAdapter(advertisesClaudeCode, requested bool) *Adapter {
	a := newTestAdapterForAgent("custom-acp")
	a.cfg.PromptSuggestions = requested
	a.claudeCodeAgent = advertisesClaudeCode
	a.sessionID = "session-1"
	return a
}

// TestAgentAdvertisesClaudeCode verifies native suggestions are negotiated from
// the initialize advertisement, not from the agent's identity.
func TestAgentAdvertisesClaudeCode(t *testing.T) {
	cases := []struct {
		name string
		meta map[string]any
		want bool
	}{
		{name: "claude code namespace", meta: map[string]any{"claudeCode": map[string]any{"promptQueueing": true}}, want: true},
		{name: "empty claude code namespace", meta: map[string]any{"claudeCode": map[string]any{}}, want: true},
		{name: "no meta"},
		{name: "other vendor", meta: map[string]any{"opencode": map[string]any{}}},
		{name: "malformed namespace", meta: map[string]any{"claudeCode": true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := agentAdvertisesClaudeCode(acpsdk.AgentCapabilities{Meta: tc.meta})
			if got != tc.want {
				t.Fatalf("agentAdvertisesClaudeCode = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestPromptSuggestionSessionMeta verifies only a Claude Code agent that the
// instance asked for suggestions receives the suggestion _meta.
func TestPromptSuggestionSessionMeta(t *testing.T) {
	want := map[string]any{
		"claudeCode": map[string]any{
			"options":            map[string]any{"promptSuggestions": true},
			"emitRawSDKMessages": []any{map[string]any{"type": "prompt_suggestion"}},
		},
	}
	if got := newPromptSuggestionAdapter(true, true).promptSuggestionSessionMeta(); !reflect.DeepEqual(got, want) {
		t.Fatalf("meta = %#v, want %#v", got, want)
	}
	if got := newPromptSuggestionAdapter(true, false).promptSuggestionSessionMeta(); got != nil {
		t.Fatalf("meta without request = %#v, want nil", got)
	}
	if got := newPromptSuggestionAdapter(false, true).promptSuggestionSessionMeta(); got != nil {
		t.Fatalf("meta for a non-Claude-Code agent = %#v, want nil", got)
	}
}

func claudeSDKMessage(sessionID, messageType, suggestion string) json.RawMessage {
	raw, _ := json.Marshal(map[string]any{
		"sessionId": sessionID,
		"message":   map[string]any{"type": messageType, "suggestion": suggestion},
	})
	return raw
}

// TestHandleClaudeSDKMessageEmitsPromptSuggestion verifies a forwarded
// prompt_suggestion becomes a prompt_suggestion stream event.
func TestHandleClaudeSDKMessageEmitsPromptSuggestion(t *testing.T) {
	a := newPromptSuggestionAdapter(true, true)
	a.handleClaudeSDKMessage(claudeSDKMessage("session-1", "prompt_suggestion", "  sim, corre os testes  "))

	events := drainEvents(a)
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].Type != streams.EventTypePromptSuggestion || events[0].Text != "sim, corre os testes" {
		t.Fatalf("event = %+v, want prompt_suggestion with trimmed text", events[0])
	}
	if events[0].SessionID != "session-1" {
		t.Fatalf("session = %q, want session-1", events[0].SessionID)
	}
}

// TestHandleClaudeSDKMessageIgnoresUnusableMessages verifies other message
// types, other sessions, empty text, and disabled instances emit nothing.
func TestHandleClaudeSDKMessageIgnoresUnusableMessages(t *testing.T) {
	cases := []struct {
		name    string
		enabled bool
		params  json.RawMessage
	}{
		{name: "other type", enabled: true, params: claudeSDKMessage("session-1", "result", "x")},
		{name: "other session", enabled: true, params: claudeSDKMessage("session-2", "prompt_suggestion", "x")},
		{name: "empty text", enabled: true, params: claudeSDKMessage("session-1", "prompt_suggestion", "   ")},
		{name: "not requested", enabled: false, params: claudeSDKMessage("session-1", "prompt_suggestion", "x")},
		{name: "malformed", enabled: true, params: json.RawMessage(`{`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newPromptSuggestionAdapter(true, tc.enabled)
			a.handleClaudeSDKMessage(tc.params)
			if events := drainEvents(a); len(events) != 0 {
				t.Fatalf("events = %+v, want none", events)
			}
		})
	}
}

// TestHandleClaudeSDKMessageDropsSuggestionDuringNewerPrompt verifies a
// suggestion that arrives while a newer prompt is in flight is discarded.
func TestHandleClaudeSDKMessageDropsSuggestionDuringNewerPrompt(t *testing.T) {
	a := newPromptSuggestionAdapter(true, true)
	a.promptTurnMu.Lock()
	a.promptTurn = &promptTurnState{promptGeneration: 2}
	a.promptTurnMu.Unlock()

	a.handleClaudeSDKMessage(claudeSDKMessage("session-1", "prompt_suggestion", "sim"))
	if events := drainEvents(a); len(events) != 0 {
		t.Fatalf("events = %+v, want none while a prompt is in flight", events)
	}
}
