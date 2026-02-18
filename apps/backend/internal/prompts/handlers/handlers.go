package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/prompts/controller"
	"github.com/kandev/kandev/internal/prompts/dto"
	"github.com/kandev/kandev/internal/prompts/service"
)

type Handlers struct {
	controller *controller.Controller
	logger     *logger.Logger
}

func NewHandlers(ctrl *controller.Controller, log *logger.Logger) *Handlers {
	return &Handlers{
		controller: ctrl,
		logger:     log.WithFields(zap.String("component", "prompts-handlers")),
	}
}

func RegisterRoutes(router *gin.Engine, ctrl *controller.Controller, log *logger.Logger) {
	handlers := NewHandlers(ctrl, log)
	api := router.Group("/api/v1")
	api.GET("/prompts", handlers.httpListPrompts)
	api.POST("/prompts", handlers.httpCreatePrompt)
	api.PATCH("/prompts/:id", handlers.httpUpdatePrompt)
	api.DELETE("/prompts/:id", handlers.httpDeletePrompt)
}

func (h *Handlers) httpListPrompts(c *gin.Context) {
	resp, err := h.controller.ListPrompts(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to list prompts", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list prompts"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpCreatePrompt(c *gin.Context) {
	var req dto.CreatePromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	resp, err := h.controller.CreatePrompt(c.Request.Context(), req)
	if err != nil {
		status := http.StatusInternalServerError
		if err == service.ErrInvalidPrompt {
			status = http.StatusBadRequest
		}
		h.logger.Error("failed to create prompt", zap.Error(err))
		c.JSON(status, gin.H{"error": "failed to create prompt"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpUpdatePrompt(c *gin.Context) {
	var req dto.UpdatePromptRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	resp, err := h.controller.UpdatePrompt(c.Request.Context(), c.Param("id"), req)
	if err != nil {
		status := http.StatusInternalServerError
		switch err {
		case service.ErrInvalidPrompt:
			status = http.StatusBadRequest
		case service.ErrPromptNotFound:
			status = http.StatusNotFound
		}
		h.logger.Error("failed to update prompt", zap.Error(err))
		c.JSON(status, gin.H{"error": "failed to update prompt"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpDeletePrompt(c *gin.Context) {
	if err := h.controller.DeletePrompt(c.Request.Context(), c.Param("id")); err != nil {
		h.logger.Error("failed to delete prompt", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete prompt"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
