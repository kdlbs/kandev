package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/mcpconfig/registry"
	"github.com/kandev/kandev/internal/common/logger"
)

type handlerRegistryCache struct {
	entries []registry.Entry
	state   registry.SyncState
}

func (c *handlerRegistryCache) ListMCPRegistryEntries(_ context.Context, query string) ([]registry.Entry, error) {
	result := make([]registry.Entry, 0)
	for _, entry := range c.entries {
		if query == "" || strings.Contains(strings.ToLower(entry.Name), strings.ToLower(query)) {
			result = append(result, entry)
		}
	}
	return result, nil
}

func (c *handlerRegistryCache) GetMCPRegistryEntry(_ context.Context, identity string) (*registry.Entry, error) {
	for index := range c.entries {
		if c.entries[index].Identity() == identity {
			return &c.entries[index], nil
		}
	}
	return nil, registry.ErrRegistryEntryNotFound
}

func (c *handlerRegistryCache) ReplaceMCPRegistryEntries(_ context.Context, entries []registry.Entry) error {
	c.entries = entries
	return nil
}

func (c *handlerRegistryCache) UpsertMCPRegistryEntries(_ context.Context, entries []registry.Entry) error {
	c.entries = append(c.entries, entries...)
	return nil
}

func (c *handlerRegistryCache) GetMCPRegistrySyncState(context.Context) (registry.SyncState, error) {
	return c.state, nil
}

func (c *handlerRegistryCache) SaveMCPRegistrySyncState(_ context.Context, state registry.SyncState) error {
	c.state = state
	return nil
}

func TestMCPMarketplaceRoutesRequireReviewAndInstallSelectedChoice(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	cache := &handlerRegistryCache{entries: []registry.Entry{{
		Name: "com.example/tools", Description: "Publisher tools", Version: "1.2.3", Revision: 1, Status: registry.StatusActive,
		Packages: []registry.Package{{RegistryType: "npm", Identifier: "@example/tools", Version: "1.2.3", Transport: registry.Transport{Type: "stdio"}}},
	}}}
	syncer := registry.NewSyncService(nil, cache)
	repo := &catalogHandlerRepository{definitions: make(map[string]*mcpconfig.MCPServerDefinition)}
	catalog := mcpconfig.NewCatalogService(repo)
	marketplace := registry.NewMarketplaceService(syncer, catalog)
	router := gin.New()
	RegisterRoutesWithMCPCatalogAndMarketplace(router, nil, nil, log, "test-interlock", repo, allowCatalogWorkspace, marketplace)

	search := performCatalogRequest(router, http.MethodGet, "/api/v1/mcp-marketplace?search=com.example/tools", "", "")
	if search.Code != http.StatusOK {
		t.Fatalf("search status/body = %d/%s", search.Code, search.Body.String())
	}
	var result registry.SearchResult
	if err := json.Unmarshal(search.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode search: %v", err)
	}
	if len(result.Entries) != 1 || !result.Entries[0].PublisherSupplied || !result.Entries[0].Choices[0].Selectable {
		t.Fatalf("search result = %#v", result)
	}

	install := performCatalogRequest(router, http.MethodPost, "/api/v1/workspaces/workspace-1/mcp-marketplace/install", `{"identity":"com.example/tools@1.2.3","expected_revision":1,"choice_id":"package-0","runtime_name":"tools","secret_bindings":[{"input_name":"TOKEN","secret_id":"secret-1"}]}`, "test-interlock")
	if install.Code != http.StatusCreated {
		t.Fatalf("install status/body = %d/%s", install.Code, install.Body.String())
	}
	if len(repo.definitions) != 1 {
		t.Fatalf("definitions = %#v", repo.definitions)
	}
	for _, definition := range repo.definitions {
		if definition.Configuration.PackageVersion != "1.2.3" || definition.Source != mcpconfig.DefinitionSourceRegistry || definition.SecretBindings[0].SecretID != "secret-1" {
			t.Fatalf("installed definition = %#v", definition)
		}
	}
}

func TestMCPMarketplaceRouteDoesNotExposeUpstreamErrorBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("NewLogger: %v", err)
	}
	cache := &handlerRegistryCache{entries: nil}
	syncer := registry.NewSyncService(nil, cache)
	marketplace := registry.NewMarketplaceService(syncer, nil)
	router := gin.New()
	RegisterRoutesWithMCPCatalogAndMarketplace(router, nil, nil, log, "test-interlock", nil, nil, marketplace)
	response := performCatalogRequest(router, http.MethodGet, "/api/v1/mcp-marketplace/entry?identity=missing", "", "")
	if response.Code != http.StatusNotFound || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("response = %d/%s", response.Code, response.Body.String())
	}
}
