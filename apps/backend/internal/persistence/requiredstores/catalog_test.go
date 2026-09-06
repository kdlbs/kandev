package requiredstores

import "testing"

func TestCatalog(t *testing.T) {
	catalog := Catalog()
	if err := ValidateCatalog(catalog); err != nil {
		t.Fatalf("ValidateCatalog() error = %v", err)
	}

	wantIDs := map[string]struct{}{
		"agent-settings": {}, "analytics": {}, "auth": {}, "auth-hostnames": {}, "automation": {},
		"azure-devops": {}, "canvas": {}, "delivery": {}, "editor": {},
		"github": {}, "gitlab": {}, "jira": {}, "linear": {}, "message-queue": {},
		"notification": {}, "office": {}, "office-config-sync": {}, "organization-units": {},
		"organizations": {}, "plugin-instance-state": {}, "plugin-instances": {},
		"plugin-marketplace": {}, "plugin-settings": {}, "plugin-state": {},
		"plugin-user-state": {}, "prompts": {}, "quick-terminal": {}, "runtime-flags": {},
		"schema-meta": {}, "secrets": {}, "sentry": {}, "storage": {}, "system-settings": {},
		"task": {}, "task-share": {}, "telemetry-contract": {}, "terminal": {},
		"user": {}, "utility": {}, "workflow": {}, "workflow-sync": {},
	}
	seen := make(map[string]struct{}, len(catalog))
	for _, descriptor := range catalog {
		if _, exists := seen[descriptor.ID]; exists {
			t.Fatalf("catalog contains duplicate ID %q", descriptor.ID)
		}
		seen[descriptor.ID] = struct{}{}
		if descriptor.OwnerPackage == "" {
			t.Errorf("catalog entry %q has no owner package", descriptor.ID)
		}
		if len(descriptor.RequiredTables) == 0 {
			t.Errorf("catalog entry %q has no required tables", descriptor.ID)
		}
	}
	if len(seen) != len(wantIDs) {
		t.Fatalf("catalog has %d IDs, want exact set of %d", len(seen), len(wantIDs))
	}
	for id := range wantIDs {
		if _, exists := seen[id]; !exists {
			t.Errorf("catalog is missing expected ID %q", id)
		}
	}
	for id := range seen {
		if _, expected := wantIDs[id]; !expected {
			t.Errorf("catalog contains unexpected ID %q", id)
		}
	}

	first := Catalog()
	first[0].ID = "changed"
	first[0].RequiredTables[0] = "changed"
	second := Catalog()
	if second[0].ID == "changed" || second[0].RequiredTables[0] == "changed" {
		t.Fatal("Catalog() exposes mutable internal data")
	}
}

func TestValidateCatalogRejectsInvalidDescriptors(t *testing.T) {
	tests := []struct {
		name    string
		catalog []Descriptor
	}{
		{
			name: "duplicate ID",
			catalog: []Descriptor{
				{ID: "one", OwnerPackage: "owner/one", RequiredTables: []string{"one"}},
				{ID: "one", OwnerPackage: "owner/two", RequiredTables: []string{"two"}},
			},
		},
		{
			name: "missing dependency",
			catalog: []Descriptor{{
				ID: "one", OwnerPackage: "owner/one", RequiredTables: []string{"one"}, DependsOn: []string{"missing"},
			}},
		},
		{
			name: "dependency cycle",
			catalog: []Descriptor{
				{ID: "one", OwnerPackage: "owner/one", RequiredTables: []string{"one"}, DependsOn: []string{"two"}},
				{ID: "two", OwnerPackage: "owner/two", RequiredTables: []string{"two"}, DependsOn: []string{"one"}},
			},
		},
		{
			name:    "invalid ID",
			catalog: []Descriptor{{ID: "One", OwnerPackage: "owner/one", RequiredTables: []string{"one"}}},
		},
		{
			name:    "missing owner",
			catalog: []Descriptor{{ID: "one", RequiredTables: []string{"one"}}},
		},
		{
			name:    "missing table",
			catalog: []Descriptor{{ID: "one", OwnerPackage: "owner/one"}},
		},
		{
			name: "invalid capability",
			catalog: []Descriptor{
				{ID: "one", OwnerPackage: "owner/one", RequiredTables: []string{"one"}, Capabilities: []Capability{"unknown"}},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := ValidateCatalog(test.catalog); err == nil {
				t.Fatal("ValidateCatalog() error = nil, want error")
			}
		})
	}
}
