package registry

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

func TestMarketplaceLabelsUnsupportedPackagesAndInstallChoices(t *testing.T) {
	cache := &fakeCacheStore{entries: []Entry{{
		Name:        "com.example/tools",
		Title:       "Tools",
		Description: "Publisher tools",
		Version:     "1.2.3",
		Packages: []Package{
			{RegistryType: "pypi", Identifier: "tools", Version: "1.2.3", Transport: Transport{Type: "stdio"}},
			{RegistryType: "npm", Identifier: "@example/tools", Version: "1.2.3", Transport: Transport{Type: "stdio"}},
		},
		Remotes: []Remote{{Type: "streamable-http", URL: "https://mcp.example.test/mcp"}},
	}}}
	syncer := NewSyncService(nil, cache)
	marketplace := NewMarketplaceService(syncer, nil)
	result, err := marketplace.Search(context.Background(), "com.example/tools")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(result.Entries) != 1 || len(result.Entries[0].Choices) != 3 {
		t.Fatalf("marketplace entries = %#v", result.Entries)
	}
	if result.Entries[0].Choices[0].Selectable || result.Entries[0].Choices[0].UnsupportedReason == "" {
		t.Fatalf("unsupported choice = %#v", result.Entries[0].Choices[0])
	}
	if !result.Entries[0].Choices[1].Selectable || !result.Entries[0].Choices[2].Selectable {
		t.Fatalf("supported choices = %#v", result.Entries[0].Choices)
	}

	if _, err := marketplace.Install(context.Background(), InstallRequest{WorkspaceID: "workspace-1", Identity: "com.example/tools@1.2.3", ExpectedRevision: 1, ChoiceID: "package-1"}); err == nil || !errors.Is(err, ErrMarketplaceCatalogUnavailable) {
		t.Fatalf("install without catalog error = %v", err)
	}
	_ = mcpconfig.ExecutionModeRemote
}

func TestCuratedMarketplaceEntriesHaveInstallRevision(t *testing.T) {
	marketplace := NewMarketplaceService(nil, nil)
	result, err := marketplace.Search(context.Background(), "com.kandev/example-tools")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("marketplace entries = %#v", result.Entries)
	}
	if result.Entries[0].Revision != 1 {
		t.Fatalf("curated revision = %d, want 1", result.Entries[0].Revision)
	}
}

func TestRegistryChoicePreservesInstallMetadata(t *testing.T) {
	entry := Entry{
		Name:    "com.example/tools",
		Version: "1.2.3",
		Packages: []Package{{
			RegistryType:         "npm",
			RegistryBaseURL:      "https://npm.example.test",
			Identifier:           "@example/tools",
			Version:              "1.2.3",
			RuntimeHint:          "tools-bin",
			RuntimeArguments:     []Argument{{Value: "--runtime"}},
			PackageArguments:     []Argument{{Value: "--stdio"}},
			EnvironmentVariables: []KeyValueInput{{Name: "MODE", Value: "safe"}},
			Transport: Transport{
				Type:    "stdio",
				Headers: []KeyValueInput{{Name: "X-Trace", Value: "trace"}},
				Variables: map[string]KeyValueInput{
					"tenant": {Name: "tenant", IsRequired: true},
				},
			},
		}},
	}
	choice := entryChoices(entry)[0]
	input, err := definitionInputForChoice(entry, choice, InstallRequest{WorkspaceID: "workspace-1"})
	if err != nil {
		t.Fatalf("definitionInputForChoice: %v", err)
	}
	configuration := input.Configuration
	if configuration.PackageRegistry != entry.Packages[0].RegistryBaseURL || configuration.PackageExecutable != "tools-bin" {
		t.Fatalf("package metadata = %#v", configuration)
	}
	if !reflect.DeepEqual(configuration.PackageRuntimeArguments, []string{"--runtime"}) || !reflect.DeepEqual(configuration.PackageArguments, []string{"--stdio"}) {
		t.Fatalf("package arguments = %#v", configuration)
	}
	if configuration.Env["MODE"] != "safe" || configuration.Headers["X-Trace"] != "trace" {
		t.Fatalf("publisher inputs = %#v", configuration)
	}
	if _, ok := configuration.Options["registry_variables"]; !ok {
		t.Fatalf("registry variables missing from options = %#v", configuration.Options)
	}
}

func TestMarketplaceRejectsUnknownRegistryStatus(t *testing.T) {
	cache := &fakeCacheStore{entries: []Entry{{
		Name: "com.example/tools", Version: "1.2.3", Status: "review",
		Packages: []Package{{RegistryType: "npm", Identifier: "tools", Version: "1.2.3", Transport: Transport{Type: "stdio"}}},
	}}}
	marketplace := NewMarketplaceService(NewSyncService(nil, cache), nil)
	result, err := marketplace.Search(context.Background(), "com.example/tools")
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(result.Entries) != 1 || result.Entries[0].Status != Status("review") {
		t.Fatalf("registry entry = %#v", result.Entries)
	}
	if !result.Entries[0].Choices[0].Selectable {
		t.Fatalf("choice unexpectedly unavailable = %#v", result.Entries[0].Choices[0])
	}
}

func TestExactVersionRequiresNumericCore(t *testing.T) {
	for value, want := range map[string]bool{
		"1.2.3":        true,
		"1.2.3-beta.1": true,
		"1.2.3+build":  true,
		"1.2.3-":       false,
		"1.2.3+":       false,
		"01.2.3":       false,
		"1.2":          false,
		"latest":       false,
		"^1.2.3":       false,
	} {
		if got := exactVersion(value); got != want {
			t.Errorf("exactVersion(%q) = %v, want %v", value, got, want)
		}
	}
}
