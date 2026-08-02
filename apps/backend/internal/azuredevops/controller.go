package azuredevops

import (
	"context"
	"encoding/json"
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
	api.GET("/views", controller.getSavedViews)
	api.PUT("/views", controller.setSavedViews)
	api.GET("/workspace-settings", controller.getWorkspaceSettings)
	api.PATCH("/workspace-settings", controller.updateWorkspaceSettings)
	api.GET("/projects", controller.listProjects)
	api.GET("/teams", controller.listTeams)
	api.GET("/boards", controller.listBoards)
	api.GET("/boards/:boardId", controller.getBoardSnapshot)
	api.PATCH("/boards/:boardId/work-items/:id", controller.updateBoardWorkItem)
	api.GET("/repositories", controller.listRepositories)
	api.GET("/branches", controller.listBranches)
	api.POST("/work-items/search", controller.searchWorkItems)
	api.GET("/work-items/:id/comments", controller.listWorkItemComments)
	api.PATCH("/work-items/:id", controller.updateWorkItem)
	api.GET("/work-items/:id", controller.getWorkItem)
	api.GET("/identity", controller.getIdentity)
	api.GET("/watches/work-items", controller.listWorkItemWatches)
	api.POST("/watches/work-items", controller.createWorkItemWatch)
	api.PATCH("/watches/work-items/:id", controller.updateWorkItemWatch)
	api.DELETE("/watches/work-items/:id", controller.deleteWorkItemWatch)
	api.POST("/watches/work-items/:id/trigger", controller.triggerWorkItemWatch)
	api.GET("/watches/work-items/:id/reset/preview", controller.previewResetWorkItemWatch)
	api.POST("/watches/work-items/:id/reset", controller.resetWorkItemWatch)
	api.GET("/watches/pull-requests", controller.listPullRequestWatches)
	api.POST("/watches/pull-requests", controller.createPullRequestWatch)
	api.PATCH("/watches/pull-requests/:id", controller.updatePullRequestWatch)
	api.DELETE("/watches/pull-requests/:id", controller.deletePullRequestWatch)
	api.POST("/watches/pull-requests/:id/trigger", controller.triggerPullRequestWatch)
	api.GET("/watches/pull-requests/:id/reset/preview", controller.previewResetPullRequestWatch)
	api.POST("/watches/pull-requests/:id/reset", controller.resetPullRequestWatch)
	api.GET("/workspaces/:workspaceId/task-work-items", controller.listWorkspaceTaskWorkItems)
	api.POST("/tasks/:taskId/work-items", controller.associateTaskWorkItem)
	api.GET("/pull-requests", controller.listPullRequests)
	api.GET("/pull-requests/:projectId/:repositoryId/:pullRequestId", controller.getPullRequest)
	api.GET("/pull-requests/:projectId/:repositoryId/:pullRequestId/feedback", controller.getPullRequestFeedback)
	api.GET("/workspaces/:workspaceId/task-prs", controller.listWorkspaceTaskPRs)
	api.POST("/tasks/:taskId/pull-requests", controller.associateTaskPR)
	api.POST("/tasks/:taskId/pull-requests/sync", controller.syncTaskPR)
}

func (c *Controller) getWorkspaceSettings(ctx *gin.Context) {
	settings, err := c.service.GetWorkspaceSettings(ctx.Request.Context(), workspaceID(ctx))
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, settings)
}

func (c *Controller) updateWorkspaceSettings(ctx *gin.Context) {
	var request UpdateWorkspaceSettingsRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	request.WorkspaceID = workspaceID(ctx)
	settings, err := c.service.UpdateWorkspaceSettings(ctx.Request.Context(), &request)
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, settings)
}

func (c *Controller) getSavedViews(ctx *gin.Context) {
	views, err := c.service.GetSavedViewsForWorkspace(ctx.Request.Context(), workspaceID(ctx))
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"views": views})
}

func (c *Controller) setSavedViews(ctx *gin.Context) {
	var request struct {
		Views []SavedView `json:"views"`
	}
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	views, err := c.service.SetSavedViewsForWorkspace(
		ctx.Request.Context(), workspaceID(ctx), request.Views,
	)
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"views": views})
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

func (c *Controller) listTeams(ctx *gin.Context) {
	teams, err := c.service.ListTeamsForWorkspace(ctx.Request.Context(), workspaceID(ctx), ctx.Query("project"))
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"teams": teams})
}

func (c *Controller) listBoards(ctx *gin.Context) {
	boards, err := c.service.ListBoardsForWorkspace(ctx.Request.Context(), workspaceID(ctx), ctx.Query("project"), ctx.Query("team"))
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"boards": boards})
}

func (c *Controller) getBoardSnapshot(ctx *gin.Context) {
	snapshot, err := c.service.GetBoardSnapshotForWorkspace(ctx.Request.Context(), workspaceID(ctx), ctx.Query("project"), ctx.Query("team"), ctx.Param("boardId"))
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, snapshot)
}

func (c *Controller) updateBoardWorkItem(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || id <= 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid work item id"})
		return
	}
	var request BoardWorkItemUpdateRequest
	decoder := json.NewDecoder(ctx.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	item, err := c.service.UpdateBoardWorkItemForWorkspace(ctx.Request.Context(), workspaceID(ctx), ctx.Query("project"), ctx.Query("team"), ctx.Param("boardId"), id, request)
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, item)
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

func (c *Controller) listBranches(ctx *gin.Context) {
	branches, err := c.service.ListBranchesForWorkspace(
		ctx.Request.Context(), workspaceID(ctx), ctx.Query("organization"), ctx.Query("project"),
		ctx.Query("repository"),
	)
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"branches": branches})
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
	item, err := c.service.GetWorkItemDetailForWorkspace(
		ctx.Request.Context(), workspaceID(ctx), ctx.Query("project"), id,
	)
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, item)
}

func (c *Controller) updateWorkItem(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || id <= 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid work item id"})
		return
	}
	var request WorkItemAssignmentRequest
	decoder := json.NewDecoder(ctx.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	item, err := c.service.UpdateWorkItemForWorkspace(ctx.Request.Context(), workspaceID(ctx), ctx.Query("project"), id, request)
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, item)
}

func (c *Controller) listWorkItemComments(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil || id <= 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid work item id"})
		return
	}
	page, err := c.service.ListWorkItemCommentsForWorkspace(
		ctx.Request.Context(), workspaceID(ctx), ctx.Query("project"), id, ctx.Query("continuation_token"),
	)
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, page)
}

func (c *Controller) getIdentity(ctx *gin.Context) {
	identity, err := c.service.GetCurrentIdentityForWorkspace(ctx.Request.Context(), workspaceID(ctx))
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, identity)
}

func (c *Controller) listWorkItemWatches(ctx *gin.Context) {
	watches, err := c.service.ListWorkItemWatches(ctx.Request.Context(), workspaceID(ctx))
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"watches": watches})
}

func (c *Controller) createWorkItemWatch(ctx *gin.Context) {
	var request CreateWorkItemWatchRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	request.WorkspaceID = workspaceID(ctx)
	watch, err := c.service.CreateWorkItemWatch(ctx.Request.Context(), &request)
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, watch)
}

func (c *Controller) updateWorkItemWatch(ctx *gin.Context) {
	var request UpdateWorkItemWatchRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	watch, err := c.service.UpdateWorkItemWatch(ctx.Request.Context(), workspaceID(ctx), ctx.Param("id"), &request)
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, watch)
}

func (c *Controller) deleteWorkItemWatch(ctx *gin.Context) {
	if err := c.service.DeleteWorkItemWatch(ctx.Request.Context(), workspaceID(ctx), ctx.Param("id")); err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (c *Controller) triggerWorkItemWatch(ctx *gin.Context) {
	result, err := c.service.TriggerWorkItemWatch(ctx.Request.Context(), workspaceID(ctx), ctx.Param("id"))
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (c *Controller) previewResetWorkItemWatch(ctx *gin.Context) {
	count, err := c.service.PreviewResetWorkItemWatch(ctx.Request.Context(), workspaceID(ctx), ctx.Param("id"))
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"taskCount": count})
}

func (c *Controller) resetWorkItemWatch(ctx *gin.Context) {
	result, err := c.service.ResetWorkItemWatch(ctx.Request.Context(), workspaceID(ctx), ctx.Param("id"))
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"generation": result.Generation, "taskCount": len(result.TaskIDs)})
}

func (c *Controller) listPullRequestWatches(ctx *gin.Context) {
	watches, err := c.service.ListPullRequestWatches(ctx.Request.Context(), workspaceID(ctx))
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"watches": watches})
}

func (c *Controller) createPullRequestWatch(ctx *gin.Context) {
	var request CreatePullRequestWatchRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	request.WorkspaceID = workspaceID(ctx)
	watch, err := c.service.CreatePullRequestWatch(ctx.Request.Context(), &request)
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, watch)
}

func (c *Controller) updatePullRequestWatch(ctx *gin.Context) {
	var request UpdatePullRequestWatchRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	watch, err := c.service.UpdatePullRequestWatch(ctx.Request.Context(), workspaceID(ctx), ctx.Param("id"), &request)
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, watch)
}

func (c *Controller) deletePullRequestWatch(ctx *gin.Context) {
	if err := c.service.DeletePullRequestWatch(ctx.Request.Context(), workspaceID(ctx), ctx.Param("id")); err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (c *Controller) triggerPullRequestWatch(ctx *gin.Context) {
	result, err := c.service.TriggerPullRequestWatch(ctx.Request.Context(), workspaceID(ctx), ctx.Param("id"))
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (c *Controller) previewResetPullRequestWatch(ctx *gin.Context) {
	count, err := c.service.PreviewResetPullRequestWatch(ctx.Request.Context(), workspaceID(ctx), ctx.Param("id"))
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"taskCount": count})
}

func (c *Controller) resetPullRequestWatch(ctx *gin.Context) {
	result, err := c.service.ResetPullRequestWatch(ctx.Request.Context(), workspaceID(ctx), ctx.Param("id"))
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"generation": result.Generation, "taskCount": len(result.TaskIDs)})
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

type taskWorkItemRequest struct {
	ProjectID  string `json:"projectId" binding:"required"`
	WorkItemID int    `json:"workItemId" binding:"required"`
}

func (c *Controller) listWorkspaceTaskWorkItems(ctx *gin.Context) {
	rows, err := c.service.ListTaskWorkItemsByWorkspace(ctx.Request.Context(), ctx.Param("workspaceId"))
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, TaskWorkItemsResponse{TaskWorkItems: rows})
}

func (c *Controller) associateTaskWorkItem(ctx *gin.Context) {
	var request taskWorkItemRequest
	if err := ctx.ShouldBindJSON(&request); err != nil || request.WorkItemID <= 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "projectId and positive workItemId are required"})
		return
	}
	row, err := c.service.AssociateTaskWorkItem(ctx.Request.Context(), workspaceID(ctx), ctx.Param("taskId"), request.ProjectID, request.WorkItemID)
	if err != nil {
		c.writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, row)
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
	case errors.Is(err, ErrWatchNotFound), errors.Is(err, ErrReservationNotFound):
		status = http.StatusNotFound
	case errors.Is(err, ErrWatchOwnershipLost):
		status = http.StatusConflict
	case errors.Is(err, ErrNotConfigured):
		status, code = http.StatusServiceUnavailable, notConfiguredCode
	default:
		var apiErr *APIError
		if errors.As(err, &apiErr) {
			if apiErr.StatusCode == http.StatusConflict {
				status, code = http.StatusConflict, "azure_devops_revision_conflict"
			} else {
				status = upstreamStatus(apiErr.StatusCode)
			}
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
