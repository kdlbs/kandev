package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/service"
)

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

type resetEnvironmentRequest struct {
	PushBranch bool `json:"push_branch"`
}

func (h *TaskHandlers) httpResetTaskEnvironment(c *gin.Context) {
	taskID := c.Param("id")
	var body resetEnvironmentRequest
	// Body is optional; ignore bind errors so an empty POST works.
	_ = c.ShouldBindJSON(&body)

	err := h.service.ResetTaskEnvironment(c.Request.Context(), taskID, service.ResetOptions{
		PushBranch: body.PushBranch,
	})
	switch {
	case err == nil:
		c.JSON(http.StatusOK, gin.H{"success": true})
	case errors.Is(err, service.ErrNoEnvironment):
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, service.ErrSessionRunning):
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		h.logger.Error("reset task environment failed",
			zap.String("task_id", taskID),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}
