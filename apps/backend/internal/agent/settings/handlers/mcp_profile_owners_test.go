package handlers

import (
	"context"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

type profileOwnersRepo struct {
	setupHandlerRepo
	reads   int
	failure bool
}

func (r *profileOwnersRepo) ListMCPProfileOwners(_ context.Context, w, after string, limit int) ([]mcpconfig.MCPProfileOwner, error) {
	r.reads++
	if r.failure {
		return nil, errors.New("private database detail")
	}
	if after != "" {
		return []mcpconfig.MCPProfileOwner{}, nil
	}
	items := []mcpconfig.MCPProfileOwner{{ID: "p1", AgentID: "a", WorkspaceID: w, Name: "Named workspace profile"}, {ID: "p2", AgentID: "a", Name: "Global profile"}}
	if limit < len(items) {
		items = items[:limit]
	}
	return items, nil
}
func (r *profileOwnersRepo) GetMCPProfileOwner(_ context.Context, w, id string) (*mcpconfig.MCPProfileOwner, error) {
	r.reads++
	if r.failure {
		return nil, errors.New("private database detail")
	}
	if id != "p1" {
		return nil, nil
	}
	return &mcpconfig.MCPProfileOwner{ID: id, AgentID: "a", WorkspaceID: w, Name: "Named workspace profile"}, nil
}

// @covers AC-AGENTS-MCP-MANAGEMENT-001.2, AC-AGENTS-MCP-MANAGEMENT-001.3
func TestMCPProfileOwnersAuthorizedRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &profileOwnersRepo{}
	svc := mcpconfig.NewSelectionService(repo, repo)
	svc.SetWorkspaceAuthorizer(allowCatalogWorkspace)
	router := gin.New()
	RegisterRoutesWithMCPCatalogAndMarketplaceAndSelections(router, nil, nil, logger.Default(), "", repo, allowCatalogWorkspace, nil, svc)
	path := "/api/v1/workspaces/workspace-1/mcp-profile-owners"
	response := performCatalogRequest(router, http.MethodGet, path+"?limit=1", "", "")
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"next_cursor":"p1"`)
	for _, test := range []struct {
		suffix string
		status int
	}{{"/p1", 200}, {"/missing", 404}, {"?limit=101", 400}, {"?limit=oops", 400}, {"?cursor=p1&cursor=p2", 400}} {
		response = performCatalogRequest(router, http.MethodGet, path+test.suffix, "", "")
		require.Equal(t, test.status, response.Code, response.Body.String())
	}
	before := repo.reads
	response = performCatalogRequest(router, http.MethodGet, "/api/v1/workspaces/denied/mcp-profile-owners", "", "")
	require.Equal(t, 404, response.Code)
	require.Equal(t, before, repo.reads)
	repo.failure = true
	response = performCatalogRequest(router, http.MethodGet, path, "", "")
	require.Equal(t, 500, response.Code)
	require.NotContains(t, response.Body.String(), "private database detail")
}

func TestMCPProfileOwnersMissingAuthorizationFailsClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, configured := range []bool{false, true} {
		repo := &profileOwnersRepo{}
		var svc *mcpconfig.SelectionService
		if configured {
			svc = mcpconfig.NewSelectionService(repo, repo)
		}
		router := gin.New()
		RegisterRoutesWithMCPCatalogAndMarketplaceAndSelections(router, nil, nil, logger.Default(), "", repo, nil, nil, svc)
		for _, path := range []string{"/api/v1/workspaces/w/mcp-profile-owners", "/api/v1/workspaces/w/mcp-profile-owners/p1"} {
			response := performCatalogRequest(router, http.MethodGet, path, "", "")
			require.Equal(t, 503, response.Code)
		}
		require.Zero(t, repo.reads)
	}
}
