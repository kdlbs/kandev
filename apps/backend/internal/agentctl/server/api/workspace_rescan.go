package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// RescanWorkspaceRequest is the body for POST /api/v1/workspace/rescan.
//
// work_dir is optional. When supplied, the manager updates its tracking
// scope to that path before re-discovering repo subdirectories — this is
// how a multi-branch transition signals "scan at the task root, not at the
// primary worktree". When empty, the manager rescans its current WorkDir
// in place.
type RescanWorkspaceRequest struct {
	WorkDir string `json:"work_dir"`
}

// handleRescanWorkspace re-runs repo discovery and reconciles trackers.
// Called by the kandev backend's branch materializer after creating a
// sibling worktree for an MCP-driven add_branch_to_task; without it, the
// new worktree's git/file events stay invisible until session restart.
//
// The handler is idempotent — a rescan with no on-disk changes is a no-op.
// Calls are cheap (one ReadDir + git probes per child), so the materializer
// can ping unconditionally rather than computing diffs client-side.
func (s *Server) handleRescanWorkspace(c *gin.Context) {
	var req RescanWorkspaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty bodies: when no work_dir is supplied the manager
		// rescans cfg.WorkDir in place.
		req = RescanWorkspaceRequest{}
	}
	s.procMgr.RescanRepositories(c.Request.Context(), req.WorkDir)
	s.logger.Debug("workspace rescan completed", zap.String("work_dir", req.WorkDir))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
