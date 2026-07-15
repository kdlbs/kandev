package workflowsync

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
)

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
	workspaceID, ok := c.requireWorkspaceID(ctx)
	if !ok {
		return
	}
	if err := c.service.DeleteConfigForWorkspace(ctx.Request.Context(), workspaceID); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
	if errors.Is(syncErr, ErrNotConfigured) {
		ctx.JSON(http.StatusNotFound, gin.H{"error": syncErr.Error()})
		return
	}
	cfg, err := c.service.GetConfigForWorkspace(ctx.Request.Context(), workspaceID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
