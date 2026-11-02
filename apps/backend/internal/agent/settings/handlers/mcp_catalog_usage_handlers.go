package handlers

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"net/http"
	"strconv"
)

func (h *Handlers) httpMCPCatalogUsage(c *gin.Context) {
	if h.selections == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{errorResponseKey: "MCP usage is unavailable"})
		return
	}
	limit := 0
	if raw, exists := c.GetQuery("limit"); exists {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			c.JSON(http.StatusBadRequest, gin.H{errorResponseKey: "invalid MCP usage page"})
			return
		}
		limit = parsed
	}
	page, err := h.selections.CatalogUsage(c.Request.Context(), c.Param("id"), c.Param("serverID"), c.Query("cursor"), limit)
	switch {
	case err == nil:
		c.JSON(http.StatusOK, page)
	case errors.Is(err, mcpconfig.ErrMCPUsageUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{errorResponseKey: "MCP usage is unavailable"})
	case errors.Is(err, mcpconfig.ErrMCPWorkspaceAccess), errors.Is(err, mcpconfig.ErrMCPServerDefinitionNotFound):
		c.JSON(http.StatusNotFound, gin.H{errorResponseKey: "MCP server definition not found"})
	case errors.Is(err, mcpconfig.ErrMCPUsageCursor), errors.Is(err, mcpconfig.ErrMCPInvalidSelection):
		c.JSON(http.StatusBadRequest, gin.H{errorResponseKey: "invalid MCP usage page"})
	default:
		h.logger.Error("failed to read MCP usage")
		c.JSON(http.StatusInternalServerError, gin.H{errorResponseKey: "failed to read MCP usage"})
	}
}
