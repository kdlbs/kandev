package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

const jsonNullLiteral = "null"

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
	api.GET("/connections", c.httpGetWorkspaceConnection)
	api.GET("/auth/gh-cli/accounts", c.httpListCLIAccounts)
	api.GET("/cli/accounts", c.httpListCLIAccounts)
	api.PUT("/workspace-connection", c.httpSetWorkspaceConnection)
	api.DELETE("/workspace-connection", c.httpDeleteWorkspaceConnection)
	api.PUT("/connections/:workspaceId", c.httpSetWorkspaceConnection)
	api.DELETE("/connections/:workspaceId", c.httpDeleteWorkspaceConnection)
	api.POST("/app/install/start", c.httpStartAppInstallation)
	api.GET("/app/registration", c.httpGetDeploymentAppRegistration)
	api.POST("/app/registration/start", c.httpStartDeploymentAppRegistration)
	api.GET("/app/registration/callback", c.httpCompleteDeploymentAppRegistration)
	api.DELETE("/app/registration", c.httpDeleteDeploymentAppRegistration)
	api.GET("/app/install/callback", c.httpCompleteAppInstallation)
	api.DELETE("/app/installation", c.httpDeleteWorkspaceConnection)
	api.POST("/app/webhook", c.httpGitHubAppWebhook)
	api.POST("/app/install/:workspaceId", c.httpStartAppInstallation)
	api.GET("/app/callback", c.httpCompleteAppInstallation)
	api.POST("/personal-connection/start", c.httpStartPersonalAuth)
	api.GET("/personal-connection/callback", c.httpCompletePersonalAuth)
	api.DELETE("/personal-connection", c.httpDisconnectPersonalAuth)
	api.POST("/personal/connect/:workspaceId", c.httpStartPersonalAuth)
	api.GET("/personal/callback", c.httpCompletePersonalAuth)
	api.DELETE("/personal/:workspaceId", c.httpDisconnectPersonalAuth)
	api.POST("/webhook", c.httpGitHubAppWebhook)
	api.GET("/credentials/resolve", c.httpCredentialBrokerReady)
	api.POST("/credentials/resolve", c.httpResolveCredentialLease)

	api.GET("/task-prs", c.httpListTaskPRs)
	api.POST("/task-prs", c.httpCreateTaskPR)
	api.GET("/task-prs/:taskId", c.httpGetTaskPR)
	api.GET("/task-issues", c.httpListTaskIssues)
	api.PUT("/tasks/:taskId/issue", c.httpLinkTaskIssue)
	api.DELETE("/tasks/:taskId/issue", c.httpUnlinkTaskIssue)
	api.GET("/tasks/:taskId/ci-options", c.httpGetTaskCIOptions)
	api.PATCH("/tasks/:taskId/ci-options", c.httpPatchTaskCIOptions)

	api.GET("/prs/:owner/:repo/:number", c.httpGetPRFeedback)
	api.GET("/prs/:owner/:repo/:number/info", c.httpGetPRInfo)
	api.GET("/prs/:owner/:repo/:number/status", c.httpGetPRStatus)
	api.POST("/prs/statuses", c.httpGetPRStatusesBatch)
	api.POST("/prs/:owner/:repo/:number/reviews", c.httpSubmitReview)
	api.PUT("/prs/:owner/:repo/:number/merge", c.httpMergePR)
	api.GET("/issues/:owner/:repo/:number/info", c.httpGetIssueInfo)

	api.GET("/watches/pr", c.httpListPRWatches)
	api.DELETE("/watches/pr/:id", c.httpDeletePRWatch)

	api.GET("/watches/review", c.httpListReviewWatches)
	api.POST("/watches/review", c.httpCreateReviewWatch)
	api.PUT("/watches/review/:id", c.httpUpdateReviewWatch)
	api.DELETE("/watches/review/:id", c.httpDeleteReviewWatch)
	api.POST("/watches/review/:id/trigger", c.httpTriggerReviewWatch)
	api.GET("/watches/review/:id/reset/preview", c.httpPreviewResetReviewWatch)
	api.POST("/watches/review/:id/reset", c.httpResetReviewWatch)
	api.POST("/watches/review/trigger-all", c.httpTriggerAllReviewChecks)

	api.GET("/watches/issue", c.httpListIssueWatches)
	api.POST("/watches/issue", c.httpCreateIssueWatch)
	api.PUT("/watches/issue/:id", c.httpUpdateIssueWatch)
	api.DELETE("/watches/issue/:id", c.httpDeleteIssueWatch)
	api.POST("/watches/issue/:id/trigger", c.httpTriggerIssueWatch)
	api.GET("/watches/issue/:id/reset/preview", c.httpPreviewResetIssueWatch)
	api.POST("/watches/issue/:id/reset", c.httpResetIssueWatch)
	api.POST("/watches/issue/trigger-all", c.httpTriggerAllIssueChecks)

	api.POST("/cleanup/review-tasks", c.httpCleanupReviewTasks)
	api.POST("/cleanup/issue-tasks", c.httpCleanupIssueTasks)

	api.GET("/orgs", c.httpListUserOrgs)
	api.GET("/repos", c.httpListAccessibleRepos)
	api.GET("/repos/search", c.httpSearchRepos)
	api.GET("/repos/:owner/:repo/branches", c.httpListRepoBranches)
	api.GET("/repos/:owner/:repo/merge-methods", c.httpGetRepoMergeMethods)

	api.GET("/user/prs", c.httpSearchUserPRs)
	api.GET("/user/issues", c.httpSearchUserIssues)

	api.GET("/action-presets", c.httpGetActionPresets)
	api.PUT("/action-presets", c.httpUpdateActionPresets)
	api.POST("/action-presets/reset", c.httpResetActionPresets)
	api.GET("/workspace-settings", c.httpGetWorkspaceSettings)
	api.PUT("/workspace-settings", c.httpUpdateWorkspaceSettings)
	api.POST("/workspace-settings/copy", c.httpCopyWorkspaceSettings)

	api.GET("/stats", c.httpGetStats)
}

func (c *Controller) httpGetStatus(ctx *gin.Context) {
	workspaceID := ctx.Query("workspace_id")
	status, err := c.service.GetWorkspaceAuthStatus(
		ctx.Request.Context(), workspaceID, currentGitHubUserID(ctx),
	)
	if err != nil {
		writeGitHubAuthError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, status)
}

func (c *Controller) httpConfigureToken(ctx *gin.Context) {
	workspaceID := ctx.Query("workspace_id")
	if workspaceID == "" {
		writeGitHubAuthError(ctx, ErrGitHubWorkspaceRequired)
		return
	}
	var req ConfigureTokenRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload: token is required"})
		return
	}

	if _, err := c.service.SetWorkspaceConnection(ctx.Request.Context(), workspaceID, SetWorkspaceConnectionRequest{
		Source: ConnectionSourcePAT,
		Token:  req.Token,
	}); err != nil {
		if errors.Is(err, ErrInvalidToken) {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Header("Deprecation", "true")
	ctx.Header("Sunset", "one-release")
	ctx.JSON(http.StatusOK, gin.H{"configured": true, "workspace_id": workspaceID})
}

func (c *Controller) httpClearToken(ctx *gin.Context) {
	workspaceID := ctx.Query("workspace_id")
	if workspaceID == "" {
		writeGitHubAuthError(ctx, ErrGitHubWorkspaceRequired)
		return
	}
	if err := c.service.DeleteWorkspaceConnection(ctx.Request.Context(), workspaceID); err != nil {
		writeGitHubAuthError(ctx, err)
		return
	}
	ctx.Header("Deprecation", "true")
	ctx.Header("Sunset", "one-release")
	ctx.JSON(http.StatusOK, gin.H{"cleared": true, "workspace_id": workspaceID})
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

// httpCreateTaskPR creates a task-PR association from a known PR URL. Used
// by the GitHub PR list "+ Task" flow: the PR is already known, so we
// persist the linkage immediately instead of waiting for branch-based
// discovery (which fails for review tasks that use synthetic worktree
// branches that don't exist on GitHub).
func (c *Controller) httpCreateTaskPR(ctx *gin.Context) {
	var req struct {
		WorkspaceID  string `json:"workspace_id"`
		TaskID       string `json:"task_id"`
		RepositoryID string `json:"repository_id"`
		PRURL        string `json:"pr_url"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if req.WorkspaceID == "" || req.TaskID == "" || req.PRURL == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id, task_id and pr_url are required"})
		return
	}
	tp, err := c.service.AssociateExistingPRByURLForWorkspace(
		ctx.Request.Context(), req.WorkspaceID, currentGitHubUserID(ctx),
		req.TaskID, req.RepositoryID, req.PRURL,
	)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrInvalidPRURL) {
			status = http.StatusBadRequest
		}
		ctx.JSON(status, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, tp)
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

func (c *Controller) httpListTaskIssues(ctx *gin.Context) {
	workspaceID := ctx.Query("workspace_id")
	if workspaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id query parameter required"})
		return
	}
	result, err := c.service.ListWorkspaceTaskIssues(ctx.Request.Context(), workspaceID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"task_issues": result})
}

func (c *Controller) httpLinkTaskIssue(ctx *gin.Context) {
	var req LinkTaskIssueRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	resp, err := c.service.LinkTaskIssueForUser(
		ctx.Request.Context(), currentGitHubUserID(ctx), ctx.Param("taskId"), req,
	)
	if err != nil {
		c.handleTaskIssueLinkError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, resp)
}

func (c *Controller) httpUnlinkTaskIssue(ctx *gin.Context) {
	if err := c.service.UnlinkTaskIssue(ctx.Request.Context(), ctx.Param("taskId")); err != nil {
		c.handleTaskIssueLinkError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"unlinked": true})
}

func (c *Controller) handleTaskIssueLinkError(ctx *gin.Context, err error) {
	switch {
	case isGitHubNotConfiguredError(err):
		ctx.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "GitHub is not configured. Connect GitHub in Settings > Integrations.",
			"code":  "github_not_configured",
		})
	case errors.Is(err, ErrInvalidIssueReference):
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case errors.Is(err, ErrIssueRepositoryMismatch):
		ctx.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
	case errors.Is(err, errStoreUnavailable):
		c.logger.Error("task issue store unavailable", zap.Error(err))
		ctx.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "GitHub task issue linking is temporarily unavailable. Please try again.",
			"code":  "github_task_issue_unavailable",
		})
	case errors.Is(err, ErrTaskNotFound):
		ctx.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
	default:
		var apiErr *GitHubAPIError
		if errors.As(err, &apiErr) {
			switch apiErr.StatusCode {
			case http.StatusNotFound, http.StatusUnauthorized, http.StatusForbidden:
				c.logger.Debug("GitHub issue fetch failed", zap.Int("status", apiErr.StatusCode), zap.Error(err))
				ctx.JSON(apiErr.StatusCode, gin.H{"error": "failed to fetch GitHub issue"})
				return
			}
		}
		c.logger.Error("task issue link request failed", zap.Error(err))
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to link GitHub issue"})
	}
}

func (c *Controller) httpGetTaskCIOptions(ctx *gin.Context) {
	resp, err := c.service.GetTaskCIOptionsResponse(ctx.Request.Context(), ctx.Param("taskId"))
	if err != nil {
		c.logger.Error("get task CI options failed", zap.String("task_id", ctx.Param("taskId")), zap.Error(err))
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load CI automation options"})
		return
	}
	ctx.JSON(http.StatusOK, resp)
}

func (c *Controller) httpPatchTaskCIOptions(ctx *gin.Context) {
	patch, err := parseTaskCIOptionsPatch(ctx)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if !patch.HasAny() {
		resp, err := c.service.GetTaskCIOptionsResponse(ctx.Request.Context(), ctx.Param("taskId"))
		if err != nil {
			c.logger.Error("load task CI options failed", zap.String("task_id", ctx.Param("taskId")), zap.Error(err))
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load CI automation options"})
			return
		}
		ctx.JSON(http.StatusOK, resp)
		return
	}
	resp, err := c.service.UpdateTaskCIOptions(ctx.Request.Context(), ctx.Param("taskId"), patch)
	if err != nil {
		c.logger.Error("update task CI options failed", zap.String("task_id", ctx.Param("taskId")), zap.Error(err))
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update CI automation options"})
		return
	}
	c.publishTaskCIOptionsUpdated(ctx.Request.Context(), resp)
	ctx.JSON(http.StatusOK, resp)
}

func parseTaskCIOptionsPatch(ctx *gin.Context) (TaskCIOptionsPatch, error) {
	if ctx.Request.Body == nil || ctx.Request.ContentLength == 0 {
		return TaskCIOptionsPatch{}, nil
	}
	var raw map[string]json.RawMessage
	if err := json.NewDecoder(ctx.Request.Body).Decode(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			return TaskCIOptionsPatch{}, nil
		}
		return TaskCIOptionsPatch{}, err
	}
	var patch TaskCIOptionsPatch
	for key, value := range raw {
		switch key {
		case "auto_fix_enabled":
			var enabled bool
			if err := json.Unmarshal(value, &enabled); err != nil {
				return TaskCIOptionsPatch{}, err
			}
			patch.AutoFixEnabled = &enabled
		case "auto_merge_enabled":
			var enabled bool
			if err := json.Unmarshal(value, &enabled); err != nil {
				return TaskCIOptionsPatch{}, err
			}
			patch.AutoMergeEnabled = &enabled
		case "auto_fix_prompt_override":
			if string(value) == jsonNullLiteral {
				empty := ""
				patch.AutoFixPromptOverride = &empty
				continue
			}
			var prompt string
			if err := json.Unmarshal(value, &prompt); err != nil {
				return TaskCIOptionsPatch{}, err
			}
			patch.AutoFixPromptOverride = &prompt
		}
	}
	return patch, nil
}

func (c *Controller) publishTaskCIOptionsUpdated(ctx context.Context, resp *TaskCIOptionsResponse) {
	if c.service == nil || c.service.eventBus == nil || resp == nil {
		return
	}
	event := bus.NewEvent(events.GitHubTaskCIOptionsUpdated, "github", resp)
	if err := c.service.eventBus.Publish(ctx, events.GitHubTaskCIOptionsUpdated, event); err != nil {
		c.logger.Debug("publish task CI options update failed", zap.String("task_id", resp.TaskID), zap.Error(err))
	}
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
	feedback, err := c.service.GetPRFeedbackForWorkspace(
		ctx.Request.Context(), ctx.Query("workspace_id"), currentGitHubUserID(ctx), owner, repo, number,
	)
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
	status, err := c.service.GetPRStatusForWorkspace(
		ctx.Request.Context(), ctx.Query("workspace_id"), currentGitHubUserID(ctx), owner, repo, number,
	)
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
		WorkspaceID string  `json:"workspace_id"`
		Refs        []PRRef `json:"refs"`
	}
	if err := ctx.ShouldBindJSON(&body); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if len(body.Refs) > prStatusesBatchMaxRefs {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "too many refs"})
		return
	}
	statuses, err := c.service.GetPRStatusesBatchForWorkspace(
		ctx.Request.Context(), body.WorkspaceID, currentGitHubUserID(ctx), body.Refs,
	)
	if err != nil {
		c.handleSearchError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"statuses": statuses})
}

func (c *Controller) httpGetPRInfo(ctx *gin.Context) {
	owner := ctx.Param("owner")
	repo := ctx.Param("repo")
	number, ok := parseRouteNumber(ctx, "invalid PR number")
	if !ok {
		return
	}
	pr, err := c.service.GetPRForWorkspace(
		ctx.Request.Context(), ctx.Query("workspace_id"), currentGitHubUserID(ctx), owner, repo, number,
	)
	if err != nil {
		handleGitHubInfoError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, pr)
}

func (c *Controller) httpGetIssueInfo(ctx *gin.Context) {
	owner := ctx.Param("owner")
	repo := ctx.Param("repo")
	number, ok := parseRouteNumber(ctx, "invalid issue number")
	if !ok {
		return
	}
	issue, err := c.service.GetIssueForWorkspace(
		ctx.Request.Context(), ctx.Query("workspace_id"), currentGitHubUserID(ctx), owner, repo, number,
	)
	if err != nil {
		handleGitHubInfoError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, issue)
}

func parseRouteNumber(ctx *gin.Context, invalidMessage string) (int, bool) {
	number, err := strconv.Atoi(ctx.Param("number"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": invalidMessage})
		return 0, false
	}
	return number, true
}

func handleGitHubInfoError(ctx *gin.Context, err error) {
	if isGitHubNotConfiguredError(err) {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "GitHub is not configured. Connect GitHub in Settings > Integrations.",
			"code":  "github_not_configured",
		})
		return
	}
	status := http.StatusInternalServerError
	message := "failed to fetch GitHub info"
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
	if status != http.StatusInternalServerError {
		message = err.Error()
	}
	ctx.JSON(status, gin.H{"error": message})
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
	validEvents := map[string]bool{reviewEventApprove: true, "COMMENT": true, "REQUEST_CHANGES": true}
	if !validEvents[req.Event] {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "event must be APPROVE, COMMENT, or REQUEST_CHANGES"})
		return
	}
	principal, err := c.service.SubmitReviewForWorkspace(
		ctx.Request.Context(), ctx.Query("workspace_id"), currentGitHubUserID(ctx),
		owner, repo, number, req.Event, req.Body,
	)
	if err != nil {
		if errors.Is(err, ErrSelfApprove) {
			ctx.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
			return
		}
		if writeGitHubOperationalAuthError(ctx, err) {
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"submitted": true, "principal": principal})
}

func (c *Controller) httpMergePR(ctx *gin.Context) {
	owner := ctx.Param("owner")
	repo := ctx.Param("repo")
	numberStr := ctx.Param("number")
	number, err := strconv.Atoi(numberStr)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid PR number"})
		return
	}
	var req struct {
		MergeMethod string `json:"merge_method"`
	}
	// Body is optional — an empty body (io.EOF) means "use the repo default
	// merge method". A non-empty but malformed body is a client bug, not a
	// silent "use default", so reject it with 400.
	if bindErr := ctx.ShouldBindJSON(&req); bindErr != nil && !errors.Is(bindErr, io.EOF) {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	validMethods := map[string]bool{"": true, "merge": true, "squash": true, "rebase": true}
	if !validMethods[req.MergeMethod] {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "merge_method must be merge, squash, or rebase"})
		return
	}
	principal, err := c.service.MergePRForWorkspace(
		ctx.Request.Context(), ctx.Query("workspace_id"), currentGitHubUserID(ctx),
		owner, repo, number, req.MergeMethod,
	)
	if err != nil {
		if writeGitHubOperationalAuthError(ctx, err) {
			return
		}
		status := http.StatusInternalServerError
		var apiErr *GitHubAPIError
		if errors.As(err, &apiErr) {
			switch apiErr.StatusCode {
			case http.StatusMethodNotAllowed, http.StatusConflict:
				status = http.StatusConflict
			case http.StatusUnauthorized:
				status = http.StatusUnauthorized
			case http.StatusForbidden:
				status = http.StatusForbidden
			case http.StatusNotFound:
				status = http.StatusNotFound
			}
		}
		ctx.JSON(status, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"merged": true, "principal": principal})
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
	rw, err := c.service.CreateReviewWatchForUser(ctx.Request.Context(), currentGitHubUserID(ctx), &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusCreated, rw)
}

func (c *Controller) httpUpdateReviewWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	if !c.requireReviewWatchInWorkspace(ctx, id) {
		return
	}
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
	if !c.requireReviewWatchInWorkspace(ctx, id) {
		return
	}
	if err := c.service.DeleteReviewWatch(ctx.Request.Context(), id); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (c *Controller) httpTriggerReviewWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	if !c.requireReviewWatchInWorkspace(ctx, id) {
		return
	}
	watch, err := c.service.GetReviewWatch(ctx.Request.Context(), id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if watch == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "review watch not found"})
		return
	}
	newPRs, err := c.service.TriggerReviewWatch(ctx.Request.Context(), watch)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Clean up tasks for merged/closed PRs that haven't been started.
	cleaned, _ := c.service.CleanupMergedReviewTasks(ctx.Request.Context(), watch)
	ctx.JSON(http.StatusOK, gin.H{
		"new_prs":       len(newPRs),
		"new_prs_found": len(newPRs),
		"prs":           newPRs,
		"cleaned":       cleaned,
	})
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
	orgs, err := c.service.ListUserOrgsForWorkspace(
		ctx.Request.Context(), ctx.Query("workspace_id"), currentGitHubUserID(ctx),
	)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"orgs": orgs})
}

// httpListAccessibleRepos returns the merged list of repos the authenticated
// user can access (own repos + each org they belong to). Accepts:
//   - q: optional GitHub search qualifier appended to the per-source query
//   - limit: 1..100, defaults to 50 when missing / non-positive
//
// Returns 503 with `{"code":"github_not_configured"}` (matches every other
// GitHub-not-configured 503 in this file) when GitHub is not configured /
// not authenticated.
func (c *Controller) httpListAccessibleRepos(ctx *gin.Context) {
	query := ctx.Query("q")
	limit, _ := strconv.Atoi(ctx.Query("limit"))
	repos, err := c.service.ListAccessibleReposForWorkspace(
		ctx.Request.Context(), ctx.Query("workspace_id"), currentGitHubUserID(ctx), query, limit,
	)
	if err != nil {
		if isGitHubNotConfiguredError(err) {
			ctx.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "GitHub is not configured. Install the gh CLI and run 'gh auth login', or add a GITHUB_TOKEN secret.",
				"code":  "github_not_configured",
			})
			return
		}
		// Preserve GitHub-side status codes (401 / 403 / 404) so the
		// frontend can render the appropriate UX — auth re-prompt for 401,
		// permission notice for 403, etc. — instead of collapsing every
		// upstream failure into an opaque 500.
		status := http.StatusInternalServerError
		var apiErr *GitHubAPIError
		if errors.As(err, &apiErr) {
			switch apiErr.StatusCode {
			case http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound:
				status = apiErr.StatusCode
			}
		}
		ctx.JSON(status, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"repos": repos})
}

func (c *Controller) httpSearchRepos(ctx *gin.Context) {
	org := ctx.Query("org")
	query := ctx.Query("q")
	if org == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "org query parameter required"})
		return
	}
	repos, err := c.service.SearchOrgReposForWorkspace(
		ctx.Request.Context(), ctx.Query("workspace_id"), org, query, 20,
	)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"repos": repos})
}

func (c *Controller) httpListRepoBranches(ctx *gin.Context) {
	owner := ctx.Param("owner")
	repo := ctx.Param("repo")
	branches, err := c.service.ListRepoBranchesForWorkspace(
		ctx.Request.Context(), ctx.Query("workspace_id"), owner, repo,
	)
	if err != nil {
		if isGitHubNotConfiguredError(err) {
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

func (c *Controller) httpGetRepoMergeMethods(ctx *gin.Context) {
	owner := ctx.Param("owner")
	repo := ctx.Param("repo")
	methods, err := c.service.GetRepoMergeMethodsForWorkspace(
		ctx.Request.Context(), ctx.Query("workspace_id"), currentGitHubUserID(ctx), owner, repo,
	)
	if err != nil {
		if isGitHubNotConfiguredError(err) {
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
	ctx.JSON(http.StatusOK, methods)
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
	workspaceID := ctx.Query("workspace_id")
	page, perPage := parsePaginationQuery(ctx)
	var (
		result *PRSearchPage
		err    error
	)
	result, err = c.service.SearchUserPRsPagedForWorkspaceUser(
		ctx.Request.Context(), workspaceID, currentGitHubUserID(ctx), filter, query, page, perPage,
	)
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
	workspaceID := ctx.Query("workspace_id")
	page, perPage := parsePaginationQuery(ctx)
	var (
		result *IssueSearchPage
		err    error
	)
	result, err = c.service.SearchUserIssuesPagedForWorkspaceUser(
		ctx.Request.Context(), workspaceID, currentGitHubUserID(ctx), filter, query, page, perPage,
	)
	if err != nil {
		c.handleSearchError(ctx, err)
		return
	}
	if result.Issues == nil {
		result.Issues = []*Issue{}
	}
	ctx.JSON(http.StatusOK, result)
}

func (c *Controller) httpGetWorkspaceSettings(ctx *gin.Context) {
	workspaceID := ctx.Query("workspace_id")
	if strings.TrimSpace(workspaceID) == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id query parameter required"})
		return
	}
	settings, err := c.service.GetWorkspaceSettings(ctx.Request.Context(), workspaceID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, settings)
}

func (c *Controller) httpUpdateWorkspaceSettings(ctx *gin.Context) {
	var req UpdateWorkspaceSettingsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	settings, err := c.service.UpdateWorkspaceSettings(ctx.Request.Context(), &req)
	if err != nil {
		if errors.Is(err, ErrWorkspaceSettingsValidation) {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		} else {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}
	ctx.JSON(http.StatusOK, settings)
}

// copyWorkspaceSettingsRequest carries the target workspace for a copy. The
// source workspace comes from the workspace_id query param, matching the other
// integration copy endpoints.
type copyWorkspaceSettingsRequest struct {
	TargetWorkspaceID string `json:"targetWorkspaceId"`
}

func (c *Controller) httpCopyWorkspaceSettings(ctx *gin.Context) {
	source := strings.TrimSpace(ctx.Query("workspace_id"))
	if source == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id query parameter required"})
		return
	}
	var req copyWorkspaceSettingsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if strings.TrimSpace(req.TargetWorkspaceID) == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "targetWorkspaceId required"})
		return
	}
	settings, err := c.service.CopyWorkspaceSettingsToWorkspace(ctx.Request.Context(), source, req.TargetWorkspaceID)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, ErrSameWorkspace), errors.Is(err, ErrWorkspaceSettingsValidation):
			status = http.StatusBadRequest
		}
		ctx.JSON(status, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, settings)
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
	if isGitHubNotConfiguredError(err) {
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
	iw, err := c.service.CreateIssueWatchForWorkspace(ctx.Request.Context(), &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusCreated, iw)
}

func (c *Controller) httpUpdateIssueWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	if !c.requireIssueWatchInWorkspace(ctx, id) {
		return
	}
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
	if !c.requireIssueWatchInWorkspace(ctx, id) {
		return
	}
	if err := c.service.DeleteIssueWatch(ctx.Request.Context(), id); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (c *Controller) httpTriggerIssueWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	workspaceID := ctx.Query("workspace_id")
	if workspaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id query parameter required"})
		return
	}
	watch, err := c.service.GetIssueWatch(ctx.Request.Context(), id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if watch == nil || watch.WorkspaceID != workspaceID {
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

// httpCleanupReviewTasks runs a sweep over every review-PR dedup row
// (including rows under enabled watches) applying each watch's cleanup
// policy. Manual trigger for users to drain a pile of merged-PR tasks
// without waiting for the next 5-minute poller cycle.
//
// SCOPE: install-wide — walks dedup rows across all workspaces. The
// trigger-all endpoints (httpTriggerAllReviewChecks /
// httpTriggerAllIssueChecks) take a required workspace_id, but cleanup
// is install-wide on purpose so a user can drain orphans whose original
// watch lived in a workspace they no longer have open. A future
// multi-tenant rollout would add an optional workspace_id query param.
func (c *Controller) httpCleanupReviewTasks(ctx *gin.Context) {
	workspaceID := ctx.Query("workspace_id")
	var (
		deleted int
		err     error
	)
	if workspaceID == "" {
		deleted, err = c.service.CleanupAllReviewTasks(ctx.Request.Context())
	} else {
		deleted, err = c.service.CleanupReviewTasksForWorkspace(ctx.Request.Context(), workspaceID)
	}
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": deleted})
}

// httpCleanupIssueTasks mirrors httpCleanupReviewTasks for issue watches.
func (c *Controller) httpCleanupIssueTasks(ctx *gin.Context) {
	workspaceID := ctx.Query("workspace_id")
	var (
		deleted int
		err     error
	)
	if workspaceID == "" {
		deleted, err = c.service.CleanupAllIssueTasks(ctx.Request.Context())
	} else {
		deleted, err = c.service.CleanupIssueTasksForWorkspace(ctx.Request.Context(), workspaceID)
	}
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": deleted})
}
