package acp

import (
	"testing"

	"github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/sessionmodel"
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
func auggieLikeModels() *sessionModelState {
	return &sessionModelState{
		CurrentModelId: "",
		AvailableModels: []modelInfo{
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

func TestValidateAvailableModelRejectsStaleProfileValue(t *testing.T) {
	available := []modelInfo{{ModelId: "mock-fast"}, {ModelId: "mock-smart"}}
	if err := validateAvailableModel(available, "mock-smart"); err != nil {
		t.Fatalf("advertised model rejected: %v", err)
	}
	if err := validateAvailableModel(available, "mock-default"); err == nil {
		t.Fatal("stale profile model was accepted")
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

	models := initialSessionModelState(nil, configOptions, nil)
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

	models := initialSessionModelState(meta, nil, nil)
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

	if models := initialSessionModelState(nil, configOptions, nil); models != nil {
		t.Fatalf("initialSessionModelState returned %+v for non-model configOptions", models)
	}
}

// TestInitialSessionModelState_NonModelConfigOptionsFallThroughToLegacy
// pins that an agent emitting typed configOptions[category="mode"] alongside
// the legacy top-level `models` field still surfaces its models — the typed
// non-model option must not block the LegacyModels tier.
func TestInitialSessionModelState_NonModelConfigOptionsFallThroughToLegacy(t *testing.T) {
	modeCategory := acp.SessionConfigOptionCategoryMode
	modeOptions := acp.SessionConfigSelectOptionsUngrouped{
		{Name: "Read Only", Value: "read-only"},
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
	legacy := &acp.LegacyModels{
		CurrentModelId: "claude-opus-4-7",
		AvailableModels: []acp.LegacyModelInfo{
			{ModelId: "claude-opus-4-7", Name: "Opus 4.7"},
		},
	}

	models := initialSessionModelState(nil, configOptions, legacy)
	if models == nil {
		t.Fatal("initialSessionModelState returned nil; expected legacy fallback")
	}
	if models.CurrentModelId != "claude-opus-4-7" {
		t.Errorf("CurrentModelId = %q, want claude-opus-4-7", models.CurrentModelId)
	}
	if len(models.AvailableModels) != 1 {
		t.Fatalf("AvailableModels = %d, want 1", len(models.AvailableModels))
	}
}

// TestInitialSessionModelState_NonModelConfigOptionsAllowMetaFallback pins
// that the _meta tier-3 fallback still fires when typed configOptions has
// only non-model entries and no LegacyModels are present.
func TestInitialSessionModelState_NonModelConfigOptionsAllowMetaFallback(t *testing.T) {
	modeCategory := acp.SessionConfigOptionCategoryMode
	modeOptions := acp.SessionConfigSelectOptionsUngrouped{
		{Name: "Read Only", Value: "read-only"},
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

	models := initialSessionModelState(meta, configOptions, nil)
	if models == nil {
		t.Fatal("initialSessionModelState returned nil; expected _meta fallback stub")
	}
	if len(models.AvailableModels) != 0 {
		t.Errorf("AvailableModels = %d, want 0 (stub state)", len(models.AvailableModels))
	}
}

func TestInitialSessionModelState_FallsBackToLegacyModels(t *testing.T) {
	// auggie 0.29.x emits the pre-v0.13.5 top-level `models` field and does
	// NOT surface a SessionConfigOption(category="model"). The kdlbs SDK fork
	// restores read-only parsing as acp.LegacyModels; the third-tier fallback
	// in initialSessionModelState must turn that into a fully populated state.
	desc := "Anthropic Claude Opus 4.7"
	legacy := &acp.LegacyModels{
		CurrentModelId: "claude-opus-4-7",
		AvailableModels: []acp.LegacyModelInfo{
			{ModelId: "claude-opus-4-7", Name: "Opus 4.7", Description: &desc},
			{ModelId: "claude-sonnet-4-5", Name: "Sonnet 4.5"},
		},
	}

	models := initialSessionModelState(nil, nil, legacy)
	if models == nil {
		t.Fatal("initialSessionModelState returned nil for legacy-models-only response")
	}
	if models.CurrentModelId != "claude-opus-4-7" {
		t.Errorf("CurrentModelId = %q, want claude-opus-4-7", models.CurrentModelId)
	}
	if len(models.AvailableModels) != 2 {
		t.Fatalf("AvailableModels len = %d, want 2", len(models.AvailableModels))
	}
	if models.AvailableModels[0].Name != "Opus 4.7" {
		t.Errorf("AvailableModels[0].Name = %q, want Opus 4.7", models.AvailableModels[0].Name)
	}

	a := newTestAdapter()
	a.emitSessionModels("sess-1", models, nil, nil)
	ev := findSessionModelsEvent(t, drainEvents(a))
	if ev.CurrentModelID != "claude-opus-4-7" {
		t.Errorf("event CurrentModelID = %q, want claude-opus-4-7", ev.CurrentModelID)
	}
	if len(ev.SessionModels) != 2 {
		t.Fatalf("event SessionModels len = %d, want 2", len(ev.SessionModels))
	}
}

func TestInitialSessionModelState_TypedConfigOptionsBeatLegacyModels(t *testing.T) {
	// When both surfaces are present, the typed configOptions[category=model]
	// list wins — matches the documented precedence.
	modelCategory := acp.SessionConfigOptionCategoryModel
	modelOptions := acp.SessionConfigSelectOptionsUngrouped{
		{Name: "GPT-5.5", Value: "gpt-5.5"},
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
	legacy := &acp.LegacyModels{
		CurrentModelId:  "claude-opus-4-7",
		AvailableModels: []acp.LegacyModelInfo{{ModelId: "claude-opus-4-7", Name: "Opus 4.7"}},
	}

	models := initialSessionModelState(nil, configOptions, legacy)
	if models == nil {
		t.Fatal("initialSessionModelState returned nil")
	}
	if models.CurrentModelId != "gpt-5.5" {
		t.Errorf("CurrentModelId = %q, want gpt-5.5 (typed must win over legacy)", models.CurrentModelId)
	}
}

// TestCurrentModelFromConfig_ReadsModelShapedOption pins the simple verbatim
// fallback used by emitSessionModels / emitSetConfigOptionEvent /
// ConfigOptionUpdate: the model-shaped option (well-known ID or category)
// surfaces its CurrentValue unchanged, regardless of any other options.
func TestCurrentModelFromConfig_ReadsModelShapedOption(t *testing.T) {
	cases := []struct {
		name    string
		options []streams.ConfigOption
		want    string
	}{
		{
			name: "well-known id",
			options: []streams.ConfigOption{
				{ID: "model", CurrentValue: "gpt-5.5"},
			},
			want: "gpt-5.5",
		},
		{
			name: "category-tagged custom id",
			options: []streams.ConfigOption{
				{ID: "primary_model", Category: "model", CurrentValue: "claude-opus-4-7"},
			},
			want: "claude-opus-4-7",
		},
		{
			name: "non-model options ignored",
			options: []streams.ConfigOption{
				{ID: "reasoning_effort", CurrentValue: "high"},
				{ID: "mode", CurrentValue: "default"},
			},
			want: "",
		},
		{
			name:    "empty options",
			options: nil,
			want:    "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := currentModelFromConfig(tc.options); got != tc.want {
				t.Errorf("currentModelFromConfig() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestEmitSessionModels_NonEmptyCurrentIDPreserved checks the happy path:
// when the agent populates CurrentModelId, we propagate it verbatim.
func TestEmitSessionModels_NonEmptyCurrentIDPreserved(t *testing.T) {
	a := newTestAdapter()
	models := &sessionModelState{
		CurrentModelId: "claude-opus-4-7",
		AvailableModels: []modelInfo{
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
	a.sessionID = "sess-1"

	cachedModels := []modelInfo{
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

// TestFinalizeSetModel_MethodNoneEmitsNothing pins the contract that when
// applySessionModel returns MethodNone (agent supports neither
// session/set_config_option nor session/set_model), the adapter must NOT emit
// a session_models convergence event nor reset the cached context-window size,
// because no switch actually happened.
func TestFinalizeSetModel_MethodNoneEmitsNothing(t *testing.T) {
	a := newTestAdapter()

	cachedModels := []modelInfo{{ModelId: "claude-opus-4-7", Name: "Opus 4.7"}}
	cachedConfig := []streams.ConfigOption{
		{Type: "select", ID: "model", Name: "Model", CurrentValue: "old-model"},
	}

	a.finalizeSetModel(sessionmodel.MethodNone, "sess-1", "claude-opus-4-7", cachedModels, cachedConfig, "", nil)

	events := drainEvents(a)
	for _, ev := range events {
		if ev.Type == streams.EventTypeSessionModels {
			t.Fatalf("expected no session_models event, got %+v", ev)
		}
	}
}

// TestFinalizeSetModel_RealMethodEmitsEvent pins the positive path: when
// applySessionModel reports a real RPC was used (MethodSetConfigOption or
// MethodSetModel), the adapter must emit a session_models event so the
// frontend converges on the new model.
func TestFinalizeSetModel_RealMethodEmitsEvent(t *testing.T) {
	a := newTestAdapter()
	a.sessionID = "sess-1"

	cachedModels := []modelInfo{{ModelId: "claude-opus-4-7", Name: "Opus 4.7"}}
	cachedConfig := []streams.ConfigOption{
		{Type: "select", ID: "model", Name: "Model", CurrentValue: "old-model"},
	}

	a.finalizeSetModel(sessionmodel.MethodSetConfigOption, "sess-1", "claude-opus-4-7", cachedModels, cachedConfig, "model", nil)

	ev := findSessionModelsEvent(t, drainEvents(a))
	if ev.CurrentModelID != "claude-opus-4-7" {
		t.Errorf("CurrentModelID = %q, want %q", ev.CurrentModelID, "claude-opus-4-7")
	}
}

// TestSetModelThenSetConfigOption_PreservesModel pins the regression where
// changing a non-model option (e.g. reasoning effort) after a SetModel call
// reverted CurrentModelID to the agent's initial default. Root cause:
// emitSetModelEvent rewrote outConfig but never refreshed the adapter's
// availableConfigOptions, so the next emitSetConfigOptionEvent read the stale
// model CurrentValue. Fix: both emit paths now write the rewritten outConfig
// back to the cache. With model and reasoning_effort as fully independent
// options (Codex 0.16.0+ / Auggie 0.27.0+), the model is whatever the
// model-shaped option's CurrentValue says — verbatim.
func TestSetModelThenSetConfigOption_PreservesModel(t *testing.T) {
	a := newTestAdapter()
	a.sessionID = "sess-1"

	reasoningOptions := []streams.ConfigOptionValue{
		{Name: "Medium", Value: "medium"},
		{Name: "High", Value: "high"},
	}
	cachedModels := []modelInfo{
		{ModelId: "gpt-5.4-mini", Name: "GPT-5.4 Mini"},
		{ModelId: "gpt-5.5", Name: "GPT-5.5"},
	}
	// Seed the adapter cache with the agent's initial state — model defaults
	// to gpt-5.5 with high effort, mirroring what session/new would surface.
	initialConfig := []streams.ConfigOption{
		{Type: "select", ID: "model", Category: "model", Name: "Model", CurrentValue: "gpt-5.5"},
		{
			Type:         "select",
			ID:           "reasoning_effort",
			Name:         "Reasoning Effort",
			CurrentValue: "high",
			Options:      reasoningOptions,
		},
	}
	a.mu.Lock()
	a.availableModels = cachedModels
	a.availableConfigOptions = initialConfig
	a.mu.Unlock()

	// 1. User selects gpt-5.4-mini — emitSetModelEvent rewrites
	// model.CurrentValue to "gpt-5.4-mini" and refreshes the cache.
	a.emitSetModelEvent("sess-1", "gpt-5.4-mini", cachedModels, initialConfig)
	_ = drainEvents(a)

	// 2. User flips reasoning effort to medium. The cached model option must
	// already reflect step 1, so currentModelFromConfig returns "gpt-5.4-mini"
	// — NOT the original gpt-5.5.
	a.mu.RLock()
	cachedAfterSetModel := a.availableConfigOptions
	a.mu.RUnlock()
	a.emitSetConfigOptionEvent("sess-1", "reasoning_effort", "medium", cachedModels, cachedAfterSetModel)

	ev := findSessionModelsEvent(t, drainEvents(a))
	if ev.CurrentModelID != "gpt-5.4-mini" {
		t.Errorf("CurrentModelID = %q, want %q (model must not revert to agent default)",
			ev.CurrentModelID, "gpt-5.4-mini")
	}
}
