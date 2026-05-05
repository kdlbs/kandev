package linear

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// RegisterRoutes wires the Linear HTTP and WebSocket handlers.
func RegisterRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, svc *Service, log *logger.Logger) {
	ctrl := &Controller{service: svc, logger: log}
	ctrl.RegisterHTTPRoutes(router)
	registerWSHandlers(dispatcher, svc)
}

// Controller holds HTTP route handlers for the Linear integration.
type Controller struct {
	service *Service
	logger  *logger.Logger
}

// RegisterHTTPRoutes attaches the Linear HTTP endpoints to router.
func (c *Controller) RegisterHTTPRoutes(router *gin.Engine) {
	api := router.Group("/api/v1/linear")
	api.GET("/config", c.httpGetConfig)
	api.POST("/config", c.httpSetConfig)
	api.DELETE("/config", c.httpDeleteConfig)
	api.POST("/config/test", c.httpTestConfig)
	api.GET("/teams", c.httpListTeams)
	api.GET("/states", c.httpListStates)
	api.GET("/issues", c.httpSearchIssues)
	api.GET("/issues/:id", c.httpGetIssue)
	api.POST("/issues/:id/state", c.httpSetIssueState)

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

func (c *Controller) httpListTeams(ctx *gin.Context) {
	teams, err := c.service.ListTeams(ctx.Request.Context())
	if err != nil {
		c.writeClientError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"teams": teams})
}

func (c *Controller) httpListStates(ctx *gin.Context) {
	teamKey := ctx.Query("team_key")
	if teamKey == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "team_key required"})
		return
	}
	states, err := c.service.ListStates(ctx.Request.Context(), teamKey)
	if err != nil {
		c.writeClientError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"states": states})
}

func (c *Controller) httpSearchIssues(ctx *gin.Context) {
	filter := SearchFilter{
		Query:    ctx.Query("query"),
		TeamKey:  ctx.Query("team_key"),
		Assigned: ctx.Query("assigned"),
	}
	if states := ctx.Query("state_ids"); states != "" {
		filter.StateIDs = splitCSV(states)
	}
	pageToken := ctx.Query("page_token")
	maxResults, _ := strconv.Atoi(ctx.Query("max_results"))
	result, err := c.service.SearchIssues(ctx.Request.Context(), filter, pageToken, maxResults)
	if err != nil {
		c.writeClientError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (c *Controller) httpGetIssue(ctx *gin.Context) {
	id := ctx.Param("id")
	issue, err := c.service.GetIssue(ctx.Request.Context(), id)
	if err != nil {
		c.writeClientError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, issue)
}

func (c *Controller) httpSetIssueState(ctx *gin.Context) {
	id := ctx.Param("id")
	var req struct {
		StateID string `json:"stateId"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil || req.StateID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "stateId required"})
		return
	}
	if err := c.service.SetIssueState(ctx.Request.Context(), id, req.StateID); err != nil {
		c.writeClientError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"transitioned": true})
}

// errCodeLinearNotConfigured is the wire-level code surfaced to the UI when
// Linear has no saved credentials.
const errCodeLinearNotConfigured = "LINEAR_NOT_CONFIGURED"

// writeClientError maps service-level errors to HTTP responses.
func (c *Controller) writeClientError(ctx *gin.Context, err error) {
	if errors.Is(err, ErrNotConfigured) {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Linear is not configured",
			"code":  errCodeLinearNotConfigured,
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
	c.logger.Warn("linear handler error", zap.Error(err))
	ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := parts[:0]
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// --- WebSocket handlers ---

func registerWSHandlers(dispatcher *ws.Dispatcher, svc *Service) {
	dispatcher.RegisterFunc(ws.ActionLinearConfigGet, wsGetConfig(svc))
	dispatcher.RegisterFunc(ws.ActionLinearConfigSet, wsSetConfig(svc))
	dispatcher.RegisterFunc(ws.ActionLinearConfigDelete, wsDeleteConfig(svc))
	dispatcher.RegisterFunc(ws.ActionLinearConfigTest, wsTestConfig(svc))
	dispatcher.RegisterFunc(ws.ActionLinearIssueGet, wsGetIssue(svc))
	dispatcher.RegisterFunc(ws.ActionLinearIssueTransition, wsSetIssueState(svc))
	dispatcher.RegisterFunc(ws.ActionLinearTeamsList, wsListTeams(svc))
}

func wsReply(msg *ws.Message, payload interface{}) (*ws.Message, error) {
	resp, err := ws.NewResponse(msg.ID, msg.Action, payload)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	return resp, nil
}

func wsFail(msg *ws.Message, err error) (*ws.Message, error) {
	if errors.Is(err, ErrNotConfigured) {
		return ws.NewError(msg.ID, msg.Action, errCodeLinearNotConfigured, err.Error(), nil)
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

func wsGetIssue(svc *Service) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var p struct {
			Identifier string `json:"identifier"`
		}
		if err := msg.ParsePayload(&p); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload", nil)
		}
		if p.Identifier == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "identifier required", nil)
		}
		issue, err := svc.GetIssue(ctx, p.Identifier)
		if err != nil {
			return wsFail(msg, err)
		}
		return wsReply(msg, issue)
	}
}

func wsSetIssueState(svc *Service) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		var p struct {
			IssueID string `json:"issueId"`
			StateID string `json:"stateId"`
		}
		if err := msg.ParsePayload(&p); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload", nil)
		}
		if p.IssueID == "" || p.StateID == "" {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "issueId and stateId required", nil)
		}
		if err := svc.SetIssueState(ctx, p.IssueID, p.StateID); err != nil {
			return wsFail(msg, err)
		}
		return wsReply(msg, gin.H{"transitioned": true})
	}
}

func wsListTeams(svc *Service) func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	return func(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
		teams, err := svc.ListTeams(ctx)
		if err != nil {
			return wsFail(msg, err)
		}
		return wsReply(msg, gin.H{"teams": teams})
	}
}
