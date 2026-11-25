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
