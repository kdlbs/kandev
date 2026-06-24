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
	api.GET("/instances", c.httpListInstances)
	api.POST("/instances", c.httpCreateInstance)
	api.GET("/instances/:id", c.httpGetInstance)
	api.PUT("/instances/:id", c.httpUpdateInstance)
	api.DELETE("/instances/:id", c.httpDeleteInstance)
	api.POST("/instances/:id/test", c.httpTestInstance)
	// Test unsaved credentials before the instance is created.
	api.POST("/test-connection", c.httpTestInstance)
	api.GET("/organizations", c.httpListOrganizations)
	api.GET("/projects", c.httpListProjects)
	api.GET("/issues", c.httpSearchIssues)
	api.GET("/issues/:id", c.httpGetIssue)
	c.registerIssueWatchRoutes(api)
}

func (c *Controller) httpListInstances(ctx *gin.Context) {
	instances, err := c.service.ListInstances(ctx.Request.Context())
	if err != nil {
		respondError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"instances": instances})
}

func (c *Controller) httpGetInstance(ctx *gin.Context) {
	cfg, err := c.service.GetInstance(ctx.Request.Context(), ctx.Param("id"))
	if err != nil {
		respondError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	if cfg == nil {
		respondErrorCode(ctx, http.StatusNotFound, "Sentry instance not found", errCodeSentryInstanceNotFound)
		return
	}
	ctx.JSON(http.StatusOK, cfg)
}

func (c *Controller) httpCreateInstance(ctx *gin.Context) {
	var req SetConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, msgInvalidPayload)
		return
	}
	cfg, err := c.service.CreateInstance(ctx.Request.Context(), &req)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrInvalidConfig) {
			status = http.StatusBadRequest
		}
		respondError(ctx, status, err.Error())
		return
	}
	ctx.JSON(http.StatusCreated, cfg)
}

func (c *Controller) httpUpdateInstance(ctx *gin.Context) {
	var req SetConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, msgInvalidPayload)
		return
	}
	cfg, err := c.service.UpdateInstance(ctx.Request.Context(), ctx.Param("id"), &req)
	if err != nil {
		switch {
		case errors.Is(err, ErrInstanceNotFound):
			respondErrorCode(ctx, http.StatusNotFound, err.Error(), errCodeSentryInstanceNotFound)
		case errors.Is(err, ErrInvalidConfig):
			respondError(ctx, http.StatusBadRequest, err.Error())
		default:
			respondError(ctx, http.StatusInternalServerError, err.Error())
		}
		return
	}
	ctx.JSON(http.StatusOK, cfg)
}

func (c *Controller) httpDeleteInstance(ctx *gin.Context) {
	err := c.service.DeleteInstance(ctx.Request.Context(), ctx.Param("id"))
	if err != nil {
		var inUse *ErrInstanceInUse
		if errors.As(err, &inUse) {
			respondErrorCode(ctx, http.StatusConflict, inUse.Error(), errCodeSentryInstanceInUse,
				gin.H{"watchCount": inUse.WatchCount})
			return
		}
		respondError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (c *Controller) httpTestInstance(ctx *gin.Context) {
	var req SetConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		respondError(ctx, http.StatusBadRequest, msgInvalidPayload)
		return
	}
	result, err := c.service.TestConnection(ctx.Request.Context(), ctx.Param("id"), &req)
	if err != nil {
		respondError(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (c *Controller) httpListOrganizations(ctx *gin.Context) {
	instanceID := ctx.Query("instanceId")
	if instanceID == "" {
		c.writeMissingInstance(ctx)
		return
	}
	organizations, err := c.service.ListOrganizations(ctx.Request.Context(), instanceID)
	if err != nil {
		c.writeClientError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"organizations": organizations})
}

func (c *Controller) httpListProjects(ctx *gin.Context) {
	instanceID := ctx.Query("instanceId")
	if instanceID == "" {
		c.writeMissingInstance(ctx)
		return
	}
	projects, err := c.service.ListProjects(ctx.Request.Context(), instanceID)
	if err != nil {
		c.writeClientError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"projects": projects})
}

func (c *Controller) httpSearchIssues(ctx *gin.Context) {
	q := ctx.Request.URL.Query()
	instanceID := q.Get("instanceId")
	if instanceID == "" {
		c.writeMissingInstance(ctx)
		return
	}
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
	result, err := c.service.SearchIssues(ctx.Request.Context(), instanceID, filter, cursor)
	if err != nil {
		c.writeClientError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (c *Controller) httpGetIssue(ctx *gin.Context) {
	instanceID := ctx.Query("instanceId")
	if instanceID == "" {
		c.writeMissingInstance(ctx)
		return
	}
	id := ctx.Param("id")
	issue, err := c.service.GetIssue(ctx.Request.Context(), instanceID, id)
	if err != nil {
		c.writeClientError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, issue)
}

// msgInvalidPayload is the response message for an unparseable request body.
const msgInvalidPayload = "invalid payload"

// Wire-level error codes surfaced to the UI.
const (
	// errCodeSentryNotConfigured: the targeted instance has no saved credentials.
	errCodeSentryNotConfigured = "SENTRY_NOT_CONFIGURED"
	// errCodeSentryInstanceNotFound: no instance matches the supplied id.
	errCodeSentryInstanceNotFound = "SENTRY_INSTANCE_NOT_FOUND"
	// errCodeSentryInstanceInUse: instance delete blocked by dependent watches.
	errCodeSentryInstanceInUse = "SENTRY_INSTANCE_IN_USE"
	// errCodeSentryInstanceRequired: a browse call omitted the instanceId param.
	errCodeSentryInstanceRequired = "SENTRY_INSTANCE_REQUIRED"
)

// respondError writes a plain {"error": msg} body with the given status.
func respondError(ctx *gin.Context, status int, msg string) {
	//nolint:goconst // "error" is the standard gin JSON error key; a constant for a response field name would obscure the wire shape.
	ctx.JSON(status, gin.H{"error": msg})
}

// respondErrorCode writes {"error": msg, "code": code} plus any extra fields
// (e.g. watchCount) with the given status.
func respondErrorCode(ctx *gin.Context, status int, msg, code string, extra ...gin.H) {
	//nolint:goconst // "error"/"code" are standard gin JSON response keys; constants would obscure the wire shape.
	body := gin.H{"error": msg, "code": code}
	if len(extra) > 0 {
		for k, v := range extra[0] {
			body[k] = v
		}
	}
	ctx.JSON(status, body)
}

// writeMissingInstance responds 400 when a browse endpoint is called without the
// required instanceId query parameter.
func (c *Controller) writeMissingInstance(ctx *gin.Context) {
	respondErrorCode(ctx, http.StatusBadRequest,
		"instanceId query parameter is required", errCodeSentryInstanceRequired)
}

func (c *Controller) writeClientError(ctx *gin.Context, err error) {
	if errors.Is(err, ErrNotConfigured) {
		respondErrorCode(ctx, http.StatusServiceUnavailable, "Sentry is not configured", errCodeSentryNotConfigured)
		return
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		status := http.StatusInternalServerError
		switch apiErr.StatusCode {
		case http.StatusNotFound, http.StatusUnauthorized, http.StatusForbidden, http.StatusBadRequest:
			status = apiErr.StatusCode
		}
		respondError(ctx, status, apiErr.Error())
		return
	}
	c.logger.Warn("sentry handler error", zap.Error(err))
	respondError(ctx, http.StatusInternalServerError, err.Error())
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
