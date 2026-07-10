package sentry

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
)

// MockController exposes HTTP endpoints that drive the in-memory MockClient
// during E2E tests. Mounted by RegisterMockRoutes only when the service was
// built with a MockClient. Every data endpoint is scoped to an instance via the
// `?instanceId=` query param so tests can seed distinct datasets per instance.
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

// RegisterRoutes mounts the mock control routes under /api/v1/sentry/mock.
func (c *MockController) RegisterRoutes(router *gin.Engine) {
	api := router.Group("/api/v1/sentry/mock")
	api.PUT("/auth-result", c.setAuthResult)
	api.PUT("/auth-health", c.setAuthHealth)
	api.POST("/organizations", c.setOrganizations)
	api.POST("/projects", c.setProjects)
	api.POST("/issues", c.addIssues)
	api.PUT("/get-issue-error", c.setGetIssueError)
	api.DELETE("/reset", c.reset)
}

func (c *MockController) requireInstanceID(ctx *gin.Context) (string, bool) {
	instanceID := strings.TrimSpace(ctx.Query("instanceId"))
	if instanceID == "" {
		writeErr(ctx, http.StatusBadRequest, "instanceId query parameter required")
		return "", false
	}
	return instanceID, true
}

// setAuthResult seeds an instance's TestAuth response. instanceId is
// optional (unlike the other seed routes): TestConnectionCandidate builds a
// client for a not-yet-persisted config using the empty-string instance ID,
// so pre-save "Test connection" specs must be able to seed that dataset too.
func (c *MockController) setAuthResult(ctx *gin.Context) {
	var req TestConnectionResult
	if err := ctx.ShouldBindJSON(&req); err != nil {
		writeBadPayload(ctx)
		return
	}
	instanceID := strings.TrimSpace(ctx.Query("instanceId"))
	r := req
	c.mock.SetAuthResult(instanceID, &r)
	ctx.JSON(http.StatusOK, gin.H{"set": true})
}

func (c *MockController) setAuthHealth(ctx *gin.Context) {
	var req struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		writeBadPayload(ctx)
		return
	}
	instanceID, ok := c.requireInstanceID(ctx)
	if !ok {
		return
	}
	if err := c.store.UpdateAuthHealthForInstance(ctx.Request.Context(), instanceID, req.OK, req.Error, time.Now().UTC()); err != nil {
		writeErr(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"set": true})
}

func (c *MockController) setOrganizations(ctx *gin.Context) {
	var req struct {
		Organizations []SentryOrganization `json:"organizations"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		writeBadPayload(ctx)
		return
	}
	instanceID, ok := c.requireInstanceID(ctx)
	if !ok {
		return
	}
	c.mock.SetOrganizations(instanceID, req.Organizations)
	ctx.JSON(http.StatusOK, gin.H{"count": len(req.Organizations)})
}

func (c *MockController) setProjects(ctx *gin.Context) {
	var req struct {
		Projects []SentryProject `json:"projects"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		writeBadPayload(ctx)
		return
	}
	instanceID, ok := c.requireInstanceID(ctx)
	if !ok {
		return
	}
	c.mock.SetProjects(instanceID, req.Projects)
	ctx.JSON(http.StatusOK, gin.H{"count": len(req.Projects)})
}

func (c *MockController) addIssues(ctx *gin.Context) {
	var req struct {
		Issues []SentryIssue `json:"issues"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		writeBadPayload(ctx)
		return
	}
	instanceID, ok := c.requireInstanceID(ctx)
	if !ok {
		return
	}
	for i := range req.Issues {
		c.mock.AddIssue(instanceID, &req.Issues[i])
	}
	ctx.JSON(http.StatusOK, gin.H{"added": len(req.Issues)})
}

func (c *MockController) setGetIssueError(ctx *gin.Context) {
	var req struct {
		StatusCode int    `json:"statusCode"`
		Message    string `json:"message"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		writeBadPayload(ctx)
		return
	}
	instanceID, ok := c.requireInstanceID(ctx)
	if !ok {
		return
	}
	if req.StatusCode == 0 {
		c.mock.SetGetIssueError(instanceID, nil)
	} else {
		c.mock.SetGetIssueError(instanceID, &APIError{StatusCode: req.StatusCode, Message: req.Message})
	}
	ctx.JSON(http.StatusOK, gin.H{"set": true})
}

func (c *MockController) reset(ctx *gin.Context) {
	c.mock.Reset()
	ctx.JSON(http.StatusOK, gin.H{"reset": true})
}
