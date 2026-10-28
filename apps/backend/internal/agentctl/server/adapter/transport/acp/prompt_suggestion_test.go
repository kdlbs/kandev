package acp

import (
	"encoding/json"
	"reflect"
	"testing"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func newPromptSuggestionAdapter(supported, requested bool) *Adapter {
	a := newTestAdapterForAgent("custom-acp")
	a.cfg.PromptSuggestions = requested
	a.promptSuggestionSupport = supported
	a.sessionID = "session-1"
	return a
}

// TestNativePromptSuggestionSupport verifies support comes from feature-specific
// initialize evidence, never from the agent identity, a vendor namespace, or
// another capability.
func TestNativePromptSuggestionSupport(t *testing.T) {
	legacyMeta := map[string]any{"claudeCode": map[string]any{"promptQueueing": true}}
	featureMeta := func(value any) map[string]any {
		return map[string]any{"claudeCode": map[string]any{"promptSuggestions": value}}
	}
	cases := []struct {
		name    string
		agent   string
		version string
		meta    map[string]any
		want    bool
		wantWhy claudePromptSuggestionEvidence
	}{
		{
			name: "explicit true with an arbitrary custom bridge", agent: "research-bridge", version: "9.9.9",
			meta: featureMeta(true), want: true, wantWhy: claudeSuggestionEvidenceExplicit,
		},
		{
			name: "explicit true with the legacy identity", agent: verifiedClaudeSuggestionBridgeName,
			version: verifiedClaudeSuggestionBridgeVersion, meta: featureMeta(true),
			want: true, wantWhy: claudeSuggestionEvidenceExplicit,
		},
		{
			name: "explicit false denies the verified legacy tuple", agent: verifiedClaudeSuggestionBridgeName,
			version: verifiedClaudeSuggestionBridgeVersion,
			meta: map[string]any{"claudeCode": map[string]any{
				"promptSuggestions": false, "promptQueueing": true,
			}},
			wantWhy: claudeSuggestionEvidenceUnsupported,
		},
		{
			name: "string declaration is malformed", agent: verifiedClaudeSuggestionBridgeName,
			version: verifiedClaudeSuggestionBridgeVersion,
			meta: map[string]any{"claudeCode": map[string]any{
				"promptSuggestions": "true", "promptQueueing": true,
			}},
			wantWhy: claudeSuggestionEvidenceUnsupported,
		},
		{
			name: "numeric declaration is malformed", meta: featureMeta(1),
			wantWhy: claudeSuggestionEvidenceUnsupported,
		},
		{
			name: "null declaration is malformed", meta: featureMeta(nil),
			wantWhy: claudeSuggestionEvidenceUnsupported,
		},
		{
			name: "object declaration is malformed", meta: featureMeta(map[string]any{}),
			wantWhy: claudeSuggestionEvidenceUnsupported,
		},
		{
			name: "verified legacy tuple", agent: verifiedClaudeSuggestionBridgeName,
			version: verifiedClaudeSuggestionBridgeVersion, meta: legacyMeta,
			want: true, wantWhy: claudeSuggestionEvidenceVerifiedLegacy,
		},
		{
			name: "queueing alone with an arbitrary identity", agent: "custom-acp", version: "1.0.0", meta: legacyMeta,
			wantWhy: claudeSuggestionEvidenceUnsupported,
		},
		{
			// A Claude-looking reported name is not the verified package identity.
			name: "claude-looking identity outside the verified tuple", agent: "claude-code", version: "1.0.0", meta: legacyMeta,
			wantWhy: claudeSuggestionEvidenceUnsupported,
		},
		{
			name: "official name with an unverified version", agent: verifiedClaudeSuggestionBridgeName,
			version: "0.99.0", meta: legacyMeta,
			wantWhy: claudeSuggestionEvidenceUnsupported,
		},
		{
			name: "official bridge name without metadata", agent: verifiedClaudeSuggestionBridgeName,
			version: verifiedClaudeSuggestionBridgeVersion,
			wantWhy: claudeSuggestionEvidenceUnsupported,
		},
		{
			name: "verified identity with an empty namespace", agent: verifiedClaudeSuggestionBridgeName,
			version: verifiedClaudeSuggestionBridgeVersion, meta: map[string]any{"claudeCode": map[string]any{}},
			wantWhy: claudeSuggestionEvidenceUnsupported,
		},
		{
			name: "verified identity with promptQueueing false", agent: verifiedClaudeSuggestionBridgeName,
			version: verifiedClaudeSuggestionBridgeVersion,
			meta:    map[string]any{"claudeCode": map[string]any{"promptQueueing": false}},
			wantWhy: claudeSuggestionEvidenceUnsupported,
		},
		{
			name: "verified identity with a non-boolean promptQueueing", agent: verifiedClaudeSuggestionBridgeName,
			version: verifiedClaudeSuggestionBridgeVersion,
			meta:    map[string]any{"claudeCode": map[string]any{"promptQueueing": "true"}},
			wantWhy: claudeSuggestionEvidenceUnsupported,
		},
		{
			name: "unrelated vendor metadata", meta: map[string]any{"opencode": map[string]any{}},
			wantWhy: claudeSuggestionEvidenceUnsupported,
		},
		{
			name: "malformed namespace", meta: map[string]any{"claudeCode": true},
			wantWhy: claudeSuggestionEvidenceUnsupported,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, why := claudePromptSuggestionSupport(tc.agent, tc.version, acpsdk.AgentCapabilities{Meta: tc.meta})
			if got != tc.want || why != tc.wantWhy {
				t.Fatalf("claudePromptSuggestionSupport = (%v, %q), want (%v, %q)", got, why, tc.want, tc.wantWhy)
			}
		})
	}
}

// TestPromptSuggestionSessionMeta verifies the configuration contract for every
// support/request pair: supported sessions get an explicit option, and only a
// requested session filters raw suggestion messages.
func TestPromptSuggestionSessionMeta(t *testing.T) {
	wantEnabled := map[string]any{
		"claudeCode": map[string]any{
			"options":            map[string]any{"promptSuggestions": true},
			"emitRawSDKMessages": []any{map[string]any{"type": "prompt_suggestion"}},
		},
	}
	wantDisabled := map[string]any{
		"claudeCode": map[string]any{
			"options": map[string]any{"promptSuggestions": false},
		},
	}
	if got := newPromptSuggestionAdapter(true, true).promptSuggestionSessionMeta(); !reflect.DeepEqual(got, wantEnabled) {
		t.Fatalf("meta = %#v, want %#v", got, wantEnabled)
	}
	if got := newPromptSuggestionAdapter(true, false).promptSuggestionSessionMeta(); !reflect.DeepEqual(got, wantDisabled) {
		t.Fatalf("meta without request = %#v, want explicit disable %#v", got, wantDisabled)
	}
	if got := newPromptSuggestionAdapter(false, true).promptSuggestionSessionMeta(); got != nil {
		t.Fatalf("meta for an unsupported agent = %#v, want nil", got)
	}
	if got := newPromptSuggestionAdapter(false, false).promptSuggestionSessionMeta(); got != nil {
		t.Fatalf("meta for an unsupported, unrequested agent = %#v, want nil", got)
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
	a.finishPromptTurn(startPromptForSuggestion(a, 1))
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
// types, other sessions, empty text, unsupported or unrequested instances, and
// malformed payloads emit nothing.
func TestHandleClaudeSDKMessageIgnoresUnusableMessages(t *testing.T) {
	cases := []struct {
		name      string
		supported bool
		requested bool
		params    json.RawMessage
	}{
		{name: "other type", supported: true, requested: true, params: claudeSDKMessage("session-1", "result", "x")},
		{name: "other session", supported: true, requested: true, params: claudeSDKMessage("session-2", "prompt_suggestion", "x")},
		{name: "empty text", supported: true, requested: true, params: claudeSDKMessage("session-1", "prompt_suggestion", "   ")},
		{name: "not requested", supported: true, params: claudeSDKMessage("session-1", "prompt_suggestion", "x")},
		{name: "not supported", requested: true, params: claudeSDKMessage("session-1", "prompt_suggestion", "x")},
		{name: "malformed", supported: true, requested: true, params: json.RawMessage(`{`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := newPromptSuggestionAdapter(tc.supported, tc.requested)
			a.finishPromptTurn(startPromptForSuggestion(a, 1))
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
