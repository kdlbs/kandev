package forgejo

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type Controller struct {
	service *Service
}

func RegisterRoutes(router *gin.Engine, service *Service) {
	controller := &Controller{service: service}
	api := router.Group("/api/v1/forgejo")
	api.GET("/config", controller.getConfig)
	api.PUT("/config", controller.setConfig)
	api.POST("/config/test", controller.testConfig)
	api.DELETE("/config", controller.deleteConfig)
}

func (c *Controller) workspaceID(ctx *gin.Context) string {
	return strings.TrimSpace(ctx.Query("workspace_id"))
}

func (c *Controller) getConfig(ctx *gin.Context) {
	config, err := c.service.GetConfig(ctx.Request.Context(), c.workspaceID(ctx))
	if err != nil {
		c.error(ctx, err)
		return
	}
	if config == nil {
		ctx.Status(http.StatusNoContent)
		return
	}
	ctx.JSON(http.StatusOK, config)
}

func (c *Controller) setConfig(ctx *gin.Context) {
	var request SetConfigRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	config, err := c.service.SetConfig(ctx.Request.Context(), c.workspaceID(ctx), &request)
	if err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, config)
}

func (c *Controller) testConfig(ctx *gin.Context) {
	var request SetConfigRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	ctx.JSON(http.StatusOK, c.service.TestConfig(ctx.Request.Context(), &request))
}

func (c *Controller) deleteConfig(ctx *gin.Context) {
	if err := c.service.DeleteConfig(ctx.Request.Context(), c.workspaceID(ctx)); err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (c *Controller) error(ctx *gin.Context, err error) {
	if errors.Is(err, ErrWorkspaceRequired) {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id query parameter required"})
		return
	}
	ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Forgejo connection operation failed"})
}
