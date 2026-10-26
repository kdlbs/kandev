package handlers

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
	"net/http"
	"strings"
	"testing"
)

// @covers AC-AGENTS-MCP-MANAGEMENT-001.4
func TestMCPCatalogUsageUnavailableIsNotEmpty(t *testing.T) {
	gin.SetMode(gin.TestMode)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	repo := &catalogHandlerRepository{definitions: map[string]*mcpconfig.MCPServerDefinition{}}
	RegisterRoutesWithMCPCatalog(router, nil, nil, log, "test-interlock", repo, allowCatalogWorkspace)
	response := performCatalogRequest(router, http.MethodGet, "/api/v1/workspaces/workspace-1/mcp-servers/definition/usage", "", "")
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s; want unavailable", response.Code, response.Body.String())
	}
}

type usageHandlerRepo struct{ setupHandlerRepo }

func (r *usageHandlerRepo) ListMCPUsageCandidates(context.Context, string, string, mcpconfig.UsageReference, int) ([]mcpconfig.UsageReference, error) {
	return []mcpconfig.UsageReference{{Scope: mcpconfig.SelectionScopeTask, OwnerID: "task"}}, nil
}

// @covers AC-AGENTS-MCP-MANAGEMENT-001.2, AC-AGENTS-MCP-MANAGEMENT-001.3, AC-AGENTS-MCP-MANAGEMENT-001.4
func TestMCPCatalogUsageRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &usageHandlerRepo{setupHandlerRepo: setupHandlerRepo{catalogHandlerRepository: catalogHandlerRepository{definitions: map[string]*mcpconfig.MCPServerDefinition{
		"definition": {ID: "definition", WorkspaceID: "workspace-1", Configuration: mcpconfig.MCPServerConfiguration{URL: "https://private.example"}},
	}}}}
	svc := mcpconfig.NewSelectionService(repo, repo)
	svc.SetWorkspaceAuthorizer(allowCatalogWorkspace)
	fail := false
	svc.SetUsageOwnerReader(func(_ context.Context, w string, ref mcpconfig.UsageReference) (*mcpconfig.UsageOwner, error) {
		if fail {
			return nil, errors.New("private lookup detail")
		}
		return &mcpconfig.UsageOwner{UsageReference: ref, WorkspaceID: w, DisplayName: "Project task"}, nil
	})
	router := gin.New()
	RegisterRoutesWithMCPCatalogAndMarketplaceAndSelections(router, nil, nil, logger.Default(), "", repo, allowCatalogWorkspace, nil, svc)
	path := "/api/v1/workspaces/workspace-1/mcp-servers/definition/usage"
	for _, tc := range []struct {
		path   string
		status int
	}{{path, 200}, {path + "?limit=oops", 400}, {path + "?limit=101", 400}, {path + "?cursor=tampered", 400}, {strings.Replace(path, "workspace-1", "denied", 1), 404}, {strings.Replace(path, "definition", "removed", 1), 404}} {
		response := performCatalogRequest(router, http.MethodGet, tc.path, "", "")
		require.Equal(t, tc.status, response.Code, response.Body.String())
		require.NotContains(t, response.Body.String(), "private.example")
		if tc.status == 200 {
			require.Contains(t, response.Body.String(), "Project task")
			require.Contains(t, response.Body.String(), `"kind":"task"`)
		}
	}
	fail = true
	response := performCatalogRequest(router, http.MethodGet, path, "", "")
	require.Equal(t, 500, response.Code)
	require.NotContains(t, response.Body.String(), "private lookup detail")
}
