package gitlab

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
)

const responseErrorKey = "error"

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

	api.GET("/workspaces/:workspaceID/task-mrs", c.httpListWorkspaceTaskMRs)
	api.GET("/tasks/:taskID/mrs", c.httpListTaskMRs)
	api.POST("/tasks/:taskID/mrs/sync", c.httpSyncTaskMR)

	api.GET("/user/mrs", c.httpSearchUserMRs)
	api.GET("/user/issues", c.httpSearchUserIssues)
}

// RegisterRoutes is the package-level entrypoint mirroring github.RegisterRoutes.
func RegisterRoutes(router *gin.Engine, svc *Service, log *logger.Logger) {
	NewController(svc, log).RegisterHTTPRoutes(router)
}

func (c *Controller) httpGetStatus(ctx *gin.Context) {
	status, err := c.service.GetStatus(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, status)
}

func (c *Controller) httpConfigureToken(ctx *gin.Context) {
	var req ConfigureTokenRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "invalid payload: token is required"})
		return
	}
	if err := c.service.ConfigureToken(ctx.Request.Context(), req.Token); err != nil {
		// errors.Is against the package-level sentinel — durable across
		// future rewording / wrapping of the ConfigureToken error message.
		if errors.Is(err, ErrInvalidToken) {
			ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: err.Error()})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"configured": true})
}

func (c *Controller) httpClearToken(ctx *gin.Context) {
	if err := c.service.ClearToken(ctx.Request.Context()); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"cleared": true})
}

func (c *Controller) httpConfigureHost(ctx *gin.Context) {
	var req ConfigureHostRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "invalid payload: host is required"})
		return
	}
	if err := c.service.ConfigureHost(ctx.Request.Context(), req.Host); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"configured": true, "host": c.service.Host()})
}

func (c *Controller) httpGetMRFeedback(ctx *gin.Context) {
	projectPath, iid, err := parseProjectAndIID(ctx)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: err.Error()})
		return
	}
	feedback, err := c.service.GetMRFeedback(ctx.Request.Context(), projectPath, iid)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
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
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "invalid payload"})
		return
	}
	note, err := c.service.CreateMRDiscussionNote(ctx.Request.Context(), req.Project, req.IID, req.DiscussionID, req.Body)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
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
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "invalid payload"})
		return
	}
	if err := c.service.ResolveMRDiscussion(ctx.Request.Context(), req.Project, req.IID, req.DiscussionID); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
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

// SyncTaskMRRequest is the JSON body for POST /tasks/:taskID/mrs/sync.
// project_path is "namespace/path"; iid is the MR's per-project sequential id;
// repository_id is the kandev repository UUID (empty for single-repo tasks).
type SyncTaskMRRequest struct {
	ProjectPath  string `json:"project_path" binding:"required"`
	IID          int    `json:"iid" binding:"required"`
	RepositoryID string `json:"repository_id"`
}

func (c *Controller) httpListWorkspaceTaskMRs(ctx *gin.Context) {
	wsID := ctx.Param("workspaceID")
	if wsID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "workspaceID required"})
		return
	}
	taskMRs, err := c.service.ListTaskMRsByWorkspace(ctx.Request.Context(), wsID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, TaskMRsResponse{TaskMRs: taskMRs})
}

func (c *Controller) httpListTaskMRs(ctx *gin.Context) {
	taskID := ctx.Param("taskID")
	if taskID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "taskID required"})
		return
	}
	mrs, err := c.service.ListTaskMRsByTask(ctx.Request.Context(), taskID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"task_mrs": mrs})
}

func (c *Controller) httpSyncTaskMR(ctx *gin.Context) {
	taskID := ctx.Param("taskID")
	if taskID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "taskID required"})
		return
	}
	var req SyncTaskMRRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "invalid payload: project_path and iid required"})
		return
	}
	row, err := c.service.SyncTaskMR(ctx.Request.Context(), taskID, req.RepositoryID, req.ProjectPath, req.IID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, row)
}

// httpSearchUserMRs surfaces the configured user's MR queue. filter is one
// of "assigned_to_me", "created_by_me", "review_requested" (the /gitlab
// page's tab values), or a raw GitLab API filter in `key=value` form;
// custom_query passes through verbatim for power users and disables tab
// translation entirely.
func (c *Controller) httpSearchUserMRs(ctx *gin.Context) {
	page, perPage := paginationFromQuery(ctx)
	filter := ctx.Query("filter")
	customQuery := ctx.Query("custom_query")
	if customQuery == "" {
		translated, err := c.translateMRFilter(ctx, filter)
		if err != nil {
			// translateMRFilter has already written the HTTP error.
			return
		}
		if translated != "" {
			filter = translated
		}
	}
	result, err := c.service.Client().SearchMRsPaged(
		ctx.Request.Context(), filter, customQuery, page, perPage,
	)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, result)
}

// httpSearchUserIssues surfaces the configured user's issue queue. filter
// supports "assigned_to_me" and "created_by_me" (the /gitlab page's issue
// tabs); GitLab has no review-requested concept for issues so that token
// is intentionally not recognized here.
func (c *Controller) httpSearchUserIssues(ctx *gin.Context) {
	page, perPage := paginationFromQuery(ctx)
	filter := ctx.Query("filter")
	customQuery := ctx.Query("custom_query")
	if customQuery == "" {
		if translated := translateUserSearchFilter(filter, ""); translated != "" {
			filter = translated
		}
	}
	result, err := c.service.Client().ListIssuesPaged(
		ctx.Request.Context(), filter, customQuery, page, perPage,
	)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, result)
}

// translateMRFilter resolves the authenticated user (only when the
// review_requested tab needs it) and runs the filter through
// translateUserSearchFilter. On a username-lookup failure — including a
// successful call that returns no username (NoopClient, an unexpected
// GitLab response) — it writes a 500 to ctx and returns an error so the
// caller can short-circuit. Silently falling back to an unfiltered listing
// would re-introduce the very bug this translation layer was added to
// prevent.
func (c *Controller) translateMRFilter(ctx *gin.Context, filter string) (string, error) {
	var username string
	if filter == "review_requested" {
		u, err := c.service.Client().GetAuthenticatedUser(ctx.Request.Context())
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
			return "", err
		}
		if u == "" {
			err := errors.New("cannot resolve authenticated GitLab user")
			ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
			return "", err
		}
		username = u
	}
	return translateUserSearchFilter(filter, username), nil
}

// paginationFromQuery reads ?page=&per_page= with the same clamps SearchMRsPaged
// applies internally — surfaced here so 400s on bad input are uniform.
func paginationFromQuery(ctx *gin.Context) (int, int) {
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(ctx.DefaultQuery("per_page", "50"))
	if perPage <= 0 {
		perPage = 50
	}
	return page, perPage
}
