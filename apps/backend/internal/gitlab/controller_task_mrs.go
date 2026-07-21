package gitlab

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// RegisterTaskMRHTTPRoutes registers the explicit link/unlink endpoints on an
// existing /api/v1/gitlab router group.
func (c *Controller) RegisterTaskMRHTTPRoutes(api *gin.RouterGroup) {
	api.POST("/task-mrs", c.httpCreateTaskMR)
	api.DELETE("/task-mrs/:associationID", c.httpDeleteTaskMR)
}

type createTaskMRRequest struct {
	TaskID       string `json:"task_id"`
	RepositoryID string `json:"repository_id"`
	MRURL        string `json:"mr_url"`
}

func (c *Controller) httpCreateTaskMR(ctx *gin.Context) {
	workspaceID := c.workspaceID(ctx)
	if workspaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "workspace_id query parameter required"})
		return
	}
	var request createTaskMRRequest
	if err := ctx.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.TaskID) == "" || strings.TrimSpace(request.MRURL) == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "task_id and mr_url are required"})
		return
	}
	association, err := c.service.AssociateExistingMRByURL(
		ctx.Request.Context(), workspaceID, request.TaskID, request.RepositoryID, request.MRURL,
	)
	if err != nil {
		writeTaskMRError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, association)
}

func (c *Controller) httpDeleteTaskMR(ctx *gin.Context) {
	workspaceID := c.workspaceID(ctx)
	associationID := strings.TrimSpace(ctx.Param("associationID"))
	if workspaceID == "" || associationID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "workspace_id and association_id are required"})
		return
	}
	if err := c.service.UnlinkTaskMR(ctx.Request.Context(), workspaceID, associationID); err != nil {
		writeTaskMRError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

func writeTaskMRError(ctx *gin.Context, err error) {
	status := http.StatusInternalServerError
	message := "GitLab task merge request operation failed"
	switch {
	case errors.Is(err, ErrWorkspaceRequired):
		status = http.StatusBadRequest
		message = "workspace_id query parameter required"
	case errors.Is(err, ErrInvalidMRURL):
		status = http.StatusBadRequest
		message = "invalid GitLab merge request URL"
	case errors.Is(err, ErrTaskMRNotFound):
		status = http.StatusNotFound
		message = "task merge request association not found"
	case errors.Is(err, ErrTaskMRRepositoryRequired):
		status = http.StatusConflict
		message = "repository_id is required for multi-repository tasks"
	case errors.Is(err, ErrTaskMRRepositoryMismatch):
		status = http.StatusUnprocessableEntity
		message = "repository does not match GitLab merge request"
	case errors.Is(err, ErrNotConfigured), errors.Is(err, ErrNoClient):
		status = http.StatusServiceUnavailable
		message = providerActionFailedMessage + ": connection resolution"
	}
	ctx.JSON(status, gin.H{responseErrorKey: message})
}
