package azuredevops

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
)

const notConfiguredCode = "azure_devops_not_configured"

type Controller struct {
	service *Service
	log     *logger.Logger
}

func RegisterRoutes(router *gin.Engine, service *Service, log *logger.Logger) {
	controller := &Controller{service: service, log: log}
	api := router.Group("/api/v1/azure-devops")
	api.GET("/config", controller.getConfig)
	api.POST("/config", controller.setConfig)
	api.DELETE("/config", controller.deleteConfig)
	api.POST("/config/test", controller.testConfig)
	api.POST("/config/copy", controller.copyConfig)
	api.GET("/projects", controller.listProjects)
	api.GET("/repositories", controller.listRepositories)
	api.POST("/work-items/search", controller.searchWorkItems)
	api.GET("/work-items/:id", controller.getWorkItem)
	api.GET("/pull-requests", controller.listPullRequests)
	api.GET("/pull-requests/:projectId/:repositoryId/:pullRequestId", controller.getPullRequest)
	api.GET("/pull-requests/:projectId/:repositoryId/:pullRequestId/feedback", controller.getPullRequestFeedback)
	api.GET("/workspaces/:workspaceId/task-prs", controller.listWorkspaceTaskPRs)
	api.POST("/tasks/:taskId/pull-requests", controller.associateTaskPR)
	api.POST("/tasks/:taskId/pull-requests/sync", controller.syncTaskPR)
}

func (c *Controller) getConfig(ctx *gin.Context) {
	cfg, err := c.service.GetConfigForWorkspace(ctx.Request.Context(), workspaceID(ctx))
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	if cfg == nil {
		ctx.Status(http.StatusNoContent)
		return
	}
	ctx.JSON(http.StatusOK, cfg)
}

func (c *Controller) setConfig(ctx *gin.Context) {
	var request SetConfigRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	cfg, err := c.service.SetConfigForWorkspace(ctx.Request.Context(), workspaceID(ctx), &request)
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, cfg)
}

func (c *Controller) deleteConfig(ctx *gin.Context) {
	if err := c.service.DeleteConfigForWorkspace(ctx.Request.Context(), workspaceID(ctx)); err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (c *Controller) testConfig(ctx *gin.Context) {
	var request SetConfigRequest
	if ctx.Request.ContentLength != 0 {
		if err := ctx.ShouldBindJSON(&request); err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
			return
		}
	}
	result, err := c.service.TestConnectionForWorkspace(ctx.Request.Context(), workspaceID(ctx), &request)
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (c *Controller) copyConfig(ctx *gin.Context) {
	var request struct {
		TargetWorkspaceID string `json:"targetWorkspaceId"`
	}
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	cfg, err := c.service.CopyConfigToWorkspace(ctx.Request.Context(), workspaceID(ctx), request.TargetWorkspaceID)
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, cfg)
}

func (c *Controller) listProjects(ctx *gin.Context) {
	projects, err := c.service.ListProjectsForWorkspace(ctx.Request.Context(), workspaceID(ctx))
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"projects": projects})
}

func (c *Controller) listRepositories(ctx *gin.Context) {
	repositories, err := c.service.ListRepositoriesForWorkspace(
		ctx.Request.Context(), workspaceID(ctx), ctx.Query("project"),
	)
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"repositories": repositories})
}

func (c *Controller) searchWorkItems(ctx *gin.Context) {
	var request struct {
		Project string `json:"project"`
		WIQL    string `json:"wiql"`
		Top     int    `json:"top"`
	}
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	result, err := c.service.SearchWorkItemsForWorkspace(
		ctx.Request.Context(), workspaceID(ctx), request.Project, request.WIQL, request.Top,
	)
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (c *Controller) getWorkItem(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid work item id"})
		return
	}
	item, err := c.service.GetWorkItemForWorkspace(
		ctx.Request.Context(), workspaceID(ctx), ctx.Query("project"), id,
	)
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, item)
}

func (c *Controller) listPullRequests(ctx *gin.Context) {
	skip, _ := strconv.Atoi(ctx.Query("skip"))
	top, _ := strconv.Atoi(ctx.Query("top"))
	result, err := c.service.ListPullRequestsForWorkspace(ctx.Request.Context(), workspaceID(ctx), PullRequestFilter{
		ProjectID: ctx.Query("project"), RepositoryID: ctx.Query("repository"), Status: ctx.Query("status"),
		CreatorID: ctx.Query("creator"), ReviewerID: ctx.Query("reviewer"),
		SourceBranch: ctx.Query("source_branch"), TargetBranch: ctx.Query("target_branch"), Skip: skip, Top: top,
	})
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (c *Controller) getPullRequest(ctx *gin.Context) {
	id, ok := pullRequestID(ctx)
	if !ok {
		return
	}
	pr, err := c.service.GetPullRequestForWorkspace(ctx.Request.Context(), workspaceID(ctx), ctx.Param("projectId"), ctx.Param("repositoryId"), id)
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, pr)
}

func (c *Controller) getPullRequestFeedback(ctx *gin.Context) {
	id, ok := pullRequestID(ctx)
	if !ok {
		return
	}
	feedback, err := c.service.GetPullRequestFeedbackForWorkspace(ctx.Request.Context(), workspaceID(ctx), ctx.Param("projectId"), ctx.Param("repositoryId"), id)
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, feedback)
}

type taskPRRequest struct {
	RepositoryID  string `json:"repositoryId" binding:"required"`
	PullRequestID int    `json:"pullRequestId" binding:"required"`
}

func (c *Controller) listWorkspaceTaskPRs(ctx *gin.Context) {
	rows, err := c.service.ListTaskPRsByWorkspace(ctx.Request.Context(), ctx.Param("workspaceId"))
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, TaskPRsResponse{TaskPRs: rows})
}

func (c *Controller) associateTaskPR(ctx *gin.Context) {
	c.writeTaskPRSync(ctx, c.service.AssociateTaskPR)
}

func (c *Controller) syncTaskPR(ctx *gin.Context) {
	c.writeTaskPRSync(ctx, c.service.SyncTaskPR)
}

func (c *Controller) writeTaskPRSync(
	ctx *gin.Context,
	sync func(context.Context, string, string, string, int) (*TaskPR, error),
) {
	var request taskPRRequest
	if err := ctx.ShouldBindJSON(&request); err != nil || request.PullRequestID <= 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "repositoryId and positive pullRequestId are required"})
		return
	}
	row, err := sync(
		ctx.Request.Context(), workspaceID(ctx), ctx.Param("taskId"),
		request.RepositoryID, request.PullRequestID,
	)
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, row)
}

func (c *Controller) writeError(ctx *gin.Context, err error) {
	status := http.StatusInternalServerError
	code := ""
	switch {
	case errors.Is(err, ErrInvalidWorkspaceID), errors.Is(err, ErrInvalidConfig),
		errors.Is(err, ErrInvalidTaskPRAssociation):
		status = http.StatusBadRequest
	case errors.Is(err, ErrNotConfigured):
		status, code = http.StatusServiceUnavailable, notConfiguredCode
	default:
		var apiErr *APIError
		if errors.As(err, &apiErr) {
			status = upstreamStatus(apiErr.StatusCode)
		}
	}
	body := gin.H{"error": err.Error()}
	if code != "" {
		body["code"] = code
	}
	ctx.JSON(status, body)
}

func workspaceID(ctx *gin.Context) string { return strings.TrimSpace(ctx.Query("workspace_id")) }
func pullRequestID(ctx *gin.Context) (int, bool) {
	id, err := strconv.Atoi(ctx.Param("pullRequestId"))
	if err != nil || id <= 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid pull request id"})
		return 0, false
	}
	return id, true
}
func upstreamStatus(status int) int {
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusTooManyRequests:
		return status
	default:
		if status >= 500 {
			return http.StatusBadGateway
		}
		return http.StatusBadRequest
	}
}
