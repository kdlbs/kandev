package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/orchestrate/dto"
)

func (h *Handlers) searchTasks(c *gin.Context) {
	wsID := c.Param("wsId")
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusOK, dto.TaskSearchResponse{Tasks: []*dto.TaskSearchResultDTO{}})
		return
	}

	limit := 50
	if v := c.Query("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	results, err := h.ctrl.Svc.SearchTasks(c.Request.Context(), wsID, query, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	tasks := make([]*dto.TaskSearchResultDTO, len(results))
	for i, r := range results {
		tasks[i] = &dto.TaskSearchResultDTO{
			ID:                      r.ID,
			WorkspaceID:             r.WorkspaceID,
			Identifier:              r.Identifier,
			Title:                   r.Title,
			Description:             r.Description,
			Status:                  r.Status,
			Priority:                r.Priority,
			ParentID:                r.ParentID,
			ProjectID:               r.ProjectID,
			AssigneeAgentInstanceID: r.AssigneeAgentInstanceID,
			Labels:                  r.Labels,
			CreatedAt:               r.CreatedAt,
			UpdatedAt:               r.UpdatedAt,
		}
	}
	c.JSON(http.StatusOK, dto.TaskSearchResponse{Tasks: tasks})
}
