package acp

import (
	"encoding/json"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

func TestModelResultCarriesVerifiedCatalog(t *testing.T) {
	for _, agentID := range []string{cursorAgentID, "generic-acp"} {
		t.Run(agentID, func(t *testing.T) {
			testModelResultCarriesVerifiedCatalog(t, agentID)
		})
	}
}

func testModelResultCarriesVerifiedCatalog(t *testing.T, agentID string) {
	t.Helper()
	modelCategory := acp.SessionConfigOptionCategoryModel
	model := acp.SessionConfigOption{Select: &acp.SessionConfigOptionSelect{
		Type: "select", Id: "model", Name: "Model", Category: &modelCategory, CurrentValue: "sonnet",
	}}
	reasoning := acp.SessionConfigOption{Select: &acp.SessionConfigOptionSelect{
		Type: "select", Id: "reasoning_effort", Name: "Effort", CurrentValue: "medium",
	}}
	boolean := acp.SessionConfigOption{Boolean: &acp.SessionConfigOptionBoolean{
		Type: "boolean", Id: "toggle", Name: "Toggle", CurrentValue: false,
	}}
	for _, tc := range []struct {
		name     string
		options  []acp.SessionConfigOption
		complete bool
		verified []string
	}{
		{name: "typed complete list", options: []acp.SessionConfigOption{model, reasoning}, complete: true},
		{name: "absent catalog"},
		{name: "partial projection", options: []acp.SessionConfigOption{model, reasoning, boolean}, verified: []string{"model", "reasoning_effort"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter := newTestAdapter()
			adapter.agentID = agentID
			adapter.sessionID = "native-session"
			adapter.availableConfigOptions = []streams.ConfigOption{
				{ID: "model", Category: "model", CurrentValue: "opus"},
				{ID: "fast", CurrentValue: "false"},
			}
			peer := &fakeModelApplier{configResponse: acp.SetSessionConfigOptionResponse{ConfigOptions: tc.options}}

			result, err := adapter.setModelWithConnResult(t.Context(), peer, "native-session", "sonnet")
			require.NoError(t, err)
			require.Equal(t, []string{"config:model:sonnet"}, peer.calls)
			wire, err := json.Marshal(result)
			require.NoError(t, err)
			var payload struct {
				ModelState *struct {
					SessionID string                 `json:"session_id"`
					Model     string                 `json:"current_model_id"`
					Options   []streams.ConfigOption `json:"config_options"`
					Complete  bool                   `json:"config_options_complete"`
					Verified  []string               `json:"verified_config_option_ids"`
				} `json:"model_state"`
			}
			require.NoError(t, json.Unmarshal(wire, &payload))
			if tc.complete || len(tc.verified) > 0 {
				require.NotNil(t, payload.ModelState)
				require.Equal(t, "native-session", payload.ModelState.SessionID)
				require.Equal(t, "sonnet", payload.ModelState.Model)
				require.Equal(t, tc.complete, payload.ModelState.Complete)
				require.Equal(t, tc.verified, payload.ModelState.Verified)
				require.Len(t, payload.ModelState.Options, 2)
			} else {
				require.Nil(t, payload.ModelState)
			}
			for _, event := range drainEvents(adapter) {
				if event.Type == streams.EventTypeSessionModels {
					complete, _ := event.Data["config_options_complete"].(bool)
					require.Equal(t, tc.complete, complete)
					require.Equal(t, tc.verified, event.VerifiedConfigOptionIDs)
				}
			}
		})
	}
}

func TestSetupCatalogCanIdentifyModelSeparately(t *testing.T) {
	adapter := newTestAdapter()
	adapter.sessionID = "native-session"
	adapter.emitSessionModels("native-session", &sessionModelState{CurrentModelId: "active"}, nil, []acp.SessionConfigOption{{
		Select: &acp.SessionConfigOptionSelect{Type: "select", Id: "reasoning", CurrentValue: "high"},
	}})
	state := adapter.GetSessionModelState()
	require.NotNil(t, state)
	require.True(t, state.ConfigOptionsComplete)
	require.Equal(t, "active", state.CurrentModelID)
}

func TestLoadSnapshotKeepsConvergenceProvenance(t *testing.T) {
	for _, tc := range []struct {
		name     string
		converge func(*Adapter, string, []streams.ConfigOption)
	}{
		{name: "model", converge: func(a *Adapter, sessionID string, config []streams.ConfigOption) {
			a.emitSetModelEvent(sessionID, "next", nil, config)
		}},
		{name: "setting", converge: func(a *Adapter, sessionID string, config []streams.ConfigOption) {
			a.emitSetConfigOptionEvent(sessionID, "fast", "false", nil, config)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, _ := newSessionResumeAdapter(t, true, true)
			const sessionID = "saved-session"
			require.NoError(t, a.LoadSession(t.Context(), sessionID, nil))
			setup := a.GetSessionModelState()
			require.NotNil(t, setup)
			require.False(t, setup.ConfigOptionsDisplayOnly)

			tc.converge(a, sessionID, append(setup.ConfigOptions, streams.ConfigOption{ID: "fast", CurrentValue: "true"}))
			snapshot := a.GetSessionModelState()
			require.NotNil(t, snapshot)
			require.True(t, snapshot.ConfigOptionsDisplayOnly)
			require.False(t, snapshot.HasConfigOptionEvidence())
			require.Greater(t, snapshot.SessionSettingsGeneration, setup.SessionSettingsGeneration)

			require.NoError(t, a.LoadSession(t.Context(), sessionID, nil))
			refreshed := a.GetSessionModelState()
			require.NotNil(t, refreshed)
			require.False(t, refreshed.ConfigOptionsDisplayOnly)
			require.Equal(t, setup.ConfigOptions, refreshed.ConfigOptions)
		})
	}
}

func TestGrokModelSwitchEvidenceUsesReportedEffort(t *testing.T) {
	for _, tc := range []struct {
		name     string
		efforts  []any
		verified []string
	}{
		{name: "advertised default", efforts: []any{map[string]any{"value": "medium", "default": true}}, verified: []string{configOptionIDReasoningEffort}},
		{name: "display fallback", efforts: []any{"medium", "low"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			models := []modelInfo{{ModelId: "next", Meta: map[string]any{
				"supportsReasoningEffort": true,
				"reasoningEfforts":        tc.efforts,
			}}}
			a := grokAdapterWithModels(t, models, []streams.ConfigOption{
				{ID: configOptionIDModel, Category: configOptionIDModel, CurrentValue: "previous"},
				{ID: configOptionIDReasoningEffort, CurrentValue: "high"},
			})
			peer := &fakeGrokSetModelConn{}
			require.NoError(t, a.setModelWithConn(t.Context(), peer, a.sessionID, "next"))
			require.Len(t, peer.reqs, 1)
			require.Empty(t, peer.reqs[0].Meta)
			event := findSessionModelsEvent(t, drainEvents(a))
			require.Equal(t, "medium", currentGrokEffortFromConfig(event.ConfigOptions))
			require.Equal(t, tc.verified, event.VerifiedConfigOptionIDs)
			require.Equal(t, tc.verified, a.GetSessionModelState().VerifiedConfigOptionIDs)
		})
	}
}
