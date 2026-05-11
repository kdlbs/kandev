package github

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
)

const defaultPRState = "open"

// MockController handles HTTP endpoints for controlling the MockClient in E2E tests.
type MockController struct {
	mock     *MockClient
	store    *Store
	eventBus bus.EventBus
	logger   *logger.Logger
}

// NewMockController creates a new MockController.
func NewMockController(mock *MockClient, store *Store, eventBus bus.EventBus, log *logger.Logger) *MockController {
	return &MockController{mock: mock, store: store, eventBus: eventBus, logger: log}
}

// RegisterRoutes registers all mock control HTTP routes.
func (c *MockController) RegisterRoutes(router *gin.Engine) {
	api := router.Group("/api/v1/github/mock")
	api.PUT("/user", c.setUser)
	api.POST("/prs", c.addPRs)
	api.POST("/orgs", c.addOrgs)
	api.POST("/repos", c.addRepos)
	api.POST("/reviews", c.addReviews)
	api.POST("/comments", c.addComments)
	api.POST("/checks", c.addCheckRuns)
	api.POST("/files", c.addPRFiles)
	api.POST("/commits", c.addPRCommits)
	api.POST("/branches", c.addBranches)
	api.POST("/task-prs", c.associateTaskPR)
	api.POST("/pr-feedback", c.seedPRFeedback)
	api.PUT("/auth-health", c.setAuthHealth)
	api.DELETE("/reset", c.reset)
}

func (c *MockController) setUser(ctx *gin.Context) {
	var req struct {
		Username string `json:"username"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	c.mock.SetUser(req.Username)
	ctx.JSON(http.StatusOK, gin.H{"username": req.Username})
}

func (c *MockController) addPRs(ctx *gin.Context) {
	var req struct {
		PRs []PR `json:"prs"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	for i := range req.PRs {
		c.mock.AddPR(&req.PRs[i])
	}
	ctx.JSON(http.StatusOK, gin.H{"added": len(req.PRs)})
}

func (c *MockController) addOrgs(ctx *gin.Context) {
	var req struct {
		Orgs []GitHubOrg `json:"orgs"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	c.mock.AddOrgs(req.Orgs)
	ctx.JSON(http.StatusOK, gin.H{"added": len(req.Orgs)})
}

func (c *MockController) addRepos(ctx *gin.Context) {
	var req struct {
		Org   string       `json:"org"`
		Repos []GitHubRepo `json:"repos"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	c.mock.AddRepos(req.Org, req.Repos)
	ctx.JSON(http.StatusOK, gin.H{"added": len(req.Repos)})
}

func (c *MockController) addReviews(ctx *gin.Context) {
	var req struct {
		Owner   string     `json:"owner"`
		Repo    string     `json:"repo"`
		Number  int        `json:"number"`
		Reviews []PRReview `json:"reviews"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	c.mock.AddReviews(req.Owner, req.Repo, req.Number, req.Reviews)
	ctx.JSON(http.StatusOK, gin.H{"added": len(req.Reviews)})
}

func (c *MockController) addComments(ctx *gin.Context) {
	var req struct {
		Owner    string      `json:"owner"`
		Repo     string      `json:"repo"`
		Number   int         `json:"number"`
		Comments []PRComment `json:"comments"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	c.mock.AddComments(req.Owner, req.Repo, req.Number, req.Comments)
	ctx.JSON(http.StatusOK, gin.H{"added": len(req.Comments)})
}

func (c *MockController) addCheckRuns(ctx *gin.Context) {
	var req struct {
		Owner  string     `json:"owner"`
		Repo   string     `json:"repo"`
		Ref    string     `json:"ref"`
		Checks []CheckRun `json:"checks"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	c.mock.AddCheckRuns(req.Owner, req.Repo, req.Ref, req.Checks)
	ctx.JSON(http.StatusOK, gin.H{"added": len(req.Checks)})
}

func (c *MockController) addPRFiles(ctx *gin.Context) {
	var req struct {
		Owner  string   `json:"owner"`
		Repo   string   `json:"repo"`
		Number int      `json:"number"`
		Files  []PRFile `json:"files"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	c.mock.AddPRFiles(req.Owner, req.Repo, req.Number, req.Files)
	ctx.JSON(http.StatusOK, gin.H{"added": len(req.Files)})
}

func (c *MockController) addPRCommits(ctx *gin.Context) {
	var req struct {
		Owner   string         `json:"owner"`
		Repo    string         `json:"repo"`
		Number  int            `json:"number"`
		Commits []PRCommitInfo `json:"commits"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	c.mock.AddPRCommits(req.Owner, req.Repo, req.Number, req.Commits)
	ctx.JSON(http.StatusOK, gin.H{"added": len(req.Commits)})
}

func (c *MockController) addBranches(ctx *gin.Context) {
	var req struct {
		Owner    string       `json:"owner"`
		Repo     string       `json:"repo"`
		Branches []RepoBranch `json:"branches"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if req.Owner == "" || req.Repo == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "owner and repo are required"})
		return
	}
	c.mock.AddBranches(req.Owner, req.Repo, req.Branches)
	ctx.JSON(http.StatusOK, gin.H{"added": len(req.Branches)})
}

// associateTaskPRRequest is the JSON body for the mock-controller's
// associateTaskPR endpoint. Pointer fields are optional — leave them nil
// to skip the corresponding TaskPR column update.
type associateTaskPRRequest struct {
	TaskID                  string `json:"task_id"`
	Owner                   string `json:"owner"`
	Repo                    string `json:"repo"`
	PRNumber                int    `json:"pr_number"`
	PRURL                   string `json:"pr_url"`
	PRTitle                 string `json:"pr_title"`
	HeadBranch              string `json:"head_branch"`
	BaseBranch              string `json:"base_branch"`
	AuthorLogin             string `json:"author_login"`
	State                   string `json:"state"`
	ReviewState             string `json:"review_state"`
	ChecksState             string `json:"checks_state"`
	MergeableState          string `json:"mergeable_state"`
	Additions               int    `json:"additions"`
	Deletions               int    `json:"deletions"`
	ReviewCount             *int   `json:"review_count,omitempty"`
	PendingReviewCount      *int   `json:"pending_review_count,omitempty"`
	RequiredReviews         *int   `json:"required_reviews,omitempty"`
	ChecksTotal             *int   `json:"checks_total,omitempty"`
	ChecksPassing           *int   `json:"checks_passing,omitempty"`
	UnresolvedReviewThreads *int   `json:"unresolved_review_threads,omitempty"`
}

// associateTaskPR directly creates (or replaces) a github_task_prs record for
// E2E testing. Repeating the call with the same (task_id, owner, repo,
// pr_number) updates the row so tests can drive the live-refresh
// scenario by re-POSTing with new aggregate counts.
func (c *MockController) associateTaskPR(ctx *gin.Context) {
	var req associateTaskPRRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if req.State == "" {
		req.State = defaultPRState
	}
	now := time.Now().UTC()
	tp := buildTaskPRFromRequest(&req, now)
	if err := c.store.ReplaceTaskPR(ctx.Request.Context(), tp); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.ensureMockPRForRequest(ctx.Request.Context(), &req, now)
	// Publish the event so the frontend Zustand store picks up the new PR
	// without requiring a page reload — mirrors real AssociatePRWithTask.
	if c.eventBus != nil {
		event := bus.NewEvent(events.GitHubTaskPRUpdated, "github", tp)
		if err := c.eventBus.Publish(ctx.Request.Context(), events.GitHubTaskPRUpdated, event); err != nil {
			c.logger.Debug("mock: failed to publish task PR updated event", zap.Error(err))
		}
	}
	ctx.JSON(http.StatusCreated, tp)
}

// buildTaskPRFromRequest copies the required fields from the JSON body into
// a TaskPR and applies the optional pointer fields when present.
func buildTaskPRFromRequest(req *associateTaskPRRequest, now time.Time) *TaskPR {
	tp := &TaskPR{
		TaskID:         req.TaskID,
		Owner:          req.Owner,
		Repo:           req.Repo,
		PRNumber:       req.PRNumber,
		PRURL:          req.PRURL,
		PRTitle:        req.PRTitle,
		HeadBranch:     req.HeadBranch,
		BaseBranch:     req.BaseBranch,
		AuthorLogin:    req.AuthorLogin,
		State:          req.State,
		ReviewState:    req.ReviewState,
		ChecksState:    req.ChecksState,
		MergeableState: req.MergeableState,
		Additions:      req.Additions,
		Deletions:      req.Deletions,
		CreatedAt:      now,
	}
	if req.ReviewCount != nil {
		tp.ReviewCount = *req.ReviewCount
	}
	if req.PendingReviewCount != nil {
		tp.PendingReviewCount = *req.PendingReviewCount
	}
	if req.RequiredReviews != nil {
		v := *req.RequiredReviews
		tp.RequiredReviews = &v
	}
	if req.ChecksTotal != nil {
		tp.ChecksTotal = *req.ChecksTotal
	}
	if req.ChecksPassing != nil {
		tp.ChecksPassing = *req.ChecksPassing
	}
	if req.UnresolvedReviewThreads != nil {
		tp.UnresolvedReviewThreads = *req.UnresolvedReviewThreads
	}
	return tp
}

// ensureMockPRForRequest seeds a synthetic PR in the mock client so the lazy
// PRFeedback path can resolve check runs by HeadSHA. Skipped when the PR was
// already seeded explicitly via addPRs (tests that pin a head_sha for
// matching check_runs must not have it overwritten).
func (c *MockController) ensureMockPRForRequest(ctx context.Context, req *associateTaskPRRequest, now time.Time) {
	if existing, _ := c.mock.GetPR(ctx, req.Owner, req.Repo, req.PRNumber); existing != nil {
		return
	}
	c.mock.AddPR(&PR{
		Number:      req.PRNumber,
		Title:       req.PRTitle,
		URL:         req.PRURL,
		HTMLURL:     req.PRURL,
		State:       req.State,
		HeadBranch:  req.HeadBranch,
		HeadSHA:     mockHeadSHA(req.Owner, req.Repo, req.PRNumber),
		BaseBranch:  req.BaseBranch,
		AuthorLogin: req.AuthorLogin,
		RepoOwner:   req.Owner,
		RepoName:    req.Repo,
		Additions:   req.Additions,
		Deletions:   req.Deletions,
		CreatedAt:   now,
		UpdatedAt:   now,
	})
}

// seedPRFeedback registers checks (and optionally reviews / comments) for a
// specific PR. The PR must already exist in the mock client (associateTaskPR
// synthesizes one). The check ref is the PR's HeadSHA so getPRFeedback
// resolves them via ListCheckRuns.
func (c *MockController) seedPRFeedback(ctx *gin.Context) {
	var req struct {
		Owner    string      `json:"owner"`
		Repo     string      `json:"repo"`
		PRNumber int         `json:"pr_number"`
		Checks   []CheckRun  `json:"checks"`
		Reviews  []PRReview  `json:"reviews"`
		Comments []PRComment `json:"comments"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if req.Owner == "" || req.Repo == "" || req.PRNumber == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "owner, repo, pr_number are required"})
		return
	}
	headSHA := mockHeadSHA(req.Owner, req.Repo, req.PRNumber)
	// If associateTaskPR ran first, an underlying PR row exists with this
	// HeadSHA. Otherwise synthesize a minimal one so getPRFeedback's GetPR
	// call doesn't fail.
	if pr, err := c.mock.GetPR(ctx.Request.Context(), req.Owner, req.Repo, req.PRNumber); err != nil || pr == nil {
		c.mock.AddPR(&PR{
			Number:    req.PRNumber,
			RepoOwner: req.Owner,
			RepoName:  req.Repo,
			HeadSHA:   headSHA,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		})
	}
	// Replace prior seeded checks/reviews/comments so a follow-up call gives
	// deterministic state; helpful for tests that drive a "then a check
	// finishes" transition.
	c.mock.ReplaceCheckRuns(req.Owner, req.Repo, headSHA, req.Checks)
	c.mock.ReplaceReviews(req.Owner, req.Repo, req.PRNumber, req.Reviews)
	c.mock.ReplaceComments(req.Owner, req.Repo, req.PRNumber, req.Comments)
	ctx.JSON(http.StatusOK, gin.H{
		"checks":   len(req.Checks),
		"reviews":  len(req.Reviews),
		"comments": len(req.Comments),
	})
}

// setAuthHealth toggles the mock client's authenticated state so e2e tests
// can drive the "Reconnect GitHub" branch in the popover.
func (c *MockController) setAuthHealth(ctx *gin.Context) {
	var req struct {
		Authenticated bool   `json:"authenticated"`
		Error         string `json:"error"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	c.mock.SetAuthHealth(req.Authenticated, req.Error)
	ctx.JSON(http.StatusOK, gin.H{"authenticated": req.Authenticated})
}

// mockHeadSHA produces a deterministic synthetic head SHA for a (owner, repo,
// pr_number) tuple so test seed calls can compute the same key.
func mockHeadSHA(owner, repo string, prNumber int) string {
	return fmt.Sprintf("mocksha-%s-%s-%d", owner, repo, prNumber)
}

func (c *MockController) reset(ctx *gin.Context) {
	c.mock.Reset()
	ctx.JSON(http.StatusOK, gin.H{"reset": true})
}
