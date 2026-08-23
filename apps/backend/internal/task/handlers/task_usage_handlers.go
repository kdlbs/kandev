package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/task/dto"
)

// httpGetTaskUsageTotals serves GET /api/v1/tasks/:id/usage
// (docs/specs/task-cost-ledger/spec.md AC-18, AC-19, AC-20): the task-scoped
// token/cost aggregate, including rows whose session_id was cleared by
// session deletion. A known task with no usage returns HTTP 200 with zeroed
// totals (AC-20), never an error.
func (h *TaskHandlers) httpGetTaskUsageTotals(c *gin.Context) {
	taskID := c.Param("id")
	totals, err := h.service.GetTaskUsageTotals(c.Request.Context(), taskID)
	if err != nil {
		handleNotFound(c, h.logger, err, "task not found")
		return
	}
	c.JSON(http.StatusOK, dto.ToTaskUsageTotalsDTO(dto.TaskUsageTotalsScopeTask, taskID, totals))
}

// httpGetTaskSessionUsageTotals serves
// GET /api/v1/tasks/:id/sessions/:sessionId/usage (AC-18, AC-19, AC-20): the
// session-scoped token/cost aggregate. Returns 404 when the task or session
// does not exist, or when the session exists but does not belong to the
// task in the path.
func (h *TaskHandlers) httpGetTaskSessionUsageTotals(c *gin.Context) {
	taskID := c.Param("id")
	sessionID := c.Param("sessionId")
	totals, err := h.service.GetTaskSessionUsageTotals(c.Request.Context(), taskID, sessionID)
	if err != nil {
		handleNotFound(c, h.logger, err, "task session not found")
		return
	}
	c.JSON(http.StatusOK, dto.ToTaskUsageTotalsDTO(dto.TaskUsageTotalsScopeSession, sessionID, totals))
}
