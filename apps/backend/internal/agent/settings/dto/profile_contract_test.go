package dto

import (
	"encoding/json"
	"testing"
)

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
	for _, required := range []string{"name", "model", "fallback_model", "auto_fallback", "mode", "config_options", "cli_flags", "env_vars", "command_prefix", "enabled"} {
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
