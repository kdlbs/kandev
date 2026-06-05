package sentry

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// RegisterRoutes wires the Sentry HTTP handlers. The dispatcher parameter is
// accepted for signature parity with other integrations; Phase 1 has no
// WebSocket surface, so it is intentionally unused.
func RegisterRoutes(router *gin.Engine, _ *ws.Dispatcher, svc *Service, log *logger.Logger) {
	ctrl := &Controller{service: svc, logger: log}
	ctrl.RegisterHTTPRoutes(router)
}

// Controller holds HTTP route handlers for the Sentry integration.
type Controller struct {
	service *Service
	logger  *logger.Logger
}

// RegisterHTTPRoutes attaches the Sentry HTTP endpoints to router.
func (c *Controller) RegisterHTTPRoutes(router *gin.Engine) {
	api := router.Group("/api/v1/sentry")
	api.GET("/config", c.httpGetConfig)
	api.PUT("/config", c.httpSetConfig)
	api.DELETE("/config", c.httpDeleteConfig)
	api.POST("/config/test", c.httpTestConfig)
	api.GET("/organizations", c.httpListOrganizations)
	api.GET("/projects", c.httpListProjects)
	api.GET("/issues", c.httpSearchIssues)
	api.GET("/issues/:id", c.httpGetIssue)
	c.registerIssueWatchRoutes(api)
}

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

func (c *Controller) httpListOrganizations(ctx *gin.Context) {
	organizations, err := c.service.ListOrganizations(ctx.Request.Context())
	if err != nil {
		c.writeClientError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"organizations": organizations})
}

func (c *Controller) httpListProjects(ctx *gin.Context) {
	projects, err := c.service.ListProjects(ctx.Request.Context())
	if err != nil {
		c.writeClientError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"projects": projects})
}

func (c *Controller) httpSearchIssues(ctx *gin.Context) {
	q := ctx.Request.URL.Query()
	filter := SearchFilter{
		OrgSlug:     q.Get("orgSlug"),
		ProjectSlug: q.Get("projectSlug"),
		Environment: q.Get("environment"),
		Query:       q.Get("query"),
		StatsPeriod: q.Get("statsPeriod"),
		Levels:      trimAll(q["level"]),
		Statuses:    trimAll(q["status"]),
	}
	cursor := q.Get("cursor")
	result, err := c.service.SearchIssues(ctx.Request.Context(), filter, cursor)
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

// errCodeSentryNotConfigured is the wire-level code surfaced to the UI when
// Sentry has no saved credentials.
const errCodeSentryNotConfigured = "SENTRY_NOT_CONFIGURED"

func (c *Controller) writeClientError(ctx *gin.Context, err error) {
	if errors.Is(err, ErrNotConfigured) {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Sentry is not configured",
			"code":  errCodeSentryNotConfigured,
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
	c.logger.Warn("sentry handler error", zap.Error(err))
	ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

func trimAll(xs []string) []string {
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		x = strings.TrimSpace(x)
		if x != "" {
			out = append(out, x)
		}
	}
	return out
}
