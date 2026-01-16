package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/notifications/controller"
	"github.com/kandev/kandev/internal/notifications/dto"
	"go.uber.org/zap"
)

type Handlers struct {
	controller *controller.Controller
	logger     *logger.Logger
}

func RegisterRoutes(router *gin.Engine, ctrl *controller.Controller, log *logger.Logger) {
	h := &Handlers{
		controller: ctrl,
		logger:     log.WithFields(zap.String("component", "notification-handlers")),
	}
	api := router.Group("/api/v1")
	api.GET("/notification-providers", h.httpListProviders)
	api.POST("/notification-providers", h.httpCreateProvider)
	api.PATCH("/notification-providers/:id", h.httpUpdateProvider)
	api.DELETE("/notification-providers/:id", h.httpDeleteProvider)
}

func (h *Handlers) httpListProviders(c *gin.Context) {
	resp, err := h.controller.ListProviders(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to list notification providers", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list notification providers"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpCreateProvider(c *gin.Context) {
	var body dto.UpsertProviderRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	resp, err := h.controller.CreateProvider(c.Request.Context(), body)
	if err != nil {
		h.logger.Error("failed to create notification provider", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpUpdateProvider(c *gin.Context) {
	providerID := c.Param("id")
	var body dto.UpdateProviderRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	resp, err := h.controller.UpdateProvider(c.Request.Context(), providerID, body)
	if err != nil {
		h.logger.Error("failed to update notification provider", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpDeleteProvider(c *gin.Context) {
	providerID := c.Param("id")
	if err := h.controller.DeleteProvider(c.Request.Context(), providerID); err != nil {
		h.logger.Error("failed to delete notification provider", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete notification provider"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
