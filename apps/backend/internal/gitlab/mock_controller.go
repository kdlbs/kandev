package gitlab

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
)

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

func (c *MockController) seedMRs(ctx *gin.Context) {
	var req mockMRRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Project == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "project and mrs required"})
		return
	}
	for i := range req.MRs {
		c.mock.SeedMR(req.Project, &req.MRs[i])
	}
	ctx.JSON(http.StatusOK, gin.H{"seeded": len(req.MRs)})
}

func (c *MockController) seedIssues(ctx *gin.Context) {
	var req mockIssueRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Project == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "project and issues required"})
		return
	}
	for i := range req.Issues {
		c.mock.SeedIssue(req.Project, &req.Issues[i])
	}
	ctx.JSON(http.StatusOK, gin.H{"seeded": len(req.Issues)})
}

func (c *MockController) seedPipelines(ctx *gin.Context) {
	var req mockPipelineRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Project == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "project and pipelines required"})
		return
	}
	c.mock.SeedPipelines(req.Project, req.Pipelines)
	ctx.JSON(http.StatusOK, gin.H{"seeded": len(req.Pipelines)})
}

func (c *MockController) seedDiscussions(ctx *gin.Context) {
	var req mockDiscussionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Project == "" || req.IID <= 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "project, iid, discussions required"})
		return
	}
	c.mock.SeedDiscussions(req.Project, req.IID, req.Discussions)
	ctx.JSON(http.StatusOK, gin.H{"seeded": len(req.Discussions)})
}

func (c *MockController) seedApprovals(ctx *gin.Context) {
	var req mockApprovalRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Project == "" || req.IID <= 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "project and iid required"})
		return
	}
	c.mock.SeedApprovals(req.Project, req.IID, req.Approvals, req.Required)
	ctx.JSON(http.StatusOK, gin.H{"seeded": len(req.Approvals), "required": req.Required})
}

func (c *MockController) seedBranches(ctx *gin.Context) {
	var req mockBranchesRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.Project == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "project and branches required"})
		return
	}
	c.mock.SeedBranches(req.Project, req.Branches)
	ctx.JSON(http.StatusOK, gin.H{"seeded": len(req.Branches)})
}
