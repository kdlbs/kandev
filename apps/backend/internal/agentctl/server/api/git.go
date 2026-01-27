package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"go.uber.org/zap"
)

// GitPullRequest for POST /api/v1/git/pull
type GitPullRequest struct {
	Rebase bool `json:"rebase"`
}

// GitPushRequest for POST /api/v1/git/push
type GitPushRequest struct {
	Force       bool `json:"force"`
	SetUpstream bool `json:"set_upstream"`
}

// GitRebaseRequest for POST /api/v1/git/rebase
type GitRebaseRequest struct {
	BaseBranch string `json:"base_branch"`
}

// GitMergeRequest for POST /api/v1/git/merge
type GitMergeRequest struct {
	BaseBranch string `json:"base_branch"`
}

// GitAbortRequest for POST /api/v1/git/abort
type GitAbortRequest struct {
	Operation string `json:"operation"` // "merge" or "rebase"
}

// GitCommitRequest for POST /api/v1/git/commit
type GitCommitRequest struct {
	Message  string `json:"message"`
	StageAll bool   `json:"stage_all"`
}

// GitStageRequest for POST /api/v1/git/stage
type GitStageRequest struct {
	Paths []string `json:"paths"` // Empty = stage all
}

// GitUnstageRequest for POST /api/v1/git/unstage
type GitUnstageRequest struct {
	Paths []string `json:"paths"` // Empty = unstage all
}

// GitShowCommitRequest for GET /api/v1/git/commit/:sha
type GitShowCommitRequest struct {
	CommitSHA string `uri:"sha" binding:"required"`
}

// GitCreatePRRequest for POST /api/v1/git/create-pr
type GitCreatePRRequest struct {
	Title      string `json:"title"`
	Body       string `json:"body"`
	BaseBranch string `json:"base_branch"`
}

// handleGitPull handles POST /api/v1/git/pull
func (s *Server) handleGitPull(c *gin.Context) {
	var req GitPullRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, process.GitOperationResult{
			Success:   false,
			Operation: "pull",
			Error:     "invalid request: " + err.Error(),
		})
		return
	}

	gitOp := s.procMgr.GitOperator()
	result, err := gitOp.Pull(c.Request.Context(), req.Rebase)
	if err != nil {
		s.handleGitError(c, "pull", err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// handleGitPush handles POST /api/v1/git/push
func (s *Server) handleGitPush(c *gin.Context) {
	var req GitPushRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, process.GitOperationResult{
			Success:   false,
			Operation: "push",
			Error:     "invalid request: " + err.Error(),
		})
		return
	}

	gitOp := s.procMgr.GitOperator()
	result, err := gitOp.Push(c.Request.Context(), req.Force, req.SetUpstream)
	if err != nil {
		s.handleGitError(c, "push", err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// handleGitRebase handles POST /api/v1/git/rebase
func (s *Server) handleGitRebase(c *gin.Context) {
	var req GitRebaseRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, process.GitOperationResult{
			Success:   false,
			Operation: "rebase",
			Error:     "invalid request: " + err.Error(),
		})
		return
	}

	if req.BaseBranch == "" {
		c.JSON(http.StatusBadRequest, process.GitOperationResult{
			Success:   false,
			Operation: "rebase",
			Error:     "base_branch is required",
		})
		return
	}

	gitOp := s.procMgr.GitOperator()
	result, err := gitOp.Rebase(c.Request.Context(), req.BaseBranch)
	if err != nil {
		s.handleGitError(c, "rebase", err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// handleGitMerge handles POST /api/v1/git/merge
func (s *Server) handleGitMerge(c *gin.Context) {
	var req GitMergeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, process.GitOperationResult{
			Success:   false,
			Operation: "merge",
			Error:     "invalid request: " + err.Error(),
		})
		return
	}

	if req.BaseBranch == "" {
		c.JSON(http.StatusBadRequest, process.GitOperationResult{
			Success:   false,
			Operation: "merge",
			Error:     "base_branch is required",
		})
		return
	}

	gitOp := s.procMgr.GitOperator()
	result, err := gitOp.Merge(c.Request.Context(), req.BaseBranch)
	if err != nil {
		s.handleGitError(c, "merge", err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// handleGitAbort handles POST /api/v1/git/abort
func (s *Server) handleGitAbort(c *gin.Context) {
	var req GitAbortRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, process.GitOperationResult{
			Success:   false,
			Operation: "abort",
			Error:     "invalid request: " + err.Error(),
		})
		return
	}

	if req.Operation != "merge" && req.Operation != "rebase" {
		c.JSON(http.StatusBadRequest, process.GitOperationResult{
			Success:   false,
			Operation: "abort",
			Error:     "operation must be 'merge' or 'rebase'",
		})
		return
	}

	gitOp := s.procMgr.GitOperator()
	result, err := gitOp.Abort(c.Request.Context(), req.Operation)
	if err != nil {
		s.handleGitError(c, "abort", err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// handleGitCommit handles POST /api/v1/git/commit
func (s *Server) handleGitCommit(c *gin.Context) {
	var req GitCommitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, process.GitOperationResult{
			Success:   false,
			Operation: "commit",
			Error:     "invalid request: " + err.Error(),
		})
		return
	}

	if req.Message == "" {
		c.JSON(http.StatusBadRequest, process.GitOperationResult{
			Success:   false,
			Operation: "commit",
			Error:     "message is required",
		})
		return
	}

	gitOp := s.procMgr.GitOperator()
	result, err := gitOp.Commit(c.Request.Context(), req.Message, req.StageAll)
	if err != nil {
		s.handleGitError(c, "commit", err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// handleGitStage handles POST /api/v1/git/stage
func (s *Server) handleGitStage(c *gin.Context) {
	var req GitStageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, process.GitOperationResult{
			Success:   false,
			Operation: "stage",
			Error:     "invalid request: " + err.Error(),
		})
		return
	}

	gitOp := s.procMgr.GitOperator()
	result, err := gitOp.Stage(c.Request.Context(), req.Paths)
	if err != nil {
		s.handleGitError(c, "stage", err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// handleGitUnstage handles POST /api/v1/git/unstage
func (s *Server) handleGitUnstage(c *gin.Context) {
	var req GitUnstageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, process.GitOperationResult{
			Success:   false,
			Operation: "unstage",
			Error:     "invalid request: " + err.Error(),
		})
		return
	}

	gitOp := s.procMgr.GitOperator()
	result, err := gitOp.Unstage(c.Request.Context(), req.Paths)
	if err != nil {
		s.handleGitError(c, "unstage", err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// handleGitCreatePR handles POST /api/v1/git/create-pr
func (s *Server) handleGitCreatePR(c *gin.Context) {
	var req GitCreatePRRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, process.PRCreateResult{
			Success: false,
			Error:   "invalid request: " + err.Error(),
		})
		return
	}

	if req.Title == "" {
		c.JSON(http.StatusBadRequest, process.PRCreateResult{
			Success: false,
			Error:   "title is required",
		})
		return
	}

	gitOp := s.procMgr.GitOperator()
	result, err := gitOp.CreatePR(c.Request.Context(), req.Title, req.Body, req.BaseBranch)
	if err != nil {
		if errors.Is(err, process.ErrOperationInProgress) {
			c.JSON(http.StatusConflict, process.PRCreateResult{
				Success: false,
				Error:   "another git operation is already in progress",
			})
			return
		}
		s.logger.Error("git create-pr failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, process.PRCreateResult{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// handleGitShowCommit handles GET /api/v1/git/commit/:sha
func (s *Server) handleGitShowCommit(c *gin.Context) {
	var req GitShowCommitRequest
	if err := c.ShouldBindUri(&req); err != nil {
		c.JSON(http.StatusBadRequest, process.CommitDiffResult{
			Success: false,
			Error:   "invalid request: " + err.Error(),
		})
		return
	}

	gitOp := s.procMgr.GitOperator()
	result, err := gitOp.ShowCommit(c.Request.Context(), req.CommitSHA)
	if err != nil {
		s.logger.Error("git show commit failed", zap.String("commit_sha", req.CommitSHA), zap.Error(err))
		c.JSON(http.StatusInternalServerError, process.CommitDiffResult{
			Success:   false,
			CommitSHA: req.CommitSHA,
			Error:     err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, result)
}

// handleGitError handles errors from git operations.
func (s *Server) handleGitError(c *gin.Context, operation string, err error) {
	if errors.Is(err, process.ErrOperationInProgress) {
		c.JSON(http.StatusConflict, process.GitOperationResult{
			Success:   false,
			Operation: operation,
			Error:     "another git operation is already in progress",
		})
		return
	}

	s.logger.Error("git operation failed", zap.String("operation", operation), zap.Error(err))
	c.JSON(http.StatusInternalServerError, process.GitOperationResult{
		Success:   false,
		Operation: operation,
		Error:     err.Error(),
	})
}
