package main

import (
	"context"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

func TestSetSessionConfigOptionReturnsAuthoritativeState(t *testing.T) {
	sessionID := acp.SessionId("session-config-test")
	agent := &mockAgent{
		sessionConfig: map[acp.SessionId][]acp.SessionConfigOption{
			sessionID: mockSessionConfigOptions(),
		},
	}

	response, err := agent.SetSessionConfigOption(context.Background(), acp.SetSessionConfigOptionRequest{
		ValueId: &acp.SetSessionConfigOptionValueId{
			SessionId: sessionID,
			ConfigId:  "effort",
			Value:     reasoningEffortLow,
		},
	})
	if err != nil {
		t.Fatalf("SetSessionConfigOption() error = %v", err)
	}

	values := make(map[string]string, len(response.ConfigOptions))
	for _, option := range response.ConfigOptions {
		if option.Select != nil {
			values[string(option.Select.Id)] = string(option.Select.CurrentValue)
		}
	}
	if values["model"] != modelFast || values["effort"] != reasoningEffortLow {
		t.Fatalf("config values = %#v, want model=mock-fast and effort=low", values)
	}
}

func TestLoadSessionReturnsCapabilitiesForResumedSession(t *testing.T) {
	sessionID := acp.SessionId("session-load-test")
	agent := &mockAgent{
		conn:            &promptCancelUpdater{started: make(chan struct{})},
		sessions:        make(map[acp.SessionId]bool),
		sessionConfig:   make(map[acp.SessionId][]acp.SessionConfigOption),
		commandsEmitted: make(map[acp.SessionId]bool),
	}

	response, err := agent.LoadSession(context.Background(), acp.LoadSessionRequest{SessionId: sessionID})
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	select {
	case <-agent.conn.(*promptCancelUpdater).started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for resumed-session commands")
	}
	if len(response.ConfigOptions) == 0 {
		t.Fatal("LoadSession() returned no config options")
	}
	if response.Modes == nil || len(response.Modes.AvailableModes) == 0 {
		t.Fatal("LoadSession() returned no session modes")
	}
	values := make(map[string]string, len(response.ConfigOptions))
	for _, option := range response.ConfigOptions {
		if option.Select != nil {
			values[string(option.Select.Id)] = string(option.Select.CurrentValue)
		}
	}
	if values["model"] != modelFast {
		t.Fatalf("loaded model = %q, want %q", values["model"], modelFast)
	}
}

func TestSetSessionConfigOptionRejectsUnknownValue(t *testing.T) {
	sessionID := acp.SessionId("session-config-test")
	agent := &mockAgent{
		sessionConfig: map[acp.SessionId][]acp.SessionConfigOption{
			sessionID: mockSessionConfigOptions(),
		},
	}

	_, err := agent.SetSessionConfigOption(context.Background(), acp.SetSessionConfigOptionRequest{
		ValueId: &acp.SetSessionConfigOptionValueId{
			SessionId: sessionID,
			ConfigId:  "effort",
			Value:     "unadvertised",
		},
	})
	if err == nil {
		t.Fatal("SetSessionConfigOption() error = nil, want invalid value error")
	}

	values := make(map[string]string)
	for _, option := range agent.sessionConfig[sessionID] {
		if option.Select != nil {
			values[string(option.Select.Id)] = string(option.Select.CurrentValue)
		}
	}
	if values["effort"] != reasoningEffortMed {
		t.Fatalf("effort = %q, want unchanged medium", values["effort"])
	}
}

func TestSetSessionConfigOptionModelChangesAvailableOptions(t *testing.T) {
	agent := &mockAgent{
		sessions:        make(map[acp.SessionId]bool),
		sessionConfig:   make(map[acp.SessionId][]acp.SessionConfigOption),
		commandsEmitted: make(map[acp.SessionId]bool),
	}
	session, err := agent.NewSession(context.Background(), acp.NewSessionRequest{})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}

	response, err := agent.SetSessionConfigOption(context.Background(), acp.SetSessionConfigOptionRequest{
		ValueId: &acp.SetSessionConfigOptionValueId{
			SessionId: session.SessionId,
			ConfigId:  "model",
			Value:     modelSmart,
		},
	})
	if err != nil {
		t.Fatalf("set model: %v", err)
	}

	values := make(map[string]string)
	for _, option := range response.ConfigOptions {
		if option.Select != nil {
			values[string(option.Select.Id)] = string(option.Select.CurrentValue)
		}
	}
	if values["model"] != modelSmart {
		t.Fatalf("model = %q, want mock-smart", values["model"])
	}
	if values["effort"] != reasoningEffortHigh {
		t.Fatalf("effort = %q, want high for mock-smart", values["effort"])
	}
	var hasMax, hasMedium bool
	for _, option := range response.ConfigOptions {
		if option.Select == nil || option.Select.Id != "effort" || option.Select.Options.Ungrouped == nil {
			continue
		}
		for _, choice := range *option.Select.Options.Ungrouped {
			hasMax = hasMax || choice.Value == "max"
			hasMedium = hasMedium || choice.Value == reasoningEffortMed
		}
	}
	if !hasMax || hasMedium {
		t.Fatalf("response = %#v, want smart-only effort choices", response.ConfigOptions)
	}
}

func TestSessionConfigIsIsolatedAcrossNewSessions(t *testing.T) {
	agent := &mockAgent{
		sessions:        make(map[acp.SessionId]bool),
		sessionConfig:   make(map[acp.SessionId][]acp.SessionConfigOption),
		commandsEmitted: make(map[acp.SessionId]bool),
	}
	first, err := agent.NewSession(context.Background(), acp.NewSessionRequest{})
	if err != nil {
		t.Fatalf("first NewSession: %v", err)
	}
	second, err := agent.NewSession(context.Background(), acp.NewSessionRequest{})
	if err != nil {
		t.Fatalf("second NewSession: %v", err)
	}
	if first.SessionId == second.SessionId {
		t.Fatalf("session IDs must be unique: %q", first.SessionId)
	}

	_, err = agent.SetSessionConfigOption(context.Background(), acp.SetSessionConfigOptionRequest{
		ValueId: &acp.SetSessionConfigOptionValueId{
			SessionId: first.SessionId,
			ConfigId:  "effort",
			Value:     reasoningEffortLow,
		},
	})
	if err != nil {
		t.Fatalf("set first effort: %v", err)
	}
	response, err := agent.SetSessionConfigOption(context.Background(), acp.SetSessionConfigOptionRequest{
		ValueId: &acp.SetSessionConfigOptionValueId{
			SessionId: second.SessionId,
			ConfigId:  "model",
			Value:     "mock-fast",
		},
	})
	if err != nil {
		t.Fatalf("read second state: %v", err)
	}
	for _, option := range response.ConfigOptions {
		if option.Select != nil && option.Select.Id == "effort" && option.Select.CurrentValue != reasoningEffortMed {
			t.Fatalf("second effort = %q, want medium", option.Select.CurrentValue)
		}
	}
}

func TestSetSessionConfigOptionRejectsClosedSession(t *testing.T) {
	agent := &mockAgent{
		sessions:        make(map[acp.SessionId]bool),
		sessionConfig:   make(map[acp.SessionId][]acp.SessionConfigOption),
		commandsEmitted: make(map[acp.SessionId]bool),
	}
	session, err := agent.NewSession(context.Background(), acp.NewSessionRequest{})
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if _, err = agent.CloseSession(context.Background(), acp.CloseSessionRequest{SessionId: session.SessionId}); err != nil {
		t.Fatalf("CloseSession: %v", err)
	}

	_, err = agent.SetSessionConfigOption(context.Background(), acp.SetSessionConfigOptionRequest{
		ValueId: &acp.SetSessionConfigOptionValueId{
			SessionId: session.SessionId,
			ConfigId:  "effort",
			Value:     reasoningEffortLow,
		},
	})
	if err == nil {
		t.Fatal("SetSessionConfigOption() error = nil, want unknown session error")
	}
	if _, ok := agent.sessionConfig[session.SessionId]; ok {
		t.Fatal("SetSessionConfigOption() recreated closed session config")
	}
}

// TestMockSessionConfigOptionsForModelAdvertisesSlowModel verifies the mock
// agent advertises "mock-slow" in the model selector. E2E fixtures use the
// slow tier for delay-timing assertions, and the no-silent-model-fallback
// strict policy fails session start when the profile model is absent from the
// advertised list — so dropping the advertisement would break both.
func TestMockSessionConfigOptionsForModelAdvertisesSlowModel(t *testing.T) {
	for _, model := range []string{modelFast, modelSmart, modelSlow} {
		options := mockSessionConfigOptionsForModel(model)
		values := make(map[string]string, len(options))
		for _, option := range options {
			if option.Select != nil {
				values[string(option.Select.Id)] = string(option.Select.CurrentValue)
			}
		}
		if values["model"] != model {
			t.Errorf("mockSessionConfigOptionsForModel(%q) current model = %q, want %q", model, values["model"], model)
		}
	}

	modelOption := findModelOption(t)
	advertised := make([]string, 0, len(*modelOption.Select.Options.Ungrouped))
	for _, option := range *modelOption.Select.Options.Ungrouped {
		advertised = append(advertised, string(option.Value))
	}
	if !containsString(advertised, modelSlow) {
		t.Fatalf("advertised models = %v, missing mock-slow (E2E + strict policy depend on it)", advertised)
	}
	for _, want := range []string{modelFast, modelSmart, modelSlow} {
		if !containsString(advertised, want) {
			t.Errorf("advertised models = %v, missing %s", advertised, want)
		}
	}
}

func TestMockSessionConfigOptionsVariationCatalogs(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "true")
	t.Setenv("MOCK_AGENT_MODEL_CATALOG", "")
	unique := modelIDs(findModelOption(t))
	if !containsString(unique, modelUnique) {
		t.Fatalf("unique catalog = %v, missing %s", unique, modelUnique)
	}
	if containsString(unique, modelAmbiguousFirst) || containsString(unique, modelAmbiguousLast) {
		t.Fatalf("unique catalog = %v, must not include ambiguous variations", unique)
	}

	t.Setenv("MOCK_AGENT_MODEL_CATALOG", "ambiguous")
	ambiguous := modelIDs(findModelOption(t))
	for _, want := range []string{modelAmbiguousFirst, modelAmbiguousLast} {
		if !containsString(ambiguous, want) {
			t.Errorf("ambiguous catalog = %v, missing %s", ambiguous, want)
		}
	}
}

func findModelOption(t *testing.T) acp.SessionConfigOption {
	t.Helper()
	for _, option := range mockSessionConfigOptionsForModel(modelFast) {
		if option.Select != nil && string(option.Select.Id) == "model" {
			return option
		}
	}
	t.Fatal("no model config option advertised")
	return acp.SessionConfigOption{}
}

func modelIDs(option acp.SessionConfigOption) []string {
	if option.Select == nil || option.Select.Options.Ungrouped == nil {
		return nil
	}
	ids := make([]string, 0, len(*option.Select.Options.Ungrouped))
	for _, model := range *option.Select.Options.Ungrouped {
		ids = append(ids, string(model.Value))
	}
	return ids
}

func containsString(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}
