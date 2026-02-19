package handlers

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type ExecutorHandlers struct {
	service *service.Service
	logger  *logger.Logger
}

func NewExecutorHandlers(svc *service.Service, log *logger.Logger) *ExecutorHandlers {
	return &ExecutorHandlers{service: svc, logger: log}
}

func RegisterExecutorRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, svc *service.Service, log *logger.Logger) {
	handlers := NewExecutorHandlers(svc, log)
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

func (h *ExecutorHandlers) listExecutors(ctx context.Context) (dto.ListExecutorsResponse, error) {
	executors, err := h.service.ListExecutors(ctx)
	if err != nil {
		return dto.ListExecutorsResponse{}, err
	}
	resp := dto.ListExecutorsResponse{
		Executors: make([]dto.ExecutorDTO, 0, len(executors)),
		Total:     len(executors),
	}
	for _, executor := range executors {
		resp.Executors = append(resp.Executors, dto.FromExecutor(executor))
	}
	return resp, nil
}

func (h *ExecutorHandlers) httpListExecutors(c *gin.Context) {
	resp, err := h.listExecutors(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to list executors", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list executors"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

type httpCreateExecutorRequest struct {
	Name      string            `json:"name"`
	Type      string            `json:"type"`
	Status    string            `json:"status"`
	IsSystem  bool              `json:"is_system"`
	Resumable *bool             `json:"resumable,omitempty"`
	Config    map[string]string `json:"config,omitempty"`
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
	resumable := true
	if body.Resumable != nil {
		resumable = *body.Resumable
	}
	executor, err := h.service.CreateExecutor(c.Request.Context(), &service.CreateExecutorRequest{
		Name:      body.Name,
		Type:      models.ExecutorType(body.Type),
		Status:    models.ExecutorStatus(status),
		IsSystem:  body.IsSystem,
		Resumable: resumable,
		Config:    body.Config,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidExecutorConfig) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		h.logger.Error("failed to create executor", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "executor not created"})
		return
	}
	c.JSON(http.StatusOK, dto.FromExecutor(executor))
}

func (h *ExecutorHandlers) httpGetExecutor(c *gin.Context) {
	executor, err := h.service.GetExecutor(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.logger.Error("failed to get executor", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "executor not found"})
		return
	}
	c.JSON(http.StatusOK, dto.FromExecutor(executor))
}

type httpUpdateExecutorRequest struct {
	Name      *string           `json:"name,omitempty"`
	Type      *string           `json:"type,omitempty"`
	Status    *string           `json:"status,omitempty"`
	Resumable *bool             `json:"resumable,omitempty"`
	Config    map[string]string `json:"config,omitempty"`
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
	executor, err := h.service.UpdateExecutor(c.Request.Context(), c.Param("id"), &service.UpdateExecutorRequest{
		Name:      body.Name,
		Type:      execType,
		Status:    execStatus,
		Resumable: body.Resumable,
		Config:    body.Config,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidExecutorConfig) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		h.logger.Error("failed to update executor", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "executor not updated"})
		return
	}
	c.JSON(http.StatusOK, dto.FromExecutor(executor))
}

func (h *ExecutorHandlers) httpDeleteExecutor(c *gin.Context) {
	if err := h.service.DeleteExecutor(c.Request.Context(), c.Param("id")); err != nil {
		if errors.Is(err, service.ErrActiveTaskSessions) {
			c.JSON(http.StatusConflict, gin.H{"error": "executor is used by an active agent session"})
			return
		}
		h.logger.Error("failed to delete executor", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "executor not deleted"})
		return
	}
	c.JSON(http.StatusOK, dto.SuccessResponse{Success: true})
}

func (h *ExecutorHandlers) wsListExecutors(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	resp, err := h.listExecutors(ctx)
	if err != nil {
		h.logger.Error("failed to list executors", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list executors", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsCreateExecutorRequest struct {
	Name      string            `json:"name"`
	Type      string            `json:"type"`
	Status    string            `json:"status"`
	IsSystem  bool              `json:"is_system"`
	Resumable *bool             `json:"resumable,omitempty"`
	Config    map[string]string `json:"config,omitempty"`
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
	resumable := true
	if req.Resumable != nil {
		resumable = *req.Resumable
	}
	executor, err := h.service.CreateExecutor(ctx, &service.CreateExecutorRequest{
		Name:      req.Name,
		Type:      models.ExecutorType(req.Type),
		Status:    models.ExecutorStatus(status),
		IsSystem:  req.IsSystem,
		Resumable: resumable,
		Config:    req.Config,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidExecutorConfig) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
		}
		h.logger.Error("failed to create executor", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create executor", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromExecutor(executor))
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
	executor, err := h.service.GetExecutor(ctx, req.ID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Executor not found", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromExecutor(executor))
}

type wsUpdateExecutorRequest struct {
	ID        string            `json:"id"`
	Name      *string           `json:"name,omitempty"`
	Type      *string           `json:"type,omitempty"`
	Status    *string           `json:"status,omitempty"`
	Resumable *bool             `json:"resumable,omitempty"`
	Config    map[string]string `json:"config,omitempty"`
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
	executor, err := h.service.UpdateExecutor(ctx, req.ID, &service.UpdateExecutorRequest{
		Name:      req.Name,
		Type:      execType,
		Status:    execStatus,
		Resumable: req.Resumable,
		Config:    req.Config,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidExecutorConfig) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
		}
		h.logger.Error("failed to update executor", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update executor", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromExecutor(executor))
}

func (h *ExecutorHandlers) wsDeleteExecutor(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return wsDeleteWithActiveSessionCheck(ctx, msg, h.logger, "executor", h.service.DeleteExecutor)
}
