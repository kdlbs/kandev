package backendapp

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	cursorcloudruntime "github.com/kandev/kandev/internal/agent/runtime/cursorcloud"
	provider "github.com/kandev/kandev/internal/cursorcloud"
	"github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type bindCursorCloudCandidateRequest struct {
	RunID string `json:"run_id"`
}

type retryCursorCloudSubmissionRequest struct {
	ResolutionID             string `json:"resolution_id"`
	AcknowledgeDuplicateWork bool   `json:"acknowledge_duplicate_work"`
}

func registerCursorCloudSubmissionRoutes(router *gin.Engine, manager *cursorCloudAgentManager) {
	if router == nil || manager == nil {
		return
	}
	// Gin requires the wildcard names to match the task/session route groups
	// already registered by task handlers at the same path positions.
	base := "/api/v1/tasks/:id/sessions/:sessionId/cursor-cloud/submission"
	router.GET(base, func(c *gin.Context) {
		result, err := manager.getSubmissionResolution(c.Request.Context(), c.Param("id"), c.Param("sessionId"))
		if err != nil {
			writeCursorCloudResolutionError(c, err)
			return
		}
		c.JSON(http.StatusOK, result)
	})
	router.POST(base+"/bind", func(c *gin.Context) {
		var request bindCursorCloudCandidateRequest
		if err := c.ShouldBindJSON(&request); err != nil || request.RunID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid Cursor Cloud run candidate"})
			return
		}
		operation, err := manager.bindSubmissionCandidate(c.Request.Context(), c.Param("id"), c.Param("sessionId"), request.RunID)
		if err != nil {
			writeCursorCloudResolutionError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"operation_id": operation.ID, "state": operation.State})
	})
	router.POST(base+"/retry", func(c *gin.Context) {
		var request retryCursorCloudSubmissionRequest
		if err := c.ShouldBindJSON(&request); err != nil || request.ResolutionID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid Cursor Cloud retry request"})
			return
		}
		operation, err := manager.retryUnknownSubmission(c.Request.Context(), c.Param("id"), c.Param("sessionId"),
			request.ResolutionID, request.AcknowledgeDuplicateWork)
		if operation == nil && err != nil {
			writeCursorCloudResolutionError(c, err)
			return
		}
		status := http.StatusOK
		if errors.Is(err, provider.ErrOutcomeUnknown) {
			status = http.StatusAccepted
		} else if err != nil {
			writeCursorCloudResolutionError(c, err)
			return
		}
		c.JSON(status, gin.H{"operation_id": operation.ID, "state": operation.State})
	})
}

func writeCursorCloudResolutionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, repository.ErrManagedAgentBindingNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "Cursor Cloud conversation not found"})
	case errors.Is(err, taskservice.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": "Cursor Cloud conversation is not available to this user"})
	case errors.Is(err, cursorcloudruntime.ErrSubmissionResolutionUnavailable),
		errors.Is(err, cursorcloudruntime.ErrSubmissionCandidateUnavailable),
		errors.Is(err, cursorcloudruntime.ErrRetryAcknowledgmentRequired),
		errors.Is(err, repository.ErrManagedAgentActiveOperation),
		errors.Is(err, repository.ErrManagedAgentBindingConflict):
		c.JSON(http.StatusConflict, gin.H{"error": "Cursor Cloud submission cannot be resolved in its current state"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Cursor Cloud submission resolution failed"})
	}
}
