package handlers

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"net/http"
	"strconv"
)

func (h *Handlers) httpListMCPProfileOwners(c *gin.Context) {
	if h.selections == nil {
		writeProfileOwnerError(c, mcpconfig.ErrMCPUsageUnavailable)
		return
	}
	query := c.Request.URL.Query()
	if len(query["cursor"]) > 1 || len(query["limit"]) > 1 {
		writeProfileOwnerError(c, mcpconfig.ErrMCPInvalidSelection)
		return
	}
	limit := 0
	if raw, ok := c.GetQuery("limit"); ok {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			writeProfileOwnerError(c, mcpconfig.ErrMCPInvalidSelection)
			return
		}
		limit = n
	}
	page, err := h.selections.ListProfileOwners(c.Request.Context(), c.Param("id"), c.Query("cursor"), limit)
	if err != nil {
		writeProfileOwnerError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, page)
}
func (h *Handlers) httpGetMCPProfileOwner(c *gin.Context) {
	if h.selections == nil {
		writeProfileOwnerError(c, mcpconfig.ErrMCPUsageUnavailable)
		return
	}
	owner, err := h.selections.GetProfileOwner(c.Request.Context(), c.Param("id"), c.Param("profileID"))
	if err != nil {
		writeProfileOwnerError(c, err)
		return
	}
	if owner == nil {
		writeProfileOwnerError(c, mcpconfig.ErrMCPSelectionOwnerAccess)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, owner)
}
func writeProfileOwnerError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	message := "failed to read MCP profile owners"
	switch {
	case errors.Is(err, mcpconfig.ErrMCPWorkspaceAccess), errors.Is(err, mcpconfig.ErrMCPSelectionOwnerAccess):
		status = http.StatusNotFound
		message = "MCP profile owner not found"
	case errors.Is(err, mcpconfig.ErrMCPInvalidSelection):
		status = http.StatusBadRequest
		message = "invalid MCP profile owner request"
	case errors.Is(err, mcpconfig.ErrMCPUsageUnavailable):
		status = http.StatusServiceUnavailable
		message = "MCP profile owners unavailable"
	}
	c.JSON(status, gin.H{errorResponseKey: message})
}
