package controller

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

func TestCursorPluginsMCPPreferenceCreatePatchAndDuplicate(t *testing.T) {
	ctrl := newTestController(map[string]agents.Agent{"test-agent": &testAgent{
		id: "test-agent", name: "test-agent", displayName: "Test Agent", enabled: true,
	}})
	st := newFakeStore()
	agent := &models.Agent{ID: "agent-1", Name: "test-agent"}
	st.agents[agent.ID] = agent
	st.byName[agent.Name] = agent
	ctrl.repo = st

	createdDefault, err := ctrl.CreateProfile(context.Background(), CreateProfileRequest{AgentID: agent.ID, Name: "default"})
	if err != nil {
		t.Fatalf("create default profile: %v", err)
	}
	if !createdDefault.CursorPluginsMCPEnabled {
		t.Fatal("omitted create preference should default to true")
	}
	if createdDefault.MCPSelectionMode != dto.MCPSelectionModeInherit || createdDefault.MCPSelectedServers == nil || len(createdDefault.MCPSelectedServers) != 0 {
		t.Fatalf("omitted selection defaults = %q / %#v, want inherit / empty", createdDefault.MCPSelectionMode, createdDefault.MCPSelectedServers)
	}
	disabled := false
	selectedServers := []string{"plugin-atlassian-atlassian", "github"}
	createdDisabled, err := ctrl.CreateProfile(context.Background(), CreateProfileRequest{
		AgentID: agent.ID, Name: "disabled", CursorPluginsMCPEnabled: &disabled,
		CursorMCPAuthEnabled: &disabled, MCPSelectionMode: stringPointer("selected"), MCPSelectedServers: &selectedServers,
	})
	if err != nil {
		t.Fatalf("create disabled profile: %v", err)
	}
	if createdDisabled.CursorPluginsMCPEnabled {
		t.Fatal("explicit false create preference was lost")
	}
	if createdDisabled.CursorMCPAuthEnabled || createdDisabled.MCPSelectionMode != "selected" || len(createdDisabled.MCPSelectedServers) != 2 {
		t.Fatalf("create MCP preferences = auth:%v mode:%q servers:%#v", createdDisabled.CursorMCPAuthEnabled, createdDisabled.MCPSelectionMode, createdDisabled.MCPSelectedServers)
	}

	updatedOmitted, err := ctrl.UpdateProfile(context.Background(), UpdateProfileRequest{ID: createdDisabled.ID, Name: stringPointer("renamed")})
	if err != nil {
		t.Fatalf("update unrelated field: %v", err)
	}
	if updatedOmitted.CursorPluginsMCPEnabled {
		t.Fatal("omitted patch reset the saved false preference")
	}
	if updatedOmitted.CursorMCPAuthEnabled || updatedOmitted.MCPSelectionMode != "selected" || len(updatedOmitted.MCPSelectedServers) != 2 {
		t.Fatalf("unrelated patch changed MCP preferences: auth:%v mode:%q servers:%#v", updatedOmitted.CursorMCPAuthEnabled, updatedOmitted.MCPSelectionMode, updatedOmitted.MCPSelectedServers)
	}

	enabled := true
	updatedEnabled, err := ctrl.UpdateProfile(context.Background(), UpdateProfileRequest{
		ID: createdDisabled.ID, CursorPluginsMCPEnabled: &enabled,
	})
	if err != nil {
		t.Fatalf("enable preference: %v", err)
	}
	if !updatedEnabled.CursorPluginsMCPEnabled {
		t.Fatal("explicit true patch was not applied")
	}

	if _, err := ctrl.UpdateProfile(context.Background(), UpdateProfileRequest{ID: createdDisabled.ID, CursorPluginsMCPEnabled: &disabled}); err != nil {
		t.Fatalf("disable preference before duplicate: %v", err)
	}
	duplicated, err := ctrl.DuplicateProfile(context.Background(), DuplicateProfileRequest{ID: createdDisabled.ID})
	if err != nil {
		t.Fatalf("duplicate disabled profile: %v", err)
	}
	if duplicated.CursorPluginsMCPEnabled {
		t.Fatal("duplicate did not preserve explicit false")
	}
	if duplicated.CursorMCPAuthEnabled || duplicated.MCPSelectionMode != "selected" || len(duplicated.MCPSelectedServers) != 2 {
		t.Fatalf("duplicate MCP preferences = auth:%v mode:%q servers:%#v", duplicated.CursorMCPAuthEnabled, duplicated.MCPSelectionMode, duplicated.MCPSelectedServers)
	}
	emptySelection := []string{}
	cleared, err := ctrl.UpdateProfile(context.Background(), UpdateProfileRequest{
		ID: createdDisabled.ID, MCPSelectedServers: &emptySelection,
	})
	if err != nil {
		t.Fatalf("clear selected servers: %v", err)
	}
	if cleared.MCPSelectionMode != "selected" || cleared.MCPSelectedServers == nil || len(cleared.MCPSelectedServers) != 0 {
		t.Fatalf("explicit empty selection = %q / %#v, want selected / []", cleared.MCPSelectionMode, cleared.MCPSelectedServers)
	}

	var omittedCreate dto.ProfileCreateRequest
	if err := json.Unmarshal([]byte(`{"agent_id":"agent-1","name":"wire"}`), &omittedCreate); err != nil {
		t.Fatal(err)
	}
	if omittedCreate.CursorPluginsMCPEnabled != nil {
		t.Fatal("omitted create preference should remain distinguishable")
	}
	var explicitFalseCreate dto.ProfileCreateRequest
	if err := json.Unmarshal([]byte(`{"agent_id":"agent-1","name":"wire","cursor_plugins_mcp_enabled":false}`), &explicitFalseCreate); err != nil {
		t.Fatal(err)
	}
	if explicitFalseCreate.CursorPluginsMCPEnabled == nil || *explicitFalseCreate.CursorPluginsMCPEnabled {
		t.Fatal("create JSON lost explicit false")
	}
	convertedCreate := CreateProfileRequestFromDTO(dto.ProfileCreateRequest{AgentID: agent.ID, Name: "wire", CursorPluginsMCPEnabled: &disabled})
	if convertedCreate.CursorPluginsMCPEnabled == nil || *convertedCreate.CursorPluginsMCPEnabled {
		t.Fatal("create DTO conversion lost explicit false")
	}
	convertedPatch := UpdateProfileRequestFromDTO(dto.ProfileUpdateRequest{ID: createdDisabled.ID, CursorPluginsMCPEnabled: &disabled})
	if convertedPatch.CursorPluginsMCPEnabled == nil || *convertedPatch.CursorPluginsMCPEnabled {
		t.Fatal("patch DTO conversion lost explicit false")
	}
}
