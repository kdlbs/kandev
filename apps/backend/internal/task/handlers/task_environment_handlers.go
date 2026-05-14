package handlers

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/service"
)

const responseKeySuccess = "success"

func (h *TaskHandlers) httpGetTaskEnvironment(c *gin.Context) {
	taskID := c.Param("id")
	env, err := h.service.GetTaskEnvironmentByTaskID(c.Request.Context(), taskID)
	if err != nil {
		handleNotFound(c, h.logger, err, "task environment not found")
		return
	}
	if env == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no environment for this task"})
		return
	}
	c.JSON(http.StatusOK, env.ToAPI())
}

// httpGetTaskEnvironmentLive returns the recorded TaskEnvironment row plus a
// real-time snapshot of the underlying container (when applicable). Designed
// to be polled from the Executor Settings popover so users see state changes
// without a page reload.
func (h *TaskHandlers) httpGetTaskEnvironmentLive(c *gin.Context) {
	taskID := c.Param("id")
	env, err := h.service.GetTaskEnvironmentByTaskID(c.Request.Context(), taskID)
	if err != nil {
		handleNotFound(c, h.logger, err, "task environment not found")
		return
	}
	if env == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no environment for this task"})
		return
	}
	live, err := h.service.GetTaskEnvironmentLiveStatus(c.Request.Context(), taskID)
	if err != nil {
		h.logger.Warn("failed to fetch live container status",
			zap.String("task_id", taskID),
			zap.Error(err))
	}
	resp := gin.H{"environment": env.ToAPI()}
	if live != nil {
		resp["container"] = live
	}
	c.JSON(http.StatusOK, resp)
}

type resetEnvironmentRequest struct {
	PushBranch bool `json:"push_branch"`
}

func (h *TaskHandlers) httpResetTaskEnvironment(c *gin.Context) {
	taskID := c.Param("id")
	var body resetEnvironmentRequest
	// The body is optional (an empty POST is valid: defaults to push_branch=false),
	// but a malformed JSON payload should be rejected loudly rather than silently
	// proceeding with a destructive reset.
	if err := c.ShouldBindJSON(&body); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body: " + err.Error()})
		return
	}

	err := h.service.ResetTaskEnvironment(c.Request.Context(), taskID, service.ResetOptions{
		PushBranch: body.PushBranch,
	})
	switch {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{responseKeySuccess: true})
	case errors.Is(err, service.ErrNoEnvironment):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrSessionRunning):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		// Log the underlying error for operators; respond with a generic
		// message so internal details (paths, wrapped error chains, etc.)
		// aren't leaked to clients.
		h.logger.Error("reset task environment failed",
			zap.String("task_id", taskID),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reset task environment"})
	}
}
