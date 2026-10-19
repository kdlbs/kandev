package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
)

func (h *WorkspaceHandlers) httpCloneWorkspace(c *gin.Context) {
	var body struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	workspace, err := h.service.CloneWorkspace(c.Request.Context(), c.Param("id"), body.Name)
	switch {
	case errors.Is(err, service.ErrWorkspaceCloneName):
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace name is required", "code": "workspace_clone_name"})
	case errors.Is(err, repoerrors.ErrWorkspaceCloneConfiguration):
		c.JSON(http.StatusConflict, gin.H{"error": "workspace configuration cannot be cloned", "code": "workspace_clone_configuration"})
	case errors.Is(err, github.ErrGHCLIOperatorRequired):
		c.JSON(http.StatusForbidden, gin.H{"error": "operator access is required to copy this GitHub connection", "code": "workspace_clone_credentials"})
	case err != nil:
		handleNotFound(c, h.logger, err, "workspace could not be cloned")
	default:
		c.JSON(http.StatusCreated, h.withAccess(c.Request.Context(), workspace))
	}
}
