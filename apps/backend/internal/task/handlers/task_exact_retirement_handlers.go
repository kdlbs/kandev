package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/service"
)

type httpExactRetirementPreviewRequest struct {
	ReplacementTaskID             string `json:"replacement_task_id"`
	WorkspaceID                   string `json:"workspace_id"`
	ExpectedOldGeneration         string `json:"expected_old_generation"`
	ExpectedReplacementGeneration string `json:"expected_replacement_generation"`
}

func (h *TaskHandlers) httpPreviewExactRetirement(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if _, ok := authn.IdentityFromContext(c.Request.Context()); !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	var body httpExactRetirementPreviewRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if body.ReplacementTaskID == "" || body.WorkspaceID == "" || body.ExpectedOldGeneration == "" || body.ExpectedReplacementGeneration == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "replacement_task_id, workspace_id, and expected generations are required"})
		return
	}
	preview, err := h.service.PreviewExactRetirement(c.Request.Context(), service.ExactRetirementPreviewRequest{
		OldTaskID: c.Param("id"), ReplacementTaskID: body.ReplacementTaskID, WorkspaceID: body.WorkspaceID,
		ExpectedOldGeneration: body.ExpectedOldGeneration, ExpectedReplacementGeneration: body.ExpectedReplacementGeneration,
	})
	if err != nil {
		if errors.Is(err, service.ErrExactRetirementPairInvalid) || errors.Is(err, service.ErrExactRetirementGenerationStale) || errors.Is(err, service.ErrExactRetirementWorkspaceInvalid) {
			c.JSON(http.StatusConflict, gin.H{"error": "exact retirement preview blocked"})
			return
		}
		handleNotFound(c, h.logger, err, "task not found")
		return
	}
	c.JSON(http.StatusOK, preview)
}
