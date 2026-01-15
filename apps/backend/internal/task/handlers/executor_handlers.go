package handlers

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/controller"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type ExecutorHandlers struct {
	controller *controller.ExecutorController
	logger     *logger.Logger
}

func NewExecutorHandlers(ctrl *controller.ExecutorController, log *logger.Logger) *ExecutorHandlers {
	return &ExecutorHandlers{controller: ctrl, logger: log}
}

func RegisterExecutorRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, ctrl *controller.ExecutorController, log *logger.Logger) {
	handlers := NewExecutorHandlers(ctrl, log)
	handlers.registerHTTP(router)
	handlers.registerWS(dispatcher)
}

func (h *ExecutorHandlers) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.GET("/executors", h.httpListExecutors)
	api.POST("/executors", h.httpCreateExecutor)
	api.GET("/executors/:id", h.httpGetExecutor)
	api.PATCH("/executors/:id", h.httpUpdateExecutor)
	api.DELETE("/executors/:id", h.httpDeleteExecutor)
}

func (h *ExecutorHandlers) registerWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc(ws.ActionExecutorList, h.wsListExecutors)
	dispatcher.RegisterFunc(ws.ActionExecutorCreate, h.wsCreateExecutor)
	dispatcher.RegisterFunc(ws.ActionExecutorGet, h.wsGetExecutor)
	dispatcher.RegisterFunc(ws.ActionExecutorUpdate, h.wsUpdateExecutor)
	dispatcher.RegisterFunc(ws.ActionExecutorDelete, h.wsDeleteExecutor)
}

func (h *ExecutorHandlers) httpListExecutors(c *gin.Context) {
	resp, err := h.controller.ListExecutors(c.Request.Context(), dto.ListExecutorsRequest{})
	if err != nil {
		h.logger.Error("failed to list executors", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list executors"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

type httpCreateExecutorRequest struct {
	Name     string            `json:"name"`
	Type     string            `json:"type"`
	Status   string            `json:"status"`
	IsSystem bool              `json:"is_system"`
	Config   map[string]string `json:"config,omitempty"`
}

func (h *ExecutorHandlers) httpCreateExecutor(c *gin.Context) {
	var body httpCreateExecutorRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if body.Name == "" || body.Type == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and type are required"})
		return
	}
	status := body.Status
	if status == "" {
		status = string(models.ExecutorStatusActive)
	}
	resp, err := h.controller.CreateExecutor(c.Request.Context(), dto.CreateExecutorRequest{
		Name:     body.Name,
		Type:     models.ExecutorType(body.Type),
		Status:   models.ExecutorStatus(status),
		IsSystem: body.IsSystem,
		Config:   body.Config,
	})
	if err != nil {
		h.logger.Error("failed to create executor", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "executor not created"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *ExecutorHandlers) httpGetExecutor(c *gin.Context) {
	resp, err := h.controller.GetExecutor(c.Request.Context(), dto.GetExecutorRequest{ID: c.Param("id")})
	if err != nil {
		h.logger.Error("failed to get executor", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "executor not found"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

type httpUpdateExecutorRequest struct {
	Name   *string           `json:"name,omitempty"`
	Type   *string           `json:"type,omitempty"`
	Status *string           `json:"status,omitempty"`
	Config map[string]string `json:"config,omitempty"`
}

func (h *ExecutorHandlers) httpUpdateExecutor(c *gin.Context) {
	var body httpUpdateExecutorRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	var execType *models.ExecutorType
	if body.Type != nil {
		value := models.ExecutorType(*body.Type)
		execType = &value
	}
	var execStatus *models.ExecutorStatus
	if body.Status != nil {
		value := models.ExecutorStatus(*body.Status)
		execStatus = &value
	}
	resp, err := h.controller.UpdateExecutor(c.Request.Context(), dto.UpdateExecutorRequest{
		ID:     c.Param("id"),
		Name:   body.Name,
		Type:   execType,
		Status: execStatus,
		Config: body.Config,
	})
	if err != nil {
		h.logger.Error("failed to update executor", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "executor not updated"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *ExecutorHandlers) httpDeleteExecutor(c *gin.Context) {
	resp, err := h.controller.DeleteExecutor(c.Request.Context(), dto.DeleteExecutorRequest{ID: c.Param("id")})
	if err != nil {
		if errors.Is(err, controller.ErrActiveTaskSessions) {
			c.JSON(http.StatusConflict, gin.H{"error": "executor is used by an active agent session"})
			return
		}
		h.logger.Error("failed to delete executor", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "executor not deleted"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *ExecutorHandlers) wsListExecutors(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	resp, err := h.controller.ListExecutors(ctx, dto.ListExecutorsRequest{})
	if err != nil {
		h.logger.Error("failed to list executors", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list executors", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsCreateExecutorRequest struct {
	Name     string            `json:"name"`
	Type     string            `json:"type"`
	Status   string            `json:"status"`
	IsSystem bool              `json:"is_system"`
	Config   map[string]string `json:"config,omitempty"`
}

func (h *ExecutorHandlers) wsCreateExecutor(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsCreateExecutorRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.Name == "" || req.Type == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "name and type are required", nil)
	}
	status := req.Status
	if status == "" {
		status = string(models.ExecutorStatusActive)
	}
	resp, err := h.controller.CreateExecutor(ctx, dto.CreateExecutorRequest{
		Name:     req.Name,
		Type:     models.ExecutorType(req.Type),
		Status:   models.ExecutorStatus(status),
		IsSystem: req.IsSystem,
		Config:   req.Config,
	})
	if err != nil {
		h.logger.Error("failed to create executor", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create executor", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsGetExecutorRequest struct {
	ID string `json:"id"`
}

func (h *ExecutorHandlers) wsGetExecutor(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsGetExecutorRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	resp, err := h.controller.GetExecutor(ctx, dto.GetExecutorRequest{ID: req.ID})
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Executor not found", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsUpdateExecutorRequest struct {
	ID     string            `json:"id"`
	Name   *string           `json:"name,omitempty"`
	Type   *string           `json:"type,omitempty"`
	Status *string           `json:"status,omitempty"`
	Config map[string]string `json:"config,omitempty"`
}

func (h *ExecutorHandlers) wsUpdateExecutor(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsUpdateExecutorRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	var execType *models.ExecutorType
	if req.Type != nil {
		value := models.ExecutorType(*req.Type)
		execType = &value
	}
	var execStatus *models.ExecutorStatus
	if req.Status != nil {
		value := models.ExecutorStatus(*req.Status)
		execStatus = &value
	}
	resp, err := h.controller.UpdateExecutor(ctx, dto.UpdateExecutorRequest{
		ID:     req.ID,
		Name:   req.Name,
		Type:   execType,
		Status: execStatus,
		Config: req.Config,
	})
	if err != nil {
		h.logger.Error("failed to update executor", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update executor", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsDeleteExecutorRequest struct {
	ID string `json:"id"`
}

func (h *ExecutorHandlers) wsDeleteExecutor(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsDeleteExecutorRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	resp, err := h.controller.DeleteExecutor(ctx, dto.DeleteExecutorRequest{ID: req.ID})
	if err != nil {
		h.logger.Error("failed to delete executor", zap.Error(err))
		if errors.Is(err, controller.ErrActiveTaskSessions) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "executor is used by an active agent session", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to delete executor", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}
