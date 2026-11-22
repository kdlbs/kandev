package mcpconfig

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestLegacyImportDoesNotPublishURLCredentials(t *testing.T) {
	for _, endpoint := range []string{
		"https://user:private-password@mcp.example.test/mcp",
		"https://mcp.example.test/mcp?token=private-token",
		"https://mcp.example.test/mcp#private-token",
	} {
		t.Run(endpoint, func(t *testing.T) {
			repo := newCatalogRepositoryFake()
			catalog := NewCatalogService(repo)
			selectionRepo := newSelectionRepositoryFake()
			states := newLegacyImportStateFake()
			reader := &legacyConfigReaderFake{config: &ProfileConfig{Enabled: true, Servers: map[string]ServerDef{"private": {Type: ServerTypeHTTP, URL: endpoint}}}}
			importer := NewLegacyImporter(reader, nil, catalog, NewSelectionService(selectionRepo, repo), states)
			for range 2 {
				result, err := importer.ImportProfileWorkspace(context.Background(), "ws", "profile")
				if !errors.Is(err, ErrMCPLegacyImportRequiresRebind) || !result.Fallback || result.Complete {
					t.Fatal("credential-bearing import must retain pending legacy fallback")
				}
			}
			defs, err := catalog.List(context.Background(), "ws")
			if err != nil {
				t.Fatal(err)
			}
			data, _ := json.Marshal(defs)
			if strings.Contains(string(data), "private-password") || strings.Contains(string(data), "private-token") {
				t.Fatal("catalog response includes legacy URL credentials")
			}
			if len(defs) != 1 || repo.created != 1 || defs[0].Configuration.URL != "https://mcp.example.test/mcp" {
				t.Fatal("import must be idempotent and publish only a stripped endpoint")
			}
			if len(selectionRepo.values) != 0 || reader.config.Servers["private"].URL != endpoint {
				t.Fatal("unsafe migration changed legacy launch configuration or enabled a catalog selection")
			}
		})
	}
}

func TestLegacyEndpointRetainsPublicURLAndSanitizesParseFailure(t *testing.T) {
	value, unsafe, err := legacyEndpointForCatalog("https://mcp.example.test/mcp")
	if err != nil || unsafe || value != "https://mcp.example.test/mcp" {
		t.Fatal("public endpoint was not preserved")
	}
	_, _, err = legacyEndpointForCatalog("https://private-password@invalid%host")
	if err == nil || strings.Contains(err.Error(), "private-password") {
		t.Fatal("malformed endpoint must fail without revealing its source")
	}
}
