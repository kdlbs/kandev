package gitlab

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func (c *Controller) registerConfigRoutes(api *gin.RouterGroup) {
	api.GET("/config", c.httpGetConfig)
	api.PUT("/config", c.httpSetConfig)
	api.DELETE("/config", c.httpDeleteConfig)
	api.POST("/config/test", c.httpTestConfig)
	api.POST("/config/copy", c.httpCopyConfig)
}

func (c *Controller) httpCopyConfig(ctx *gin.Context) {
	var req struct {
		TargetWorkspaceID string `json:"targetWorkspaceId"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.TargetWorkspaceID) == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "targetWorkspaceId required"})
		return
	}
	cfg, err := c.service.CopyConfigToWorkspace(ctx.Request.Context(), c.workspaceID(ctx), req.TargetWorkspaceID)
	if err != nil {
		writeConfigMutationError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, cfg)
}

func (c *Controller) workspaceID(ctx *gin.Context) string {
	return strings.TrimSpace(ctx.Query("workspace_id"))
}

func (c *Controller) workspaceClient(ctx *gin.Context) (Client, bool) {
	workspaceID := c.workspaceID(ctx)
	if workspaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "workspace_id query parameter required"})
		return nil, false
	}
	client, err := c.service.ClientForWorkspaceHost(
		ctx.Request.Context(), workspaceID, ctx.Query("expected_host"),
	)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrWorkspaceHostMismatch) {
			status = http.StatusNotFound
		} else if errors.Is(err, ErrNotConfigured) {
			status = http.StatusServiceUnavailable
		}
		message := providerActionFailedMessage + ": connection resolution"
		if status == http.StatusNotFound {
			message = "GitLab resource not found"
		}
		ctx.JSON(status, gin.H{responseErrorKey: message})
		return nil, false
	}
	return client, true
}

func (c *Controller) runWorkspaceClientAction(ctx *gin.Context, action func(Client) error) error {
	return c.service.RunWithWorkspaceClient(
		ctx.Request.Context(), c.workspaceID(ctx), ctx.Query("expected_host"), action,
	)
}

func writeWorkspaceClientActionError(ctx *gin.Context, err error, action string) {
	switch {
	case errors.Is(err, ErrWorkspaceRequired):
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "workspace_id query parameter required"})
	case errors.Is(err, ErrNotConfigured):
		ctx.JSON(http.StatusServiceUnavailable, gin.H{responseErrorKey: providerActionFailedMessage + ": connection resolution"})
	default:
		writeProviderActionError(ctx, err, action)
	}
}

func (c *Controller) httpSetConfig(ctx *gin.Context) {
	var req SetConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "invalid payload"})
		return
	}
	cfg, err := c.service.SetConfigForWorkspace(ctx.Request.Context(), c.workspaceID(ctx), &req)
	if err != nil {
		writeConfigMutationError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, cfg)
}

func (c *Controller) httpDeleteConfig(ctx *gin.Context) {
	if err := c.service.DeleteConfigForWorkspace(ctx.Request.Context(), c.workspaceID(ctx)); err != nil {
		writeConfigMutationError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (c *Controller) httpTestConfig(ctx *gin.Context) {
	var req SetConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "invalid payload"})
		return
	}
	result := c.service.TestConfigForWorkspace(ctx.Request.Context(), c.workspaceID(ctx), &req)
	ctx.JSON(http.StatusOK, result)
}

func (c *Controller) httpGetConfig(ctx *gin.Context) {
	cfg, err := c.service.GetConfigForWorkspace(ctx.Request.Context(), c.workspaceID(ctx))
	if err != nil {
		writeConfigMutationError(ctx, err)
		return
	}
	if cfg == nil {
		ctx.Status(http.StatusNoContent)
		return
	}
	ctx.JSON(http.StatusOK, cfg)
}

func writeConfigMutationError(ctx *gin.Context, err error) {
	status := http.StatusInternalServerError
	message := "GitLab connection operation failed"
	switch {
	case errors.Is(err, ErrWorkspaceRequired):
		status = http.StatusBadRequest
		message = "workspace_id query parameter required"
	case errors.Is(err, ErrInvalidConfig):
		status = http.StatusBadRequest
		message = "invalid GitLab connection configuration"
	case errors.Is(err, ErrSameWorkspace):
		status = http.StatusBadRequest
		message = "source and target workspace must differ"
	case errors.Is(err, ErrNothingToCopy):
		status = http.StatusBadRequest
		message = "source workspace has no GitLab connection"
	}
	ctx.JSON(status, gin.H{responseErrorKey: message})
}
