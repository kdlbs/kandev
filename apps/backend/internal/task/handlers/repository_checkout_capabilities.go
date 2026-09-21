package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/task/service"
)

func (h *RepositoryHandlers) httpRepositoryCheckoutCapabilities(c *gin.Context) {
	var request struct {
		Repository        service.TaskRepositoryInput `json:"repository"`
		ExecutorProfileID string                      `json:"executor_profile_id"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid repository checkout capability request"})
		return
	}
	capabilities, err := h.service.GetRepositoryCheckoutCapabilities(c.Request.Context(), c.Param("id"), request.Repository, request.ExecutorProfileID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "repository checkout capabilities unavailable", "code": "checkout_capabilities_unavailable"})
		return
	}
	c.JSON(http.StatusOK, capabilities)
}
