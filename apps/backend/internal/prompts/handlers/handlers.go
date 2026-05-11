package handlers

import (
	"errors"
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
		status, message := http.StatusInternalServerError, "failed to create prompt"
		switch {
		case errors.Is(err, service.ErrInvalidPrompt):
			status, message = http.StatusBadRequest, err.Error()
		case errors.Is(err, service.ErrPromptAlreadyExists):
			status, message = http.StatusConflict, err.Error()
		}
		logRejection(h.logger, "create prompt rejected", "failed to create prompt", err, status)
		c.JSON(status, gin.H{"error": message})
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
		status, message := http.StatusInternalServerError, "failed to update prompt"
		switch {
		case errors.Is(err, service.ErrInvalidPrompt):
			status, message = http.StatusBadRequest, err.Error()
		case errors.Is(err, service.ErrPromptNotFound):
			status, message = http.StatusNotFound, err.Error()
		case errors.Is(err, service.ErrPromptAlreadyExists):
			status, message = http.StatusConflict, err.Error()
		}
		logRejection(h.logger, "update prompt rejected", "failed to update prompt", err, status)
		c.JSON(status, gin.H{"error": message})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// logRejection keeps the error-rate dashboard clean by logging client-driven
// 4xx outcomes at Info level and reserving Error for unexpected 500s.
func logRejection(log *logger.Logger, infoMsg, errorMsg string, err error, status int) {
	if status >= http.StatusInternalServerError {
		log.Error(errorMsg, zap.Error(err))
		return
	}
	log.Info(infoMsg, zap.Error(err), zap.Int("status", status))
}

func (h *Handlers) httpDeletePrompt(c *gin.Context) {
	if err := h.controller.DeletePrompt(c.Request.Context(), c.Param("id")); err != nil {
		h.logger.Error("failed to delete prompt", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete prompt"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
