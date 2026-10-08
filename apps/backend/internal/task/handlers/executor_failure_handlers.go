package handlers

import (
	"github.com/gin-gonic/gin"
	"net/http"
)

type executorFailureRecheckRequest struct {
	EpisodeID string `json:"episode_id" binding:"required"`
	Revision  int64  `json:"revision" binding:"required,min=1"`
}

func (h *TaskHandlers) httpRecheckExecutorFailure(c *gin.Context) {
	if err := h.service.AuthorizeTaskAccess(c.Request.Context(), c.Param("id")); err != nil {
		handleNotFound(c, h.logger, err, "task not found")
		return
	}
	var request executorFailureRecheckRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid executor recheck request"})
		return
	}
	episode, err := h.service.RecheckExecutorFailure(c.Request.Context(), c.Param("id"), request.EpisodeID, request.Revision)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "executor_status_unverified"})
		return
	}
	c.JSON(http.StatusOK, episode)
}
