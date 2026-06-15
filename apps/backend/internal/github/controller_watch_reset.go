package github

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// httpPreviewResetReviewWatch returns the count of tasks a reset on the
// review watch would cascade-delete. Used by the confirmation dialog so the
// user sees "delete N task(s)" before they commit.
func (c *Controller) httpPreviewResetReviewWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	watch, err := c.service.GetReviewWatch(ctx.Request.Context(), id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if watch == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "review watch not found"})
		return
	}
	n, err := c.service.PreviewResetReviewWatch(ctx.Request.Context(), id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
	watch, err := c.service.GetReviewWatch(ctx.Request.Context(), id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if watch == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "review watch not found"})
		return
	}
	n, err := c.service.ResetReviewWatch(ctx.Request.Context(), id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"tasksDeleted": n})
}

// httpPreviewResetIssueWatch returns the count of tasks a reset on the
// issue watch would cascade-delete.
func (c *Controller) httpPreviewResetIssueWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	watch, err := c.service.GetIssueWatch(ctx.Request.Context(), id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if watch == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "issue watch not found"})
		return
	}
	n, err := c.service.PreviewResetIssueWatch(ctx.Request.Context(), id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"taskCount": n})
}

// httpResetIssueWatch executes the destructive reset for an issue watch.
func (c *Controller) httpResetIssueWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	watch, err := c.service.GetIssueWatch(ctx.Request.Context(), id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if watch == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "issue watch not found"})
		return
	}
	n, err := c.service.ResetIssueWatch(ctx.Request.Context(), id)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"tasksDeleted": n})
}
