package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
)

type conversationTaskDTO struct {
	dto.TaskDTO
	ConversationMatch models.ConversationMatch `json:"conversation_match"`
}

func (h *TaskHandlers) httpSearchTaskConversations(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	limit, _ := strconv.Atoi(c.Query("limit"))
	if limit <= 0 {
		limit = 20
	}
	tasks, page, err := h.service.SearchTaskConversations(c.Request.Context(), c.Param("id"), c.Query("query"), limit)
	if err != nil {
		handleNotFound(c, h.logger, err, "tasks not found")
		return
	}
	taskDTOs, err := h.toTaskDTOsWithSessionInfo(c.Request.Context(), tasks)
	if err != nil {
		handleNotFound(c, h.logger, err, "tasks not found")
		return
	}
	byID := make(map[string]dto.TaskDTO, len(taskDTOs))
	for _, task := range taskDTOs {
		byID[task.ID] = task
	}
	results := make([]conversationTaskDTO, 0, len(taskDTOs))
	for _, hit := range page.Hits {
		if task, ok := byID[hit.TaskID]; ok {
			results = append(results, conversationTaskDTO{task, hit.Match})
		}
	}
	c.JSON(http.StatusOK, conversationSearchResponse{Tasks: results, State: page.State, HasMore: page.HasMore})
}

type conversationSearchResponse struct {
	Tasks   []conversationTaskDTO `json:"tasks"`
	State   string                `json:"state"`
	HasMore bool                  `json:"has_more"`
}
