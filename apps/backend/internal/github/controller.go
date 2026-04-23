package github

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// Controller handles HTTP endpoints for GitHub integration.
type Controller struct {
	service *Service
	logger  *logger.Logger
}

// NewController creates a new GitHub controller.
func NewController(svc *Service, log *logger.Logger) *Controller {
	return &Controller{service: svc, logger: log}
}

// RegisterHTTPRoutes registers all GitHub HTTP routes.
func (c *Controller) RegisterHTTPRoutes(router *gin.Engine) {
	api := router.Group("/api/v1/github")
	api.GET("/status", c.httpGetStatus)

	api.GET("/task-prs", c.httpListTaskPRs)
	api.GET("/task-prs/:taskId", c.httpGetTaskPR)

	api.GET("/prs/:owner/:repo/:number", c.httpGetPRFeedback)
	api.GET("/prs/:owner/:repo/:number/info", c.httpGetPRInfo)
	api.POST("/prs/:owner/:repo/:number/reviews", c.httpSubmitReview)

	api.GET("/watches/pr", c.httpListPRWatches)
	api.DELETE("/watches/pr/:id", c.httpDeletePRWatch)

	api.GET("/watches/review", c.httpListReviewWatches)
	api.POST("/watches/review", c.httpCreateReviewWatch)
	api.PUT("/watches/review/:id", c.httpUpdateReviewWatch)
	api.DELETE("/watches/review/:id", c.httpDeleteReviewWatch)
	api.POST("/watches/review/:id/trigger", c.httpTriggerReviewWatch)
	api.POST("/watches/review/trigger-all", c.httpTriggerAllReviewChecks)

	api.GET("/watches/issue", c.httpListIssueWatches)
	api.POST("/watches/issue", c.httpCreateIssueWatch)
	api.PUT("/watches/issue/:id", c.httpUpdateIssueWatch)
	api.DELETE("/watches/issue/:id", c.httpDeleteIssueWatch)
	api.POST("/watches/issue/:id/trigger", c.httpTriggerIssueWatch)
	api.POST("/watches/issue/trigger-all", c.httpTriggerAllIssueChecks)

	api.GET("/orgs", c.httpListUserOrgs)
	api.GET("/repos/search", c.httpSearchRepos)
	api.GET("/repos/:owner/:repo/branches", c.httpListRepoBranches)

	api.GET("/stats", c.httpGetStats)
}

func (c *Controller) httpGetStatus(ctx *gin.Context) {
	status, err := c.service.GetStatus(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, status)
}

func (c *Controller) httpListTaskPRs(ctx *gin.Context) {
	// Workspace-scoped: returns all PRs for a workspace, triggers background refresh for stale ones
	workspaceID := ctx.Query("workspace_id")
	if workspaceID != "" {
		result, err := c.service.ListWorkspaceTaskPRs(ctx.Request.Context(), workspaceID)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		ctx.JSON(http.StatusOK, gin.H{"task_prs": result})
		return
	}

	// Legacy: filter by task IDs
	taskIDsParam := ctx.Query("task_ids")
	if taskIDsParam == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id or task_ids query parameter required"})
		return
	}
	taskIDs := strings.Split(taskIDsParam, ",")
	result, err := c.service.ListTaskPRs(ctx.Request.Context(), taskIDs)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"task_prs": result})
}

func (c *Controller) httpGetTaskPR(ctx *gin.Context) {
	taskID := ctx.Param("taskId")
	tp, err := c.service.GetTaskPR(ctx.Request.Context(), taskID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if tp == nil {
		ctx.Status(http.StatusNoContent)
		return
	}
	ctx.JSON(http.StatusOK, tp)
}

func (c *Controller) httpGetPRFeedback(ctx *gin.Context) {
	owner := ctx.Param("owner")
	repo := ctx.Param("repo")
	numberStr := ctx.Param("number")
	number, err := strconv.Atoi(numberStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid PR number"})
		return
	}
	feedback, err := c.service.GetPRFeedback(ctx.Request.Context(), owner, repo, number)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, feedback)
}

func (c *Controller) httpGetPRInfo(ctx *gin.Context) {
	owner := ctx.Param("owner")
	repo := ctx.Param("repo")
	numberStr := ctx.Param("number")
	number, err := strconv.Atoi(numberStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid PR number"})
		return
	}
	pr, err := c.service.GetPR(ctx.Request.Context(), owner, repo, number)
	if err != nil {
		status := http.StatusInternalServerError
		var apiErr *GitHubAPIError
		if errors.As(err, &apiErr) {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				status = http.StatusNotFound
			case http.StatusUnauthorized:
				status = http.StatusUnauthorized
			case http.StatusForbidden:
				status = http.StatusForbidden
			}
		}
		ctx.JSON(status, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, pr)
}

func (c *Controller) httpSubmitReview(ctx *gin.Context) {
	owner := ctx.Param("owner")
	repo := ctx.Param("repo")
	numberStr := ctx.Param("number")
	number, err := strconv.Atoi(numberStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid PR number"})
		return
	}
	var req struct {
		Event string `json:"event"`
		Body  string `json:"body"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	validEvents := map[string]bool{"APPROVE": true, "COMMENT": true, "REQUEST_CHANGES": true}
	if !validEvents[req.Event] {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "event must be APPROVE, COMMENT, or REQUEST_CHANGES"})
		return
	}
	if err := c.service.SubmitReview(ctx.Request.Context(), owner, repo, number, req.Event, req.Body); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"submitted": true})
}

func (c *Controller) httpListPRWatches(ctx *gin.Context) {
	watches, err := c.service.ListActivePRWatches(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"watches": watches})
}

func (c *Controller) httpDeletePRWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	if err := c.service.DeletePRWatch(ctx.Request.Context(), id); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (c *Controller) httpListReviewWatches(ctx *gin.Context) {
	workspaceID := ctx.Query("workspace_id")
	if workspaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id query parameter required"})
		return
	}
	watches, err := c.service.ListReviewWatches(ctx.Request.Context(), workspaceID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"watches": watches})
}

func (c *Controller) httpCreateReviewWatch(ctx *gin.Context) {
	var req CreateReviewWatchRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	rw, err := c.service.CreateReviewWatch(ctx.Request.Context(), &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusCreated, rw)
}

func (c *Controller) httpUpdateReviewWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	var req UpdateReviewWatchRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if err := c.service.UpdateReviewWatch(ctx.Request.Context(), id, &req); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"updated": true})
}

func (c *Controller) httpDeleteReviewWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	if err := c.service.DeleteReviewWatch(ctx.Request.Context(), id); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (c *Controller) httpTriggerReviewWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	watch, err := c.service.GetReviewWatch(ctx.Request.Context(), id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if watch == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "review watch not found"})
		return
	}
	newPRs, err := c.service.CheckReviewWatch(ctx.Request.Context(), watch)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Clean up tasks for merged/closed PRs that haven't been started.
	cleaned, _ := c.service.CleanupMergedReviewTasks(ctx.Request.Context(), watch)
	ctx.JSON(http.StatusOK, gin.H{"new_prs": len(newPRs), "prs": newPRs, "cleaned": cleaned})
}

func (c *Controller) httpTriggerAllReviewChecks(ctx *gin.Context) {
	workspaceID := ctx.Query("workspace_id")
	if workspaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id query parameter required"})
		return
	}
	count, err := c.service.TriggerAllReviewChecks(ctx.Request.Context(), workspaceID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"new_prs_found": count})
}

func (c *Controller) httpListUserOrgs(ctx *gin.Context) {
	orgs, err := c.service.ListUserOrgs(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"orgs": orgs})
}

func (c *Controller) httpSearchRepos(ctx *gin.Context) {
	org := ctx.Query("org")
	query := ctx.Query("q")
	if org == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "org query parameter required"})
		return
	}
	repos, err := c.service.SearchOrgRepos(ctx.Request.Context(), org, query, 20)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"repos": repos})
}

func (c *Controller) httpListRepoBranches(ctx *gin.Context) {
	owner := ctx.Param("owner")
	repo := ctx.Param("repo")
	branches, err := c.service.ListRepoBranches(ctx.Request.Context(), owner, repo)
	if err != nil {
		if errors.Is(err, ErrNoClient) {
			ctx.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "GitHub is not configured. Install the gh CLI and run 'gh auth login', or add a GITHUB_TOKEN secret.",
				"code":  "github_not_configured",
			})
			return
		}
		status := http.StatusInternalServerError
		var apiErr *GitHubAPIError
		if errors.As(err, &apiErr) {
			switch apiErr.StatusCode {
			case http.StatusNotFound:
				status = http.StatusNotFound
			case http.StatusUnauthorized:
				status = http.StatusUnauthorized
			case http.StatusForbidden:
				status = http.StatusForbidden
			}
		}
		ctx.JSON(status, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"branches": branches})
}

func (c *Controller) httpGetStats(ctx *gin.Context) {
	req := &PRStatsRequest{
		WorkspaceID: ctx.Query("workspace_id"),
	}
	if s := ctx.Query("start_date"); s != "" {
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid start_date format, expected YYYY-MM-DD"})
			return
		}
		req.StartDate = &t
	}
	if s := ctx.Query("end_date"); s != "" {
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid end_date format, expected YYYY-MM-DD"})
			return
		}
		req.EndDate = &t
	}
	stats, err := c.service.GetPRStats(ctx.Request.Context(), req)
	if err != nil {
		c.logger.Error("failed to get PR stats", zap.Error(err))
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, stats)
}

// --- Issue watch HTTP handlers ---

func (c *Controller) httpListIssueWatches(ctx *gin.Context) {
	workspaceID := ctx.Query("workspace_id")
	if workspaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id query parameter required"})
		return
	}
	watches, err := c.service.ListIssueWatches(ctx.Request.Context(), workspaceID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"watches": watches})
}

func (c *Controller) httpCreateIssueWatch(ctx *gin.Context) {
	var req CreateIssueWatchRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	iw, err := c.service.CreateIssueWatch(ctx.Request.Context(), &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusCreated, iw)
}

func (c *Controller) httpUpdateIssueWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	var req UpdateIssueWatchRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if err := c.service.UpdateIssueWatch(ctx.Request.Context(), id, &req); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	iw, err := c.service.GetIssueWatch(ctx.Request.Context(), id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, iw)
}

func (c *Controller) httpDeleteIssueWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	if err := c.service.DeleteIssueWatch(ctx.Request.Context(), id); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (c *Controller) httpTriggerIssueWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	watch, err := c.service.GetIssueWatch(ctx.Request.Context(), id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if watch == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "issue watch not found"})
		return
	}
	newIssues, err := c.service.CheckIssueWatch(ctx.Request.Context(), watch)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for _, issue := range newIssues {
		c.service.publishNewIssueEvent(ctx.Request.Context(), watch, issue)
	}
	// Clean up tasks for closed issues that haven't been started.
	cleaned, cleanErr := c.service.CleanupClosedIssueTasks(ctx.Request.Context(), watch)
	if cleanErr != nil {
		c.service.logger.Warn("cleanup closed issue tasks failed", zap.String("watch_id", id), zap.Error(cleanErr))
	}
	ctx.JSON(http.StatusOK, gin.H{"new_issues_found": len(newIssues), "issues": newIssues, "cleaned": cleaned})
}

func (c *Controller) httpTriggerAllIssueChecks(ctx *gin.Context) {
	workspaceID := ctx.Query("workspace_id")
	if workspaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id query parameter required"})
		return
	}
	count, err := c.service.TriggerAllIssueChecks(ctx.Request.Context(), workspaceID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"new_issues_found": count})
}
