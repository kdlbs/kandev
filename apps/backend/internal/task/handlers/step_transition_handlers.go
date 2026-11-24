package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

// stepTransitionResponse is one recorded task move as returned by
// GET /api/v1/tasks/:id/step-transitions.
type stepTransitionResponse struct {
	ID                   string  `json:"id"`
	FromWorkflowID       *string `json:"from_workflow_id"`
	FromWorkflowStepID   *string `json:"from_workflow_step_id"`
	FromWorkflowStepName *string `json:"from_workflow_step_name"`
	ToWorkflowID         *string `json:"to_workflow_id"`
	ToWorkflowStepID     *string `json:"to_workflow_step_id"`
	ToWorkflowStepName   *string `json:"to_workflow_step_name"`
	Trigger              string  `json:"trigger"`
	ActorKind            string  `json:"actor_kind"`
	ActorID              *string `json:"actor_id"`
	OccurredAt           string  `json:"occurred_at"`
}

type stepTransitionPageResponse struct {
	Items      []stepTransitionResponse `json:"items"`
	NextCursor string                   `json:"next_cursor"`
	HasMore    bool                     `json:"has_more"`
}

func (h *TaskHandlers) httpListTaskStepTransitions(c *gin.Context) {
	limit := 0
	if raw := c.Query("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid limit"})
			return
		}
		limit = parsed
	}
	items, next, err := h.service.ListTaskStepTransitionRecords(c.Request.Context(), c.Param("id"), limit, c.Query("cursor"))
	if errors.Is(err, service.ErrInvalidTransitionCursor) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid cursor"})
		return
	}
	if err != nil {
		handleNotFound(c, h.logger, err, "task not found")
		return
	}
	result := make([]stepTransitionResponse, len(items))
	for i, item := range items {
		result[i] = toStepTransitionResponse(item)
	}
	c.JSON(http.StatusOK, stepTransitionPageResponse{Items: result, NextCursor: next, HasMore: next != ""})
}

func toStepTransitionResponse(item models.StepTransitionRecord) stepTransitionResponse {
	return stepTransitionResponse{
		ID:                   strconv.FormatInt(item.ID, 10),
		FromWorkflowID:       item.FromWorkflowID,
		FromWorkflowStepID:   item.FromWorkflowStepID,
		FromWorkflowStepName: item.FromWorkflowStepName,
		ToWorkflowID:         item.ToWorkflowID,
		ToWorkflowStepID:     item.ToWorkflowStepID,
		ToWorkflowStepName:   item.ToWorkflowStepName,
		Trigger:              item.Trigger,
		ActorKind:            item.ActorKind,
		ActorID:              item.ActorID,
		OccurredAt:           item.OccurredAt.UTC().Format(time.RFC3339Nano),
	}
}
