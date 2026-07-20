package gitlab

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func (c *Controller) httpPreviewResetReviewWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	if !c.requireReviewWatchInWorkspace(ctx, id) {
		return
	}
	n, err := c.service.PreviewResetReviewWatch(ctx.Request.Context(), id)
	if err != nil {
		c.writeWatchResetError(ctx, "preview reset review watch", err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"taskCount": n})
}

func (c *Controller) httpResetReviewWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	if !c.requireReviewWatchInWorkspace(ctx, id) {
		return
	}
	n, err := c.service.ResetReviewWatch(ctx.Request.Context(), id)
	if err != nil {
		c.writeWatchResetError(ctx, "reset review watch", err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"tasksDeleted": n})
}

func (c *Controller) httpPreviewResetIssueWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	if !c.requireIssueWatchInWorkspace(ctx, id) {
		return
	}
	n, err := c.service.PreviewResetIssueWatch(ctx.Request.Context(), id)
	if err != nil {
		c.writeWatchResetError(ctx, "preview reset issue watch", err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"taskCount": n})
}

func (c *Controller) httpResetIssueWatch(ctx *gin.Context) {
	id := ctx.Param("id")
	if !c.requireIssueWatchInWorkspace(ctx, id) {
		return
	}
	n, err := c.service.ResetIssueWatch(ctx.Request.Context(), id)
	if err != nil {
		c.writeWatchResetError(ctx, "reset issue watch", err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"tasksDeleted": n})
}

func (c *Controller) requireReviewWatchInWorkspace(ctx *gin.Context, id string) bool {
	watch, err := c.service.GetReviewWatch(ctx.Request.Context(), id)
	if err != nil {
		c.writeWatchResetError(ctx, "load review watch", err)
		return false
	}
	if watch == nil || watch.WorkspaceID != c.workspaceID(ctx) {
		ctx.JSON(http.StatusNotFound, gin.H{responseErrorKey: "review watch not found"})
		return false
	}
	return true
}

func (c *Controller) requireIssueWatchInWorkspace(ctx *gin.Context, id string) bool {
	watch, err := c.service.GetIssueWatch(ctx.Request.Context(), id)
	if err != nil {
		c.writeWatchResetError(ctx, "load issue watch", err)
		return false
	}
	if watch == nil || watch.WorkspaceID != c.workspaceID(ctx) {
		ctx.JSON(http.StatusNotFound, gin.H{responseErrorKey: "issue watch not found"})
		return false
	}
	return true
}

func (c *Controller) requireReviewWatchDeleteInWorkspace(ctx *gin.Context, id string) bool {
	watch, err := c.service.GetReviewWatchIncludingDeleting(ctx.Request.Context(), id)
	if err != nil {
		c.writeWatchResetError(ctx, "load review watch", err)
		return false
	}
	if watch == nil || watch.WorkspaceID != c.workspaceID(ctx) {
		ctx.JSON(http.StatusNotFound, gin.H{responseErrorKey: "review watch not found"})
		return false
	}
	return true
}

func (c *Controller) requireIssueWatchDeleteInWorkspace(ctx *gin.Context, id string) bool {
	watch, err := c.service.GetIssueWatchIncludingDeleting(ctx.Request.Context(), id)
	if err != nil {
		c.writeWatchResetError(ctx, "load issue watch", err)
		return false
	}
	if watch == nil || watch.WorkspaceID != c.workspaceID(ctx) {
		ctx.JSON(http.StatusNotFound, gin.H{responseErrorKey: "issue watch not found"})
		return false
	}
	return true
}

func (c *Controller) writeWatchResetError(ctx *gin.Context, op string, err error) {
	c.logger.Error(op, zap.Error(err))
	ctx.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: op + " failed"})
}
