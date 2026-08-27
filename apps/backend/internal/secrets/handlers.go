package secrets

import (
	"context"
	"encoding/json"
	"errors"
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

// registerHTTP registers the secrets HTTP routes under /api/v1.
func (h *Handler) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.POST("/secrets", h.httpCreateSecret)
	api.GET("/secrets", h.httpListSecrets)
	api.GET("/secrets/:id", h.httpGetSecret)
	api.PUT("/secrets/:id", h.httpUpdateSecret)
	api.DELETE("/secrets/:id", h.httpDeleteSecret)
	api.POST("/secrets/:id/reveal", h.httpRevealSecret)
	api.POST("/secrets/:id/copy", h.httpCopySecret)
	api.POST("/secrets/:id/move", h.httpMoveSecret)
}

// registerWS registers the secrets WebSocket actions on the dispatcher.
func (h *Handler) registerWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc(ws.ActionSecretList, h.wsList)
	dispatcher.RegisterFunc(ws.ActionSecretCreate, h.wsCreate)
	dispatcher.RegisterFunc(ws.ActionSecretUpdate, h.wsUpdate)
	dispatcher.RegisterFunc(ws.ActionSecretDelete, h.wsDelete)
	dispatcher.RegisterFunc(ws.ActionSecretReveal, h.wsReveal)
	dispatcher.RegisterFunc(ws.ActionSecretCopy, h.wsCopy)
	dispatcher.RegisterFunc(ws.ActionSecretMove, h.wsMove)
}

// HTTP handlers

// httpCreateSecret handles POST /api/v1/secrets.
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

// httpListSecrets handles GET /api/v1/secrets.
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

// httpGetSecret handles GET /api/v1/secrets/:id.
func (h *Handler) httpGetSecret(c *gin.Context) {
	id := c.Param("id")
	secret, err := h.getSecret(c, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, secret)
}

// httpUpdateSecret handles PUT /api/v1/secrets/:id.
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

// httpDeleteSecret handles DELETE /api/v1/secrets/:id.
func (h *Handler) httpDeleteSecret(c *gin.Context) {
	id := c.Param("id")
	if err := h.deleteSecret(c, id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// httpRevealSecret handles POST /api/v1/secrets/:id/reveal.
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

// wsList handles the secrets.list WebSocket action.
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

// wsCreate handles the secrets.create WebSocket action.
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

// wsUpdate handles the secrets.update WebSocket action.
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

// wsDelete handles the secrets.delete WebSocket action.
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

// wsReveal handles the secrets.reveal WebSocket action.
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

// getSecret fetches a secret by ID, resolving workspace-scoped access from
// the request's workspace_id query parameter.
func (h *Handler) getSecret(c *gin.Context, id string) (*Secret, error) {
	if workspaceID := c.Query("workspace_id"); workspaceID != "" {
		return h.service.GetWorkspaceSecret(c.Request.Context(), id, workspaceID)
	}
	return h.service.Get(c.Request.Context(), id)
}

// updateSecret updates a secret, resolving workspace-scoped access from the
// request's workspace_id query parameter.
func (h *Handler) updateSecret(c *gin.Context, id string, req *UpdateSecretRequest) (*SecretListItem, error) {
	if workspaceID := c.Query("workspace_id"); workspaceID != "" {
		return h.service.UpdateWorkspaceSecret(c.Request.Context(), id, workspaceID, req)
	}
	return h.service.Update(c.Request.Context(), id, req)
}

// updateSecretForWorkspace updates a secret, targeting the workspace when
// workspaceID is non-empty and falling back to the global scope otherwise.
func (h *Handler) updateSecretForWorkspace(ctx context.Context, id, workspaceID string, req *UpdateSecretRequest) (*SecretListItem, error) {
	if workspaceID != "" {
		return h.service.UpdateWorkspaceSecret(ctx, id, workspaceID, req)
	}
	return h.service.Update(ctx, id, req)
}

// deleteSecret deletes a secret, resolving workspace-scoped access from the
// request's workspace_id query parameter.
func (h *Handler) deleteSecret(c *gin.Context, id string) error {
	if workspaceID := c.Query("workspace_id"); workspaceID != "" {
		return h.service.DeleteWorkspaceSecret(c.Request.Context(), id, workspaceID)
	}
	return h.service.Delete(c.Request.Context(), id)
}

// deleteSecretForWorkspace deletes a secret, targeting the workspace when
// workspaceID is non-empty and falling back to the global scope otherwise.
func (h *Handler) deleteSecretForWorkspace(ctx context.Context, id, workspaceID string) error {
	if workspaceID != "" {
		return h.service.DeleteWorkspaceSecret(ctx, id, workspaceID)
	}
	return h.service.Delete(ctx, id)
}

// revealSecret returns a secret's value, resolving workspace-scoped access
// from the request's workspace_id query parameter.
func (h *Handler) revealSecret(c *gin.Context, id string) (string, error) {
	if workspaceID := c.Query("workspace_id"); workspaceID != "" {
		return h.service.RevealWorkspaceSecret(c.Request.Context(), id, workspaceID)
	}
	return h.service.Reveal(c.Request.Context(), id)
}

// revealSecretForWorkspace returns a secret's value, targeting the workspace
// when workspaceID is non-empty and falling back to the global scope otherwise.
func (h *Handler) revealSecretForWorkspace(ctx context.Context, id, workspaceID string) (string, error) {
	if workspaceID != "" {
		return h.service.RevealWorkspaceSecret(ctx, id, workspaceID)
	}
	return h.service.Reveal(ctx, id)
}

// HTTP copy/move handlers

// httpCopySecret handles POST /api/v1/secrets/:id/copy.
func (h *Handler) httpCopySecret(c *gin.Context) {
	h.httpTransferSecret(c, false)
}

// httpMoveSecret handles POST /api/v1/secrets/:id/move.
func (h *Handler) httpMoveSecret(c *gin.Context) {
	h.httpTransferSecret(c, true)
}

// httpTransferSecret implements the shared HTTP copy/move flow for a secret;
// move selects move semantics, false selects copy.
func (h *Handler) httpTransferSecret(c *gin.Context, move bool) {
	id := c.Param("id")
	var req CopySecretRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	item, err := h.transferSecret(c.Request.Context(), id, c.Query("workspace_id"), &req, move)
	if err != nil {
		status, message := h.transferError(err)
		c.JSON(status, gin.H{"error": message})
		return
	}
	c.JSON(http.StatusCreated, item)
}

// transferSecret dispatches copy/move using the existing source-scoping
// convention: a non-empty workspace_id selects the workspace-scoped source.
func (h *Handler) transferSecret(ctx context.Context, id, sourceWorkspaceID string, req *CopySecretRequest, move bool) (*SecretListItem, error) {
	if move {
		return h.service.Move(ctx, id, sourceWorkspaceID, req)
	}
	return h.service.Copy(ctx, id, sourceWorkspaceID, req)
}

// transferError classifies a transfer error into an HTTP status and a safe
// message. Only the known sentinels map to 400/404/409; unexpected failures
// become a sanitized 500 with the details logged server-side.
func (h *Handler) transferError(err error) (int, string) {
	switch {
	case errors.Is(err, ErrSecretValidation):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrWorkspaceAccessDenied):
		return http.StatusNotFound, err.Error()
	case errors.Is(err, ErrSecretNameConflict):
		return http.StatusConflict, err.Error()
	default:
		h.logger.Error("secret transfer failed", zap.Error(err))
		return http.StatusInternalServerError, "internal error"
	}
}

// WS copy/move handlers

// wsCopy handles the secrets.copy WebSocket action.
func (h *Handler) wsCopy(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return h.wsTransfer(ctx, msg, false)
}

// wsMove handles the secrets.move WebSocket action.
func (h *Handler) wsMove(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return h.wsTransfer(ctx, msg, true)
}

// wsTransferPayload is the WS envelope for secrets.copy / secrets.move. It
// decodes id/workspace_id separately, then delegates the transfer fields to
// the presence-aware CopySecretRequest decoder (a flat struct would silently
// collapse an explicit null name into "omitted").
type wsTransferPayload struct {
	ID          string
	WorkspaceID string
	CopySecretRequest
}

// UnmarshalJSON implements the presence-aware envelope decode. The
// CopySecretRequest decoder runs on the whole payload and resets every field,
// including its presence markers, so a reused envelope cannot leak state
// between messages.
func (p *wsTransferPayload) UnmarshalJSON(data []byte) error {
	// Reset every field before parsing so a failed decode can never leave
	// stale state from an earlier message on a reused envelope, and assign
	// only after every sub-decode succeeds so a semantic failure cannot leave
	// a partially populated receiver.
	p.ID = ""
	p.WorkspaceID = ""
	p.CopySecretRequest = CopySecretRequest{}
	var aux struct {
		ID          string `json:"id"`
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	var req CopySecretRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return err
	}
	p.ID = aux.ID
	p.WorkspaceID = aux.WorkspaceID
	p.CopySecretRequest = req
	return nil
}

// wsTransfer implements the shared WebSocket copy/move flow for a secret;
// move selects move semantics, false selects copy.
func (h *Handler) wsTransfer(ctx context.Context, msg *ws.Message, move bool) (*ws.Message, error) {
	var payload wsTransferPayload
	if err := msg.ParsePayload(&payload); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+err.Error(), nil)
	}

	item, err := h.transferSecret(ctx, payload.ID, payload.WorkspaceID, &payload.CopySecretRequest, move)
	if err != nil {
		code, message := h.transferWSError(err)
		return ws.NewError(msg.ID, msg.Action, code, message, nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, item)
}

// transferWSError maps a transfer error to its WebSocket payload and status.
func (h *Handler) transferWSError(err error) (string, string) {
	switch {
	case errors.Is(err, ErrSecretValidation):
		return ws.ErrorCodeBadRequest, err.Error()
	case errors.Is(err, ErrNotFound), errors.Is(err, ErrWorkspaceAccessDenied):
		return ws.ErrorCodeNotFound, err.Error()
	case errors.Is(err, ErrSecretNameConflict):
		return ws.ErrorCodeConflict, err.Error()
	default:
		h.logger.Error("secret transfer failed", zap.Error(err))
		return ws.ErrorCodeInternalError, "internal error"
	}
}
