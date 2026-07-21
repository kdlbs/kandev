package gitlab

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
)

const mockWorkspaceTokenPrefix = "e2e-gitlab:"

// MockController exposes HTTP endpoints to seed the in-memory MockClient.
// Activated only when KANDEV_MOCK_GITLAB=true.
type MockController struct {
	mock    *MockClient
	service *Service
	logger  *logger.Logger
}

// NewMockController creates a new MockController.
func NewMockController(mock *MockClient, svc *Service, log *logger.Logger) *MockController {
	return &MockController{mock: mock, service: svc, logger: log}
}

// RegisterRoutes wires the mock control endpoints under /api/v1/gitlab/mock.
func (c *MockController) RegisterRoutes(router *gin.Engine) {
	api := router.Group("/api/v1/gitlab/mock")
	api.PUT("/user", c.setUser)
	api.POST("/mrs", c.seedMRs)
	api.POST("/issues", c.seedIssues)
	api.POST("/pipelines", c.seedPipelines)
	api.POST("/discussions", c.seedDiscussions)
	api.POST("/approvals", c.seedApprovals)
	api.POST("/branches", c.seedBranches)
	api.POST("/members", c.seedMembers)
	api.POST("/files", c.seedFiles)
	api.POST("/commits", c.seedCommits)
	api.DELETE("/reset", c.reset)

	// The agentctl GitLab creator uses the upstream REST v4 shape. Keeping
	// this route in the mock controller lets Playwright exercise that real
	// code path without an external GitLab instance.
	router.Any("/api/v4/projects/*path", c.gitLabAPI)
}

type mockCreateMRRequest struct {
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	Title        string `json:"title"`
	Description  string `json:"description"`
}

func (c *MockController) gitLabAPI(ctx *gin.Context) {
	workspaceID := strings.TrimPrefix(ctx.GetHeader("PRIVATE-TOKEN"), mockWorkspaceTokenPrefix)
	if workspaceID == "" || workspaceID == ctx.GetHeader("PRIVATE-TOKEN") {
		ctx.JSON(http.StatusUnauthorized, gin.H{"message": "invalid mock GitLab token"})
		return
	}
	client, err := c.service.ClientForWorkspace(ctx.Request.Context(), workspaceID)
	if err != nil {
		ctx.JSON(http.StatusUnauthorized, gin.H{"message": "unknown mock GitLab workspace"})
		return
	}
	mock, ok := client.(*MockClient)
	if !ok {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"message": "mock GitLab client unavailable"})
		return
	}

	rawPath := strings.TrimPrefix(ctx.Param("path"), "/")
	decodedPath, decodeErr := url.PathUnescape(rawPath)
	if decodeErr != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"message": "invalid project path"})
		return
	}
	const mergeRequestsSuffix = "/merge_requests"
	if !strings.HasSuffix(decodedPath, mergeRequestsSuffix) {
		if ctx.Request.Method == http.MethodGet {
			ctx.JSON(http.StatusOK, gin.H{"default_branch": "main"})
			return
		}
		ctx.JSON(http.StatusNotFound, gin.H{"message": "mock GitLab endpoint not found"})
		return
	}
	projectPath := strings.TrimSuffix(decodedPath, mergeRequestsSuffix)
	if strings.TrimSpace(projectPath) == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"message": "project path required"})
		return
	}

	switch ctx.Request.Method {
	case http.MethodGet:
		c.listCreatedMRs(ctx, mock, projectPath)
	case http.MethodPost:
		c.createMR(ctx, mock, projectPath)
	default:
		ctx.Status(http.StatusMethodNotAllowed)
	}
}

func (c *MockController) listCreatedMRs(ctx *gin.Context, mock *MockClient, projectPath string) {
	mrs, _ := mock.SearchMRs(ctx.Request.Context(), "", "")
	sourceBranch := ctx.Query("source_branch")
	targetBranch := ctx.Query("target_branch")
	rows := make([]gin.H, 0)
	for _, mr := range mrs {
		if mr.ProjectPath != projectPath || (sourceBranch != "" && mr.HeadBranch != sourceBranch) ||
			(targetBranch != "" && mr.BaseBranch != targetBranch) {
			continue
		}
		rows = append(rows, gin.H{
			"web_url": mr.WebURL, "source_branch": mr.HeadBranch, "target_branch": mr.BaseBranch,
		})
	}
	ctx.JSON(http.StatusOK, rows)
}

func (c *MockController) createMR(ctx *gin.Context, mock *MockClient, projectPath string) {
	var req mockCreateMRRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.SourceBranch) == "" ||
		strings.TrimSpace(req.TargetBranch) == "" || strings.TrimSpace(req.Title) == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"message": "source_branch, target_branch, and title required"})
		return
	}
	mr, err := mock.CreateMR(ctx.Request.Context(), projectPath, req.SourceBranch, req.TargetBranch,
		req.Title, req.Description, strings.HasPrefix(strings.ToLower(strings.TrimSpace(req.Title)), "draft:"))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	ctx.JSON(http.StatusCreated, gin.H{
		"id": mr.ID, "iid": mr.IID, "web_url": mr.WebURL,
		"source_branch": mr.HeadBranch, "target_branch": mr.BaseBranch,
	})
}

type mockMRRequest struct {
	Project string `json:"project"`
	MRs     []MR   `json:"mrs"`
}

type mockIssueRequest struct {
	Project string  `json:"project"`
	Issues  []Issue `json:"issues"`
}

type mockPipelineRequest struct {
	Project   string     `json:"project"`
	Pipelines []Pipeline `json:"pipelines"`
}

type mockDiscussionRequest struct {
	Project     string         `json:"project"`
	IID         int            `json:"iid"`
	Discussions []MRDiscussion `json:"discussions"`
}

type mockApprovalRequest struct {
	Project   string       `json:"project"`
	IID       int          `json:"iid"`
	Approvals []MRApproval `json:"approvals"`
	Required  int          `json:"required"`
}

type mockBranchesRequest struct {
	Project  string       `json:"project"`
	Branches []RepoBranch `json:"branches"`
}

type mockMembersRequest struct {
	Project string          `json:"project"`
	Members []ProjectMember `json:"members"`
}

type mockFilesRequest struct {
	Project string   `json:"project"`
	IID     int      `json:"iid"`
	Files   []MRFile `json:"files"`
}

type mockCommitsRequest struct {
	Project string         `json:"project"`
	IID     int            `json:"iid"`
	Commits []MRCommitInfo `json:"commits"`
}

func (c *MockController) mockClient(ctx *gin.Context) (*MockClient, bool) {
	workspaceID := strings.TrimSpace(ctx.Query("workspace_id"))
	if workspaceID == "" {
		return c.mock, true
	}
	client, err := c.service.ClientForWorkspace(ctx.Request.Context(), workspaceID)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, ErrNotConfigured) {
			status = http.StatusServiceUnavailable
		}
		ctx.JSON(status, gin.H{"error": "mock GitLab workspace client unavailable"})
		return nil, false
	}
	mock, ok := client.(*MockClient)
	if !ok {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "mock GitLab client unavailable"})
		return nil, false
	}
	return mock, true
}

func (c *MockController) setUser(ctx *gin.Context) {
	var req struct {
		Username string `json:"username"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	mock, ok := c.mockClient(ctx)
	if !ok {
		return
	}
	mock.SetUser(req.Username)
	ctx.JSON(http.StatusOK, gin.H{"username": req.Username})
}

func (c *MockController) seedMRs(ctx *gin.Context) {
	var req mockMRRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Project == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "project and mrs required"})
		return
	}
	mock, ok := c.mockClient(ctx)
	if !ok {
		return
	}
	for i := range req.MRs {
		mock.SeedMR(req.Project, &req.MRs[i])
	}
	ctx.JSON(http.StatusOK, gin.H{"seeded": len(req.MRs)})
}

func (c *MockController) seedIssues(ctx *gin.Context) {
	var req mockIssueRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Project == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "project and issues required"})
		return
	}
	mock, ok := c.mockClient(ctx)
	if !ok {
		return
	}
	for i := range req.Issues {
		mock.SeedIssue(req.Project, &req.Issues[i])
	}
	ctx.JSON(http.StatusOK, gin.H{"seeded": len(req.Issues)})
}

func (c *MockController) seedPipelines(ctx *gin.Context) {
	var req mockPipelineRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Project == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "project and pipelines required"})
		return
	}
	mock, ok := c.mockClient(ctx)
	if !ok {
		return
	}
	mock.SeedPipelines(req.Project, req.Pipelines)
	ctx.JSON(http.StatusOK, gin.H{"seeded": len(req.Pipelines)})
}

func (c *MockController) seedDiscussions(ctx *gin.Context) {
	var req mockDiscussionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Project == "" || req.IID <= 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "project, iid, discussions required"})
		return
	}
	mock, ok := c.mockClient(ctx)
	if !ok {
		return
	}
	mock.SeedDiscussions(req.Project, req.IID, req.Discussions)
	ctx.JSON(http.StatusOK, gin.H{"seeded": len(req.Discussions)})
}

func (c *MockController) seedApprovals(ctx *gin.Context) {
	var req mockApprovalRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Project == "" || req.IID <= 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "project and iid required"})
		return
	}
	mock, ok := c.mockClient(ctx)
	if !ok {
		return
	}
	mock.SeedApprovals(req.Project, req.IID, req.Approvals, req.Required)
	ctx.JSON(http.StatusOK, gin.H{"seeded": len(req.Approvals), "required": req.Required})
}

func (c *MockController) seedBranches(ctx *gin.Context) {
	var req mockBranchesRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Project == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "project and branches required"})
		return
	}
	mock, ok := c.mockClient(ctx)
	if !ok {
		return
	}
	mock.SeedBranches(req.Project, req.Branches)
	ctx.JSON(http.StatusOK, gin.H{"seeded": len(req.Branches)})
}

func (c *MockController) seedMembers(ctx *gin.Context) {
	var req mockMembersRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Project == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "project and members required"})
		return
	}
	mock, ok := c.mockClient(ctx)
	if !ok {
		return
	}
	mock.SeedProjectMembers(req.Project, req.Members)
	ctx.JSON(http.StatusOK, gin.H{"seeded": len(req.Members)})
}

func (c *MockController) seedFiles(ctx *gin.Context) {
	var req mockFilesRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Project == "" || req.IID <= 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "project, iid, and files required"})
		return
	}
	mock, ok := c.mockClient(ctx)
	if !ok {
		return
	}
	mock.SeedFiles(req.Project, req.IID, req.Files)
	ctx.JSON(http.StatusOK, gin.H{"seeded": len(req.Files)})
}

func (c *MockController) seedCommits(ctx *gin.Context) {
	var req mockCommitsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Project == "" || req.IID <= 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "project, iid, and commits required"})
		return
	}
	mock, ok := c.mockClient(ctx)
	if !ok {
		return
	}
	mock.SeedCommits(req.Project, req.IID, req.Commits)
	ctx.JSON(http.StatusOK, gin.H{"seeded": len(req.Commits)})
}

func (c *MockController) reset(ctx *gin.Context) {
	mock, ok := c.mockClient(ctx)
	if !ok {
		return
	}
	mock.Reset()
	ctx.JSON(http.StatusOK, gin.H{"reset": true})
}
