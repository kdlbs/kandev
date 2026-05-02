package linear

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
)

// MockController exposes HTTP endpoints that drive the in-memory MockClient
// during E2E tests. Mounted by RegisterMockRoutes only when the service was
// built with a MockClient.
type MockController struct {
	mock  *MockClient
	store *Store
	log   *logger.Logger
}

// NewMockController wires the controller to the mock + store so auth-health
// writes can be triggered synchronously from tests.
func NewMockController(mock *MockClient, store *Store, log *logger.Logger) *MockController {
	return &MockController{mock: mock, store: store, log: log}
}

// RegisterRoutes mounts the mock control routes under /api/v1/linear/mock.
func (c *MockController) RegisterRoutes(router *gin.Engine) {
	api := router.Group("/api/v1/linear/mock")
	api.PUT("/auth-result", c.setAuthResult)
	api.PUT("/auth-health", c.setAuthHealth)
	api.POST("/teams", c.setTeams)
	api.POST("/states", c.setStates)
	api.POST("/issues", c.addIssues)
	api.PUT("/get-issue-error", c.setGetIssueError)
	api.DELETE("/reset", c.reset)
}

func (c *MockController) setAuthResult(ctx *gin.Context) {
	var req TestConnectionResult
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	r := req
	c.mock.SetAuthResult(&r)
	ctx.JSON(http.StatusOK, gin.H{"set": true})
}

func (c *MockController) setAuthHealth(ctx *gin.Context) {
	var req struct {
		OK      bool   `json:"ok"`
		Error   string `json:"error"`
		OrgSlug string `json:"orgSlug"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if err := c.store.UpdateAuthHealth(ctx.Request.Context(), req.OK, req.Error, req.OrgSlug, time.Now().UTC()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"set": true})
}

func (c *MockController) setTeams(ctx *gin.Context) {
	var req struct {
		Teams []LinearTeam `json:"teams"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	c.mock.SetTeams(req.Teams)
	ctx.JSON(http.StatusOK, gin.H{"count": len(req.Teams)})
}

func (c *MockController) setStates(ctx *gin.Context) {
	var req struct {
		TeamKey string                `json:"teamKey"`
		States  []LinearWorkflowState `json:"states"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil || req.TeamKey == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "teamKey required"})
		return
	}
	c.mock.SetStates(req.TeamKey, req.States)
	ctx.JSON(http.StatusOK, gin.H{"count": len(req.States)})
}

func (c *MockController) addIssues(ctx *gin.Context) {
	var req struct {
		Issues []LinearIssue `json:"issues"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	for i := range req.Issues {
		c.mock.AddIssue(&req.Issues[i])
	}
	ctx.JSON(http.StatusOK, gin.H{"added": len(req.Issues)})
}

func (c *MockController) setGetIssueError(ctx *gin.Context) {
	var req struct {
		StatusCode int    `json:"statusCode"`
		Message    string `json:"message"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if req.StatusCode == 0 {
		c.mock.SetGetIssueError(nil)
	} else {
		c.mock.SetGetIssueError(&APIError{StatusCode: req.StatusCode, Message: req.Message})
	}
	ctx.JSON(http.StatusOK, gin.H{"set": true})
}

func (c *MockController) reset(ctx *gin.Context) {
	c.mock.Reset()
	ctx.JSON(http.StatusOK, gin.H{"reset": true})
}
