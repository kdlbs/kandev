package registry

import (
	"context"
	"errors"
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
