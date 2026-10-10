package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
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

func (h *TaskHandlers) httpGetTaskSessionUsageTurns(c *gin.Context) {
	taskID := c.Param("id")
	sessionID := c.Param("sessionId")
	var cursor int64
	if raw := c.Query("cursor"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid usage cursor"})
			return
		}
		cursor = parsed
	}
	limit := 50
	if raw := c.Query("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "usage limit must be between 1 and 100"})
			return
		}
		limit = parsed
	}
	turns, nextCursor, err := h.service.ListTaskSessionUsageTurns(c.Request.Context(), taskID, sessionID, cursor, limit)
	if err != nil {
		handleNotFound(c, h.logger, err, "task session not found")
		return
	}
	page := dto.UsageTurnsPageDTO{SessionID: sessionID, Turns: make([]dto.UsageTurnDTO, 0, len(turns))}
	for _, turn := range turns {
		page.Turns = append(page.Turns, dto.ToUsageTurnDTO(turn, false))
	}
	if nextCursor != cursor {
		page.NextCursor = strconv.FormatInt(nextCursor, 10)
	}
	c.JSON(http.StatusOK, page)
}

func (h *TaskHandlers) httpGetTaskSessionUsageTurn(c *gin.Context) {
	taskID := c.Param("id")
	sessionID := c.Param("sessionId")
	turnID := c.Param("turnId")
	events, err := h.service.GetTaskSessionUsageTurn(c.Request.Context(), taskID, sessionID, turnID)
	if err != nil {
		handleNotFound(c, h.logger, err, "task session not found")
		return
	}
	if len(events) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "usage turn not found"})
		return
	}
	turn := dto.ToUsageTurnDTO(models.TaskUsageTurnEvents{TurnID: turnID, Events: events}, true)
	c.JSON(http.StatusOK, turn)
}
