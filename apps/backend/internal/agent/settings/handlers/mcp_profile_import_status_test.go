package handlers

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
	"net/http"
	"testing"
)

type importStatusHandlerRepo struct{ setupHandlerRepo }

func (r *importStatusHandlerRepo) GetMCPImportState(context.Context, string, string) (mcpconfig.LegacyImportState, error) {
	return mcpconfig.LegacyImportState{Status: "pending", FailureReason: "private reason"}, nil
}
func TestProfileMCPSelectionsExposeSafeImportStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &importStatusHandlerRepo{}
	svc := mcpconfig.NewSelectionService(repo, repo)
	svc.SetWorkspaceAuthorizer(allowCatalogWorkspace)
	svc.SetOwnerValidator(func(context.Context, mcpconfig.SelectionScope, string, string) error { return nil })
	router := gin.New()
	RegisterRoutesWithMCPCatalogAndMarketplaceAndSelections(router, nil, nil, logger.Default(), "", repo, allowCatalogWorkspace, nil, svc)
	response := performCatalogRequest(router, http.MethodGet, "/api/v1/agent-profiles/profile/mcp-selections?workspace_id=workspace-1", "", "")
	require.Equal(t, 200, response.Code)
	require.Contains(t, response.Body.String(), `"legacy_import_status":"pending"`)
	require.NotContains(t, response.Body.String(), "private reason")
}
