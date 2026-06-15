package github

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// httpPreviewResetReviewWatch returns the count of tasks a reset on the
// review watch would cascade-delete. Used by the confirmation dialog so the
// user sees "delete N task(s)" before they commit.
func (c *Controller) httpPreviewResetReviewWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	if !c.requireReviewWatchInWorkspace(ctx, id) {
		return
	}
	n, err := c.service.PreviewResetReviewWatch(ctx.Request.Context(), id)
	if err != nil {
		c.writeResetError(ctx, "preview reset review watch", err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"taskCount": n})
}

// httpResetReviewWatch executes the destructive reset: cascade-deletes
// every task previously created by the review watch (including archived),
// wipes its dedup table, and nulls last_polled_at so the next poll
// re-imports every currently-matching PR.
func (c *Controller) httpResetReviewWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	if !c.requireReviewWatchInWorkspace(ctx, id) {
		return
	}
	n, err := c.service.ResetReviewWatch(ctx.Request.Context(), id)
	if err != nil {
		c.writeResetError(ctx, "reset review watch", err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"tasksDeleted": n})
}

// httpPreviewResetIssueWatch returns the count of tasks a reset on the
// issue watch would cascade-delete.
func (c *Controller) httpPreviewResetIssueWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	if !c.requireIssueWatchInWorkspace(ctx, id) {
		return
	}
	n, err := c.service.PreviewResetIssueWatch(ctx.Request.Context(), id)
	if err != nil {
		c.writeResetError(ctx, "preview reset issue watch", err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"taskCount": n})
}

// httpResetIssueWatch executes the destructive reset for an issue watch.
func (c *Controller) httpResetIssueWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	if !c.requireIssueWatchInWorkspace(ctx, id) {
		return
	}
	n, err := c.service.ResetIssueWatch(ctx.Request.Context(), id)
	if err != nil {
		c.writeResetError(ctx, "reset issue watch", err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"tasksDeleted": n})
}

// requireReviewWatchInWorkspace guards the reset endpoints against IDOR:
// the caller must supply `?workspace_id=...` matching the watch's stored
// workspace. Mismatch and not-found both return 404 so a probing client
// can't tell whether a watch ID exists in another workspace.
func (c *Controller) requireReviewWatchInWorkspace(ctx *gin.Context, id string) bool {
	watch, err := c.service.GetReviewWatch(ctx.Request.Context(), id)
	if err != nil {
		c.writeResetError(ctx, "load review watch", err)
		return false
	}
	if watch == nil || ctx.Query("workspace_id") == "" || ctx.Query("workspace_id") != watch.WorkspaceID {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "review watch not found"})
		return false
	}
	return true
}

// requireIssueWatchInWorkspace mirrors requireReviewWatchInWorkspace for
// the issue watch reset endpoints.
func (c *Controller) requireIssueWatchInWorkspace(ctx *gin.Context, id string) bool {
	watch, err := c.service.GetIssueWatch(ctx.Request.Context(), id)
	if err != nil {
		c.writeResetError(ctx, "load issue watch", err)
		return false
	}
	if watch == nil || ctx.Query("workspace_id") == "" || ctx.Query("workspace_id") != watch.WorkspaceID {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "issue watch not found"})
		return false
	}
	return true
}

// writeResetError logs the raw error and returns a generic message to the
// client so internal store/SQL details don't leak through the HTTP response.
func (c *Controller) writeResetError(ctx *gin.Context, op string, err error) {
	c.logger.Error(op, zap.Error(err))
	ctx.JSON(http.StatusInternalServerError, gin.H{"error": op + " failed"})
}
