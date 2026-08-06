package workflowsync

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// workspaceDenied reports whether err is the per-user workspace access
// denial surfaced by the workspace authorizer. Handlers map it to 404 so a
// workspace the caller may not access is indistinguishable from a missing
// one — the same convention used by every other integration in this
// codebase (see internal/jira/handlers.go).
func workspaceDenied(err error) bool {
	return errors.Is(err, repoerrors.ErrWorkspaceNotFound)
}

// Controller holds HTTP route handlers for workflow sync.
type Controller struct {
	service *Service
	logger  *logger.Logger
}

// RegisterRoutes wires the workflow sync HTTP endpoints.
func RegisterRoutes(router *gin.Engine, svc *Service, log *logger.Logger) {
	ctrl := &Controller{service: svc, logger: log}
	api := router.Group("/api/v1/workflow-sync")
	api.GET("/config", ctrl.httpGetConfig)
	api.POST("/config", ctrl.httpSetConfig)
	api.DELETE("/config", ctrl.httpDeleteConfig)
	api.POST("/sync", ctrl.httpForceSync)
}

func (c *Controller) workspaceID(ctx *gin.Context) string {
	return strings.TrimSpace(ctx.Query("workspace_id"))
}

func (c *Controller) requireWorkspaceID(ctx *gin.Context) (string, bool) {
	id := c.workspaceID(ctx)
	if id == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return "", false
	}
	return id, true
}

func (c *Controller) httpGetConfig(ctx *gin.Context) {
	workspaceID, ok := c.requireWorkspaceID(ctx)
	if !ok {
		return
	}
	cfg, err := c.service.GetConfigForWorkspace(ctx.Request.Context(), workspaceID)
	if workspaceDenied(err) {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return
	}
	if err != nil {
		c.internalError(ctx, "failed to load workflow sync config", err)
		return
	}
	if cfg == nil {
		ctx.Status(http.StatusNoContent)
		return
	}
	ctx.JSON(http.StatusOK, cfg)
}

// internalError logs the underlying failure and returns a generic message so
// driver/query details never leak to clients.
func (c *Controller) internalError(ctx *gin.Context, msg string, err error) {
	c.logger.Error(msg, zap.Error(err))
	ctx.JSON(http.StatusInternalServerError, gin.H{"error": msg})
}

func (c *Controller) httpSetConfig(ctx *gin.Context) {
	workspaceID, ok := c.requireWorkspaceID(ctx)
	if !ok {
		return
	}
	var req SetConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	cfg, err := c.service.SetConfigForWorkspace(ctx.Request.Context(), workspaceID, &req)
	if workspaceDenied(err) {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return
	}
	if errors.Is(err, ErrInvalidConfig) {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		c.internalError(ctx, "failed to save workflow sync config", err)
		return
	}
	ctx.JSON(http.StatusOK, cfg)
}

func (c *Controller) httpDeleteConfig(ctx *gin.Context) {
	workspaceID, ok := c.requireWorkspaceID(ctx)
	if !ok {
		return
	}
	err := c.service.DeleteConfigForWorkspace(ctx.Request.Context(), workspaceID)
	if workspaceDenied(err) {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return
	}
	if err != nil {
		c.internalError(ctx, "failed to remove workflow sync config", err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

// httpForceSync runs a sync immediately instead of waiting for the poller
// (every sync — manual or periodic — reconciles, repairing local drift). Sync
// failures still return 200 with the error embedded — the outcome is also
// recorded on the config row, which is returned for the status banner.
func (c *Controller) httpForceSync(ctx *gin.Context) {
	workspaceID, ok := c.requireWorkspaceID(ctx)
	if !ok {
		return
	}
	result, syncErr := c.service.SyncWorkspace(ctx.Request.Context(), workspaceID)
	if workspaceDenied(syncErr) {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return
	}
	if errors.Is(syncErr, ErrNotConfigured) {
		ctx.JSON(http.StatusNotFound, gin.H{"error": syncErr.Error()})
		return
	}
	cfg, err := c.service.GetConfigForWorkspace(ctx.Request.Context(), workspaceID)
	if err != nil {
		c.internalError(ctx, "failed to load workflow sync config", err)
		return
	}
	response := gin.H{"config": cfg}
	if syncErr != nil {
		response["error"] = syncErr.Error()
	}
	if result != nil {
		response["result"] = result
	}
	ctx.JSON(http.StatusOK, response)
}
