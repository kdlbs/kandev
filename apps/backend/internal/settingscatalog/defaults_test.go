package settingscatalog

import "testing"

func TestDefaultRegistryContainsEveryRequiredResourceType(t *testing.T) {
	registry, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"agent_profile", "agent_profile_mcp", "agent", "user_settings", "workflow", "workflow_step",
		"workspace", "repository", "repository_script", "repository_set", "executor", "executor_profile",
		"environment", "task", "prompt", "utility_agent", "editor", "notification_provider",
		"issue_integration", "code_host_integration", "automation", "automation_trigger", "runtime_flag",
		"storage_maintenance",
	}
	for _, resourceType := range want {
		if _, ok := registry.Domain(resourceType); !ok {
			t.Errorf("resource type %q is missing", resourceType)
		}
	}
}

func TestDefaultProfileFieldsAreWritableAndBound(t *testing.T) {
	registry, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	domain, ok := registry.Domain("agent_profile")
	if !ok {
		t.Fatal("agent_profile descriptor missing")
	}
	if len(domain.Fields) < 14 {
		t.Fatalf("profile field count = %d, want complete profile contract", len(domain.Fields))
	}
	for _, field := range domain.Fields {
		if !field.Writable || field.Validator == "" || field.Authority == "" {
			t.Errorf("profile field %q is not bound for writes: %#v", field.Key, field)
		}
	}
}

func TestWritableDomainsExposeConcreteOperationAuthorities(t *testing.T) {
	registry, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, domain := range registry.Domains() {
		writable := false
		for _, field := range domain.Fields {
			writable = writable || field.Writable
		}
		if !writable {
			continue
		}
		for _, operation := range domain.Operations {
			if operation.Name == "update" && (operation.Authority == "" || operation.Authority == "domain.owner") {
				t.Errorf("%s update authority = %q, want concrete field authority", domain.ResourceType, operation.Authority)
			}
		}
	}
}

func TestCatalogOperationsNeverUsePlaceholderAuthorities(t *testing.T) {
	registry, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, domain := range registry.Domains() {
		for _, operation := range domain.Operations {
			if operation.Authority == "domain.owner" {
				t.Fatalf("%s operation %q uses placeholder authority", domain.ResourceType, operation.Name)
			}
		}
	}
}
