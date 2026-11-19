package promptsync

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type controller struct {
	service *Service
	logger  *logger.Logger
}

// RegisterRoutes mounts the prompt sync HTTP API. Reads follow saved-prompt
// read access; mutations and Sync now require configuration management.
func RegisterRoutes(router *gin.Engine, svc *Service, log *logger.Logger) {
	ctrl := &controller{service: svc, logger: log.WithFields(zap.String("component", "promptsync-handlers"))}
	api := router.Group("/api/v1/prompt-sync")
	manageConfig := authz.RequireOrgScope(authz.ScopeOrgConfigManage)
	api.GET("/config", ctrl.httpGetConfig)
	api.POST("/config", manageConfig, ctrl.httpSetConfig)
	api.DELETE("/config", manageConfig, ctrl.httpDeleteConfig)
	api.POST("/sync", manageConfig, ctrl.httpSync)
}

func workspaceDenied(err error) bool {
	return errors.Is(err, repoerrors.ErrWorkspaceNotFound)
}

func (c *controller) internalError(ctx *gin.Context, msg string, err error) {
	c.logger.Error(msg, zap.Error(err))
	ctx.JSON(http.StatusInternalServerError, gin.H{"error": msg})
}

func (c *controller) httpGetConfig(ctx *gin.Context) {
	cfg, err := c.service.GetConfig(ctx.Request.Context())
	if err != nil {
		c.internalError(ctx, "failed to load prompt sync config", err)
		return
	}
	if cfg == nil {
		ctx.Status(http.StatusNoContent)
		return
	}
	ctx.JSON(http.StatusOK, cfg)
}

func (c *controller) httpSetConfig(ctx *gin.Context) {
	var req SetConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	cfg, err := c.service.SetConfig(ctx.Request.Context(), &req)
	switch {
	case workspaceDenied(err):
		ctx.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
	case errors.Is(err, ErrInvalidConfig):
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case err != nil:
		c.internalError(ctx, "failed to save prompt sync config", err)
	default:
		ctx.JSON(http.StatusOK, cfg)
	}
}

func (c *controller) httpDeleteConfig(ctx *gin.Context) {
	err := c.service.DeleteConfig(ctx.Request.Context())
	switch {
	case workspaceDenied(err):
		ctx.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
	case err != nil:
		c.internalError(ctx, "failed to remove prompt sync config", err)
	default:
		ctx.JSON(http.StatusOK, gin.H{"deleted": true})
	}
}

// httpSync runs one sync. A completed run returns 200 even when it carries a
// sanitized error, so the client always receives the current status.
func (c *controller) httpSync(ctx *gin.Context) {
	result, syncErr := c.service.Sync(ctx.Request.Context())
	if ctx.Request.Context().Err() != nil {
		return
	}
	if workspaceDenied(syncErr) {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
		return
	}
	if errors.Is(syncErr, ErrNotConfigured) {
		ctx.JSON(http.StatusNotFound, gin.H{"error": syncErr.Error()})
		return
	}
	cfg, err := c.service.GetConfig(ctx.Request.Context())
	if err != nil {
		c.internalError(ctx, "failed to load prompt sync config", err)
		return
	}
	response := gin.H{"config": cfg}
	if syncErr != nil {
		response["error"] = safeErrorMessage(syncErr)
	}
	if result != nil {
		response["result"] = result
	}
	ctx.JSON(http.StatusOK, response)
}
