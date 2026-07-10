package sentry

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

// registerIssueWatchRoutes attaches the Sentry issue-watch HTTP endpoints to
// the existing /api/v1/sentry route group.
func (c *Controller) registerIssueWatchRoutes(api *gin.RouterGroup) {
	api.GET("/watches/issue", c.httpListIssueWatches)
	api.POST("/watches/issue", c.httpCreateIssueWatch)
	api.GET("/watches/issue/:id", c.httpGetIssueWatch)
	api.PATCH("/watches/issue/:id", c.httpUpdateIssueWatch)
	api.DELETE("/watches/issue/:id", c.httpDeleteIssueWatch)
	api.POST("/watches/issue/:id/trigger", c.httpTriggerIssueWatch)
	api.GET("/watches/issue/:id/reset/preview", c.httpPreviewResetIssueWatch)
	api.POST("/watches/issue/:id/reset", c.httpResetIssueWatch)
}

// httpListIssueWatches returns watches scoped to one workspace when
// `workspace_id` is supplied, or every watch across all workspaces when it is
// absent (used by the install-wide settings page).
func (c *Controller) httpListIssueWatches(ctx *gin.Context) {
	workspaceID := ctx.Query("workspace_id")
	var (
		watches []*IssueWatch
		err     error
	)
	if workspaceID == "" {
		watches, err = c.service.ListAllIssueWatches(ctx.Request.Context())
	} else {
		watches, err = c.service.ListIssueWatches(ctx.Request.Context(), workspaceID)
	}
	if err != nil {
		writeErr(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"watches": watches})
}

func (c *Controller) httpCreateIssueWatch(ctx *gin.Context) {
	var req CreateIssueWatchRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		writeBadPayload(ctx)
		return
	}
	if !workspaceMatches(ctx, req.WorkspaceID) {
		writeErr(ctx, http.StatusBadRequest, "workspace_id query must match body workspaceId")
		return
	}
	w, err := c.service.CreateIssueWatch(ctx.Request.Context(), &req)
	if err != nil {
		c.writeIssueWatchError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, w)
}

func (c *Controller) httpGetIssueWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	w, err := c.service.GetIssueWatch(ctx.Request.Context(), id)
	if err != nil {
		c.writeIssueWatchError(ctx, err)
		return
	}
	if !workspaceMatches(ctx, w.WorkspaceID) {
		writeErr(ctx, http.StatusNotFound, ErrIssueWatchNotFound.Error())
		return
	}
	ctx.JSON(http.StatusOK, w)
}

func (c *Controller) httpUpdateIssueWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	if !c.assertWatchInWorkspace(ctx, id) {
		return
	}
	var req UpdateIssueWatchRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		writeBadPayload(ctx)
		return
	}
	w, err := c.service.UpdateIssueWatch(ctx.Request.Context(), id, &req)
	if err != nil {
		c.writeIssueWatchError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, w)
}

func (c *Controller) httpDeleteIssueWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	if !c.assertWatchInWorkspace(ctx, id) {
		return
	}
	if err := c.service.DeleteIssueWatch(ctx.Request.Context(), id); err != nil {
		writeErr(ctx, http.StatusInternalServerError, err.Error())
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

// httpTriggerIssueWatch runs a single immediate poll, ignoring the per-watch
// interval gating. Publishes events for each unseen issue and returns the
// count so the user gets feedback without waiting for the orchestrator to fan
// tasks out.
func (c *Controller) httpTriggerIssueWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	w, err := c.service.GetIssueWatch(ctx.Request.Context(), id)
	if err != nil {
		c.writeIssueWatchError(ctx, err)
		return
	}
	if !workspaceMatches(ctx, w.WorkspaceID) {
		writeErr(ctx, http.StatusNotFound, ErrIssueWatchNotFound.Error())
		return
	}
	instanceID, issues, err := c.service.CheckIssueWatch(ctx.Request.Context(), w)
	if err != nil {
		c.writeClientError(ctx, err)
		return
	}
	for _, issue := range issues {
		c.service.publishNewSentryIssueEvent(ctx.Request.Context(), w, instanceID, issue)
	}
	ctx.JSON(http.StatusOK, gin.H{"published": len(issues)})
}

// httpPreviewResetIssueWatch returns the count of tasks that a reset on the
// watch would cascade-delete. Used by the confirmation dialog so the user
// sees "delete N task(s)" before they commit.
func (c *Controller) httpPreviewResetIssueWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	if !c.assertWatchInWorkspace(ctx, id) {
		return
	}
	n, err := c.service.PreviewResetIssueWatch(ctx.Request.Context(), id)
	if err != nil {
		c.writeIssueWatchError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"taskCount": n})
}

// httpResetIssueWatch executes the destructive reset: cascade-deletes all
// tasks previously created by the watch (including archived), wipes its
// dedup table, and nulls last_polled_at so the next poll re-imports
// everything currently matching the filter.
func (c *Controller) httpResetIssueWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	if !c.assertWatchInWorkspace(ctx, id) {
		return
	}
	n, err := c.service.ResetIssueWatch(ctx.Request.Context(), id)
	if err != nil {
		c.writeIssueWatchError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"tasksDeleted": n})
}

// assertWatchInWorkspace guards mutation/trigger endpoints against IDOR: the
// caller must supply `?workspace_id=...` matching the watch's workspace.
// Writes the response and returns false on mismatch.
func (c *Controller) assertWatchInWorkspace(ctx *gin.Context, id string) bool {
	w, err := c.service.GetIssueWatch(ctx.Request.Context(), id)
	if err != nil {
		c.writeIssueWatchError(ctx, err)
		return false
	}
	if !workspaceMatches(ctx, w.WorkspaceID) {
		writeErr(ctx, http.StatusNotFound, ErrIssueWatchNotFound.Error())
		return false
	}
	return true
}

// workspaceMatches returns true when the request's `workspace_id` query
// matches the resource's stored workspace. Empty query is rejected so callers
// can't bypass the check by omitting the parameter.
func workspaceMatches(ctx *gin.Context, resourceWorkspace string) bool {
	q := ctx.Query("workspace_id")
	return q != "" && q == resourceWorkspace
}

func (c *Controller) writeIssueWatchError(ctx *gin.Context, err error) {
	if errors.Is(err, ErrIssueWatchNotFound) {
		writeErr(ctx, http.StatusNotFound, err.Error())
		return
	}
	if errors.Is(err, ErrInstanceNotFound) {
		writeCoded(ctx, http.StatusNotFound, err.Error(), errCodeSentryInstanceNotFound)
		return
	}
	if errors.Is(err, ErrInstanceRequired) {
		writeCoded(ctx, http.StatusBadRequest, err.Error(), errCodeSentryInstanceRequired)
		return
	}
	if errors.Is(err, ErrInvalidConfig) {
		writeErr(ctx, http.StatusBadRequest, err.Error())
		return
	}
	c.writeClientError(ctx, err)
}
