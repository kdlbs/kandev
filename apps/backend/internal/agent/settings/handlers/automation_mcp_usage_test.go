package handlers

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

func TestAutomationMCPCatalogUsageIsBatchedAndGuardsDelete(t *testing.T) {
	repo := &setupHandlerRepo{catalogHandlerRepository: catalogHandlerRepository{definitions: map[string]*mcpconfig.MCPServerDefinition{
		"one": {ID: "one", WorkspaceID: "workspace", DisplayName: "One", Revision: 1},
		"two": {ID: "two", WorkspaceID: "workspace", DisplayName: "Two", Revision: 1},
	}}}
	selections := mcpconfig.NewSelectionService(repo, repo)
	calls := 0
	selections.SetAutomationUsageReader(func(_ context.Context, workspace string) (map[string]int, error) {
		require.Equal(t, "workspace", workspace)
		calls++
		return map[string]int{"one": 2, "two": 1}, nil
	})
	router := gin.New()
	RegisterRoutesWithMCPCatalogAndMarketplaceAndSelections(router, nil, nil, logger.Default(), "interlock", repo, nil, nil, selections)
	response := performCatalogRequest(router, http.MethodGet, "/api/v1/workspaces/workspace/mcp-servers", "", "")
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, 1, calls)
	require.Contains(t, response.Body.String(), `"automation":2`)
	response = performCatalogRequest(router, http.MethodDelete, "/api/v1/workspaces/workspace/mcp-servers/one", `{"expected_revision":1}`, "interlock")
	require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"automation":2`)
	require.Contains(t, repo.definitions, "one")
}

type automationMixedUsageRepo struct{ *setupHandlerRepo }

func (*automationMixedUsageRepo) SelectionImpact(context.Context, string, string) (mcpconfig.SelectionImpact, error) {
	return mcpconfig.SelectionImpact{Profile: 1, Repository: 2, Task: 3, TaskSession: 4}, nil
}

func TestAutomationMCPHTTPDeleteReportsMixedUsage(t *testing.T) {
	repo := &automationMixedUsageRepo{&setupHandlerRepo{catalogHandlerRepository: catalogHandlerRepository{definitions: map[string]*mcpconfig.MCPServerDefinition{
		"one": {ID: "one", WorkspaceID: "workspace", DisplayName: "One", Revision: 1},
	}}}}
	selections := mcpconfig.NewSelectionService(repo, repo)
	selections.SetAutomationUsageReader(func(_ context.Context, workspace string) (map[string]int, error) {
		require.Equal(t, "workspace", workspace)
		return map[string]int{"one": 2}, nil
	})
	router := gin.New()
	RegisterRoutesWithMCPCatalogAndMarketplaceAndSelections(router, nil, nil, logger.Default(), "interlock", repo, nil, nil, selections)
	response := performCatalogRequest(router, http.MethodDelete, "/api/v1/workspaces/workspace/mcp-servers/one", `{"expected_revision":1}`, "interlock")
	require.Equal(t, http.StatusConflict, response.Code, response.Body.String())
	var body struct {
		Impact mcpconfig.SelectionImpact `json:"impact"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Equal(t, mcpconfig.SelectionImpact{Automation: 2, Profile: 1, Repository: 2, Task: 3, TaskSession: 4}, body.Impact)
	require.Contains(t, repo.definitions, "one")
}
