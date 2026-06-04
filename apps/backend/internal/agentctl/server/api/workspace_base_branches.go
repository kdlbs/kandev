package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"go.uber.org/zap"
)

// SetBaseBranchesRequest is the body for POST /api/v1/workspace/base-branches.
//
// BaseBranches keys repositoryName (empty key = root / single-repo) to the
// branch ref each WorkspaceTracker should compare against for BaseCommit /
// Ahead / Behind. A nil or empty map clears all overrides — every tracker
// falls back to the hardcoded origin/main → master priority list.
type SetBaseBranchesRequest struct {
	BaseBranches map[string]string `json:"base_branches"`
}

// handleSetBaseBranches replaces the manager's per-repo base-branch map. The
// kandev backend calls this after persisting a new value via
// service.UpdateRepositoryBaseBranch so the live tracker stamps the new ref
// onto its next emit, not just the next session start.
//
// Idempotent — calling with the existing map is safe: SetBaseBranch is a
// trivial field swap and RefreshGitStatus is the same call the UI already
// fires after stage/unstage.
func (s *Server) handleSetBaseBranches(c *gin.Context) {
	var req SetBaseBranchesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		s.logger.Warn("set base-branches request rejected: malformed json", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{errKey: "invalid JSON body"})
		return
	}
	// Sanitize incoming refs at the HTTP boundary. WorkspaceTracker
	// SetBaseBranch already rejects unsafe values, but transforming each
	// value through SanitizeGitRef (rather than a bool guard) here makes
	// the safety contract explicit to static analysis — CodeQL recognises
	// the regex-backed transformer as a sanitiser barrier between the
	// untrusted request body and the downstream `git` subprocess args,
	// where a bool check before a verbatim copy is not recognised.
	safe := make(map[string]string, len(req.BaseBranches))
	for k, v := range req.BaseBranches {
		if sanitised := process.SanitizeGitRef(v); sanitised != "" {
			safe[k] = sanitised
		}
	}
	s.procMgr.UpdateBaseBranches(c.Request.Context(), safe)
	s.logger.Debug("base branches updated", zap.Int("entries", len(req.BaseBranches)))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
