package backendapp

import (
	"testing"

	agentsettingsdto "github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/settingscatalog"
)

func TestBuildSettingsRegistryUsesSharedProfileContract(t *testing.T) {
	registry, err := buildSettingsRegistry()
	if err != nil {
		t.Fatal(err)
	}

	profile, ok := registry.Domain("agent_profile")
	if !ok {
		t.Fatal("agent profile domain is missing")
	}
	fields := make(map[string]struct{}, len(profile.Fields))
	for _, field := range profile.Fields {
		fields[field.FieldPath] = struct{}{}
	}
	for _, contract := range agentsettingsdto.ProfileContractFields() {
		if _, ok := fields[contract.Path]; !ok {
			t.Fatalf("shared profile field %q is missing from runtime catalog", contract.Path)
		}
	}

	mcp, ok := registry.Domain("agent_profile_mcp")
	if !ok {
		t.Fatal("agent profile MCP domain is missing")
	}
	for _, field := range mcp.Fields {
		if field.FieldPath == "meta" && field.Support == "supported" {
			t.Fatal("MCP metadata must remain read-only")
		}
	}
}

func TestSettingsRegistryCoversRequiredDomainsWithoutPendingFields(t *testing.T) {
	registry, err := buildSettingsRegistry()
	if err != nil {
		t.Fatal(err)
	}

	required := map[string]struct{}{
		"agent": {}, "agent_profile": {}, "agent_profile_mcp": {}, "user_settings": {},
		"workflow": {}, "workflow_step": {}, "workspace": {}, "repository": {},
		"repository_script": {}, "repository_set": {}, "executor": {}, "executor_profile": {},
		"environment": {}, "task": {}, "prompt": {}, "utility_agent": {}, "editor": {},
		"notification_provider": {}, "automation": {}, "automation_trigger": {},
		"runtime_flag": {}, "storage_maintenance": {},
	}
	for _, domain := range registry.Domains() {
		for _, field := range domain.Fields {
			if field.Support == settingscatalog.SupportPending {
				t.Fatalf("required field %q is still pending", field.Key)
			}
		}
		delete(required, domain.ResourceType)
	}
	for resourceType := range required {
		t.Errorf("required resource type %q is missing", resourceType)
	}
}
