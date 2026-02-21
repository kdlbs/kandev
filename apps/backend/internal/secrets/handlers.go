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
	items, err := h.service.List(c.Request.Context())
	if err != nil {
		h.logger.Error("failed to list secrets", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list secrets"})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *Handler) httpGetSecret(c *gin.Context) {
	id := c.Param("id")
	secret, err := h.service.Get(c.Request.Context(), id)
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

	item, err := h.service.Update(c.Request.Context(), id, &req)
	if err != nil {
		h.logger.Error("failed to update secret", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *Handler) httpDeleteSecret(c *gin.Context) {
	id := c.Param("id")
	if err := h.service.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *Handler) httpRevealSecret(c *gin.Context) {
	id := c.Param("id")
	value, err := h.service.Reveal(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, RevealSecretResponse{Value: value})
}

// WS handlers

func (h *Handler) wsList(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	items, err := h.service.List(ctx)
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
		ID string `json:"id"`
		UpdateSecretRequest
	}
	if err := msg.ParsePayload(&payload); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+err.Error(), nil)
	}

	item, err := h.service.Update(ctx, payload.ID, &payload.UpdateSecretRequest)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, item)
}

func (h *Handler) wsDelete(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var payload struct {
		ID string `json:"id"`
	}
	if err := msg.ParsePayload(&payload); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+err.Error(), nil)
	}

	if err := h.service.Delete(ctx, payload.ID); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"success": true})
}

func (h *Handler) wsReveal(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var payload struct {
		ID string `json:"id"`
	}
	if err := msg.ParsePayload(&payload); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+err.Error(), nil)
	}

	value, err := h.service.Reveal(ctx, payload.ID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, RevealSecretResponse{Value: value})
}
