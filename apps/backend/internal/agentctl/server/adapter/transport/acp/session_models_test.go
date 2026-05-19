package acp

import (
	"testing"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// findSessionModelsEvent returns the first session_models event in events,
// or fails the test if none is present.
func findSessionModelsEvent(t *testing.T, events []AgentEvent) AgentEvent {
	t.Helper()
	for _, ev := range events {
		if ev.Type == streams.EventTypeSessionModels {
			return ev
		}
	}
	t.Fatalf("no %s event emitted; got %d events", streams.EventTypeSessionModels, len(events))
	return AgentEvent{}
}

// auggieLikeModels mimics Auggie's response: empty CurrentModelId with an
// alphabetically-sorted list whose [0] is a pseudo-agent ("Build Analyzer").
func auggieLikeModels() *acp.SessionModelState {
	return &acp.SessionModelState{
		CurrentModelId: "",
		AvailableModels: []acp.ModelInfo{
			{ModelId: "build-fix-gpt5-2-responses-high-200k-v1-c4-p2-agent", Name: "Build Analyzer"},
			{ModelId: "claude-opus-4-7", Name: "Opus 4.7"},
		},
	}
}

// TestEmitSessionModels_EmptyCurrentIDNoFallback pins the regression: when the
// agent returns currentModelId="" with no model-shaped configOption, the
// adapter must NOT invent a "current" model from AvailableModels[0]. Auggie
// returns alphabetically-sorted models whose [0] is a pseudo-agent ("Build
// Analyzer"), so the previous fallback caused the UI to show the wrong model.
func TestEmitSessionModels_EmptyCurrentIDNoFallback(t *testing.T) {
	a := newTestAdapter()
	a.emitSessionModels("sess-1", auggieLikeModels(), nil, nil)

	ev := findSessionModelsEvent(t, drainEvents(a))
	if ev.CurrentModelID != "" {
		t.Errorf("CurrentModelID = %q, want empty (let frontend fall through to profile)", ev.CurrentModelID)
	}
	if len(ev.SessionModels) != 2 {
		t.Errorf("SessionModels len = %d, want 2", len(ev.SessionModels))
	}
}

// TestEmitSessionModels_EmptyCurrentIDFromConfigOption pins the legitimate
// fallback that we keep: some agents expose the current model via a
// configOption (id="model") rather than CurrentModelId.
func TestEmitSessionModels_EmptyCurrentIDFromConfigOption(t *testing.T) {
	a := newTestAdapter()
	meta := map[string]any{
		"configOptions": []any{
			map[string]any{
				"type":         "select",
				"id":           "model",
				"name":         "Model",
				"currentValue": "claude-opus-4-7",
			},
		},
	}
	a.emitSessionModels("sess-1", auggieLikeModels(), meta, nil)

	ev := findSessionModelsEvent(t, drainEvents(a))
	if ev.CurrentModelID != "claude-opus-4-7" {
		t.Errorf("CurrentModelID = %q, want %q", ev.CurrentModelID, "claude-opus-4-7")
	}
}

// TestEmitSessionModels_NonEmptyCurrentIDPreserved checks the happy path:
// when the agent populates CurrentModelId, we propagate it verbatim.
func TestEmitSessionModels_NonEmptyCurrentIDPreserved(t *testing.T) {
	a := newTestAdapter()
	models := &acp.SessionModelState{
		CurrentModelId: "claude-opus-4-7",
		AvailableModels: []acp.ModelInfo{
			{ModelId: "claude-opus-4-7", Name: "Opus 4.7"},
		},
	}
	a.emitSessionModels("sess-1", models, nil, nil)

	ev := findSessionModelsEvent(t, drainEvents(a))
	if ev.CurrentModelID != "claude-opus-4-7" {
		t.Errorf("CurrentModelID = %q, want %q", ev.CurrentModelID, "claude-opus-4-7")
	}
}

// TestEmitSetModelEvent_EmitsSessionModelsWithCachedState pins that after a
// successful SetModel call the adapter emits a session_models convergence
// event carrying the requested model and cached available models / config
// options. This is what corrects the frontend after the lifecycle manager
// applies the profile model at session init.
func TestEmitSetModelEvent_EmitsSessionModelsWithCachedState(t *testing.T) {
	a := newTestAdapter()

	a.mu.Lock()
	a.sessionID = "sess-1"
	a.availableModels = []acp.ModelInfo{
		{ModelId: "claude-opus-4-7", Name: "Opus 4.7"},
		{ModelId: "build-analyzer", Name: "Build Analyzer"},
	}
	a.availableConfigOptions = []streams.ConfigOption{
		{Type: "select", ID: "model", Name: "Model", CurrentValue: "claude-opus-4-7"},
	}
	a.mu.Unlock()

	a.emitSetModelEvent("claude-opus-4-7")

	ev := findSessionModelsEvent(t, drainEvents(a))
	if ev.SessionID != "sess-1" {
		t.Errorf("SessionID = %q, want %q", ev.SessionID, "sess-1")
	}
	if ev.CurrentModelID != "claude-opus-4-7" {
		t.Errorf("CurrentModelID = %q, want %q", ev.CurrentModelID, "claude-opus-4-7")
	}
	if len(ev.SessionModels) != 2 {
		t.Errorf("SessionModels len = %d, want 2", len(ev.SessionModels))
	}
	if len(ev.ConfigOptions) != 1 || ev.ConfigOptions[0].ID != "model" {
		t.Errorf("ConfigOptions = %+v, want one option with ID=model", ev.ConfigOptions)
	}
}
