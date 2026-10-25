package controller

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
)

func TestCursorMCPAuthPreferenceCreatePatchAndDuplicate(t *testing.T) {
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
	if !createdDefault.CursorMCPAuthEnabled {
		t.Fatal("omitted create preference should default to true")
	}
	disabled := false
	createdDisabled, err := ctrl.CreateProfile(context.Background(), CreateProfileRequest{
		AgentID: agent.ID, Name: "disabled", CursorMCPAuthEnabled: &disabled,
	})
	if err != nil {
		t.Fatalf("create disabled profile: %v", err)
	}
	if createdDisabled.CursorMCPAuthEnabled {
		t.Fatal("explicit false create preference was lost")
	}

	updatedOmitted, err := ctrl.UpdateProfile(context.Background(), UpdateProfileRequest{ID: createdDisabled.ID, Name: stringPointer("renamed")})
	if err != nil {
		t.Fatalf("update unrelated field: %v", err)
	}
	if updatedOmitted.CursorMCPAuthEnabled {
		t.Fatal("omitted patch reset the saved false preference")
	}

	enabled := true
	updatedEnabled, err := ctrl.UpdateProfile(context.Background(), UpdateProfileRequest{
		ID: createdDisabled.ID, CursorMCPAuthEnabled: &enabled,
	})
	if err != nil {
		t.Fatalf("enable preference: %v", err)
	}
	if !updatedEnabled.CursorMCPAuthEnabled {
		t.Fatal("explicit true patch was not applied")
	}

	if _, err := ctrl.UpdateProfile(context.Background(), UpdateProfileRequest{ID: createdDisabled.ID, CursorMCPAuthEnabled: &disabled}); err != nil {
		t.Fatalf("disable preference before duplicate: %v", err)
	}
	duplicated, err := ctrl.DuplicateProfile(context.Background(), DuplicateProfileRequest{ID: createdDisabled.ID})
	if err != nil {
		t.Fatalf("duplicate disabled profile: %v", err)
	}
	if duplicated.CursorMCPAuthEnabled {
		t.Fatal("duplicate did not preserve explicit false")
	}

	var omittedCreate dto.ProfileCreateRequest
	if err := json.Unmarshal([]byte(`{"agent_id":"agent-1","name":"wire"}`), &omittedCreate); err != nil {
		t.Fatal(err)
	}
	if omittedCreate.CursorMCPAuthEnabled != nil {
		t.Fatal("omitted create preference should remain distinguishable")
	}
	var explicitFalseCreate dto.ProfileCreateRequest
	if err := json.Unmarshal([]byte(`{"agent_id":"agent-1","name":"wire","cursor_mcp_auth_enabled":false}`), &explicitFalseCreate); err != nil {
		t.Fatal(err)
	}
	if explicitFalseCreate.CursorMCPAuthEnabled == nil || *explicitFalseCreate.CursorMCPAuthEnabled {
		t.Fatal("create JSON lost explicit false")
	}
	convertedCreate := CreateProfileRequestFromDTO(dto.ProfileCreateRequest{AgentID: agent.ID, Name: "wire", CursorMCPAuthEnabled: &disabled})
	if convertedCreate.CursorMCPAuthEnabled == nil || *convertedCreate.CursorMCPAuthEnabled {
		t.Fatal("create DTO conversion lost explicit false")
	}
	convertedPatch := UpdateProfileRequestFromDTO(dto.ProfileUpdateRequest{ID: createdDisabled.ID, CursorMCPAuthEnabled: &disabled})
	if convertedPatch.CursorMCPAuthEnabled == nil || *convertedPatch.CursorMCPAuthEnabled {
		t.Fatal("patch DTO conversion lost explicit false")
	}
}

func stringPointer(value string) *string { return &value }
