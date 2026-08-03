package secrets

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// Handler provides HTTP and WebSocket handlers for secrets CRUD.
type Handler struct {
	service *Service
	logger  *logger.Logger
}

// NewHandler creates a new secrets handler.
func NewHandler(svc *Service, log *logger.Logger) *Handler {
	return &Handler{service: svc, logger: log}
}

// RegisterRoutes registers both HTTP and WS handlers.
func RegisterRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, svc *Service, log *logger.Logger) {
	h := NewHandler(svc, log)
	h.registerHTTP(router)
	h.registerWS(dispatcher)
}

func (h *Handler) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.POST("/secrets", h.httpCreateSecret)
	api.GET("/secrets", h.httpListSecrets)
	api.GET("/secrets/:id", h.httpGetSecret)
	api.PUT("/secrets/:id", h.httpUpdateSecret)
	api.DELETE("/secrets/:id", h.httpDeleteSecret)
	api.POST("/secrets/:id/reveal", h.httpRevealSecret)
}

func (h *Handler) registerWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc(ws.ActionSecretList, h.wsList)
	dispatcher.RegisterFunc(ws.ActionSecretCreate, h.wsCreate)
	dispatcher.RegisterFunc(ws.ActionSecretUpdate, h.wsUpdate)
	dispatcher.RegisterFunc(ws.ActionSecretDelete, h.wsDelete)
	dispatcher.RegisterFunc(ws.ActionSecretReveal, h.wsReveal)
}

// HTTP handlers

func (h *Handler) httpCreateSecret(c *gin.Context) {
	var req CreateSecretRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	item, err := h.service.Create(c.Request.Context(), &req)
	if err != nil {
		h.logger.Error("failed to create secret", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *Handler) httpListSecrets(c *gin.Context) {
	opts := SecretListOptions{
		Scope:         SecretScope(c.Query("scope")),
		WorkspaceID:   c.Query("workspace_id"),
		IncludeGlobal: c.Query("include_global") == "true",
	}
	if opts.Scope == "" && opts.WorkspaceID != "" {
		opts.Scope = ScopeWorkspace
	}
	items, err := h.service.ListScoped(c.Request.Context(), opts)
	if err != nil {
		h.logger.Error("failed to list secrets", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list secrets"})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) httpGetSecret(c *gin.Context) {
	id := c.Param("id")
	secret, err := h.getSecret(c, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, secret)
}

func (h *Handler) httpUpdateSecret(c *gin.Context) {
	id := c.Param("id")
	var req UpdateSecretRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	item, err := h.updateSecret(c, id, &req)
	if err != nil {
		h.logger.Error("failed to update secret", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *Handler) httpDeleteSecret(c *gin.Context) {
	id := c.Param("id")
	if err := h.deleteSecret(c, id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) httpRevealSecret(c *gin.Context) {
	id := c.Param("id")
	value, err := h.revealSecret(c, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, RevealSecretResponse{Value: value})
}

// WS handlers

func (h *Handler) wsList(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		Scope         SecretScope `json:"scope"`
		WorkspaceID   string      `json:"workspace_id"`
		IncludeGlobal bool        `json:"include_global"`
	}
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+err.Error(), nil)
	}
	if req.Scope == "" && req.WorkspaceID != "" {
		req.Scope = ScopeWorkspace
	}
	items, err := h.service.ListScoped(ctx, SecretListOptions{
		Scope: req.Scope, WorkspaceID: req.WorkspaceID, IncludeGlobal: req.IncludeGlobal,
	})
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, items)
}

func (h *Handler) wsCreate(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req CreateSecretRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+err.Error(), nil)
	}

	item, err := h.service.Create(ctx, &req)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, item)
}

func (h *Handler) wsUpdate(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var payload struct {
		ID          string `json:"id"`
		WorkspaceID string `json:"workspace_id"`
		UpdateSecretRequest
	}
	if err := msg.ParsePayload(&payload); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+err.Error(), nil)
	}

	item, err := h.updateSecretForWorkspace(ctx, payload.ID, payload.WorkspaceID, &payload.UpdateSecretRequest)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, item)
}

func (h *Handler) wsDelete(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var payload struct {
		ID          string `json:"id"`
		WorkspaceID string `json:"workspace_id"`
	}
	if err := msg.ParsePayload(&payload); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+err.Error(), nil)
	}

	if err := h.deleteSecretForWorkspace(ctx, payload.ID, payload.WorkspaceID); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"success": true})
}

func (h *Handler) wsReveal(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var payload struct {
		ID          string `json:"id"`
		WorkspaceID string `json:"workspace_id"`
	}
	if err := msg.ParsePayload(&payload); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+err.Error(), nil)
	}

	value, err := h.revealSecretForWorkspace(ctx, payload.ID, payload.WorkspaceID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, RevealSecretResponse{Value: value})
}

func (h *Handler) getSecret(c *gin.Context, id string) (*Secret, error) {
	if workspaceID := c.Query("workspace_id"); workspaceID != "" {
		return h.service.GetWorkspaceSecret(c.Request.Context(), id, workspaceID)
	}
	return h.service.Get(c.Request.Context(), id)
}

func (h *Handler) updateSecret(c *gin.Context, id string, req *UpdateSecretRequest) (*SecretListItem, error) {
	if workspaceID := c.Query("workspace_id"); workspaceID != "" {
		return h.service.UpdateWorkspaceSecret(c.Request.Context(), id, workspaceID, req)
	}
	return h.service.Update(c.Request.Context(), id, req)
}

func (h *Handler) updateSecretForWorkspace(ctx context.Context, id, workspaceID string, req *UpdateSecretRequest) (*SecretListItem, error) {
	if workspaceID != "" {
		return h.service.UpdateWorkspaceSecret(ctx, id, workspaceID, req)
	}
	return h.service.Update(ctx, id, req)
}

func (h *Handler) deleteSecret(c *gin.Context, id string) error {
	if workspaceID := c.Query("workspace_id"); workspaceID != "" {
		return h.service.DeleteWorkspaceSecret(c.Request.Context(), id, workspaceID)
	}
	return h.service.Delete(c.Request.Context(), id)
}

func (h *Handler) deleteSecretForWorkspace(ctx context.Context, id, workspaceID string) error {
	if workspaceID != "" {
		return h.service.DeleteWorkspaceSecret(ctx, id, workspaceID)
	}
	return h.service.Delete(ctx, id)
}

func (h *Handler) revealSecret(c *gin.Context, id string) (string, error) {
	if workspaceID := c.Query("workspace_id"); workspaceID != "" {
		return h.service.RevealWorkspaceSecret(c.Request.Context(), id, workspaceID)
	}
	return h.service.Reveal(c.Request.Context(), id)
}

func (h *Handler) revealSecretForWorkspace(ctx context.Context, id, workspaceID string) (string, error) {
	if workspaceID != "" {
		return h.service.RevealWorkspaceSecret(ctx, id, workspaceID)
	}
	return h.service.Reveal(ctx, id)
}
