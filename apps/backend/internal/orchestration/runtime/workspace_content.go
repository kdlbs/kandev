package runtime

import (
	"context"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/orchestration/models"
)

type workspaceContentReader interface {
	WorkspaceTaskContent(context.Context, string, string, models.WorkspaceContentQuery) (any, error)
}

type workspacePermissionReader interface {
	WorkspaceTaskPermissions(context.Context, string, string, string) (any, error)
}

func (h *Handler) taskPermissions(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok || !h.privateRuntimeAllowed(c, claims, c.Param("id")) {
		return
	}
	if _, linked := c.Get(workspaceSelectionKey); linked {
		c.AbortWithStatus(403)
		return
	}
	reader, ok := h.Service.Manager.(workspacePermissionReader)
	if !ok {
		c.AbortWithStatus(503)
		return
	}
	result, err := reader.WorkspaceTaskPermissions(c.Request.Context(), claims.WorkspaceID, c.Param("id"), c.Query("session_id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, result)
}

func (h *Handler) taskContent(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok || !h.privateRuntimeAllowed(c, claims, c.Param("id")) {
		return
	}
	// Linked workspace exports retain their separate grant contract.
	if _, linked := c.Get(workspaceSelectionKey); linked {
		c.AbortWithStatus(403)
		return
	}
	reader, ok := h.Service.Manager.(workspaceContentReader)
	if !ok {
		c.AbortWithStatus(503)
		return
	}
	var query models.WorkspaceContentQuery
	if err := c.ShouldBindQuery(&query); err != nil {
		fail(c, err)
		return
	}
	result, err := reader.WorkspaceTaskContent(c.Request.Context(), claims.WorkspaceID, c.Param("id"), query)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, result)
}
