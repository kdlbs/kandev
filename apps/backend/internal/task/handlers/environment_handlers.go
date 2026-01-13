package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/controller"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type EnvironmentHandlers struct {
	controller *controller.EnvironmentController
	logger     *logger.Logger
}

func NewEnvironmentHandlers(ctrl *controller.EnvironmentController, log *logger.Logger) *EnvironmentHandlers {
	return &EnvironmentHandlers{controller: ctrl, logger: log}
}

func RegisterEnvironmentRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, ctrl *controller.EnvironmentController, log *logger.Logger) {
	handlers := NewEnvironmentHandlers(ctrl, log)
	handlers.registerHTTP(router)
	handlers.registerWS(dispatcher)
}

func (h *EnvironmentHandlers) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.GET("/environments", h.httpListEnvironments)
	api.POST("/environments", h.httpCreateEnvironment)
	api.GET("/environments/:id", h.httpGetEnvironment)
	api.PATCH("/environments/:id", h.httpUpdateEnvironment)
	api.DELETE("/environments/:id", h.httpDeleteEnvironment)
}

func (h *EnvironmentHandlers) registerWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc(ws.ActionEnvironmentList, h.wsListEnvironments)
	dispatcher.RegisterFunc(ws.ActionEnvironmentCreate, h.wsCreateEnvironment)
	dispatcher.RegisterFunc(ws.ActionEnvironmentGet, h.wsGetEnvironment)
	dispatcher.RegisterFunc(ws.ActionEnvironmentUpdate, h.wsUpdateEnvironment)
	dispatcher.RegisterFunc(ws.ActionEnvironmentDelete, h.wsDeleteEnvironment)
}

func (h *EnvironmentHandlers) httpListEnvironments(c *gin.Context) {
	resp, err := h.controller.ListEnvironments(c.Request.Context(), dto.ListEnvironmentsRequest{})
	if err != nil {
		h.logger.Error("failed to list environments", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list environments"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

type httpCreateEnvironmentRequest struct {
	Name         string            `json:"name"`
	Kind         string            `json:"kind"`
	WorktreeRoot string            `json:"worktree_root,omitempty"`
	ImageTag     string            `json:"image_tag,omitempty"`
	Dockerfile   string            `json:"dockerfile,omitempty"`
	BuildConfig  map[string]string `json:"build_config,omitempty"`
}

func (h *EnvironmentHandlers) httpCreateEnvironment(c *gin.Context) {
	var body httpCreateEnvironmentRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if body.Name == "" || body.Kind == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and kind are required"})
		return
	}
	resp, err := h.controller.CreateEnvironment(c.Request.Context(), dto.CreateEnvironmentRequest{
		Name:         body.Name,
		Kind:         models.EnvironmentKind(body.Kind),
		WorktreeRoot: body.WorktreeRoot,
		ImageTag:     body.ImageTag,
		Dockerfile:   body.Dockerfile,
		BuildConfig:  body.BuildConfig,
	})
	if err != nil {
		h.logger.Error("failed to create environment", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "environment not created"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *EnvironmentHandlers) httpGetEnvironment(c *gin.Context) {
	resp, err := h.controller.GetEnvironment(c.Request.Context(), dto.GetEnvironmentRequest{ID: c.Param("id")})
	if err != nil {
		h.logger.Error("failed to get environment", zap.Error(err))
		c.JSON(http.StatusNotFound, gin.H{"error": "environment not found"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

type httpUpdateEnvironmentRequest struct {
	Name         *string           `json:"name,omitempty"`
	Kind         *string           `json:"kind,omitempty"`
	WorktreeRoot *string           `json:"worktree_root,omitempty"`
	ImageTag     *string           `json:"image_tag,omitempty"`
	Dockerfile   *string           `json:"dockerfile,omitempty"`
	BuildConfig  map[string]string `json:"build_config,omitempty"`
}

func (h *EnvironmentHandlers) httpUpdateEnvironment(c *gin.Context) {
	var body httpUpdateEnvironmentRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	var kind *models.EnvironmentKind
	if body.Kind != nil {
		value := models.EnvironmentKind(*body.Kind)
		kind = &value
	}
	resp, err := h.controller.UpdateEnvironment(c.Request.Context(), dto.UpdateEnvironmentRequest{
		ID:           c.Param("id"),
		Name:         body.Name,
		Kind:         kind,
		WorktreeRoot: body.WorktreeRoot,
		ImageTag:     body.ImageTag,
		Dockerfile:   body.Dockerfile,
		BuildConfig:  body.BuildConfig,
	})
	if err != nil {
		h.logger.Error("failed to update environment", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "environment not updated"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *EnvironmentHandlers) httpDeleteEnvironment(c *gin.Context) {
	resp, err := h.controller.DeleteEnvironment(c.Request.Context(), dto.DeleteEnvironmentRequest{ID: c.Param("id")})
	if err != nil {
		h.logger.Error("failed to delete environment", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "environment not deleted"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *EnvironmentHandlers) wsListEnvironments(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	resp, err := h.controller.ListEnvironments(ctx, dto.ListEnvironmentsRequest{})
	if err != nil {
		h.logger.Error("failed to list environments", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list environments", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsCreateEnvironmentRequest struct {
	Name         string            `json:"name"`
	Kind         string            `json:"kind"`
	WorktreeRoot string            `json:"worktree_root,omitempty"`
	ImageTag     string            `json:"image_tag,omitempty"`
	Dockerfile   string            `json:"dockerfile,omitempty"`
	BuildConfig  map[string]string `json:"build_config,omitempty"`
}

func (h *EnvironmentHandlers) wsCreateEnvironment(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsCreateEnvironmentRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.Name == "" || req.Kind == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "name and kind are required", nil)
	}
	resp, err := h.controller.CreateEnvironment(ctx, dto.CreateEnvironmentRequest{
		Name:         req.Name,
		Kind:         models.EnvironmentKind(req.Kind),
		WorktreeRoot: req.WorktreeRoot,
		ImageTag:     req.ImageTag,
		Dockerfile:   req.Dockerfile,
		BuildConfig:  req.BuildConfig,
	})
	if err != nil {
		h.logger.Error("failed to create environment", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create environment", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsGetEnvironmentRequest struct {
	ID string `json:"id"`
}

func (h *EnvironmentHandlers) wsGetEnvironment(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsGetEnvironmentRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	resp, err := h.controller.GetEnvironment(ctx, dto.GetEnvironmentRequest{ID: req.ID})
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Environment not found", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsUpdateEnvironmentRequest struct {
	ID           string            `json:"id"`
	Name         *string           `json:"name,omitempty"`
	Kind         *string           `json:"kind,omitempty"`
	WorktreeRoot *string           `json:"worktree_root,omitempty"`
	ImageTag     *string           `json:"image_tag,omitempty"`
	Dockerfile   *string           `json:"dockerfile,omitempty"`
	BuildConfig  map[string]string `json:"build_config,omitempty"`
}

func (h *EnvironmentHandlers) wsUpdateEnvironment(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsUpdateEnvironmentRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	var kind *models.EnvironmentKind
	if req.Kind != nil {
		value := models.EnvironmentKind(*req.Kind)
		kind = &value
	}
	resp, err := h.controller.UpdateEnvironment(ctx, dto.UpdateEnvironmentRequest{
		ID:           req.ID,
		Name:         req.Name,
		Kind:         kind,
		WorktreeRoot: req.WorktreeRoot,
		ImageTag:     req.ImageTag,
		Dockerfile:   req.Dockerfile,
		BuildConfig:  req.BuildConfig,
	})
	if err != nil {
		h.logger.Error("failed to update environment", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update environment", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsDeleteEnvironmentRequest struct {
	ID string `json:"id"`
}

func (h *EnvironmentHandlers) wsDeleteEnvironment(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsDeleteEnvironmentRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	resp, err := h.controller.DeleteEnvironment(ctx, dto.DeleteEnvironmentRequest{ID: req.ID})
	if err != nil {
		h.logger.Error("failed to delete environment", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to delete environment", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}
