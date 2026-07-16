package acp

import (
	"context"
	"strings"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

func TestEmitAuthoritativeConfigOptionsUsesCompleteResponseState(t *testing.T) {
	a := newTestAdapter()
	a.sessionID = "sess-1"
	values := func(current string) acp.SessionConfigOption {
		options := acp.SessionConfigSelectOptionsUngrouped{{Value: acp.SessionConfigValueId(current), Name: current}}
		return acp.SessionConfigOption{Select: &acp.SessionConfigOptionSelect{
			Type: "select", Id: acp.SessionConfigId(current), Name: current,
			CurrentValue: acp.SessionConfigValueId(current), Options: acp.SessionConfigSelectOptions{Ungrouped: &options},
		}}
	}
	response := []acp.SessionConfigOption{values("low"), values("on")}
	response[0].Select.Id = "reasoning_effort"
	response[1].Select.Id = "fast_mode"

	a.emitAuthoritativeConfigOptions("sess-1", "reasoning_effort", response, nil)

	event := findSessionModelsEvent(t, drainEvents(a))
	if got := currentConfigValue(event.ConfigOptions, "reasoning_effort"); got != "low" {
		t.Errorf("reasoning_effort = %q, want response value low", got)
	}
	if got := currentConfigValue(event.ConfigOptions, "fast_mode"); got != "on" {
		t.Errorf("fast_mode = %q, want dependent response value on", got)
	}
	if event.Data["config_options_source"] != "provider_response" ||
		event.Data["config_options_config_id"] != "reasoning_effort" {
		t.Fatalf("authoritative metadata = %#v", event.Data)
	}
}

func TestEmitAuthoritativeConfigOptionsIgnoresReplacedSession(t *testing.T) {
	a := newTestAdapter()
	a.sessionID = "sess-2"
	a.availableConfigOptions = []streams.ConfigOption{{
		ID: "reasoning_effort", CurrentValue: "high",
	}}
	options := acp.SessionConfigSelectOptionsUngrouped{{Value: "low", Name: "Low"}}

	a.emitAuthoritativeConfigOptions("sess-1", "reasoning_effort", []acp.SessionConfigOption{{
		Select: &acp.SessionConfigOptionSelect{
			Type: "select", Id: "reasoning_effort", Name: "Reasoning effort",
			CurrentValue: "low", Options: acp.SessionConfigSelectOptions{Ungrouped: &options},
		},
	}}, nil)

	if events := drainEvents(a); len(events) != 0 {
		t.Fatalf("stale response emitted %d events, want none", len(events))
	}
	if got := currentConfigValue(a.availableConfigOptions, "reasoning_effort"); got != "high" {
		t.Fatalf("active session cache = %q, want high", got)
	}
}

func currentConfigValue(options []streams.ConfigOption, id string) string {
	for _, option := range options {
		if option.ID == id {
			return option.CurrentValue
		}
	}
	return ""
}

// TestSetConfigOption_WithoutConnectionReturnsError pins the precondition
// that SetConfigOption must surface an error rather than panic when invoked
// before Initialize() has wired up the ACP connection. The same precondition
// is enforced by SetMode/SetModel; this test keeps the new method aligned.
func TestSetConfigOption_WithoutConnectionReturnsError(t *testing.T) {
	a := newTestAdapter()

	err := a.SetConfigOption(context.Background(), "model", "claude-3-7-sonnet")
	if err == nil {
		t.Fatalf("expected error when adapter not initialized")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("error = %q, want one containing %q", err.Error(), "not initialized")
	}
}

// TestIsModelConfigID pins the recognizer used by SetConfigOption to decide
// whether a successful set_config_option RPC must also emit a session_models
// convergence event (so the orchestrator persists AgentProfileSnapshot["model"]
// and the selection survives a page refresh).
func TestIsModelConfigID(t *testing.T) {
	cachedConfig := []streams.ConfigOption{
		{Type: "select", ID: "model", Name: "Model"},
		{Type: "select", ID: "thought_level", Category: "model", Name: "Thought"},
		{Type: "select", ID: "reasoning_effort", Name: "Reasoning"},
	}

	cases := []struct {
		name     string
		configID string
		want     bool
	}{
		{"well-known model ID matches", "model", true},
		{"custom ID with model category matches", "thought_level", true},
		{"unrelated config ID does not match", "reasoning_effort", false},
		{"unknown ID does not match", "missing", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isModelConfigID(tc.configID, cachedConfig); got != tc.want {
				t.Errorf("isModelConfigID(%q) = %v, want %v", tc.configID, got, tc.want)
			}
		})
	}

	// Empty cached config still treats the well-known ID as the model so the
	// agent can be served before its first ConfigOptionUpdate has refreshed
	// the cache.
	if !isModelConfigID("model", nil) {
		t.Errorf("isModelConfigID(\"model\", nil) = false, want true")
	}
}

// TestEmitSetConfigOptionEvent_RewritesChangedOptionAndKeepsModel pins that
// emitSetConfigOptionEvent (called from SetConfigOption for non-model option
// changes) emits a session_models convergence event with the changed option's
// CurrentValue updated and the existing model's CurrentValue carried through
// unchanged. Without this, the orchestrator's handleSessionModelsEvent never
// runs for non-model changes and reasoning_effort / thought_level updates
// would be lost on page refresh after a backend restart.
func TestEmitSetConfigOptionEvent_RewritesChangedOptionAndKeepsModel(t *testing.T) {
	a := newTestAdapter()
	cachedModels := []modelInfo{{ModelId: "gpt-5", Name: "GPT-5"}}
	cachedConfig := []streams.ConfigOption{
		{Type: "select", ID: "model", Category: "model", Name: "Model", CurrentValue: "gpt-5"},
		{Type: "select", ID: "reasoning_effort", Name: "Reasoning", CurrentValue: "low"},
	}

	a.emitSetConfigOptionEvent("sess-1", "reasoning_effort", "high", cachedModels, cachedConfig)

	events := drainEvents(a)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	ev := events[0]
	if ev.Type != streams.EventTypeSessionModels {
		t.Errorf("event Type = %q, want %q", ev.Type, streams.EventTypeSessionModels)
	}
	if ev.SessionID != "sess-1" {
		t.Errorf("event SessionID = %q, want %q", ev.SessionID, "sess-1")
	}
	if ev.CurrentModelID != "gpt-5" {
		t.Errorf("event CurrentModelID = %q, want %q (carried from existing model option)", ev.CurrentModelID, "gpt-5")
	}

	got := map[string]string{}
	for _, opt := range ev.ConfigOptions {
		got[opt.ID] = opt.CurrentValue
	}
	if got["reasoning_effort"] != "high" {
		t.Errorf("ConfigOptions[reasoning_effort] CurrentValue = %q, want %q", got["reasoning_effort"], "high")
	}
	if got["model"] != "gpt-5" {
		t.Errorf("ConfigOptions[model] CurrentValue = %q, want %q (must not be reset)", got["model"], "gpt-5")
	}

	// Cached config must not be mutated — emitSetConfigOptionEvent copies before rewriting.
	if cachedConfig[1].CurrentValue != "low" {
		t.Errorf("cachedConfig[reasoning_effort] mutated to %q; expected event-local copy only", cachedConfig[1].CurrentValue)
	}
}

// TestEmitSetConfigOptionEvent_SkipsWhenCacheEmpty pins that when the local
// config cache hasn't been seeded yet (e.g. the agent hasn't sent its first
// ConfigOptionUpdate), emitSetConfigOptionEvent returns without emitting any
// event rather than broadcasting a blank session_models frame that would
// temporarily clear the UI's model selector. The agent's own ConfigOptionUpdate
// remains the authoritative event for this path.
func TestEmitSetConfigOptionEvent_SkipsWhenCacheEmpty(t *testing.T) {
	a := newTestAdapter()

	a.emitSetConfigOptionEvent("sess-1", "reasoning_effort", "high", nil, nil)

	events := drainEvents(a)
	if len(events) != 0 {
		t.Fatalf("expected 0 events when cachedConfig is empty; got %d", len(events))
	}
}
