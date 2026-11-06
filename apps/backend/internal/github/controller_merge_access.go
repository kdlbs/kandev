package github

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func (c *Controller) httpGetRepoMergeAccess(ctx *gin.Context) {
	ctx.Header("Cache-Control", "no-store")
	access, err := c.service.GetRepoMergeAccessForWorkspace(
		ctx.Request.Context(), ctx.Query("workspace_id"), currentGitHubUserID(ctx),
		ctx.Param("owner"), ctx.Param("repo"),
	)
	if err == nil {
		ctx.JSON(http.StatusOK, access)
		return
	}
	if writeGitHubOperationalAuthError(ctx, err) {
		return
	}
	status := http.StatusServiceUnavailable
	var apiErr *GitHubAPIError
	if errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		status = http.StatusNotFound
	} else if errors.As(err, &apiErr) && apiErr.StatusCode >= 400 && apiErr.StatusCode <= 599 {
		status = apiErr.StatusCode
	}
	ctx.JSON(status, gin.H{
		"code": "github_merge_access_unavailable", "error": "Unable to check merge permissions.",
	})
}
