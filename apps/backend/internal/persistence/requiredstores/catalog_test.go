package requiredstores

import "testing"

func TestCatalog(t *testing.T) {
	catalog := Catalog()
	if len(catalog) < 30 {
		t.Fatalf("catalog has %d entries, want at least 30", len(catalog))
	}
	if err := ValidateCatalog(catalog); err != nil {
		t.Fatalf("ValidateCatalog() error = %v", err)
	}

	seen := make(map[string]bool, len(catalog))
	for _, descriptor := range catalog {
		if seen[descriptor.ID] {
			t.Fatalf("catalog contains duplicate ID %q", descriptor.ID)
		}
		seen[descriptor.ID] = true
		if descriptor.OwnerPackage == "" {
			t.Errorf("catalog entry %q has no owner package", descriptor.ID)
		}
		if len(descriptor.RequiredTables) == 0 {
			t.Errorf("catalog entry %q has no required tables", descriptor.ID)
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
