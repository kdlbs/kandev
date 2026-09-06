package storeconformance

import (
	"testing"

	"github.com/kandev/kandev/internal/persistence/requiredstores"
	testconformance "github.com/kandev/kandev/internal/testutil/storeconformance"
)

func TestStoreCatalogCompleteness(t *testing.T) {
	if err := testconformance.ValidateAdapters(requiredstores.Catalog(), Adapters()); err != nil {
		t.Fatalf("store catalog completeness: %v", err)
	}
}

func TestAdapterIDsMatchExpectedCatalogSet(t *testing.T) {
	want := map[string]struct{}{
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
	seen := make(map[string]struct{})
	for _, adapter := range Adapters() {
		seen[adapter.ID] = struct{}{}
	}
	if len(seen) != len(want) {
		t.Fatalf("adapter set has %d IDs, want exact set of %d", len(seen), len(want))
	}
	for id := range want {
		if _, ok := seen[id]; !ok {
			t.Errorf("adapter set is missing expected ID %q", id)
		}
	}
	for id := range seen {
		if _, ok := want[id]; !ok {
			t.Errorf("adapter set contains unexpected ID %q", id)
		}
	}
}

func TestSchemaInitializerRejectsUnmappedDescriptor(t *testing.T) {
	descriptor := requiredstores.Descriptor{
		ID: "unmapped", OwnerPackage: "test/unmapped", RequiredTables: []string{"unmapped"},
	}
	if err := schemaInitializerFor(descriptor)(testconformance.ScenarioContext{}); err == nil {
		t.Fatal("schemaInitializerFor() error = nil, want unmapped descriptor error")
	}
}

func TestStoreConformance(t *testing.T) {
	testconformance.Run(t, requiredstores.Catalog(), Adapters(), testconformance.RunOptions{})
}
