package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/editors/controller"
	"github.com/kandev/kandev/internal/editors/dto"
)

type Handlers struct {
	controller *controller.Controller
	logger     *logger.Logger
}

func NewHandlers(ctrl *controller.Controller, log *logger.Logger) *Handlers {
	return &Handlers{
		controller: ctrl,
		logger:     log.WithFields(zap.String("component", "editors-handlers")),
	}
}

func RegisterRoutes(router *gin.Engine, ctrl *controller.Controller, log *logger.Logger) {
	handlers := NewHandlers(ctrl, log)
	api := router.Group("/api/v1")
	api.GET("/editors", handlers.httpListEditors)
	api.POST("/editors", handlers.httpCreateEditor)
	api.PATCH("/editors/:id", handlers.httpUpdateEditor)
	api.DELETE("/editors/:id", handlers.httpDeleteEditor)
	api.POST("/task-sessions/:id/open-editor", handlers.httpOpenSessionEditor)
}

func (h *Handlers) httpListEditors(c *gin.Context) {
	resp, err := h.controller.ListEditors(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to list editors", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list editors"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpOpenSessionEditor(c *gin.Context) {
	var req dto.OpenEditorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	resp, err := h.controller.OpenSessionEditor(c.Request.Context(), c.Param("id"), req)
	if err != nil {
		h.logger.Error("failed to open editor", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to open editor"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpCreateEditor(c *gin.Context) {
	var req dto.CreateEditorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	resp, err := h.controller.CreateEditor(c.Request.Context(), req)
	if err != nil {
		h.logger.Error("failed to create editor", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create editor"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpUpdateEditor(c *gin.Context) {
	var req dto.UpdateEditorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	resp, err := h.controller.UpdateEditor(c.Request.Context(), c.Param("id"), req)
	if err != nil {
		h.logger.Error("failed to update editor", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update editor"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) httpDeleteEditor(c *gin.Context) {
	if err := h.controller.DeleteEditor(c.Request.Context(), c.Param("id")); err != nil {
		h.logger.Error("failed to delete editor", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete editor"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
