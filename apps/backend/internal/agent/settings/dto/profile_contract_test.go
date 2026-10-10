package dto

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestProfileMCPSelectionContractPreservesDefaultsAndExplicitEmpty(t *testing.T) {
	fields := ProfileContractFields()
	fieldByPath := make(map[string]ProfileContractField, len(fields))
	for _, field := range fields {
		fieldByPath[field.Path] = field
	}
	if field, ok := fieldByPath["mcp_selection_mode"]; !ok || field.JSONType != "string" || field.Support != "read_write" {
		t.Fatalf("mcp_selection_mode contract = %#v, want writable string", field)
	}
	if field, ok := fieldByPath["mcp_selected_servers"]; !ok || field.JSONType != "array" || field.Support != "read_write" || !field.Replacement {
		t.Fatalf("mcp_selected_servers contract = %#v, want writable replacement array", field)
	}

	var omitted ProfileCreateRequest
	if err := json.Unmarshal([]byte(`{"agent_id":"agent-1","name":"profile"}`), &omitted); err != nil {
		t.Fatal(err)
	}
	if omitted.MCPSelectionMode != nil {
		t.Fatal("omitted create mode must remain distinguishable for the inherit default")
	}
	if omitted.MCPSelectedServers != nil {
		t.Fatal("omitted create selection must remain distinguishable from an explicit empty list")
	}

	var patch ProfileUpdateRequest
	if err := json.Unmarshal([]byte(`{"id":"profile-1","mcp_selection_mode":"selected","mcp_selected_servers":[]}`), &patch); err != nil {
		t.Fatal(err)
	}
	if patch.MCPSelectionMode == nil || *patch.MCPSelectionMode != "selected" {
		t.Fatalf("explicit selected mode was not preserved: %#v", patch.MCPSelectionMode)
	}
	if patch.MCPSelectedServers == nil || len(*patch.MCPSelectedServers) != 0 {
		t.Fatalf("explicit empty list was not preserved: %#v", patch.MCPSelectedServers)
	}

	var omittedPatch ProfileUpdateRequest
	if err := json.Unmarshal([]byte(`{"id":"profile-1"}`), &omittedPatch); err != nil {
		t.Fatal(err)
	}
	if omittedPatch.MCPSelectionMode != nil || omittedPatch.MCPSelectedServers != nil {
		t.Fatal("omitted patch fields must preserve the saved selection")
	}
}

func TestProfileUpdateRequestPreservesOmissionAndExplicitValues(t *testing.T) {
	var req ProfileUpdateRequest
	if err := json.Unmarshal([]byte(`{"id":"profile-1","auto_approve":false,"name":""}`), &req); err != nil {
		t.Fatalf("decode profile update: %v", err)
	}
	if req.ID != "profile-1" {
		t.Fatalf("id = %q, want profile-1", req.ID)
	}
	if req.AutoApprove == nil || *req.AutoApprove {
		t.Fatalf("auto_approve = %#v, want explicit false", req.AutoApprove)
	}
	if req.Name == nil || *req.Name != "" {
		t.Fatalf("name = %#v, want explicit empty string", req.Name)
	}
	if req.Model != nil {
		t.Fatalf("omitted model = %#v, want nil", req.Model)
	}
}

func TestProfileContractFieldsAreUniqueAndClassified(t *testing.T) {
	fields := ProfileContractFields()
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if field.Path == "" || field.JSONType == "" || field.Support == "" {
			t.Fatalf("incomplete profile field: %#v", field)
		}
		if _, exists := seen[field.Path]; exists {
			t.Fatalf("duplicate profile field %q", field.Path)
		}
		seen[field.Path] = struct{}{}
	}
	for _, required := range []string{"name", "model", "fallback_model", "auto_fallback", "require_exact_model", "mode", "config_options", "cli_flags", "env_vars", "command_prefix", "provider_kind", "provider_base_url", "provider_api_key_secret_id", "enabled"} {
		if _, ok := seen[required]; !ok {
			t.Fatalf("missing profile field %q", required)
		}
	}
}

func TestProfileCreateRequestValidatesRequiredIdentity(t *testing.T) {
	if err := (ProfileCreateRequest{}).Validate(); err == nil {
		t.Fatal("empty profile create request unexpectedly validated")
	}
	if err := (ProfileCreateRequest{AgentID: "agent-1", Name: "Profile"}).Validate(); err != nil {
		t.Fatalf("profile create request with optional model rejected: %v", err)
	}
}

func TestProfileMCPSelectionValidation(t *testing.T) {
	tests := []struct {
		name    string
		mode    *string
		servers *[]string
		wantErr string
	}{
		{name: "unknown mode", mode: stringPointer("all"), wantErr: "mcp_selection_mode"},
		{name: "empty id", servers: stringSlicePointer([]string{" "}), wantErr: "mcp_selected_servers[0]"},
		{name: "duplicate id", servers: stringSlicePointer([]string{"github", "github"}), wantErr: "duplicate identifier"},
		{name: "too many ids", servers: stringSlicePointer(make([]string, maxMCPSelectedServers+1)), wantErr: "cannot contain more than"},
		{name: "valid explicit empty", mode: stringPointer(MCPSelectionModeSelected), servers: stringSlicePointer([]string{})},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateMCPSelection(tc.mode, tc.servers)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateMCPSelection() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("ValidateMCPSelection() error = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func stringPointer(value string) *string { return &value }

func stringSlicePointer(value []string) *[]string { return &value }
