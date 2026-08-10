package forgejo

import (
	"errors"
	"net/http"
	"strconv"
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
	api.GET("/repositories", controller.listRepositories)
	api.GET("/issues", controller.listIssues)
	api.GET("/tasks/:taskID/pull-requests", controller.listTaskPRs)
	api.POST("/task-pull-requests", controller.associatePullRequest)
}

func (c *Controller) listTaskPRs(ctx *gin.Context) {
	prs, err := c.service.store.ListTaskPRs(ctx.Request.Context(), c.workspaceID(ctx), ctx.Param("taskID"))
	if err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"pull_requests": prs})
}

func (c *Controller) associatePullRequest(ctx *gin.Context) {
	var request struct {
		TaskID       string `json:"task_id"`
		RepositoryID string `json:"repository_id"`
		Owner        string `json:"owner"`
		Repo         string `json:"repo"`
		Number       int    `json:"number"`
	}
	if err := ctx.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.TaskID) == "" || strings.TrimSpace(request.Owner) == "" || strings.TrimSpace(request.Repo) == "" || request.Number < 1 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "task_id, owner, repo, and number are required"})
		return
	}
	pr, err := c.service.AssociatePullRequest(ctx.Request.Context(), c.workspaceID(ctx), request.TaskID, request.RepositoryID, request.Owner, request.Repo, request.Number)
	if err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, pr)
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

func (c *Controller) listRepositories(ctx *gin.Context) {
	repositories, total, err := c.service.ListRepositories(ctx.Request.Context(), c.workspaceID(ctx), queryPage(ctx), queryLimit(ctx))
	if err != nil {
		c.error(ctx, err)
		return
	}
	ctx.Header("X-Total-Count", strconv.Itoa(total))
	ctx.JSON(http.StatusOK, gin.H{"repositories": repositories, "total_count": total})
}

func (c *Controller) listIssues(ctx *gin.Context) {
	issues, total, err := c.service.ListIssues(
		ctx.Request.Context(), c.workspaceID(ctx), strings.TrimSpace(ctx.Query("owner")), strings.TrimSpace(ctx.Query("repo")), queryPage(ctx), queryLimit(ctx),
	)
	if err != nil {
		c.error(ctx, err)
		return
	}
	ctx.Header("X-Total-Count", strconv.Itoa(total))
	ctx.JSON(http.StatusOK, gin.H{"issues": issues, "total_count": total})
}

func queryPage(ctx *gin.Context) int {
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	if page < 1 {
		return 1
	}
	return page
}

func queryLimit(ctx *gin.Context) int {
	limit, _ := strconv.Atoi(ctx.DefaultQuery("limit", "30"))
	if limit < 1 {
		return 30
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func (c *Controller) error(ctx *gin.Context, err error) {
	if errors.Is(err, ErrWorkspaceRequired) {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id query parameter required"})
		return
	}
	if errors.Is(err, ErrNotConfigured) {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "Forgejo workspace is not configured"})
		return
	}
	if strings.Contains(err.Error(), "owner and repository are required") {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "owner and repo query parameters required"})
		return
	}
	ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Forgejo connection operation failed"})
}
