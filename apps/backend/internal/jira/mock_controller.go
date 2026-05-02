package jira

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
)

// MockController exposes HTTP endpoints that drive the in-memory MockClient
// during E2E tests. Mounted by RegisterMockRoutes only when the service was
// built with a MockClient — production builds never see these routes.
type MockController struct {
	mock  *MockClient
	store *Store
	log   *logger.Logger
}

// NewMockController wires the controller to the shared mock + store so that
// auth-health writes (which the real poller does asynchronously) can be
// triggered synchronously from tests.
func NewMockController(mock *MockClient, store *Store, log *logger.Logger) *MockController {
	return &MockController{mock: mock, store: store, log: log}
}

// RegisterRoutes mounts the mock control routes under /api/v1/jira/mock.
func (c *MockController) RegisterRoutes(router *gin.Engine) {
	api := router.Group("/api/v1/jira/mock")
	api.PUT("/auth-result", c.setAuthResult)
	api.PUT("/auth-health", c.setAuthHealth)
	api.POST("/projects", c.setProjects)
	api.POST("/tickets", c.addTickets)
	api.POST("/transitions", c.addTransitions)
	api.POST("/search-hits", c.setSearchHits)
	api.PUT("/get-ticket-error", c.setGetTicketError)
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
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if err := c.store.UpdateAuthHealth(ctx.Request.Context(), req.OK, req.Error, time.Now().UTC()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"set": true})
}

func (c *MockController) setProjects(ctx *gin.Context) {
	var req struct {
		Projects []JiraProject `json:"projects"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	c.mock.SetProjects(req.Projects)
	ctx.JSON(http.StatusOK, gin.H{"count": len(req.Projects)})
}

func (c *MockController) addTickets(ctx *gin.Context) {
	var req struct {
		Tickets []JiraTicket `json:"tickets"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	for i := range req.Tickets {
		c.mock.AddTicket(&req.Tickets[i])
	}
	ctx.JSON(http.StatusOK, gin.H{"added": len(req.Tickets)})
}

func (c *MockController) addTransitions(ctx *gin.Context) {
	var req struct {
		TicketKey   string           `json:"ticketKey"`
		Transitions []JiraTransition `json:"transitions"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil || req.TicketKey == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "ticketKey required"})
		return
	}
	c.mock.AddTransitions(req.TicketKey, req.Transitions)
	ctx.JSON(http.StatusOK, gin.H{"added": len(req.Transitions)})
}

func (c *MockController) setSearchHits(ctx *gin.Context) {
	var req struct {
		Tickets []JiraTicket `json:"tickets"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	c.mock.SetSearchHits(req.Tickets)
	ctx.JSON(http.StatusOK, gin.H{"count": len(req.Tickets)})
}

func (c *MockController) setGetTicketError(ctx *gin.Context) {
	var req struct {
		StatusCode int    `json:"statusCode"`
		Message    string `json:"message"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if req.StatusCode == 0 {
		c.mock.SetGetTicketError(nil)
	} else {
		c.mock.SetGetTicketError(&APIError{StatusCode: req.StatusCode, Message: req.Message})
	}
	ctx.JSON(http.StatusOK, gin.H{"set": true})
}

func (c *MockController) reset(ctx *gin.Context) {
	c.mock.Reset()
	ctx.JSON(http.StatusOK, gin.H{"reset": true})
}
