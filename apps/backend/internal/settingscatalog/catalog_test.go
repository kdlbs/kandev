package settingscatalog

import (
	"strings"
	"testing"
)

func TestRegistryRejectsDuplicateKeysAndUnboundWritableFields(t *testing.T) {
	_, err := NewRegistry([]DomainDescriptor{
		{
			Domain:       "example",
			ResourceType: "example",
			Owner:        "example",
			Target:       TargetRules{RequiresResourceID: true},
			Fields: []FieldDescriptor{
				{Key: "example.name", FieldPath: "name", Owner: "example", Support: SupportSupported},
				{Key: "example.name", FieldPath: "name_copy", Owner: "example", Support: SupportSupported},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate setting key") {
		t.Fatalf("NewRegistry error = %v, want duplicate-key validation", err)
	}

	_, err = NewRegistry([]DomainDescriptor{{
		Domain:       "example",
		ResourceType: "example",
		Owner:        "example",
		Target:       TargetRules{RequiresResourceID: true},
		Fields: []FieldDescriptor{{
			Key:       "example.name",
			FieldPath: "name",
			Owner:     "example",
			Support:   SupportSupported,
			Writable:  true,
			Validator: "",
			Authority: "",
		}},
	}})
	if err == nil || !strings.Contains(err.Error(), "writable setting") {
		t.Fatalf("NewRegistry error = %v, want writable binding validation", err)
	}
}

func TestSearchRanksExactKeyBeforeDescriptionAndNeverSearchesValues(t *testing.T) {
	registry, err := NewRegistry([]DomainDescriptor{{
		Domain:       "profiles",
		ResourceType: "profile",
		Owner:        "profiles",
		Target:       TargetRules{RequiresResourceID: true},
		Fields: []FieldDescriptor{
			{
				Key:         "profile.model",
				FieldPath:   "model",
				Label:       "Model",
				Description: "Select the preferred model.",
				Aliases:     []string{"provider choice"},
				Owner:       "profiles",
				Support:     SupportSupported,
			},
			{
				Key:         "profile.name",
				FieldPath:   "name",
				Label:       "Name",
				Description: "The saved profile label.",
				Owner:       "profiles",
				Support:     SupportSupported,
			},
		},
	}})
	if err != nil {
		t.Fatal(err)
	}

	results, err := registry.Search(SearchRequest{Query: "profile.model"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results.Items) != 1 || results.Items[0].Key != "profile.model" || results.Items[0].MatchReason != MatchExact {
		t.Fatalf("exact search = %#v, want one exact model result", results.Items)
	}

	results, err = registry.Search(SearchRequest{Query: "secret-value"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results.Items) != 0 {
		t.Fatalf("value-like query returned %#v", results.Items)
	}
}

func TestRegistryValidatesTargetSelectors(t *testing.T) {
	registry, err := NewRegistry([]DomainDescriptor{{
		Domain:       "user",
		ResourceType: "user_settings",
		Owner:        "user",
		Target:       TargetRules{Singleton: true},
		Fields: []FieldDescriptor{{
			Key:       "user_settings.appearance",
			FieldPath: "appearance",
			Owner:     "user",
			Support:   SupportSupported,
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}

	if err := registry.ValidateTarget(ResourceTarget{ResourceType: "user_settings"}); err != nil {
		t.Fatalf("singleton target rejected: %v", err)
	}
	if err := registry.ValidateTarget(ResourceTarget{ResourceType: "user_settings", ResourceID: stringPtr("other")}); err == nil {
		t.Fatal("resource ID accepted for singleton target")
	}
	if err := registry.ValidateTarget(ResourceTarget{ResourceType: "unknown"}); err == nil {
		t.Fatal("unknown resource type accepted")
	}
}

func stringPtr(value string) *string { return &value }
