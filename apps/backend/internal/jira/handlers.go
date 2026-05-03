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
	registerWSHandlers(dispatcher, svc)
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

	api.GET("/watches/issue", c.httpListIssueWatches)
	api.POST("/watches/issue", c.httpCreateIssueWatch)
	api.PATCH("/watches/issue/:id", c.httpUpdateIssueWatch)
	api.DELETE("/watches/issue/:id", c.httpDeleteIssueWatch)
	api.POST("/watches/issue/:id/trigger", c.httpTriggerIssueWatch)
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
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (c *Controller) httpListProjects(ctx *gin.Context) {
	projects, err := c.service.ListProjects(ctx.Request.Context())
	if err != nil {
		c.writeClientError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"projects": projects})
}

func (c *Controller) httpSearchTickets(ctx *gin.Context) {
	jql := ctx.Query("jql")
	pageToken := ctx.Query("page_token")
	maxResults, _ := strconv.Atoi(ctx.Query("max_results"))
	result, err := c.service.SearchTickets(ctx.Request.Context(), jql, pageToken, maxResults)
	if err != nil {
		c.writeClientError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (c *Controller) httpGetTicket(ctx *gin.Context) {
	key := ctx.Param("key")
	ticket, err := c.service.GetTicket(ctx.Request.Context(), key)
	if err != nil {
		c.writeClientError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, ticket)
}

func (c *Controller) httpDoTransition(ctx *gin.Context) {
	key := ctx.Param("key")
	var req struct {
		TransitionID string `json:"transitionId"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil || req.TransitionID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "transitionId required"})
		return
	}
	if err := c.service.DoTransition(ctx.Request.Context(), key, req.TransitionID); err != nil {
		c.writeClientError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"transitioned": true})
}

// --- Issue watch HTTP handlers ---

// httpListIssueWatches returns watches scoped to one workspace when
// `workspace_id` is supplied, or every watch across all workspaces when it
// is absent. The integration settings page uses the unscoped form so the
// table can render a Workspace column without requiring an upfront pick.
func (c *Controller) httpListIssueWatches(ctx *gin.Context) {
	workspaceID := ctx.Query("workspace_id")
	var (
		watches []*IssueWatch
		err     error
	)
	if workspaceID == "" {
		watches, err = c.service.ListAllIssueWatches(ctx.Request.Context())
	} else {
		watches, err = c.service.ListIssueWatches(ctx.Request.Context(), workspaceID)
	}
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"watches": watches})
}

func (c *Controller) httpCreateIssueWatch(ctx *gin.Context) {
	var req CreateIssueWatchRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	w, err := c.service.CreateIssueWatch(ctx.Request.Context(), &req)
	if err != nil {
		c.writeIssueWatchError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, w)
}

func (c *Controller) httpUpdateIssueWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	if !c.assertWatchInWorkspace(ctx, id) {
		return
	}
	var req UpdateIssueWatchRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	w, err := c.service.UpdateIssueWatch(ctx.Request.Context(), id, &req)
	if err != nil {
		c.writeIssueWatchError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, w)
}

func (c *Controller) httpDeleteIssueWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	if !c.assertWatchInWorkspace(ctx, id) {
		return
	}
	if err := c.service.DeleteIssueWatch(ctx.Request.Context(), id); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

// httpTriggerIssueWatch runs a single immediate poll of the watch. Useful from
// the UI to verify a JQL change without waiting for the next 5-minute tick.
// Returns the count of newly-discovered tickets so the user gets feedback even
// when the matching tickets fan out asynchronously through the orchestrator.
func (c *Controller) httpTriggerIssueWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	w, err := c.service.GetIssueWatch(ctx.Request.Context(), id)
	if err != nil {
		c.writeIssueWatchError(ctx, err)
		return
	}
	if !workspaceMatches(ctx, w.WorkspaceID) {
		// 404 not 403 — don't reveal whether the ID exists.
		ctx.JSON(http.StatusNotFound, gin.H{"error": ErrIssueWatchNotFound.Error()})
		return
	}
	tickets, err := c.service.CheckIssueWatch(ctx.Request.Context(), w)
	if err != nil {
		c.writeClientError(ctx, err)
		return
	}
	for _, t := range tickets {
		c.service.publishNewJiraIssueEvent(ctx.Request.Context(), w, t)
	}
	ctx.JSON(http.StatusOK, gin.H{"newIssues": len(tickets)})
}

// assertWatchInWorkspace guards mutation/trigger endpoints against IDOR: the
// caller must supply `?workspace_id=...` matching the watch's workspace. The
// list/create endpoints already require this query; mirroring it here closes
// the gap where a known watch UUID from another workspace could be mutated.
// Writes the response and returns false on mismatch — caller bails out.
func (c *Controller) assertWatchInWorkspace(ctx *gin.Context, id string) bool {
	w, err := c.service.GetIssueWatch(ctx.Request.Context(), id)
	if err != nil {
		c.writeIssueWatchError(ctx, err)
		return false
	}
	if !workspaceMatches(ctx, w.WorkspaceID) {
		// Use 404 so a probing client can't tell whether a watch ID exists in
		// another workspace.
		ctx.JSON(http.StatusNotFound, gin.H{"error": ErrIssueWatchNotFound.Error()})
		return false
	}
	return true
}

// workspaceMatches returns true when the request's `workspace_id` query param
// matches the resource's stored workspace. Empty query is rejected so the
// caller can't bypass the check by omitting the parameter.
func workspaceMatches(ctx *gin.Context, resourceWorkspace string) bool {
	q := ctx.Query("workspace_id")
	return q != "" && q == resourceWorkspace
}

func (c *Controller) writeIssueWatchError(ctx *gin.Context, err error) {
	if errors.Is(err, ErrIssueWatchNotFound) {
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, ErrInvalidConfig) {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.writeClientError(ctx, err)
}

// errCodeJiraNotConfigured is the wire-level code surfaced to the UI when
// Jira has no saved credentials. The same string is used by both the HTTP
// and WebSocket layers so the frontend can branch on a single value.
const errCodeJiraNotConfigured = "JIRA_NOT_CONFIGURED"

// writeClientError maps service-level errors to HTTP responses. ErrNotConfigured
// surfaces as 503 so the UI can prompt the user to configure Jira; upstream
// API errors propagate their status codes.
func (c *Controller) writeClientError(ctx *gin.Context, err error) {
	if errors.Is(err, ErrNotConfigured) {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Jira is not configured",
			"code":  errCodeJiraNotConfigured,
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

func registerWSHandlers(dispatcher *ws.Dispatcher, svc *Service) {
	dispatcher.RegisterFunc(ws.ActionJiraConfigGet, wsGetConfig(svc))
	dispatcher.RegisterFunc(ws.ActionJiraConfigSet, wsSetConfig(svc))
	dispatcher.RegisterFunc(ws.ActionJiraConfigDelete, wsDeleteConfig(svc))
	dispatcher.RegisterFunc(ws.ActionJiraConfigTest, wsTestConfig(svc))
	dispatcher.RegisterFunc(ws.ActionJiraTicketGet, wsGetTicket(svc))
	dispatcher.RegisterFunc(ws.ActionJiraTicketTransition, wsDoTransition(svc))
	dispatcher.RegisterFunc(ws.ActionJiraProjectsList, wsListProjects(svc))
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
		return ws.NewError(msg.ID, msg.Action, errCodeJiraNotConfigured, err.Error(), nil)
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

func wsGetTicket(svc *Service) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var p struct {
			TicketKey string `json:"ticketKey"`
		}
		if err := msg.ParsePayload(&p); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload", nil)
		}
		if p.TicketKey == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "ticketKey required", nil)
		}
		ticket, err := svc.GetTicket(ctx, p.TicketKey)
		if err != nil {
			return wsFail(msg, err)
		}
		return wsReply(msg, ticket)
	}
}

func wsDoTransition(svc *Service) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var p struct {
			TicketKey    string `json:"ticketKey"`
			TransitionID string `json:"transitionId"`
		}
		if err := msg.ParsePayload(&p); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload", nil)
		}
		if p.TicketKey == "" || p.TransitionID == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "ticketKey and transitionId required", nil)
		}
		if err := svc.DoTransition(ctx, p.TicketKey, p.TransitionID); err != nil {
			return wsFail(msg, err)
		}
		return wsReply(msg, gin.H{"transitioned": true})
	}
}

func wsListProjects(svc *Service) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		projects, err := svc.ListProjects(ctx)
		if err != nil {
			return wsFail(msg, err)
		}
		return wsReply(msg, gin.H{"projects": projects})
	}
}
