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

// TestEmitSessionModels_EmptyCurrentIDComposesReasoningEffort pins Codex's
// split config-option shape: configOptions reports model="gpt-5.5" and
// reasoning_effort="medium", while availableModels carries the actual
// selectable ID "gpt-5.5/medium".
func TestEmitSessionModels_EmptyCurrentIDComposesReasoningEffort(t *testing.T) {
	a := newTestAdapter()
	models := &acp.SessionModelState{
		CurrentModelId: "",
		AvailableModels: []acp.ModelInfo{
			{ModelId: "gpt-5.5/low", Name: "GPT-5.5 (low)"},
			{ModelId: "gpt-5.5/medium", Name: "GPT-5.5 (medium)"},
		},
	}
	meta := map[string]any{
		"configOptions": []any{
			map[string]any{
				"type":         "select",
				"id":           "model",
				"name":         "Model",
				"category":     "model",
				"currentValue": "gpt-5.5",
			},
			map[string]any{
				"type":         "select",
				"id":           "reasoning_effort",
				"name":         "Reasoning Effort",
				"category":     "thought_level",
				"currentValue": "medium",
			},
		},
	}

	a.emitSessionModels("sess-1", models, meta, nil)

	ev := findSessionModelsEvent(t, drainEvents(a))
	if ev.CurrentModelID != "gpt-5.5/medium" {
		t.Errorf("CurrentModelID = %q, want %q", ev.CurrentModelID, "gpt-5.5/medium")
	}
}

func TestEmitSessionModels_EmptyCurrentIDComposesReasoningEffortFromTypedOptions(t *testing.T) {
	a := newTestAdapter()
	modelCategory := acp.SessionConfigOptionCategoryModel
	thoughtCategory := acp.SessionConfigOptionCategoryThoughtLevel
	modelOptions := acp.SessionConfigSelectOptionsUngrouped{
		{Name: "GPT-5.5", Value: "gpt-5.5"},
	}
	effortOptions := acp.SessionConfigSelectOptionsUngrouped{
		{Name: "Low", Value: reasoningEffortLow},
		{Name: "Medium", Value: reasoningEffortMedium},
	}
	models := &acp.SessionModelState{
		CurrentModelId: "",
		AvailableModels: []acp.ModelInfo{
			{ModelId: "gpt-5.5/low", Name: "GPT-5.5 (low)"},
			{ModelId: "gpt-5.5/medium", Name: "GPT-5.5 (medium)"},
		},
	}
	configOptions := []acp.SessionConfigOption{
		{Select: &acp.SessionConfigOptionSelect{
			Type:         "select",
			Id:           "model",
			Name:         "Model",
			Category:     &modelCategory,
			CurrentValue: "gpt-5.5",
			Options:      acp.SessionConfigSelectOptions{Ungrouped: &modelOptions},
		}},
		{Select: &acp.SessionConfigOptionSelect{
			Type:         "select",
			Id:           "reasoning_effort",
			Name:         "Reasoning Effort",
			Category:     &thoughtCategory,
			CurrentValue: reasoningEffortMedium,
			Options:      acp.SessionConfigSelectOptions{Ungrouped: &effortOptions},
		}},
	}

	a.emitSessionModels("sess-1", models, nil, configOptions)

	ev := findSessionModelsEvent(t, drainEvents(a))
	if ev.CurrentModelID != "gpt-5.5/medium" {
		t.Errorf("CurrentModelID = %q, want %q", ev.CurrentModelID, "gpt-5.5/medium")
	}
}

func TestInitialSessionModelState_UsesConfigOptionsWithoutModels(t *testing.T) {
	modelCategory := acp.SessionConfigOptionCategoryModel
	modelOptions := acp.SessionConfigSelectOptionsUngrouped{
		{Name: "GPT-5.5", Value: "gpt-5.5"},
		{Name: "GPT-5.3-Codex-Spark", Value: "gpt-5.3-codex-spark"},
	}
	configOptions := []acp.SessionConfigOption{
		{Select: &acp.SessionConfigOptionSelect{
			Type:         "select",
			Id:           "model",
			Name:         "Model",
			Category:     &modelCategory,
			CurrentValue: "gpt-5.5",
			Options:      acp.SessionConfigSelectOptions{Ungrouped: &modelOptions},
		}},
	}

	models := initialSessionModelState(nil, nil, configOptions)
	if models == nil {
		t.Fatal("initialSessionModelState returned nil for configOptions-only response")
	}

	a := newTestAdapter()
	a.emitSessionModels("sess-1", models, nil, configOptions)

	ev := findSessionModelsEvent(t, drainEvents(a))
	if ev.CurrentModelID != "gpt-5.5" {
		t.Errorf("CurrentModelID = %q, want %q", ev.CurrentModelID, "gpt-5.5")
	}
	if len(ev.ConfigOptions) != 1 {
		t.Fatalf("ConfigOptions len = %d, want 1", len(ev.ConfigOptions))
	}
	if ev.ConfigOptions[0].ID != "model" {
		t.Errorf("ConfigOptions[0].ID = %q, want model", ev.ConfigOptions[0].ID)
	}
}

func TestInitialSessionModelState_UsesMetaConfigOptionsWithoutModels(t *testing.T) {
	meta := map[string]any{
		"configOptions": []any{
			map[string]any{
				"type":         "select",
				"id":           "model",
				"name":         "Model",
				"category":     "model",
				"currentValue": "gpt-5.5",
			},
		},
	}

	models := initialSessionModelState(nil, meta, nil)
	if models == nil {
		t.Fatal("initialSessionModelState returned nil for meta configOptions-only response")
	}

	a := newTestAdapter()
	a.emitSessionModels("sess-1", models, meta, nil)

	ev := findSessionModelsEvent(t, drainEvents(a))
	if ev.CurrentModelID != "gpt-5.5" {
		t.Errorf("CurrentModelID = %q, want %q", ev.CurrentModelID, "gpt-5.5")
	}
	if len(ev.ConfigOptions) != 1 {
		t.Fatalf("ConfigOptions len = %d, want 1", len(ev.ConfigOptions))
	}
	if ev.ConfigOptions[0].ID != "model" {
		t.Errorf("ConfigOptions[0].ID = %q, want model", ev.ConfigOptions[0].ID)
	}
}

func TestInitialSessionModelState_IgnoresNonModelConfigOptions(t *testing.T) {
	modeCategory := acp.SessionConfigOptionCategoryMode
	modeOptions := acp.SessionConfigSelectOptionsUngrouped{
		{Name: "Read Only", Value: "read-only"},
		{Name: "Default", Value: "auto"},
	}
	configOptions := []acp.SessionConfigOption{
		{Select: &acp.SessionConfigOptionSelect{
			Type:         "select",
			Id:           "mode",
			Name:         "Approval Preset",
			Category:     &modeCategory,
			CurrentValue: "read-only",
			Options:      acp.SessionConfigSelectOptions{Ungrouped: &modeOptions},
		}},
	}

	if models := initialSessionModelState(nil, nil, configOptions); models != nil {
		t.Fatalf("initialSessionModelState returned %+v for non-model configOptions", models)
	}
}

func TestResolveCurrentModelFromConfig_ComposesReasoningEffort(t *testing.T) {
	options := []streams.ConfigOption{
		{Type: "select", ID: "model", Category: "model", CurrentValue: "gpt-5.5"},
		{Type: "select", ID: "reasoning_effort", Category: "thought_level", CurrentValue: reasoningEffortMedium},
	}
	available := []acp.ModelInfo{
		{ModelId: "gpt-5.5/low", Name: "GPT-5.5 (low)"},
		{ModelId: "gpt-5.5/medium", Name: "GPT-5.5 (medium)"},
	}

	got := resolveCurrentModelFromConfig(options, available)
	if got != "gpt-5.5/medium" {
		t.Errorf("resolveCurrentModelFromConfig() = %q, want %q", got, "gpt-5.5/medium")
	}
}

func TestResolveCurrentModelFromConfig_PrefersReasoningModelWhenBaseAlsoAvailable(t *testing.T) {
	options := []streams.ConfigOption{
		{Type: "select", ID: "model", Category: "model", CurrentValue: "gpt-5.5"},
		{Type: "select", ID: "reasoning_effort", Category: "thought_level", CurrentValue: reasoningEffortMedium},
	}
	available := []acp.ModelInfo{
		{ModelId: "gpt-5.5", Name: "GPT-5.5"},
		{ModelId: "gpt-5.5/medium", Name: "GPT-5.5 (medium)"},
	}

	got := resolveCurrentModelFromConfig(options, available)
	if got != "gpt-5.5/medium" {
		t.Errorf("resolveCurrentModelFromConfig() = %q, want %q", got, "gpt-5.5/medium")
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

	cachedModels := []acp.ModelInfo{
		{ModelId: "claude-opus-4-7", Name: "Opus 4.7"},
		{ModelId: "build-analyzer", Name: "Build Analyzer"},
	}
	// Cover both rewrite paths: ID == "model" and Category == "model"
	// (some agents identify the model option by category, not ID).
	cachedConfig := []streams.ConfigOption{
		{Type: "select", ID: "other", Name: "Other", CurrentValue: "keep-me"},
		{Type: "select", ID: "model", Name: "Model", CurrentValue: "old-model"},
		{Type: "select", ID: "model-cat", Category: "model", Name: "ModelCat", CurrentValue: "old-model"},
	}

	a.emitSetModelEvent("sess-1", "claude-opus-4-7", cachedModels, cachedConfig)

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
	if len(ev.ConfigOptions) != 3 {
		t.Fatalf("ConfigOptions len = %d, want 3", len(ev.ConfigOptions))
	}

	// Both the ID-matched and Category-matched model options must have their
	// CurrentValue rewritten to the new model so consumers reading either
	// don't see a stale value. Non-model options are untouched.
	for _, opt := range ev.ConfigOptions {
		switch opt.ID {
		case "model", "model-cat":
			if opt.CurrentValue != "claude-opus-4-7" {
				t.Errorf("option %q CurrentValue = %q, want %q", opt.ID, opt.CurrentValue, "claude-opus-4-7")
			}
		case "other":
			if opt.CurrentValue != "keep-me" {
				t.Errorf("non-model option CurrentValue = %q, want %q (untouched)", opt.CurrentValue, "keep-me")
			}
		}
	}

	// The caller's cachedConfig must not be mutated — we copy before rewrite.
	if cachedConfig[1].CurrentValue != "old-model" || cachedConfig[2].CurrentValue != "old-model" {
		t.Errorf("caller cachedConfig was mutated: got %+v", cachedConfig)
	}
}

func TestEmitSetModelEvent_RewritesSplitReasoningOptions(t *testing.T) {
	a := newTestAdapter()

	reasoningOptions := []streams.ConfigOptionValue{
		{Name: "Medium", Value: reasoningEffortMedium},
		{Name: "High", Value: reasoningEffortHigh},
	}
	cachedModels := []acp.ModelInfo{
		{ModelId: "gpt-5.5/medium", Name: "GPT-5.5 (medium)"},
		{ModelId: "gpt-5.5/high", Name: "GPT-5.5 (high)"},
	}
	cachedConfig := []streams.ConfigOption{
		{Type: "select", ID: "model", Category: "model", Name: "Model", CurrentValue: "gpt-5.5"},
		{
			Type:         "select",
			ID:           "reasoning_effort",
			Category:     "thought_level",
			Name:         "Reasoning Effort",
			CurrentValue: reasoningEffortMedium,
			Options:      reasoningOptions,
		},
	}

	a.emitSetModelEvent("sess-1", "gpt-5.5/high", cachedModels, cachedConfig)

	ev := findSessionModelsEvent(t, drainEvents(a))
	if ev.CurrentModelID != "gpt-5.5/high" {
		t.Errorf("CurrentModelID = %q, want %q", ev.CurrentModelID, "gpt-5.5/high")
	}
	if len(ev.ConfigOptions) != 2 {
		t.Fatalf("ConfigOptions len = %d, want 2", len(ev.ConfigOptions))
	}
	for _, opt := range ev.ConfigOptions {
		switch opt.ID {
		case "model":
			if opt.CurrentValue != "gpt-5.5" {
				t.Errorf("model option CurrentValue = %q, want %q", opt.CurrentValue, "gpt-5.5")
			}
		case "reasoning_effort":
			if opt.CurrentValue != reasoningEffortHigh {
				t.Errorf("reasoning option CurrentValue = %q, want %q", opt.CurrentValue, reasoningEffortHigh)
			}
		default:
			t.Errorf("unexpected option ID %q in ConfigOptions", opt.ID)
		}
	}
}

func TestEmitSetModelEvent_RewritesSplitReasoningOptionsWithSlashInBaseModel(t *testing.T) {
	a := newTestAdapter()

	reasoningOptions := []streams.ConfigOptionValue{
		{Name: "Medium", Value: reasoningEffortMedium},
		{Name: "High", Value: reasoningEffortHigh},
	}
	cachedModels := []acp.ModelInfo{
		{ModelId: "vendor/gpt-5.5/medium", Name: "Vendor GPT-5.5 (medium)"},
		{ModelId: "vendor/gpt-5.5/high", Name: "Vendor GPT-5.5 (high)"},
	}
	cachedConfig := []streams.ConfigOption{
		{Type: "select", ID: "model", Category: "model", Name: "Model", CurrentValue: "vendor/gpt-5.5"},
		{
			Type:         "select",
			ID:           "reasoning_effort",
			Category:     "thought_level",
			Name:         "Reasoning Effort",
			CurrentValue: reasoningEffortMedium,
			Options:      reasoningOptions,
		},
	}

	a.emitSetModelEvent("sess-1", "vendor/gpt-5.5/high", cachedModels, cachedConfig)

	ev := findSessionModelsEvent(t, drainEvents(a))
	if ev.CurrentModelID != "vendor/gpt-5.5/high" {
		t.Errorf("CurrentModelID = %q, want %q", ev.CurrentModelID, "vendor/gpt-5.5/high")
	}
	if len(ev.ConfigOptions) != 2 {
		t.Fatalf("ConfigOptions len = %d, want 2", len(ev.ConfigOptions))
	}
	for _, opt := range ev.ConfigOptions {
		switch opt.ID {
		case "model":
			if opt.CurrentValue != "vendor/gpt-5.5" {
				t.Errorf("model option CurrentValue = %q, want %q", opt.CurrentValue, "vendor/gpt-5.5")
			}
		case "reasoning_effort":
			if opt.CurrentValue != reasoningEffortHigh {
				t.Errorf("reasoning option CurrentValue = %q, want %q", opt.CurrentValue, reasoningEffortHigh)
			}
		default:
			t.Errorf("unexpected option ID %q in ConfigOptions", opt.ID)
		}
	}
}

func TestEmitSetModelEvent_DoesNotSplitSlashModelWithoutReasoningSuffix(t *testing.T) {
	a := newTestAdapter()

	reasoningOptions := []streams.ConfigOptionValue{
		{Name: "Low", Value: reasoningEffortLow},
		{Name: "Medium", Value: reasoningEffortMedium},
		{Name: "High", Value: reasoningEffortHigh},
	}
	cachedModels := []acp.ModelInfo{
		{ModelId: "vendor/gpt-5.5", Name: "Vendor GPT-5.5"},
	}
	cachedConfig := []streams.ConfigOption{
		{Type: "select", ID: "model", Category: "model", Name: "Model", CurrentValue: "old-model"},
		{
			Type:         "select",
			ID:           "reasoning_effort",
			Category:     "thought_level",
			Name:         "Reasoning Effort",
			CurrentValue: reasoningEffortMedium,
			Options:      reasoningOptions,
		},
	}

	a.emitSetModelEvent("sess-1", "vendor/gpt-5.5", cachedModels, cachedConfig)

	ev := findSessionModelsEvent(t, drainEvents(a))
	if ev.CurrentModelID != "vendor/gpt-5.5" {
		t.Errorf("CurrentModelID = %q, want %q", ev.CurrentModelID, "vendor/gpt-5.5")
	}
	if len(ev.ConfigOptions) != 2 {
		t.Fatalf("ConfigOptions len = %d, want 2", len(ev.ConfigOptions))
	}
	for _, opt := range ev.ConfigOptions {
		switch opt.ID {
		case "model":
			if opt.CurrentValue != "vendor/gpt-5.5" {
				t.Errorf("model option CurrentValue = %q, want %q", opt.CurrentValue, "vendor/gpt-5.5")
			}
		case "reasoning_effort":
			if opt.CurrentValue != reasoningEffortMedium {
				t.Errorf("reasoning option CurrentValue = %q, want %q", opt.CurrentValue, reasoningEffortMedium)
			}
		default:
			t.Errorf("unexpected option ID %q in ConfigOptions", opt.ID)
		}
	}
}

// TestConfigOptionUpdate_RefreshesCachedConfig pins that an inbound
// ConfigOptionUpdate notification refreshes the adapter's availableConfigOptions
// cache, so a subsequent SetModel convergence event emits the latest options
// instead of the snapshot taken at session/new.
func TestConfigOptionUpdate_RefreshesCachedConfig(t *testing.T) {
	a := newTestAdapter()

	a.mu.Lock()
	a.availableConfigOptions = []streams.ConfigOption{
		{Type: "select", ID: "model", Name: "Model", CurrentValue: "stale"},
	}
	a.mu.Unlock()

	notif := acp.SessionNotification{
		SessionId: "sess-1",
		Update: acp.SessionUpdate{
			ConfigOptionUpdate: &acp.SessionConfigOptionUpdate{
				ConfigOptions: []acp.SessionConfigOption{
					{Select: &acp.SessionConfigOptionSelect{
						Type:         "select",
						Id:           "model",
						Name:         "Model",
						CurrentValue: "fresh",
					}},
				},
			},
		},
	}

	ev := a.convertNotification(notif)
	if ev == nil {
		t.Fatalf("expected a session_models event from ConfigOptionUpdate")
	}
	if ev.CurrentModelID != "fresh" {
		t.Errorf("event CurrentModelID = %q, want %q (resolved from refreshed configOption)", ev.CurrentModelID, "fresh")
	}

	a.mu.RLock()
	got := a.availableConfigOptions
	a.mu.RUnlock()

	if len(got) != 1 || got[0].CurrentValue != "fresh" {
		t.Errorf("availableConfigOptions = %+v, want one option with CurrentValue=fresh", got)
	}
}

func TestConfigOptionUpdate_ComposesReasoningEffortCurrentModel(t *testing.T) {
	a := newTestAdapter()
	thoughtCategory := acp.SessionConfigOptionCategoryThoughtLevel
	modelOptions := acp.SessionConfigSelectOptionsUngrouped{
		{Name: "GPT-5.5", Value: "gpt-5.5"},
	}
	effortOptions := acp.SessionConfigSelectOptionsUngrouped{
		{Name: "Medium", Value: reasoningEffortMedium},
		{Name: "High", Value: reasoningEffortHigh},
	}

	a.mu.Lock()
	a.availableModels = []acp.ModelInfo{
		{ModelId: "gpt-5.5/medium", Name: "GPT-5.5 (medium)"},
		{ModelId: "gpt-5.5/high", Name: "GPT-5.5 (high)"},
	}
	a.mu.Unlock()

	notif := acp.SessionNotification{
		SessionId: "sess-1",
		Update: acp.SessionUpdate{
			ConfigOptionUpdate: &acp.SessionConfigOptionUpdate{
				ConfigOptions: []acp.SessionConfigOption{
					{Select: &acp.SessionConfigOptionSelect{
						Type:         "select",
						Id:           "model",
						Name:         "Model",
						CurrentValue: "gpt-5.5",
						Options:      acp.SessionConfigSelectOptions{Ungrouped: &modelOptions},
					}},
					{Select: &acp.SessionConfigOptionSelect{
						Type:         "select",
						Id:           "reasoning_effort",
						Name:         "Reasoning Effort",
						Category:     &thoughtCategory,
						CurrentValue: reasoningEffortHigh,
						Options:      acp.SessionConfigSelectOptions{Ungrouped: &effortOptions},
					}},
				},
			},
		},
	}

	ev := a.convertNotification(notif)
	if ev == nil {
		t.Fatalf("expected a session_models event from ConfigOptionUpdate")
	}
	if ev.CurrentModelID != "gpt-5.5/high" {
		t.Errorf("event CurrentModelID = %q, want %q", ev.CurrentModelID, "gpt-5.5/high")
	}
}
