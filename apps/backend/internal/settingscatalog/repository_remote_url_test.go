package settingscatalog

import "testing"

func TestRepositoryRemoteURLRepairCatalogMetadata(t *testing.T) {
	registry, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	domain, ok := registry.Domain("repository")
	if !ok {
		t.Fatal("repository domain is missing")
	}
	var remoteURL FieldDescriptor
	for _, field := range domain.Fields {
		if field.FieldPath == "remote_url" {
			remoteURL = field
		}
	}
	if remoteURL.Key != "repository.remote_url" || !remoteURL.Writable || remoteURL.JSONType != "string" {
		t.Fatalf("remote_url field = %+v, want writable repository string", remoteURL)
	}
	if remoteURL.Description == "" || remoteURL.Description == "Repository setting." {
		t.Fatalf("remote_url description = %q, want identity constraints", remoteURL.Description)
	}
	if len(domain.Operations) != 1 {
		t.Fatalf("repository operations = %d, want one update operation", len(domain.Operations))
	}
	properties, ok := domain.Operations[0].Options["properties"].(map[string]any)
	if !ok {
		t.Fatalf("repository operation options = %#v, want validate_only property", domain.Operations[0].Options)
	}
	validateOnly, ok := properties["validate_only"].(map[string]any)
	if !ok || validateOnly["type"] != "boolean" {
		t.Fatalf("validate_only option = %#v, want boolean", properties["validate_only"])
	}
}
