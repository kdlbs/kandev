package backendapp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/runtimeflags"
	"github.com/kandev/kandev/internal/settingscatalog"
)

func TestSettingsMutationRequiresCallerIdentity(t *testing.T) {
	registry, err := settingscatalog.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	operations := &settingsOperations{registry: registry}
	target := settingscatalog.ResourceTarget{ResourceType: "user_settings"}
	changes := map[string]json.RawMessage{"confirm_task_archive": json.RawMessage(`false`)}
	if err := operations.authorizeSettingsMutation(context.Background(), target, changes); err == nil {
		t.Fatal("mutation without caller identity was accepted")
	}
	if err := operations.authorizeSettingsMutation(
		authn.WithIdentity(context.Background(), authn.Identity{Synthetic: true}), target, changes,
	); err != nil {
		t.Fatalf("synthetic identity was rejected: %v", err)
	}
}

func TestSettingsSensitiveValuesAreRedactedWithoutDroppingReferences(t *testing.T) {
	registry, err := settingscatalog.DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]any{
		"env_vars":       []any{map[string]any{"key": "TOKEN", "value": "secret-value", "secret_id": "secret-1"}},
		"config_options": map[string]any{"safe": "kept"},
	}
	redactSettingsValues(values, registry, "agent_profile")

	encoded, _ := json.Marshal(values)
	if strings.Contains(string(encoded), "secret-value") {
		t.Fatalf("sensitive value leaked: %s", encoded)
	}
	entry := values["env_vars"].([]any)[0].(map[string]any)
	if entry["secret_id"] != "secret-1" || entry["redacted"] != true {
		t.Fatalf("secret reference was not preserved safely: %#v", entry)
	}
}

func TestRuntimeFlagSettingsValueUsesCatalogOverridePath(t *testing.T) {
	override := true
	value := runtimeFlagSettingsValue(runtimeflags.RuntimeFlagState{
		Key:           "features.example",
		OverrideValue: &override,
	})
	if value["override"] != true {
		t.Fatalf("override = %#v, want true", value["override"])
	}
	if _, ok := value["override_value"]; ok {
		t.Fatalf("service-specific override_value leaked into settings contract: %#v", value)
	}
}

func TestDecodeUserSettingsUpdateMapsSidebarTaskColors(t *testing.T) {
	request, err := decodeUserSettingsUpdate(map[string]json.RawMessage{
		"user_settings.sidebar_task_colors": json.RawMessage(`{"task-1":"blue","task-2":null}`),
	})
	if err != nil {
		t.Fatalf("decode user settings: %v", err)
	}
	if request.SidebarTaskColorPatch == nil {
		t.Fatal("sidebar task color patch is nil")
	}
	if request.SidebarTaskColorPatch.Colors["task-1"] == nil ||
		*request.SidebarTaskColorPatch.Colors["task-1"] != "blue" {
		t.Fatalf("task-1 color = %#v", request.SidebarTaskColorPatch.Colors["task-1"])
	}
	if value, exists := request.SidebarTaskColorPatch.Colors["task-2"]; !exists || value != nil {
		t.Fatalf("task-2 color = %#v, want a clear tombstone", value)
	}
}

func TestAgentProfileCollectionsRedactEmbeddedEnvironmentValues(t *testing.T) {
	values := map[string]any{
		"profiles": []any{
			map[string]any{
				"name":     "Default",
				"env_vars": []any{map[string]any{"key": "TOKEN", "value": "secret-value", "secret_id": "secret-1"}},
			},
		},
	}
	redactAgentProfileCollections(values)

	profile := values["profiles"].([]any)[0].(map[string]any)
	entry := profile["env_vars"].([]any)[0].(map[string]any)
	if entry["value"] != "[redacted]" || entry["secret_id"] != "secret-1" {
		t.Fatalf("embedded environment value was not safely redacted: %#v", entry)
	}
}
