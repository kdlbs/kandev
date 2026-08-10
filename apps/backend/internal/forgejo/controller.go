package forgejo

import (
	"errors"
	"io"
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
	api.POST("/webhooks", controller.handleWebhook)
	api.GET("/repositories", controller.listRepositories)
	api.GET("/issues", controller.listIssues)
	api.GET("/pull-requests", controller.listPullRequests)
	api.GET("/pull-request-details", controller.getPullRequestDetails)
	api.POST("/pull-request-comments", controller.createPullRequestComment)
	api.POST("/pull-request-reviews", controller.submitPullRequestReview)
	api.GET("/queue", controller.listQueue)
	api.POST("/connection/refresh", controller.refreshConnection)
	api.GET("/issue-watches", controller.listIssueWatches)
	api.PUT("/issue-watches", controller.saveIssueWatch)
	api.DELETE("/issue-watches/:watchID", controller.deleteIssueWatch)
	api.POST("/issue-watches/:watchID/poll", controller.pollIssueWatch)
	api.GET("/review-watches", controller.listReviewWatches)
	api.PUT("/review-watches", controller.saveReviewWatch)
	api.DELETE("/review-watches/:watchID", controller.deleteReviewWatch)
	api.POST("/review-watches/:watchID/poll", controller.pollReviewWatch)
	api.GET("/action-presets", controller.listActionPresets)
	api.PUT("/action-presets", controller.saveActionPreset)
	api.DELETE("/action-presets/:presetID", controller.deleteActionPreset)
	api.GET("/tasks/:taskID/pull-requests", controller.listTaskPRs)
	api.POST("/task-pull-requests", controller.associatePullRequest)
	api.POST("/task-pull-requests/create", controller.createTaskPullRequest)
	api.GET("/tasks/:taskID/issues", controller.listTaskIssues)
	api.POST("/task-issues", controller.associateIssue)
	api.DELETE("/task-issues/:linkID", controller.unlinkTaskIssue)
	api.DELETE("/task-pull-requests/:linkID", controller.unlinkTaskPullRequest)
	api.POST("/task-issues/:linkID/refresh", controller.refreshTaskIssue)
	api.POST("/task-pull-requests/:linkID/refresh", controller.refreshTaskPullRequest)
}

func (c *Controller) handleWebhook(ctx *gin.Context) {
	payload, err := io.ReadAll(http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxResponseSize))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid webhook payload"})
		return
	}
	deliveryID := strings.TrimSpace(ctx.GetHeader("X-Gitea-Delivery"))
	if deliveryID == "" {
		deliveryID = strings.TrimSpace(ctx.GetHeader("X-Forgejo-Delivery"))
	}
	if deliveryID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "webhook delivery ID required"})
		return
	}
	signature := ctx.GetHeader("X-Forgejo-Signature")
	if signature == "" {
		signature = ctx.GetHeader("X-Gitea-Signature")
	}
	if err := c.service.HandleWebhook(ctx.Request.Context(), c.workspaceID(ctx), deliveryID, signature, payload); err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusAccepted, gin.H{"accepted": true})
}

func (c *Controller) listIssueWatches(ctx *gin.Context) {
	watches, err := c.service.ListIssueWatches(ctx.Request.Context(), c.workspaceID(ctx))
	if err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"watches": watches})
}
func (c *Controller) saveIssueWatch(ctx *gin.Context) {
	var watch IssueWatch
	if err := ctx.ShouldBindJSON(&watch); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid issue watch"})
		return
	}
	if err := c.service.SaveIssueWatch(ctx.Request.Context(), c.workspaceID(ctx), &watch); err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, watch)
}
func (c *Controller) deleteIssueWatch(ctx *gin.Context) {
	if err := c.service.DeleteIssueWatch(ctx.Request.Context(), c.workspaceID(ctx), strings.TrimSpace(ctx.Param("watchID"))); err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}
func (c *Controller) pollIssueWatch(ctx *gin.Context) {
	issues, err := c.service.PollIssueWatch(ctx.Request.Context(), c.workspaceID(ctx), strings.TrimSpace(ctx.Param("watchID")))
	if err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"issues": issues})
}

func (c *Controller) listReviewWatches(ctx *gin.Context) {
	watches, err := c.service.ListReviewWatches(ctx.Request.Context(), c.workspaceID(ctx))
	if err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"watches": watches})
}
func (c *Controller) saveReviewWatch(ctx *gin.Context) {
	var watch ReviewWatch
	if err := ctx.ShouldBindJSON(&watch); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid review watch"})
		return
	}
	if err := c.service.SaveReviewWatch(ctx.Request.Context(), c.workspaceID(ctx), &watch); err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, watch)
}
func (c *Controller) deleteReviewWatch(ctx *gin.Context) {
	if err := c.service.DeleteReviewWatch(ctx.Request.Context(), c.workspaceID(ctx), strings.TrimSpace(ctx.Param("watchID"))); err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}
func (c *Controller) pollReviewWatch(ctx *gin.Context) {
	pulls, err := c.service.PollReviewWatchByID(ctx.Request.Context(), c.workspaceID(ctx), strings.TrimSpace(ctx.Param("watchID")))
	if err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"pull_requests": pulls})
}
func (c *Controller) listActionPresets(ctx *gin.Context) {
	presets, err := c.service.ListActionPresets(ctx.Request.Context(), c.workspaceID(ctx))
	if err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"presets": presets})
}
func (c *Controller) saveActionPreset(ctx *gin.Context) {
	var preset ActionPreset
	if err := ctx.ShouldBindJSON(&preset); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid action preset"})
		return
	}
	if err := c.service.SaveActionPreset(ctx.Request.Context(), c.workspaceID(ctx), &preset); err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, preset)
}
func (c *Controller) deleteActionPreset(ctx *gin.Context) {
	if err := c.service.DeleteActionPreset(ctx.Request.Context(), c.workspaceID(ctx), strings.TrimSpace(ctx.Param("presetID"))); err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (c *Controller) refreshTaskIssue(ctx *gin.Context) {
	link, err := c.service.RefreshTaskIssue(ctx.Request.Context(), c.workspaceID(ctx), strings.TrimSpace(ctx.Param("linkID")))
	if err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, link)
}

func (c *Controller) refreshTaskPullRequest(ctx *gin.Context) {
	link, err := c.service.RefreshTaskPullRequest(ctx.Request.Context(), c.workspaceID(ctx), strings.TrimSpace(ctx.Param("linkID")))
	if err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, link)
}

func (c *Controller) refreshConnection(ctx *gin.Context) {
	config, err := c.service.RefreshConnection(ctx.Request.Context(), c.workspaceID(ctx))
	if err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, config)
}

func (c *Controller) listQueue(ctx *gin.Context) {
	issues, pulls, err := c.service.ListWorkspaceQueue(ctx.Request.Context(), c.workspaceID(ctx))
	if err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"issues": issues, "pull_requests": pulls})
}

func (c *Controller) unlinkTaskIssue(ctx *gin.Context) {
	if err := c.service.UnlinkTaskIssue(ctx.Request.Context(), c.workspaceID(ctx), strings.TrimSpace(ctx.Param("linkID"))); err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (c *Controller) unlinkTaskPullRequest(ctx *gin.Context) {
	if err := c.service.UnlinkTaskPullRequest(ctx.Request.Context(), c.workspaceID(ctx), strings.TrimSpace(ctx.Param("linkID"))); err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (c *Controller) createTaskPullRequest(ctx *gin.Context) {
	var request struct {
		TaskID       string `json:"task_id"`
		RepositoryID string `json:"repository_id"`
		Owner        string `json:"owner"`
		Repo         string `json:"repo"`
		Title        string `json:"title"`
		Body         string `json:"body"`
		Head         string `json:"head"`
		Base         string `json:"base"`
		Draft        bool   `json:"draft"`
	}
	if err := ctx.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.TaskID) == "" || strings.TrimSpace(request.Owner) == "" || strings.TrimSpace(request.Repo) == "" || strings.TrimSpace(request.Title) == "" || strings.TrimSpace(request.Head) == "" || strings.TrimSpace(request.Base) == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "task_id, owner, repo, title, head, and base are required"})
		return
	}
	pr, err := c.service.CreateTaskPullRequest(ctx.Request.Context(), c.workspaceID(ctx), request.TaskID, request.RepositoryID, CreatePullRequestInput{Owner: request.Owner, Repo: request.Repo, Title: request.Title, Body: request.Body, Head: request.Head, Base: request.Base, Draft: request.Draft})
	if err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, pr)
}

func (c *Controller) listPullRequests(ctx *gin.Context) {
	pulls, total, err := c.service.ListPullRequests(ctx.Request.Context(), c.workspaceID(ctx), strings.TrimSpace(ctx.Query("owner")), strings.TrimSpace(ctx.Query("repo")), queryPage(ctx), queryLimit(ctx))
	if err != nil {
		c.error(ctx, err)
		return
	}
	ctx.Header("X-Total-Count", strconv.Itoa(total))
	ctx.JSON(http.StatusOK, gin.H{"pull_requests": pulls, "total_count": total})
}

func (c *Controller) getPullRequestDetails(ctx *gin.Context) {
	number, err := strconv.Atoi(ctx.Query("number"))
	if err != nil || number < 1 || strings.TrimSpace(ctx.Query("owner")) == "" || strings.TrimSpace(ctx.Query("repo")) == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "owner, repo, and positive number are required"})
		return
	}
	details, err := c.service.GetPullRequestDetails(ctx.Request.Context(), c.workspaceID(ctx), strings.TrimSpace(ctx.Query("owner")), strings.TrimSpace(ctx.Query("repo")), number)
	if err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, details)
}

func (c *Controller) createPullRequestComment(ctx *gin.Context) {
	var request struct {
		Owner  string `json:"owner"`
		Repo   string `json:"repo"`
		Number int    `json:"number"`
		Body   string `json:"body"`
	}
	if err := ctx.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.Owner) == "" || strings.TrimSpace(request.Repo) == "" || request.Number < 1 || strings.TrimSpace(request.Body) == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "owner, repo, positive number, and body are required"})
		return
	}
	comment, err := c.service.CreatePullRequestComment(ctx.Request.Context(), c.workspaceID(ctx), request.Owner, request.Repo, request.Number, request.Body)
	if err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, comment)
}

func (c *Controller) submitPullRequestReview(ctx *gin.Context) {
	var input SubmitPullRequestReviewInput
	if err := ctx.ShouldBindJSON(&input); err != nil || strings.TrimSpace(input.Owner) == "" || strings.TrimSpace(input.Repo) == "" || input.Number < 1 || strings.TrimSpace(input.Event) == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "owner, repo, positive number, and event are required"})
		return
	}
	review, err := c.service.SubmitPullRequestReview(ctx.Request.Context(), c.workspaceID(ctx), input)
	if err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusCreated, review)
}

func (c *Controller) listTaskIssues(ctx *gin.Context) {
	issues, err := c.service.store.ListTaskIssues(ctx.Request.Context(), c.workspaceID(ctx), ctx.Param("taskID"))
	if err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"issues": issues})
}

func (c *Controller) associateIssue(ctx *gin.Context) {
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
	issue, err := c.service.AssociateIssue(ctx.Request.Context(), c.workspaceID(ctx), request.TaskID, request.RepositoryID, request.Owner, request.Repo, request.Number)
	if err != nil {
		c.error(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, issue)
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
	if errors.Is(err, ErrTaskLinkNotFound) {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Forgejo task link not found"})
		return
	}
	if errors.Is(err, ErrWatchNotFound) {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Forgejo issue watch not found"})
		return
	}
	if errors.Is(err, ErrInvalidWebhookSignature) {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "invalid Forgejo webhook signature"})
		return
	}
	if errors.Is(err, ErrUnsupported) {
		ctx.JSON(http.StatusNotImplemented, gin.H{"error": "this Forgejo server does not expose the requested capability"})
		return
	}
	if strings.Contains(err.Error(), "owner and repository are required") {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "owner and repo query parameters required"})
		return
	}
	ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Forgejo connection operation failed"})
}
