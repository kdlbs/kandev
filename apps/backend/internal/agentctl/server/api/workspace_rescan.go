package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// errKey is the JSON field name for error responses on this handler. Hoisted
// out to satisfy goconst's repeated-string rule across the api package.
const errKey = "error"

// RescanWorkspaceRequest is the body for POST /api/v1/workspace/rescan.
//
// work_dir is optional. When supplied, the manager updates its tracking
// scope to that path before re-discovering repo subdirectories — this is
// how a multi-branch transition signals "scan at the task root, not at the
// primary worktree". When empty, the manager rescans its current WorkDir
// in place.
type RescanWorkspaceRequest struct {
	WorkDir              string   `json:"work_dir"`
	WorkspaceSourceRoots []string `json:"workspace_source_roots,omitempty"`
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
	// Empty bodies are legal — rescan cfg.WorkDir in place. Malformed JSON
	// is NOT: silently treating it as an empty rescan returned 200 to a
	// caller who never got the work_dir promotion they asked for.
	if c.Request.ContentLength != 0 && c.Request.Body != http.NoBody {
		if err := c.ShouldBindJSON(&req); err != nil {
			s.logger.Warn("workspace rescan request rejected: malformed json", zap.Error(err))
			c.JSON(http.StatusBadRequest, gin.H{errKey: "invalid JSON body"})
			return
		}
	}
	if err := s.procMgr.RescanRepositoriesWithSourceRoots(c.Request.Context(), req.WorkDir, req.WorkspaceSourceRoots); err != nil {
		s.logger.Warn("workspace rescan failed", zap.Error(err), zap.String("work_dir", req.WorkDir))
		c.JSON(http.StatusUnprocessableEntity, gin.H{errKey: err.Error()})
		return
	}
	s.logger.Debug("workspace rescan completed", zap.String("work_dir", req.WorkDir))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// handleReconcileWorkspace prunes tracker state after rollback has removed
// newly-created checkouts. It deliberately has no work_dir: changing roots is
// a host-rebind concern, while rollback must retain the current root tracker
// and its existing workspace-stream subscriptions.
func (s *Server) handleReconcileWorkspace(c *gin.Context) {
	var req RescanWorkspaceRequest
	if c.Request.ContentLength != 0 && c.Request.Body != http.NoBody {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{errKey: "invalid JSON body"})
			return
		}
	}
	if err := s.procMgr.ReconcileRepositories(c.Request.Context()); err != nil {
		s.logger.Warn("workspace reconcile failed", zap.Error(err))
		c.JSON(http.StatusUnprocessableEntity, gin.H{errKey: err.Error()})
		return
	}
	if req.WorkspaceSourceRoots != nil {
		s.procMgr.SetWorkspaceSourceRoots(req.WorkspaceSourceRoots)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// handleRebindWorkspace performs the authenticated, destructive tracker
// replacement used only after lifecycle has stopped a native host child.
func (s *Server) handleRebindWorkspace(c *gin.Context) {
	var req RescanWorkspaceRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.WorkDir == "" {
		c.JSON(http.StatusBadRequest, gin.H{errKey: "work_dir is required"})
		return
	}
	if err := s.procMgr.RebindWorkspaceWithSourceRoots(c.Request.Context(), req.WorkDir, req.WorkspaceSourceRoots); err != nil {
		s.logger.Warn("workspace rebind failed", zap.Error(err), zap.String("work_dir", req.WorkDir))
		c.JSON(http.StatusUnprocessableEntity, gin.H{errKey: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
