package handlers

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/settingscatalog"
)

func TestDecodeSettingsUpdatePreservesExplicitEmptyValues(t *testing.T) {
	request, err := decodeSettingsUpdate([]byte(`{"target":{"resource_type":"agent_profile","resource_id":"profile-1"},"changes":{"auto_approve":false,"config_options":{},"env_vars":[]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if request.Changes["auto_approve"] == nil || string(request.Changes["auto_approve"]) != "false" {
		t.Fatalf("auto_approve = %s, want false", request.Changes["auto_approve"])
	}
	if string(request.Changes["config_options"]) != "{}" || string(request.Changes["env_vars"]) != "[]" {
		t.Fatalf("explicit empty values were not preserved: %#v", request.Changes)
	}
}

func TestValidateSettingsPatchRejectsUnknownAndComputedFields(t *testing.T) {
	registry, err := settingscatalog.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	target := settingscatalog.ResourceTarget{ResourceType: "agent_profile", ResourceID: stringPtr("profile-1")}
	if err := validateSettingsPatch(registry, target, map[string]json.RawMessage{"missing": json.RawMessage(`true`)}); err == nil {
		t.Fatal("unknown setting was accepted")
	}
	if err := validateSettingsPatch(registry, settingscatalog.ResourceTarget{ResourceType: "agent_profile_mcp", ResourceID: stringPtr("profile-1")}, map[string]json.RawMessage{"meta": json.RawMessage(`{}`)}); err == nil {
		t.Fatal("computed setting was accepted")
	}
}

func TestValidateSettingsPatchRejectsFractionalIntegers(t *testing.T) {
	registry, err := settingscatalog.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	target := settingscatalog.ResourceTarget{ResourceType: "user_settings"}
	if err := validateSettingsPatch(registry, target, map[string]json.RawMessage{
		"terminal_font_size": json.RawMessage(`13.5`),
	}); err == nil {
		t.Fatal("fractional integer was accepted")
	}
}

func stringPtr(value string) *string { return &value }
