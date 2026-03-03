package github

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
)

const defaultPRState = "open"

// MockController handles HTTP endpoints for controlling the MockClient in E2E tests.
type MockController struct {
	mock  *MockClient
	store *Store
}

// NewMockController creates a new MockController.
func NewMockController(mock *MockClient, store *Store, _ *logger.Logger) *MockController {
	return &MockController{mock: mock, store: store}
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
	api.POST("/task-prs", c.associateTaskPR)
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

// associateTaskPR directly creates a github_task_prs record for E2E testing.
func (c *MockController) associateTaskPR(ctx *gin.Context) {
	var req struct {
		TaskID      string `json:"task_id"`
		Owner       string `json:"owner"`
		Repo        string `json:"repo"`
		PRNumber    int    `json:"pr_number"`
		PRURL       string `json:"pr_url"`
		PRTitle     string `json:"pr_title"`
		HeadBranch  string `json:"head_branch"`
		BaseBranch  string `json:"base_branch"`
		AuthorLogin string `json:"author_login"`
		State       string `json:"state"`
		Additions   int    `json:"additions"`
		Deletions   int    `json:"deletions"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if req.State == "" {
		req.State = defaultPRState
	}
	now := time.Now().UTC()
	tp := &TaskPR{
		TaskID:      req.TaskID,
		Owner:       req.Owner,
		Repo:        req.Repo,
		PRNumber:    req.PRNumber,
		PRURL:       req.PRURL,
		PRTitle:     req.PRTitle,
		HeadBranch:  req.HeadBranch,
		BaseBranch:  req.BaseBranch,
		AuthorLogin: req.AuthorLogin,
		State:       req.State,
		Additions:   req.Additions,
		Deletions:   req.Deletions,
		CreatedAt:   now,
	}
	if err := c.store.CreateTaskPR(ctx.Request.Context(), tp); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusCreated, tp)
}

func (c *MockController) reset(ctx *gin.Context) {
	c.mock.Reset()
	ctx.JSON(http.StatusOK, gin.H{"reset": true})
}
