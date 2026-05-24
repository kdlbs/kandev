package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/worktree/copyfiles"
)

// CopyFilesRequest is the JSON body for POST /workspace/copy-files.
// Repo is the optional per-repo subpath inside the workspace root
// (matches the convention used by the other workspace/file/* endpoints
// for multi-repo task workspaces). Entries are the planned files to
// write — the caller (kandev backend) is responsible for reading the
// bytes from the source repo and applying the per-source path-traversal
// guard via copyfiles.Plan. The handler re-validates against the target
// root so a compromised caller cannot escape the workspace.
type CopyFilesRequest struct {
	Repo    string            `json:"repo,omitempty"`
	Entries []copyfiles.Entry `json:"entries"`
}

// CopyFilesResponse mirrors the host-side copyfiles return shape.
type CopyFilesResponse struct {
	Copied   []string `json:"copied,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Error    string   `json:"error,omitempty"`
}

// handleWorkspaceCopyFiles writes a batch of pre-planned files into the
// workspace (or a per-repo subdir). Idempotent: existing destinations are
// skipped. Used by remote executors (Docker, Sprites) whose containers
// clone the workspace independently of the host and therefore can't
// receive copy_files seeding via the worktree path.
func (s *Server) handleWorkspaceCopyFiles(c *gin.Context) {
	var req CopyFilesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, CopyFilesResponse{Error: "invalid request: " + err.Error()})
		return
	}
	if len(req.Entries) == 0 {
		c.JSON(http.StatusOK, CopyFilesResponse{})
		return
	}

	targetDir, err := s.procMgr.ResolveRepoSubdir(req.Repo)
	if err != nil {
		c.JSON(http.StatusBadRequest, CopyFilesResponse{Error: err.Error()})
		return
	}

	// Pass the agentctl WorkDir as the containment root — this is the
	// trusted workspace bound that WriteEntries verifies targetDir lies
	// under before any filesystem access. Both come from the same source
	// (procMgr) but the explicit two-argument shape is what CodeQL needs
	// to recognise the path-injection sanitizer.
	copied, warnings, err := copyfiles.WriteEntries(
		c.Request.Context(),
		s.procMgr.WorkDir(),
		targetDir,
		req.Entries,
		s.logger.Zap(),
	)
	if err != nil {
		s.logger.Warn("workspace/copy-files: write failed",
			zap.String("target", targetDir),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, CopyFilesResponse{
			Copied:   copied,
			Warnings: warnings,
			Error:    err.Error(),
		})
		return
	}
	s.logger.Info("workspace/copy-files: completed",
		zap.String("target", targetDir),
		zap.Int("copied", len(copied)),
		zap.Int("warnings", len(warnings)))
	c.JSON(http.StatusOK, CopyFilesResponse{Copied: copied, Warnings: warnings})
}
