package runtime

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

type workspaceAdministrator interface {
	ManageWorkspace(context.Context, string, json.RawMessage) (any, error)
}

func (h *Handler) manageWorkspace(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok {
		return
	}
	manager, ok := h.Service.Manager.(workspaceAdministrator)
	if !ok {
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}
	var request json.RawMessage
	if err := c.ShouldBindJSON(&request); err != nil {
		fail(c, err)
		return
	}
	result, err := manager.ManageWorkspace(c.Request.Context(), claims.WorkspaceID, request)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"result": result})
}
