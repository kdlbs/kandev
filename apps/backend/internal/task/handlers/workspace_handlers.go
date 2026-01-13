package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/controller"
	"github.com/kandev/kandev/internal/task/dto"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type WorkspaceHandlers struct {
	controller *controller.WorkspaceController
	logger     *logger.Logger
}

func NewWorkspaceHandlers(ctrl *controller.WorkspaceController, log *logger.Logger) *WorkspaceHandlers {
	return &WorkspaceHandlers{
		controller: ctrl,
		logger:     log.WithFields(zap.String("component", "task-workspace-handlers")),
	}
}

func RegisterWorkspaceRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, ctrl *controller.WorkspaceController, log *logger.Logger) {
	handlers := NewWorkspaceHandlers(ctrl, log)
	handlers.registerHTTP(router)
	handlers.registerWS(dispatcher)
}

func (h *WorkspaceHandlers) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.GET("/workspaces", h.httpListWorkspaces)
	api.POST("/workspaces", h.httpCreateWorkspace)
	api.GET("/workspaces/:id", h.httpGetWorkspace)
	api.PATCH("/workspaces/:id", h.httpUpdateWorkspace)
	api.DELETE("/workspaces/:id", h.httpDeleteWorkspace)
}

func (h *WorkspaceHandlers) registerWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc(ws.ActionWorkspaceList, h.wsListWorkspaces)
	dispatcher.RegisterFunc(ws.ActionWorkspaceCreate, h.wsCreateWorkspace)
	dispatcher.RegisterFunc(ws.ActionWorkspaceGet, h.wsGetWorkspace)
	dispatcher.RegisterFunc(ws.ActionWorkspaceUpdate, h.wsUpdateWorkspace)
	dispatcher.RegisterFunc(ws.ActionWorkspaceDelete, h.wsDeleteWorkspace)
}

// HTTP handlers

func (h *WorkspaceHandlers) httpListWorkspaces(c *gin.Context) {
	resp, err := h.controller.ListWorkspaces(c.Request.Context(), dto.ListWorkspacesRequest{})
	if err != nil {
		h.logger.Error("failed to list workspaces", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list workspaces"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

type httpCreateWorkspaceRequest struct {
	Name              string `json:"name"`
	Description       string `json:"description,omitempty"`
	OwnerID           string `json:"owner_id,omitempty"`
	DefaultExecutorID string `json:"default_executor_id,omitempty"`
}

func (h *WorkspaceHandlers) httpCreateWorkspace(c *gin.Context) {
	var body httpCreateWorkspaceRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if body.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	resp, err := h.controller.CreateWorkspace(c.Request.Context(), dto.CreateWorkspaceRequest{
		Name:              body.Name,
		Description:       body.Description,
		OwnerID:           body.OwnerID,
		DefaultExecutorID: body.DefaultExecutorID,
	})
	if err != nil {
		handleNotFound(c, h.logger, err, "workspace not created")
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *WorkspaceHandlers) httpGetWorkspace(c *gin.Context) {
	resp, err := h.controller.GetWorkspace(c.Request.Context(), dto.GetWorkspaceRequest{ID: c.Param("id")})
	if err != nil {
		handleNotFound(c, h.logger, err, "workspace not found")
		return
	}
	c.JSON(http.StatusOK, resp)
}

type httpUpdateWorkspaceRequest struct {
	Name              *string `json:"name,omitempty"`
	Description       *string `json:"description,omitempty"`
	DefaultExecutorID *string `json:"default_executor_id,omitempty"`
}

func (h *WorkspaceHandlers) httpUpdateWorkspace(c *gin.Context) {
	var body httpUpdateWorkspaceRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	resp, err := h.controller.UpdateWorkspace(c.Request.Context(), dto.UpdateWorkspaceRequest{
		ID:                c.Param("id"),
		Name:              body.Name,
		Description:       body.Description,
		DefaultExecutorID: body.DefaultExecutorID,
	})
	if err != nil {
		handleNotFound(c, h.logger, err, "workspace not updated")
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *WorkspaceHandlers) httpDeleteWorkspace(c *gin.Context) {
	resp, err := h.controller.DeleteWorkspace(c.Request.Context(), dto.DeleteWorkspaceRequest{ID: c.Param("id")})
	if err != nil {
		handleNotFound(c, h.logger, err, "workspace not deleted")
		return
	}
	c.JSON(http.StatusOK, resp)
}

// WS handlers

func (h *WorkspaceHandlers) wsListWorkspaces(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	resp, err := h.controller.ListWorkspaces(ctx, dto.ListWorkspacesRequest{})
	if err != nil {
		h.logger.Error("failed to list workspaces", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list workspaces", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsCreateWorkspaceRequest struct {
	Name              string `json:"name"`
	Description       string `json:"description,omitempty"`
	OwnerID           string `json:"owner_id,omitempty"`
	DefaultExecutorID string `json:"default_executor_id,omitempty"`
}

func (h *WorkspaceHandlers) wsCreateWorkspace(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsCreateWorkspaceRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.Name == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "name is required", nil)
	}

	resp, err := h.controller.CreateWorkspace(ctx, dto.CreateWorkspaceRequest{
		Name:              req.Name,
		Description:       req.Description,
		OwnerID:           req.OwnerID,
		DefaultExecutorID: req.DefaultExecutorID,
	})
	if err != nil {
		h.logger.Error("failed to create workspace", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create workspace", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsGetWorkspaceRequest struct {
	ID string `json:"id"`
}

func (h *WorkspaceHandlers) wsGetWorkspace(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsGetWorkspaceRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}

	resp, err := h.controller.GetWorkspace(ctx, dto.GetWorkspaceRequest{ID: req.ID})
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Workspace not found", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsUpdateWorkspaceRequest struct {
	ID                string  `json:"id"`
	Name              *string `json:"name,omitempty"`
	Description       *string `json:"description,omitempty"`
	DefaultExecutorID *string `json:"default_executor_id,omitempty"`
}

func (h *WorkspaceHandlers) wsUpdateWorkspace(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsUpdateWorkspaceRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}

	resp, err := h.controller.UpdateWorkspace(ctx, dto.UpdateWorkspaceRequest{
		ID:                req.ID,
		Name:              req.Name,
		Description:       req.Description,
		DefaultExecutorID: req.DefaultExecutorID,
	})
	if err != nil {
		h.logger.Error("failed to update workspace", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update workspace", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsDeleteWorkspaceRequest struct {
	ID string `json:"id"`
}

func (h *WorkspaceHandlers) wsDeleteWorkspace(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsDeleteWorkspaceRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}

	resp, err := h.controller.DeleteWorkspace(ctx, dto.DeleteWorkspaceRequest{ID: req.ID})
	if err != nil {
		h.logger.Error("failed to delete workspace", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to delete workspace", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}
