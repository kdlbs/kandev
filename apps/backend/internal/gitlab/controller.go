package gitlab

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
)

// Controller handles HTTP endpoints for GitLab integration.
type Controller struct {
	service *Service
	logger  *logger.Logger
}

// NewController creates a new GitLab controller.
func NewController(svc *Service, log *logger.Logger) *Controller {
	return &Controller{service: svc, logger: log}
}

// RegisterHTTPRoutes registers the v1 HTTP surface.
func (c *Controller) RegisterHTTPRoutes(router *gin.Engine) {
	api := router.Group("/api/v1/gitlab")
	api.GET("/status", c.httpGetStatus)
	api.POST("/token", c.httpConfigureToken)
	api.DELETE("/token", c.httpClearToken)
	api.POST("/host", c.httpConfigureHost)

	api.GET("/mrs/feedback", c.httpGetMRFeedback)
	api.POST("/mrs/discussions/notes", c.httpCreateDiscussionNote)
	api.POST("/mrs/discussions/resolve", c.httpResolveDiscussion)
}

// RegisterRoutes is the package-level entrypoint mirroring github.RegisterRoutes.
func RegisterRoutes(router *gin.Engine, svc *Service, log *logger.Logger) {
	NewController(svc, log).RegisterHTTPRoutes(router)
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

func (c *Controller) httpConfigureHost(ctx *gin.Context) {
	var req ConfigureHostRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload: host is required"})
		return
	}
	if err := c.service.ConfigureHost(ctx.Request.Context(), req.Host); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"configured": true, "host": c.service.Host()})
}

func (c *Controller) httpGetMRFeedback(ctx *gin.Context) {
	projectPath, iid, err := parseProjectAndIID(ctx)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	feedback, err := c.service.GetMRFeedback(ctx.Request.Context(), projectPath, iid)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, feedback)
}

func (c *Controller) httpCreateDiscussionNote(ctx *gin.Context) {
	var req struct {
		Project      string `json:"project" binding:"required"`
		IID          int    `json:"iid" binding:"required"`
		DiscussionID string `json:"discussion_id" binding:"required"`
		Body         string `json:"body" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	note, err := c.service.CreateMRDiscussionNote(ctx.Request.Context(), req.Project, req.IID, req.DiscussionID, req.Body)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, note)
}

func (c *Controller) httpResolveDiscussion(ctx *gin.Context) {
	var req struct {
		Project      string `json:"project" binding:"required"`
		IID          int    `json:"iid" binding:"required"`
		DiscussionID string `json:"discussion_id" binding:"required"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if err := c.service.ResolveMRDiscussion(ctx.Request.Context(), req.Project, req.IID, req.DiscussionID); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"resolved": true})
}

// parseProjectAndIID reads ?project=<path>&iid=<n> query params and validates
// them. project is the namespace/path slug (URL-decoded by Gin); iid must be
// a positive integer.
func parseProjectAndIID(ctx *gin.Context) (string, int, error) {
	project := ctx.Query("project")
	if project == "" {
		return "", 0, errors.New("project query param required")
	}
	iidStr := ctx.Query("iid")
	if iidStr == "" {
		return "", 0, errors.New("iid query param required")
	}
	iid, err := strconv.Atoi(iidStr)
	if err != nil || iid <= 0 {
		return "", 0, errors.New("iid must be a positive integer")
	}
	return project, iid, nil
}
