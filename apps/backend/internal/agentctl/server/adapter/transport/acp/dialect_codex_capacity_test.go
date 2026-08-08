package acp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestCodexSystemErrorAndCapacityEvidence(t *testing.T) {
	for name, meta := range map[string]map[string]any{
		"top-level thread status": {
			"threadStatus": map[string]any{"type": codexSystemErrorType},
		},
		"codex thread status": {
			"codex": map[string]any{
				"threadStatus": map[string]any{"type": codexSystemErrorType},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if !codexSystemErrorMeta(meta) {
				t.Fatal("expected Codex system-error marker")
			}
		})
	}

	if codexSystemErrorMeta(map[string]any{
		"threadStatus": map[string]any{"type": "completed"},
	}) {
		t.Fatal("completed thread status must not be treated as a system error")
	}
	if !codexModelCapacityMessage("Selected   model is at capacity. Please try a different model.") {
		t.Fatal("expected normalized capacity message to match")
	}
	if codexModelCapacityMessage("The model completed successfully") {
		t.Fatal("ordinary model text must not match capacity evidence")
	}
}

func TestObserveCodexProviderEvidenceRequiresMatchingPrompt(t *testing.T) {
	a := newTestAdapter()
	a.agentID = codexAgentID
	t.Cleanup(func() { _ = a.Close() })

	_, turn := a.registerPromptTurn(context.Background(), 7)
	t.Cleanup(func() { a.clearPromptTurn(turn) })

	if a.observeCodexProviderEvidence(8, &AgentEvent{
		Type:        streams.EventTypeSessionInfo,
		SessionMeta: map[string]any{"threadStatus": map[string]any{"type": codexSystemErrorType}},
	}) {
		t.Fatal("stale prompt evidence must be ignored")
	}
	if a.observeCodexProviderEvidence(7, &AgentEvent{
		Type:        streams.EventTypeSessionInfo,
		SessionMeta: map[string]any{"threadStatus": map[string]any{"type": codexSystemErrorType}},
	}) {
		t.Fatal("system-error metadata alone must not settle a capacity failure")
	}
	if !a.observeCodexProviderEvidence(7, &AgentEvent{
		Type: streams.EventTypeMessageChunk,
		Text: "Selected model is at capacity. Please try a different model.",
	}) {
		t.Fatal("matching system-error and capacity evidence must suppress the explanatory chunk")
	}
	if !turn.codexCapacityFailure() {
		t.Fatal("expected correlated evidence to mark the prompt as a capacity failure")
	}
}
