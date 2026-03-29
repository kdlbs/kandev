package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
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
