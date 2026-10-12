package controller

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/dto"
)

func TestProfileContractConversionsPreserveExplicitValues(t *testing.T) {
	model := ""
	autoApprove := false
	cursorMCPAuthEnabled := false
	flags := []dto.CLIFlagDTO{}
	request := dto.ProfileUpdateRequest{
		ID:                   "profile-1",
		Model:                &model,
		AutoApprove:          &autoApprove,
		CursorMCPAuthEnabled: &cursorMCPAuthEnabled,
		CLIFlags:             &flags,
		Force:                true,
	}
	converted := UpdateProfileRequestFromDTO(request)
	if converted.ID != request.ID || converted.Model == nil || *converted.Model != "" ||
		converted.AutoApprove == nil || *converted.AutoApprove || converted.CursorMCPAuthEnabled == nil || *converted.CursorMCPAuthEnabled || converted.CLIFlags == nil ||
		len(*converted.CLIFlags) != 0 || !converted.Force {
		t.Fatalf("converted update = %#v, explicit values were not preserved", converted)
	}
}

func TestProfileMCPSelectionConversionsPreserveExplicitValues(t *testing.T) {
	var create dto.ProfileCreateRequest
	if err := json.Unmarshal([]byte(`{"agent_id":"agent-1","name":"Profile","mcp_selection_mode":"selected","mcp_selected_servers":["plugin-atlassian-atlassian"]}`), &create); err != nil {
		t.Fatal(err)
	}
	convertedCreate := CreateProfileRequestFromDTO(create)
	if convertedCreate.MCPSelectionMode == nil || *convertedCreate.MCPSelectionMode != "selected" {
		t.Fatalf("create mode conversion = %#v, want selected", convertedCreate.MCPSelectionMode)
	}
	if convertedCreate.MCPSelectedServers == nil || len(*convertedCreate.MCPSelectedServers) != 1 || (*convertedCreate.MCPSelectedServers)[0] != "plugin-atlassian-atlassian" {
		t.Fatalf("create selection conversion = %#v", convertedCreate.MCPSelectedServers)
	}

	var patch dto.ProfileUpdateRequest
	if err := json.Unmarshal([]byte(`{"id":"profile-1","mcp_selection_mode":"selected","mcp_selected_servers":[]}`), &patch); err != nil {
		t.Fatal(err)
	}
	convertedPatch := UpdateProfileRequestFromDTO(patch)
	if convertedPatch.MCPSelectionMode == nil || *convertedPatch.MCPSelectionMode != "selected" {
		t.Fatalf("patch mode conversion = %#v", convertedPatch.MCPSelectionMode)
	}
	if convertedPatch.MCPSelectedServers == nil || len(*convertedPatch.MCPSelectedServers) != 0 {
		t.Fatalf("explicit empty patch conversion = %#v", convertedPatch.MCPSelectedServers)
	}

	omittedPatch := UpdateProfileRequestFromDTO(dto.ProfileUpdateRequest{ID: "profile-1"})
	if omittedPatch.MCPSelectionMode != nil || omittedPatch.MCPSelectedServers != nil {
		t.Fatalf("omitted patch fields = %#v / %#v, want nil pointers", omittedPatch.MCPSelectionMode, omittedPatch.MCPSelectedServers)
	}
}

func TestProfileCreateContractConversionCopiesAllFields(t *testing.T) {
	request := dto.ProfileCreateRequest{
		AgentID: "agent-1", Name: "Profile", Model: "model", FallbackModel: "fallback",
		AutoFallback: true, Mode: "plan", ConfigOptions: map[string]string{"x": "y"},
		AllowIndexing: true, AutoApprove: true, CLIPassthrough: true,
		CLIFlags: []dto.CLIFlagDTO{{Flag: "--verbose"}}, EnvVars: []dto.ProfileEnvVarDTO{{Key: "X", Value: "Y"}},
		CommandPrefix:        "prefix",
		CursorMCPAuthEnabled: boolPointer(false),
	}
	converted := CreateProfileRequestFromDTO(request)
	if converted.AgentID != request.AgentID || converted.Name != request.Name || converted.CommandPrefix != request.CommandPrefix ||
		len(converted.CLIFlags) != 1 || len(converted.EnvVars) != 1 || !converted.AutoFallback || !converted.CLIPassthrough ||
		converted.CursorMCPAuthEnabled == nil || *converted.CursorMCPAuthEnabled {
		t.Fatalf("converted create = %#v, fields were not copied", converted)
	}
}

func boolPointer(value bool) *bool { return &value }
