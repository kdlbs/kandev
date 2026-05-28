package gitlab

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// httpRespondError maps known sentinel errors to their HTTP code and
// writes a JSON {"error": "..."} body. Returns true when a response was
// written so callers can early-return.
func httpRespondError(ctx *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, ErrWatchNotFound):
		ctx.JSON(http.StatusNotFound, gin.H{responseErrorKey: err.Error()})
	default:
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
	}
	return true
}

// RegisterWatchHTTPRoutes wires the watch + presets + write-action HTTP
// surface on top of the v1 routes. Called from RegisterHTTPRoutes after the
// base set is registered.
func (c *Controller) RegisterWatchHTTPRoutes(router *gin.Engine) {
	api := router.Group("/api/v1/gitlab")

	// MR / Review / Issue watches
	api.GET("/watches/mr", c.httpListMRWatches)
	api.DELETE("/watches/mr/:id", c.httpDeleteMRWatch)

	api.GET("/watches/review", c.httpListReviewWatches)
	api.POST("/watches/review", c.httpCreateReviewWatch)
	api.PUT("/watches/review/:id", c.httpUpdateReviewWatch)
	api.DELETE("/watches/review/:id", c.httpDeleteReviewWatch)
	api.POST("/watches/review/:id/trigger", c.httpTriggerReviewWatch)
	api.POST("/watches/review/trigger-all", c.httpTriggerAllReviewChecks)

	api.GET("/watches/issue", c.httpListIssueWatches)
	api.POST("/watches/issue", c.httpCreateIssueWatch)
	api.PUT("/watches/issue/:id", c.httpUpdateIssueWatch)
	api.DELETE("/watches/issue/:id", c.httpDeleteIssueWatch)
	api.POST("/watches/issue/:id/trigger", c.httpTriggerIssueWatch)
	api.POST("/watches/issue/trigger-all", c.httpTriggerAllIssueChecks)

	// Cleanup
	api.POST("/cleanup/review-tasks", c.httpCleanupReviewTasks)
	api.POST("/cleanup/issue-tasks", c.httpCleanupIssueTasks)

	// Projects + branches autocomplete
	api.GET("/projects", c.httpListUserProjects)
	api.GET("/projects/search", c.httpSearchProjects)
	api.GET("/projects/branches", c.httpListProjectBranches)
	api.GET("/projects/merge-methods", c.httpGetProjectMergeMethods)

	// MR write actions
	api.PUT("/mrs/merge", c.httpMergeMR)
	api.POST("/mrs/approve", c.httpApproveMR)
	api.POST("/mrs/unapprove", c.httpUnapproveMR)
	api.PUT("/mrs/labels", c.httpSetMRLabels)
	api.PUT("/mrs/assignees", c.httpSetMRAssignees)
	api.GET("/mrs/files", c.httpGetMRFiles)
	api.GET("/mrs/commits", c.httpGetMRCommits)

	// Action presets
	api.GET("/action-presets", c.httpGetActionPresets)
	api.PUT("/action-presets", c.httpUpdateActionPresets)
	api.POST("/action-presets/reset", c.httpResetActionPresets)

	// Stats
	api.GET("/stats", c.httpGetStats)
}

// --- Helpers ---

func bindJSON[T any](ctx *gin.Context, out *T) bool {
	if err := ctx.ShouldBindJSON(out); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: errors.New("invalid payload").Error()})
		return false
	}
	return true
}

// --- MR watches ---

func (c *Controller) httpListMRWatches(ctx *gin.Context) {
	sessionID := ctx.Query("session_id")
	taskID := ctx.Query("task_id")
	switch {
	case sessionID != "":
		watches, err := c.service.ListMRWatchesBySession(ctx.Request.Context(), sessionID)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
			return
		}
		ctx.JSON(http.StatusOK, gin.H{"watches": watches})
	case taskID != "":
		watches, err := c.service.ListMRWatchesByTask(ctx.Request.Context(), taskID)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
			return
		}
		ctx.JSON(http.StatusOK, gin.H{"watches": watches})
	default:
		watches, err := c.service.ListActiveMRWatches(ctx.Request.Context())
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
			return
		}
		ctx.JSON(http.StatusOK, gin.H{"watches": watches})
	}
}

func (c *Controller) httpDeleteMRWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	if err := c.service.DeleteMRWatch(ctx.Request.Context(), id); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

// --- Review watches ---

func (c *Controller) httpListReviewWatches(ctx *gin.Context) {
	wsID := ctx.Query("workspace_id")
	var (
		watches []*ReviewWatch
		err     error
	)
	if wsID == "" {
		watches, err = c.service.ListAllReviewWatches(ctx.Request.Context())
	} else {
		watches, err = c.service.ListReviewWatches(ctx.Request.Context(), wsID)
	}
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"watches": watches})
}

func (c *Controller) httpCreateReviewWatch(ctx *gin.Context) {
	var req CreateReviewWatchRequest
	if !bindJSON(ctx, &req) {
		return
	}
	rw, err := c.service.CreateReviewWatch(ctx.Request.Context(), &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, rw)
}

func (c *Controller) httpUpdateReviewWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	var req UpdateReviewWatchRequest
	if !bindJSON(ctx, &req) {
		return
	}
	if err := c.service.UpdateReviewWatch(ctx.Request.Context(), id, &req); httpRespondError(ctx, err) {
		return
	}
	rw, err := c.service.GetReviewWatch(ctx.Request.Context(), id)
	if err != nil || rw == nil {
		ctx.JSON(http.StatusOK, gin.H{"updated": true})
		return
	}
	ctx.JSON(http.StatusOK, rw)
}

func (c *Controller) httpDeleteReviewWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	if err := c.service.DeleteReviewWatch(ctx.Request.Context(), id); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (c *Controller) httpTriggerReviewWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	mrs, err := c.service.TriggerReviewWatch(ctx.Request.Context(), id)
	if httpRespondError(ctx, err) {
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"mrs": mrs, "count": len(mrs)})
}

func (c *Controller) httpTriggerAllReviewChecks(ctx *gin.Context) {
	n, err := c.service.TriggerReviewWatchAll(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"count": n})
}

// --- Issue watches ---

func (c *Controller) httpListIssueWatches(ctx *gin.Context) {
	wsID := ctx.Query("workspace_id")
	var (
		watches []*IssueWatch
		err     error
	)
	if wsID == "" {
		watches, err = c.service.ListAllIssueWatches(ctx.Request.Context())
	} else {
		watches, err = c.service.ListIssueWatches(ctx.Request.Context(), wsID)
	}
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"watches": watches})
}

func (c *Controller) httpCreateIssueWatch(ctx *gin.Context) {
	var req CreateIssueWatchRequest
	if !bindJSON(ctx, &req) {
		return
	}
	iw, err := c.service.CreateIssueWatch(ctx.Request.Context(), &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, iw)
}

func (c *Controller) httpUpdateIssueWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	var req UpdateIssueWatchRequest
	if !bindJSON(ctx, &req) {
		return
	}
	if err := c.service.UpdateIssueWatch(ctx.Request.Context(), id, &req); httpRespondError(ctx, err) {
		return
	}
	iw, err := c.service.GetIssueWatch(ctx.Request.Context(), id)
	if err != nil || iw == nil {
		ctx.JSON(http.StatusOK, gin.H{"updated": true})
		return
	}
	ctx.JSON(http.StatusOK, iw)
}

func (c *Controller) httpDeleteIssueWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	if err := c.service.DeleteIssueWatch(ctx.Request.Context(), id); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (c *Controller) httpTriggerIssueWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	issues, err := c.service.TriggerIssueWatch(ctx.Request.Context(), id)
	if httpRespondError(ctx, err) {
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"issues": issues, "count": len(issues)})
}

func (c *Controller) httpTriggerAllIssueChecks(ctx *gin.Context) {
	n, err := c.service.TriggerIssueWatchAll(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"count": n})
}

// --- Cleanup ---

func (c *Controller) httpCleanupReviewTasks(ctx *gin.Context) {
	n, err := c.service.CleanupAllReviewTasks(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": n})
}

func (c *Controller) httpCleanupIssueTasks(ctx *gin.Context) {
	n, err := c.service.CleanupAllIssueTasks(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": n})
}

// --- Projects ---

func (c *Controller) httpListUserProjects(ctx *gin.Context) {
	projects, err := c.service.ListUserProjects(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"projects": projects})
}

func (c *Controller) httpSearchProjects(ctx *gin.Context) {
	query := ctx.Query("query")
	projects, err := c.service.SearchProjects(ctx.Request.Context(), query, 50)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"projects": projects})
}

func (c *Controller) httpListProjectBranches(ctx *gin.Context) {
	project := ctx.Query("project")
	if project == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "project required"})
		return
	}
	branches, err := c.service.ListProjectBranches(ctx.Request.Context(), project)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"branches": branches})
}

func (c *Controller) httpGetProjectMergeMethods(ctx *gin.Context) {
	project := ctx.Query("project")
	if project == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "project required"})
		return
	}
	methods, err := c.service.GetProjectMergeMethods(ctx.Request.Context(), project)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, methods)
}

// --- Write actions ---

type mergeMRRequest struct {
	Project             string `json:"project" binding:"required"`
	IID                 int    `json:"iid" binding:"required"`
	Method              string `json:"method"`
	SquashCommitMessage string `json:"squash_commit_message"`
}

func (c *Controller) httpMergeMR(ctx *gin.Context) {
	var req mergeMRRequest
	if !bindJSON(ctx, &req) {
		return
	}
	mr, err := c.service.MergeMR(ctx.Request.Context(), req.Project, req.IID, req.Method, req.SquashCommitMessage)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, mr)
}

type approveMRRequest struct {
	Project string `json:"project" binding:"required"`
	IID     int    `json:"iid" binding:"required"`
}

func (c *Controller) httpApproveMR(ctx *gin.Context) {
	var req approveMRRequest
	if !bindJSON(ctx, &req) {
		return
	}
	if err := c.service.SubmitMRApproval(ctx.Request.Context(), req.Project, req.IID); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"approved": true})
}

func (c *Controller) httpUnapproveMR(ctx *gin.Context) {
	var req approveMRRequest
	if !bindJSON(ctx, &req) {
		return
	}
	if err := c.service.SubmitMRUnapproval(ctx.Request.Context(), req.Project, req.IID); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"unapproved": true})
}

type setMRLabelsRequest struct {
	Project string   `json:"project" binding:"required"`
	IID     int      `json:"iid" binding:"required"`
	Labels  []string `json:"labels"`
}

func (c *Controller) httpSetMRLabels(ctx *gin.Context) {
	var req setMRLabelsRequest
	if !bindJSON(ctx, &req) {
		return
	}
	if err := c.service.SetMRLabels(ctx.Request.Context(), req.Project, req.IID, req.Labels); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"updated": true})
}

type setMRAssigneesRequest struct {
	Project     string `json:"project" binding:"required"`
	IID         int    `json:"iid" binding:"required"`
	AssigneeIDs []int  `json:"assignee_ids"`
}

func (c *Controller) httpSetMRAssignees(ctx *gin.Context) {
	var req setMRAssigneesRequest
	if !bindJSON(ctx, &req) {
		return
	}
	if err := c.service.SetMRAssignees(ctx.Request.Context(), req.Project, req.IID, req.AssigneeIDs); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"updated": true})
}

func (c *Controller) httpGetMRFiles(ctx *gin.Context) {
	project, iid, err := parseProjectAndIID(ctx)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: err.Error()})
		return
	}
	files, err := c.service.GetMRFiles(ctx.Request.Context(), project, iid)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"files": files})
}

func (c *Controller) httpGetMRCommits(ctx *gin.Context) {
	project, iid, err := parseProjectAndIID(ctx)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: err.Error()})
		return
	}
	commits, err := c.service.GetMRCommits(ctx.Request.Context(), project, iid)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"commits": commits})
}

// --- Action presets ---

func (c *Controller) httpGetActionPresets(ctx *gin.Context) {
	wsID := ctx.Query("workspace_id")
	if wsID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "workspace_id required"})
		return
	}
	presets, err := c.service.GetActionPresetsOrDefault(ctx.Request.Context(), wsID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, presets)
}

func (c *Controller) httpUpdateActionPresets(ctx *gin.Context) {
	var req UpdateActionPresetsRequest
	if !bindJSON(ctx, &req) {
		return
	}
	if req.WorkspaceID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "workspace_id required"})
		return
	}
	presets, err := c.service.UpdateActionPresets(ctx.Request.Context(), &req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, presets)
}

func (c *Controller) httpResetActionPresets(ctx *gin.Context) {
	wsID := ctx.Query("workspace_id")
	if wsID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "workspace_id required"})
		return
	}
	presets, err := c.service.ResetActionPresets(ctx.Request.Context(), wsID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, presets)
}

// --- Stats ---

func (c *Controller) httpGetStats(ctx *gin.Context) {
	stats, err := c.service.GetStats(ctx.Request.Context())
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, stats)
}
