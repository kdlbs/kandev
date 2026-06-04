package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
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
	// Sanitize incoming refs at the HTTP boundary. The inline
	// securityutil.IsValidBranchName allowlist check mirrors agentctl's
	// existing Rebase / Merge handlers — sharing the canonical helper
	// (rather than wrapping it in a passthrough) is what CodeQL's taint
	// tracker recognises as a sanitiser barrier between this request body
	// and the downstream `git` subprocess args.
	safe := make(map[string]string, len(req.BaseBranches))
	for k, v := range req.BaseBranches {
		check, hasOriginPrefix := strings.CutPrefix(v, "origin/")
		if !hasOriginPrefix {
			check = v
		}
		if !safeBranchRefPattern.MatchString(check) || strings.Contains(check, "..") || strings.HasSuffix(check, ".lock") {
			continue
		}
		safe[k] = v
	}
	s.procMgr.UpdateBaseBranches(c.Request.Context(), safe)
	s.logger.Debug("base branches updated", zap.Int("entries", len(req.BaseBranches)))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
