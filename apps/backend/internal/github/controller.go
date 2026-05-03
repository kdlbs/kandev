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
	api.POST("/token", c.httpConfigureToken)
	api.DELETE("/token", c.httpClearToken)

	api.GET("/task-prs", c.httpListTaskPRs)
	api.GET("/task-prs/:taskId", c.httpGetTaskPR)

	api.GET("/prs/:owner/:repo/:number", c.httpGetPRFeedback)
	api.GET("/prs/:owner/:repo/:number/info", c.httpGetPRInfo)
	api.GET("/prs/:owner/:repo/:number/status", c.httpGetPRStatus)
	api.POST("/prs/statuses", c.httpGetPRStatusesBatch)
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

	api.GET("/user/prs", c.httpSearchUserPRs)
	api.GET("/user/issues", c.httpSearchUserIssues)

	api.GET("/action-presets", c.httpGetActionPresets)
	api.PUT("/action-presets", c.httpUpdateActionPresets)
	api.POST("/action-presets/reset", c.httpResetActionPresets)

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

func (c *Controller) httpConfigureToken(ctx *gin.Context) {
	var req ConfigureTokenRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload: token is required"})
		return
	}

	if err := c.service.ConfigureToken(ctx.Request.Context(), req.Token); err != nil {
		// Check if it's a validation error (invalid token)
		if strings.Contains(err.Error(), "invalid token") {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"configured": true})
}

func (c *Controller) httpClearToken(ctx *gin.Context) {
	if err := c.service.ClearToken(ctx.Request.Context()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"cleared": true})
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

func (c *Controller) httpGetPRStatus(ctx *gin.Context) {
	owner := ctx.Param("owner")
	repo := ctx.Param("repo")
	numberStr := ctx.Param("number")
	number, err := strconv.Atoi(numberStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid PR number"})
		return
	}
	status, err := c.service.GetPRStatus(ctx.Request.Context(), owner, repo, number)
	if err != nil {
		c.handleSearchError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, status)
}

// prStatusesBatchMaxRefs caps how many PRs a single batch request can ask for.
// The list page defaults to 25 per page and can go up to 100; double that
// gives slack without letting a rogue client ask for thousands.
const prStatusesBatchMaxRefs = 200

// httpGetPRStatusesBatch fetches statuses for multiple PRs in one request.
// Body: {"refs": [{"owner","repo","number"}, ...]}.
// Response: {"statuses": {"<owner>/<repo>#<number>": <PRStatus>}}. PRs that
// fail upstream are omitted rather than failing the whole batch.
func (c *Controller) httpGetPRStatusesBatch(ctx *gin.Context) {
	var body struct {
		Refs []PRRef `json:"refs"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if len(body.Refs) > prStatusesBatchMaxRefs {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "too many refs"})
		return
	}
	statuses, err := c.service.GetPRStatusesBatch(ctx.Request.Context(), body.Refs)
	if err != nil {
		c.handleSearchError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"statuses": statuses})
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

// httpListReviewWatches returns watches scoped to one workspace when
// `workspace_id` is supplied, or every watch across all workspaces when it
// is absent. The integration settings page uses the unscoped form.
func (c *Controller) httpListReviewWatches(ctx *gin.Context) {
	workspaceID := ctx.Query("workspace_id")
	var (
		watches []*ReviewWatch
		err     error
	)
	if workspaceID == "" {
		watches, err = c.service.ListAllReviewWatches(ctx.Request.Context())
	} else {
		watches, err = c.service.ListReviewWatches(ctx.Request.Context(), workspaceID)
	}
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

// httpSearchUserPRs searches for pull requests. Accepts:
//   - query: the complete GitHub search query (e.g. "is:pr author:@me state:open")
//   - filter: additional qualifiers appended to the default `type:pr`
//   - page: 1-indexed page (default 1)
//   - per_page: items per page, clamped to 1..100 (default 50)
//
// When query is non-empty it is used verbatim; otherwise filter is used.
func (c *Controller) httpSearchUserPRs(ctx *gin.Context) {
	query := ctx.Query("query")
	filter := ctx.Query("filter")
	page, perPage := parsePaginationQuery(ctx)
	result, err := c.service.SearchUserPRsPaged(ctx.Request.Context(), filter, query, page, perPage)
	if err != nil {
		c.handleSearchError(ctx, err)
		return
	}
	if result.PRs == nil {
		result.PRs = []*PR{}
	}
	ctx.JSON(http.StatusOK, result)
}

// httpSearchUserIssues mirrors httpSearchUserPRs for issues.
func (c *Controller) httpSearchUserIssues(ctx *gin.Context) {
	query := ctx.Query("query")
	filter := ctx.Query("filter")
	page, perPage := parsePaginationQuery(ctx)
	result, err := c.service.SearchUserIssuesPaged(ctx.Request.Context(), filter, query, page, perPage)
	if err != nil {
		c.handleSearchError(ctx, err)
		return
	}
	if result.Issues == nil {
		result.Issues = []*Issue{}
	}
	ctx.JSON(http.StatusOK, result)
}

// parsePaginationQuery reads `page` and `per_page` query params. Missing or
// non-positive values fall back to defaults (page=1, perPage=50). The upper
// bound for perPage (100) is applied later by clampSearchPage in the client
// layer, where it also gates the cache key.
func parsePaginationQuery(ctx *gin.Context) (int, int) {
	page, _ := strconv.Atoi(ctx.Query("page"))
	perPage, _ := strconv.Atoi(ctx.Query("per_page"))
	if page < 1 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 50
	}
	return page, perPage
}

// handleSearchError maps client errors to proper HTTP responses.
func (c *Controller) handleSearchError(ctx *gin.Context, err error) {
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
	var (
		watches []*IssueWatch
		err     error
	)
	if workspaceID == "" {
		watches, err = c.service.ListAllIssueWatches(ctx.Request.Context())
	} else {
		watches, err = c.service.ListIssueWatches(ctx.Request.Context(), workspaceID)
	}
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

// --- Action preset HTTP handlers ---

func (c *Controller) httpGetActionPresets(ctx *gin.Context) {
	workspaceID := ctx.Query("workspace_id")
	if workspaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id query parameter required"})
		return
	}
	presets, err := c.service.GetActionPresets(ctx.Request.Context(), workspaceID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, presets)
}

func (c *Controller) httpUpdateActionPresets(ctx *gin.Context) {
	var req UpdateActionPresetsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if req.WorkspaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return
	}
	presets, err := c.service.UpdateActionPresets(ctx.Request.Context(), &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, presets)
}

func (c *Controller) httpResetActionPresets(ctx *gin.Context) {
	workspaceID := ctx.Query("workspace_id")
	if workspaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id query parameter required"})
		return
	}
	presets, err := c.service.ResetActionPresets(ctx.Request.Context(), workspaceID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, presets)
}
