package jira

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// RegisterRoutes wires the Jira HTTP and WebSocket handlers.
func RegisterRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, svc *Service, log *logger.Logger) {
	ctrl := &Controller{service: svc, logger: log}
	ctrl.RegisterHTTPRoutes(router)
	registerWSHandlers(dispatcher, svc, log)
}

// Controller holds HTTP route handlers for the Jira integration.
type Controller struct {
	service *Service
	logger  *logger.Logger
}

// RegisterHTTPRoutes attaches the Jira HTTP endpoints to router.
func (c *Controller) RegisterHTTPRoutes(router *gin.Engine) {
	api := router.Group("/api/v1/jira")
	api.GET("/config", c.httpGetConfig)
	api.POST("/config", c.httpSetConfig)
	api.DELETE("/config", c.httpDeleteConfig)
	api.POST("/config/test", c.httpTestConfig)
	api.GET("/projects", c.httpListProjects)
	api.GET("/tickets", c.httpSearchTickets)
	api.GET("/tickets/:key", c.httpGetTicket)
	api.POST("/tickets/:key/transitions", c.httpDoTransition)
}

// --- HTTP handlers ---

func (c *Controller) httpGetConfig(ctx *gin.Context) {
	workspaceID := ctx.Query("workspace_id")
	if workspaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id required"})
		return
	}
	cfg, err := c.service.GetConfig(ctx.Request.Context(), workspaceID)
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
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, cfg)
}

func (c *Controller) httpDeleteConfig(ctx *gin.Context) {
	workspaceID := ctx.Query("workspace_id")
	if workspaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id required"})
		return
	}
	if err := c.service.DeleteConfig(ctx.Request.Context(), workspaceID); err != nil {
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
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (c *Controller) httpListProjects(ctx *gin.Context) {
	workspaceID := ctx.Query("workspace_id")
	if workspaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id required"})
		return
	}
	projects, err := c.service.ListProjects(ctx.Request.Context(), workspaceID)
	if err != nil {
		c.writeClientError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"projects": projects})
}

func (c *Controller) httpSearchTickets(ctx *gin.Context) {
	workspaceID := ctx.Query("workspace_id")
	if workspaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id required"})
		return
	}
	jql := ctx.Query("jql")
	startAt, _ := strconv.Atoi(ctx.Query("start_at"))
	maxResults, _ := strconv.Atoi(ctx.Query("max_results"))
	result, err := c.service.SearchTickets(ctx.Request.Context(), workspaceID, jql, startAt, maxResults)
	if err != nil {
		c.writeClientError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (c *Controller) httpGetTicket(ctx *gin.Context) {
	workspaceID := ctx.Query("workspace_id")
	if workspaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id required"})
		return
	}
	key := ctx.Param("key")
	ticket, err := c.service.GetTicket(ctx.Request.Context(), workspaceID, key)
	if err != nil {
		c.writeClientError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, ticket)
}

func (c *Controller) httpDoTransition(ctx *gin.Context) {
	workspaceID := ctx.Query("workspace_id")
	if workspaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id required"})
		return
	}
	key := ctx.Param("key")
	var req struct {
		TransitionID string `json:"transitionId"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil || req.TransitionID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "transitionId required"})
		return
	}
	if err := c.service.DoTransition(ctx.Request.Context(), workspaceID, key, req.TransitionID); err != nil {
		c.writeClientError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"transitioned": true})
}

// writeClientError maps service-level errors to HTTP responses. ErrNotConfigured
// surfaces as 503 so the UI can prompt the user to configure Jira; upstream
// API errors propagate their status codes.
func (c *Controller) writeClientError(ctx *gin.Context, err error) {
	if errors.Is(err, ErrNotConfigured) {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Jira is not configured for this workspace",
			"code":  "jira_not_configured",
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
		// 3xx from the upstream means Atlassian redirected to login (step-up
		// auth, expired session, etc.); treat it as unauthorized for the UI.
		if apiErr.StatusCode >= 300 && apiErr.StatusCode < 400 {
			status = http.StatusUnauthorized
		}
		ctx.JSON(status, gin.H{"error": apiErr.Error()})
		return
	}
	c.logger.Warn("jira handler error", zap.Error(err))
	ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

// --- WebSocket handlers ---

func registerWSHandlers(dispatcher *ws.Dispatcher, svc *Service, log *logger.Logger) {
	dispatcher.RegisterFunc(ws.ActionJiraConfigGet, wsGetConfig(svc))
	dispatcher.RegisterFunc(ws.ActionJiraConfigSet, wsSetConfig(svc))
	dispatcher.RegisterFunc(ws.ActionJiraConfigDelete, wsDeleteConfig(svc))
	dispatcher.RegisterFunc(ws.ActionJiraConfigTest, wsTestConfig(svc))
	dispatcher.RegisterFunc(ws.ActionJiraTicketGet, wsGetTicket(svc))
	dispatcher.RegisterFunc(ws.ActionJiraTicketTransition, wsDoTransition(svc))
	dispatcher.RegisterFunc(ws.ActionJiraProjectsList, wsListProjects(svc))
	_ = log
}

// wsReply wraps the common boilerplate: try to build a response, fall back to
// an internal-error envelope if marshaling fails.
func wsReply(msg *ws.Message, payload interface{}) (*ws.Message, error) {
	resp, err := ws.NewResponse(msg.ID, msg.Action, payload)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	return resp, nil
}

func wsFail(msg *ws.Message, err error) (*ws.Message, error) {
	if errors.Is(err, ErrNotConfigured) {
		return ws.NewError(msg.ID, msg.Action, "JIRA_NOT_CONFIGURED", err.Error(), nil)
	}
	return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
}

func wsGetConfig(svc *Service) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var p struct {
			WorkspaceID string `json:"workspaceId"`
		}
		if err := msg.ParsePayload(&p); err != nil || p.WorkspaceID == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "workspaceId required", nil)
		}
		cfg, err := svc.GetConfig(ctx, p.WorkspaceID)
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
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, err.Error(), nil)
		}
		return wsReply(msg, gin.H{"config": cfg})
	}
}

func wsDeleteConfig(svc *Service) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var p struct {
			WorkspaceID string `json:"workspaceId"`
		}
		if err := msg.ParsePayload(&p); err != nil || p.WorkspaceID == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "workspaceId required", nil)
		}
		if err := svc.DeleteConfig(ctx, p.WorkspaceID); err != nil {
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

func wsGetTicket(svc *Service) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var p struct {
			WorkspaceID string `json:"workspaceId"`
			TicketKey   string `json:"ticketKey"`
		}
		if err := msg.ParsePayload(&p); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload", nil)
		}
		if p.WorkspaceID == "" || p.TicketKey == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "workspaceId and ticketKey required", nil)
		}
		ticket, err := svc.GetTicket(ctx, p.WorkspaceID, p.TicketKey)
		if err != nil {
			return wsFail(msg, err)
		}
		return wsReply(msg, ticket)
	}
}

func wsDoTransition(svc *Service) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var p struct {
			WorkspaceID  string `json:"workspaceId"`
			TicketKey    string `json:"ticketKey"`
			TransitionID string `json:"transitionId"`
		}
		if err := msg.ParsePayload(&p); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload", nil)
		}
		if p.WorkspaceID == "" || p.TicketKey == "" || p.TransitionID == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "workspaceId, ticketKey, transitionId required", nil)
		}
		if err := svc.DoTransition(ctx, p.WorkspaceID, p.TicketKey, p.TransitionID); err != nil {
			return wsFail(msg, err)
		}
		return wsReply(msg, gin.H{"transitioned": true})
	}
}

func wsListProjects(svc *Service) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var p struct {
			WorkspaceID string `json:"workspaceId"`
		}
		if err := msg.ParsePayload(&p); err != nil || p.WorkspaceID == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "workspaceId required", nil)
		}
		projects, err := svc.ListProjects(ctx, p.WorkspaceID)
		if err != nil {
			return wsFail(msg, err)
		}
		return wsReply(msg, gin.H{"projects": projects})
	}
}
