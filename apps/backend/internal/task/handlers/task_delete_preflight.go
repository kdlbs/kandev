package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	"go.uber.org/zap"
)

type httpTaskDeletePreflightRequest struct {
	AllowUnverifiedWorkspaceChanges bool                        `json:"allow_unverified_workspace_changes"`
	WorkspaceCleanup                models.WorkspaceCleanupMode `json:"workspace_cleanup"`
	TaskIDs                         []string                    `json:"task_ids"`
	Cascade                         bool                        `json:"cascade"`
	DiscardWorktreeChanges          bool                        `json:"discard_worktree_changes"`
}

// httpTaskDeletePreflight returns a no-store, read-only cleanup consent
// decision for the requested delete scope. The delete route still performs
// its own authoritative inspection immediately before mutation.
func (h *TaskHandlers) httpTaskDeletePreflight(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var body httpTaskDeletePreflightRequest
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task delete preflight request"})
		return
	}
	result, err := h.service.TaskDeletePreflightWithOptions(c.Request.Context(), body.TaskIDs, body.Cascade, service.DeleteTaskOptions{DiscardWorktreeChanges: body.DiscardWorktreeChanges, WorkspaceCleanup: body.WorkspaceCleanup, AllowUnverifiedWorkspaceChanges: body.AllowUnverifiedWorkspaceChanges})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrTaskDeletePreflightInvalid):
			c.JSON(http.StatusBadRequest, taskErrorBody(err))
		case errors.Is(err, service.ErrTaskDeleteConfirmationIdentity):
			c.JSON(http.StatusUnauthorized, gin.H{"error": "task deletion preview requires an authenticated user"})
		case errors.Is(err, service.ErrTaskDeletePreflightUnavailable):
			if fields := cleanupInspectionFields(err); len(fields) > 0 {
				h.logger.Warn("task delete preflight inspection failed", append(fields, zap.Int("task_count", len(body.TaskIDs)))...)
			}
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "task delete preflight unavailable"})
		default:
			handleNotFound(c, h.logger, err, "task not found")
		}
		return
	}
	c.JSON(http.StatusOK, result)
}
