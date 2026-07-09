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
// accepted for signature parity with other integrations; there is no WebSocket
// surface, so it is intentionally unused.
func RegisterRoutes(router *gin.Engine, _ *ws.Dispatcher, svc *Service, log *logger.Logger) {
	ctrl := &Controller{service: svc, logger: log}
	ctrl.RegisterHTTPRoutes(router)
}

// Controller holds HTTP route handlers for the Sentry integration.
type Controller struct {
	service *Service
	logger  *logger.Logger
}

// RegisterHTTPRoutes attaches the Sentry HTTP endpoints to router. Every route
// is scoped to an authoritative `?workspace_id=` query param; instance routes
// additionally 404 when the instance does not belong to that workspace.
func (c *Controller) RegisterHTTPRoutes(router *gin.Engine) {
	api := router.Group("/api/v1/sentry")
	api.GET("/instances", c.httpListInstances)
	api.POST("/instances", c.httpCreateInstance)
	api.GET("/instances/:id", c.httpGetInstance)
	api.PUT("/instances/:id", c.httpUpdateInstance)
	api.DELETE("/instances/:id", c.httpDeleteInstance)
	api.POST("/instances/:id/test", c.httpTestInstance)
	api.POST("/test-connection", c.httpTestConnection)
	api.POST("/config/copy", c.httpCopyConfig)
	api.GET("/organizations", c.httpListOrganizations)
	api.GET("/projects", c.httpListProjects)
	api.GET("/issues", c.httpSearchIssues)
	api.GET("/issues/:id", c.httpGetIssue)
	c.registerIssueWatchRoutes(api)
}

func (c *Controller) httpListInstances(ctx *gin.Context) {
	workspaceID, ok := c.requireWorkspace(ctx)
	if !ok {
		return
	}
	instances, err := c.service.ListInstances(ctx.Request.Context(), workspaceID)
	if err != nil {
		writeErr(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"instances": instances})
}

func (c *Controller) httpCreateInstance(ctx *gin.Context) {
	workspaceID, ok := c.requireWorkspace(ctx)
	if !ok {
		return
	}
	var req CreateConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		writeBadPayload(ctx)
		return
	}
	if strings.TrimSpace(req.WorkspaceID) != "" && req.WorkspaceID != workspaceID {
		writeErr(ctx, http.StatusBadRequest, "workspaceId does not match workspace_id query")
		return
	}
	cfg, err := c.service.CreateInstance(ctx.Request.Context(), workspaceID, &req)
	if err != nil {
		c.writeInstanceError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, cfg)
}

func (c *Controller) httpGetInstance(ctx *gin.Context) {
	workspaceID, ok := c.requireWorkspace(ctx)
	if !ok {
		return
	}
	cfg, err := c.service.GetInstance(ctx.Request.Context(), workspaceID, ctx.Param("id"))
	if err != nil {
		c.writeInstanceError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, cfg)
}

func (c *Controller) httpUpdateInstance(ctx *gin.Context) {
	workspaceID, ok := c.requireWorkspace(ctx)
	if !ok {
		return
	}
	var req UpdateConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		writeBadPayload(ctx)
		return
	}
	cfg, err := c.service.UpdateInstance(ctx.Request.Context(), workspaceID, ctx.Param("id"), &req)
	if err != nil {
		c.writeInstanceError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, cfg)
}

func (c *Controller) httpDeleteInstance(ctx *gin.Context) {
	workspaceID, ok := c.requireWorkspace(ctx)
	if !ok {
		return
	}
	if err := c.service.DeleteInstance(ctx.Request.Context(), workspaceID, ctx.Param("id")); err != nil {
		c.writeInstanceError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (c *Controller) httpTestInstance(ctx *gin.Context) {
	workspaceID, ok := c.requireWorkspace(ctx)
	if !ok {
		return
	}
	result, err := c.service.TestInstance(ctx.Request.Context(), workspaceID, ctx.Param("id"))
	if err != nil {
		c.writeInstanceError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (c *Controller) httpTestConnection(ctx *gin.Context) {
	if _, ok := c.requireWorkspace(ctx); !ok {
		return
	}
	var req CreateConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		writeBadPayload(ctx)
		return
	}
	result, err := c.service.TestConnectionCandidate(ctx.Request.Context(), &req)
	if err != nil {
		writeErr(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (c *Controller) httpCopyConfig(ctx *gin.Context) {
	var req CopyConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		writeBadPayload(ctx)
		return
	}
	source := strings.TrimSpace(req.SourceWorkspaceID)
	target := strings.TrimSpace(req.TargetWorkspaceID)
	if source == "" || target == "" {
		writeErr(ctx, http.StatusBadRequest, "sourceWorkspaceId and targetWorkspaceId are required")
		return
	}
	copied, err := c.service.CopyConfigToWorkspace(ctx.Request.Context(), source, target)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ErrSameWorkspace), errors.Is(err, ErrNothingToCopy), errors.Is(err, ErrInvalidConfig):
			status = http.StatusBadRequest
		case errors.Is(err, ErrDuplicateInstanceName):
			status = http.StatusConflict
		}
		writeErr(ctx, status, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"instances": copied})
}

func (c *Controller) httpListOrganizations(ctx *gin.Context) {
	workspaceID, ok := c.requireWorkspace(ctx)
	if !ok {
		return
	}
	organizations, err := c.service.ListOrganizations(ctx.Request.Context(), workspaceID, c.instanceID(ctx))
	if err != nil {
		c.writeInstanceError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"organizations": organizations})
}

func (c *Controller) httpListProjects(ctx *gin.Context) {
	workspaceID, ok := c.requireWorkspace(ctx)
	if !ok {
		return
	}
	projects, err := c.service.ListProjects(ctx.Request.Context(), workspaceID, c.instanceID(ctx))
	if err != nil {
		c.writeInstanceError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"projects": projects})
}

func (c *Controller) httpSearchIssues(ctx *gin.Context) {
	workspaceID, ok := c.requireWorkspace(ctx)
	if !ok {
		return
	}
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
	result, err := c.service.SearchIssues(ctx.Request.Context(), workspaceID, c.instanceID(ctx), filter, q.Get("cursor"))
	if err != nil {
		c.writeInstanceError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (c *Controller) httpGetIssue(ctx *gin.Context) {
	workspaceID, ok := c.requireWorkspace(ctx)
	if !ok {
		return
	}
	issue, err := c.service.GetIssue(ctx.Request.Context(), workspaceID, c.instanceID(ctx), ctx.Param("id"))
	if err != nil {
		c.writeInstanceError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, issue)
}

// requireWorkspace reads the authoritative `?workspace_id=` query param,
// writing a 400 and returning false when it is absent.
func (c *Controller) requireWorkspace(ctx *gin.Context) (string, bool) {
	workspaceID := strings.TrimSpace(ctx.Query("workspace_id"))
	if workspaceID == "" {
		writeErr(ctx, http.StatusBadRequest, "workspace_id query parameter required")
		return "", false
	}
	return workspaceID, true
}

func (c *Controller) instanceID(ctx *gin.Context) string {
	return strings.TrimSpace(ctx.Query("instanceId"))
}

// Wire-level error codes surfaced to the UI.
const (
	errCodeSentryNotConfigured     = "SENTRY_NOT_CONFIGURED"
	errCodeSentryInstanceRequired  = "SENTRY_INSTANCE_REQUIRED"
	errCodeSentryInstanceNotFound  = "SENTRY_INSTANCE_NOT_FOUND"
	errCodeSentryInstanceInUse     = "SENTRY_INSTANCE_IN_USE"
	errCodeSentryInstanceNameTaken = "SENTRY_INSTANCE_NAME_TAKEN"
)

// Repeated gin.H keys / messages, centralized to satisfy goconst.
const (
	jsonKeyError  = "error"
	jsonKeyCode   = "code"
	badPayloadMsg = "invalid payload"
)

// writeErr responds with a bare error message at the given status.
func writeErr(ctx *gin.Context, status int, msg string) {
	ctx.JSON(status, gin.H{jsonKeyError: msg})
}

// writeBadPayload responds with a 400 for an unparseable JSON body.
func writeBadPayload(ctx *gin.Context) {
	writeErr(ctx, http.StatusBadRequest, badPayloadMsg)
}

// writeCoded responds with an error message + wire code at the given status.
func writeCoded(ctx *gin.Context, status int, msg, code string) {
	ctx.JSON(status, gin.H{jsonKeyError: msg, jsonKeyCode: code})
}

// writeInstanceError maps the instance-scoped service errors onto HTTP status
// codes + wire codes, falling back to writeClientError for connection/API
// errors (ErrNotConfigured, APIError, generic).
func (c *Controller) writeInstanceError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrInstanceRequired):
		writeCoded(ctx, http.StatusBadRequest, err.Error(), errCodeSentryInstanceRequired)
		return
	case errors.Is(err, ErrInstanceNotFound):
		writeCoded(ctx, http.StatusNotFound, err.Error(), errCodeSentryInstanceNotFound)
		return
	case errors.Is(err, ErrDuplicateInstanceName):
		writeCoded(ctx, http.StatusConflict, err.Error(), errCodeSentryInstanceNameTaken)
		return
	case errors.Is(err, ErrInvalidConfig):
		writeErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	var inUse ErrInstanceInUse
	if errors.As(err, &inUse) {
		ctx.JSON(http.StatusConflict, gin.H{
			jsonKeyError: inUse.Error(),
			jsonKeyCode:  errCodeSentryInstanceInUse,
			"watchCount": inUse.WatchCount,
		})
		return
	}
	c.writeClientError(ctx, err)
}

func (c *Controller) writeClientError(ctx *gin.Context, err error) {
	if errors.Is(err, ErrNotConfigured) {
		writeCoded(ctx, http.StatusServiceUnavailable, "Sentry is not configured", errCodeSentryNotConfigured)
		return
	}
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		status := http.StatusInternalServerError
		switch apiErr.StatusCode {
		case http.StatusNotFound, http.StatusUnauthorized, http.StatusForbidden, http.StatusBadRequest:
			status = apiErr.StatusCode
		}
		writeErr(ctx, status, apiErr.Error())
		return
	}
	c.logger.Warn("sentry handler error", zap.Error(err))
	writeErr(ctx, http.StatusInternalServerError, err.Error())
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
