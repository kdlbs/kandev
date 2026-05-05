package linear

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
)

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
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"watches": watches})
}

func (c *Controller) httpCreateIssueWatch(ctx *gin.Context) {
	var req CreateIssueWatchRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	w, err := c.service.CreateIssueWatch(ctx.Request.Context(), &req)
	if err != nil {
		c.writeIssueWatchError(ctx, err)
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
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
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
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

// httpTriggerIssueWatch runs a single immediate poll, ignoring the per-watch
// interval gating. Returns the count of newly-discovered issues so the user
// gets feedback without waiting for the orchestrator to fan tasks out.
func (c *Controller) httpTriggerIssueWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	w, err := c.service.GetIssueWatch(ctx.Request.Context(), id)
	if err != nil {
		c.writeIssueWatchError(ctx, err)
		return
	}
	if !workspaceMatches(ctx, w.WorkspaceID) {
		ctx.JSON(http.StatusNotFound, gin.H{"error": ErrIssueWatchNotFound.Error()})
		return
	}
	issues, err := c.service.CheckIssueWatch(ctx.Request.Context(), w)
	if err != nil {
		c.writeClientError(ctx, err)
		return
	}
	for _, issue := range issues {
		c.service.publishNewLinearIssueEvent(ctx.Request.Context(), w, issue)
	}
	ctx.JSON(http.StatusOK, gin.H{"newIssues": len(issues)})
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
		ctx.JSON(http.StatusNotFound, gin.H{"error": ErrIssueWatchNotFound.Error()})
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
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, ErrInvalidConfig) {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.writeClientError(ctx, err)
}
