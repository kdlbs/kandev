package slack

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// RegisterRoutes wires the Slack HTTP and WebSocket handlers.
func RegisterRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, svc *Service, log *logger.Logger) {
	ctrl := &Controller{service: svc, logger: log}
	ctrl.RegisterHTTPRoutes(router)
	registerWSHandlers(dispatcher, svc)
}

// Controller holds HTTP route handlers for the Slack integration.
type Controller struct {
	service *Service
	logger  *logger.Logger
}

// RegisterHTTPRoutes attaches the Slack HTTP endpoints to router.
func (c *Controller) RegisterHTTPRoutes(router *gin.Engine) {
	api := router.Group("/api/v1/slack")
	api.GET("/config", c.httpGetConfig)
	api.POST("/config", c.httpSetConfig)
	api.DELETE("/config", c.httpDeleteConfig)
	api.POST("/config/test", c.httpTestConfig)
}

// --- HTTP handlers ---

func (c *Controller) httpGetConfig(ctx *gin.Context) {
	cfg, err := c.service.GetConfig(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if cfg == nil {
		ctx.Status(http.StatusNoContent)
		return
	}
	ctx.JSON(http.StatusOK, cfg)
}

func (c *Controller) httpSetConfig(ctx *gin.Context) {
	var req SetConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	cfg, err := c.service.SetConfig(ctx.Request.Context(), &req)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrInvalidConfig) {
			status = http.StatusBadRequest
		}
		ctx.JSON(status, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, cfg)
}

func (c *Controller) httpDeleteConfig(ctx *gin.Context) {
	if err := c.service.DeleteConfig(ctx.Request.Context()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (c *Controller) httpTestConfig(ctx *gin.Context) {
	var req SetConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	result, err := c.service.TestConnection(ctx.Request.Context(), &req)
	if err != nil {
		c.writeClientError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, result)
}

// errCodeSlackNotConfigured is the wire-level code surfaced to the UI when
// no Slack credentials are saved.
const errCodeSlackNotConfigured = "SLACK_NOT_CONFIGURED"

func (c *Controller) writeClientError(ctx *gin.Context, err error) {
	if errors.Is(err, ErrNotConfigured) {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Slack is not configured",
			"code":  errCodeSlackNotConfigured,
		})
		return
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		status := http.StatusInternalServerError
		switch apiErr.StatusCode {
		case http.StatusNotFound, http.StatusUnauthorized, http.StatusForbidden, http.StatusBadRequest:
			status = apiErr.StatusCode
		}
		ctx.JSON(status, gin.H{"error": apiErr.Error()})
		return
	}
	c.logger.Warn("slack handler error", zap.Error(err))
	ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

// --- WebSocket handlers ---

func registerWSHandlers(dispatcher *ws.Dispatcher, svc *Service) {
	dispatcher.RegisterFunc(ws.ActionSlackConfigGet, wsGetConfig(svc))
	dispatcher.RegisterFunc(ws.ActionSlackConfigSet, wsSetConfig(svc))
	dispatcher.RegisterFunc(ws.ActionSlackConfigDelete, wsDeleteConfig(svc))
	dispatcher.RegisterFunc(ws.ActionSlackConfigTest, wsTestConfig(svc))
}

func wsReply(msg *ws.Message, payload interface{}) (*ws.Message, error) {
	resp, err := ws.NewResponse(msg.ID, msg.Action, payload)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	return resp, nil
}

// wsFail mirrors writeClientError's HTTP-side mapping so WS-driven flows
// surface bad-request / unauthorized / not-found / forbidden distinctly
// instead of collapsing every non-ErrNotConfigured failure into
// INTERNAL_ERROR.
func wsFail(msg *ws.Message, err error) (*ws.Message, error) {
	if errors.Is(err, ErrNotConfigured) {
		return ws.NewError(msg.ID, msg.Action, errCodeSlackNotConfigured, err.Error(), nil)
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusBadRequest:
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, apiErr.Error(), nil)
		case http.StatusUnauthorized:
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeUnauthorized, apiErr.Error(), nil)
		case http.StatusForbidden:
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, apiErr.Error(), nil)
		case http.StatusNotFound:
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, apiErr.Error(), nil)
		}
	}
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
}

func wsGetConfig(svc *Service) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		cfg, err := svc.GetConfig(ctx)
		if err != nil {
			return wsFail(msg, err)
		}
		return wsReply(msg, gin.H{"config": cfg})
	}
}

func wsSetConfig(svc *Service) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req SetConfigRequest
		if err := msg.ParsePayload(&req); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload", nil)
		}
		cfg, err := svc.SetConfig(ctx, &req)
		if err != nil {
			if errors.Is(err, ErrInvalidConfig) {
				return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, err.Error(), nil)
			}
			return wsFail(msg, err)
		}
		return wsReply(msg, gin.H{"config": cfg})
	}
}

func wsDeleteConfig(svc *Service) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		if err := svc.DeleteConfig(ctx); err != nil {
			return wsFail(msg, err)
		}
		return wsReply(msg, gin.H{"deleted": true})
	}
}

func wsTestConfig(svc *Service) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var req SetConfigRequest
		if err := msg.ParsePayload(&req); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload", nil)
		}
		result, err := svc.TestConnection(ctx, &req)
		if err != nil {
			return wsFail(msg, err)
		}
		return wsReply(msg, result)
	}
}
