package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// httpGetTaskContext serves GET /api/v1/tasks/:id/context — the office
// task-handoffs context envelope used by both the task detail React
// panel and the prompt builder. Returns 404 when the task does not
// exist, 503 when the handoff service is not configured, and the
// composed v1.TaskContext otherwise.
//
// Auth: relies on the caller having access to the task's workspace.
// The phase 2 access rules guard cross-workspace document leakage on
// the document fetch path, so this endpoint can return parent /
// sibling references safely — the relations themselves are not
// sensitive and are needed by the UI to render labels.
func (h *TaskHandlers) httpGetTaskContext(c *gin.Context) {
	taskID := c.Param("id")
	if h.handoffSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "task context service is not configured",
		})
		return
	}
	ctx, err := h.handoffSvc.GetTaskContext(c.Request.Context(), taskID)
	if err != nil {
		h.logger.Warn("get task context", zap.String("task_id", taskID), zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ctx)
}
