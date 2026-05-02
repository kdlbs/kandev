package api

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"go.uber.org/zap"
)

// sortCommitsByCommittedAtDesc sorts commits newest-first using committed_at.
// Commits with unparseable timestamps preserve their original relative order
// (sort.SliceStable). Used when merging logs from multiple repos so the
// combined timeline reads chronologically.
func sortCommitsByCommittedAtDesc(commits []*process.GitCommitInfo) {
	sort.SliceStable(commits, func(i, j int) bool {
		ti, errI := time.Parse(time.RFC3339, commits[i].CommittedAt)
		tj, errJ := time.Parse(time.RFC3339, commits[j].CommittedAt)
		if errI != nil || errJ != nil {
			return false
		}
		return ti.After(tj)
	})
}

// Repo selects a repository sub-directory of the workspace for multi-repo
// task roots. Optional. Empty = workspace root (single-repo behavior).
// All git request structs below embed this field via the standard `repo` JSON
// key (or `repo` form key for GET endpoints).

// GitPullRequest for POST /api/v1/git/pull
type GitPullRequest struct {
	Rebase bool   `json:"rebase"`
	Repo   string `json:"repo,omitempty"`
}

// GitPushRequest for POST /api/v1/git/push
type GitPushRequest struct {
	Force       bool   `json:"force"`
	SetUpstream bool   `json:"set_upstream"`
	Repo        string `json:"repo,omitempty"`
}

// GitRebaseRequest for POST /api/v1/git/rebase
type GitRebaseRequest struct {
	BaseBranch string `json:"base_branch"`
	Repo       string `json:"repo,omitempty"`
}

// GitMergeRequest for POST /api/v1/git/merge
type GitMergeRequest struct {
	BaseBranch string `json:"base_branch"`
	Repo       string `json:"repo,omitempty"`
}

// GitAbortRequest for POST /api/v1/git/abort
type GitAbortRequest struct {
	Operation string `json:"operation"` // "merge" or "rebase"
	Repo      string `json:"repo,omitempty"`
}

// GitCommitRequest for POST /api/v1/git/commit
type GitCommitRequest struct {
	Message  string `json:"message"`
	StageAll bool   `json:"stage_all"`
	Amend    bool   `json:"amend"`
	Repo     string `json:"repo,omitempty"`
}

// GitRenameBranchRequest for POST /api/v1/git/rename-branch
type GitRenameBranchRequest struct {
	NewName string `json:"new_name"`
	Repo    string `json:"repo,omitempty"`
}

// GitStageRequest for POST /api/v1/git/stage
type GitStageRequest struct {
	Paths []string `json:"paths"` // Empty = stage all
	Repo  string   `json:"repo,omitempty"`
}

// GitUnstageRequest for POST /api/v1/git/unstage
type GitUnstageRequest struct {
	Paths []string `json:"paths"` // Empty = unstage all
	Repo  string   `json:"repo,omitempty"`
}

// GitDiscardRequest for POST /api/v1/git/discard
type GitDiscardRequest struct {
	Paths []string `json:"paths"` // Required - files to discard
	Repo  string   `json:"repo,omitempty"`
}

// GitShowCommitRequest for GET /api/v1/git/commit/:sha
type GitShowCommitRequest struct {
	CommitSHA string `uri:"sha" binding:"required"`
}

// GitRevertCommitRequest for POST /api/v1/git/revert-commit
type GitRevertCommitRequest struct {
	CommitSHA string `json:"commit_sha"`
	Repo      string `json:"repo,omitempty"`
}

// GitCreatePRRequest for POST /api/v1/git/create-pr
type GitCreatePRRequest struct {
	Title      string `json:"title"`
	Body       string `json:"body"`
	BaseBranch string `json:"base_branch"`
	Draft      bool   `json:"draft"`
	Repo       string `json:"repo,omitempty"`
}

// GitResetRequest for POST /api/v1/git/reset
type GitResetRequest struct {
	CommitSHA string `json:"commit_sha"`
	Mode      string `json:"mode"` // "soft", "mixed", or "hard"
	Repo      string `json:"repo,omitempty"`
}

// GitLogRequest for GET /api/v1/git/log
type GitLogRequest struct {
	Since        string `form:"since"`         // Base commit SHA (exclusive)
	TargetBranch string `form:"target_branch"` // Target branch for merge-base calculation (e.g., "origin/main")
	Limit        int    `form:"limit"`         // Max commits to return
	Repo         string `form:"repo"`
}

// GitCumulativeDiffRequest for GET /api/v1/git/cumulative-diff
type GitCumulativeDiffRequest struct {
	Base string `form:"base" binding:"required"` // Base commit SHA
	Repo string `form:"repo"`
}

// gitOpForRepo resolves the optional repo subpath to a per-repo GitOperator.
// On invalid subpath it writes a 400 response and returns nil so callers can
// `return` immediately. Empty subpath returns the workspace-root operator.
func (s *Server) gitOpForRepo(c *gin.Context, operation, subpath string) *process.GitOperator {
	op, err := s.procMgr.GitOperatorFor(subpath)
	if err != nil {
		c.JSON(http.StatusBadRequest, process.GitOperationResult{
			Success:   false,
			Operation: operation,
			Error:     err.Error(),
		})
		return nil
	}
	return op
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

	gitOp := s.gitOpForRepo(c, "pull", req.Repo)
	if gitOp == nil {
		return
	}
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

	gitOp := s.gitOpForRepo(c, "push", req.Repo)
	if gitOp == nil {
		return
	}
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

	gitOp := s.gitOpForRepo(c, "rebase", req.Repo)
	if gitOp == nil {
		return
	}
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

	gitOp := s.gitOpForRepo(c, "merge", req.Repo)
	if gitOp == nil {
		return
	}
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

	gitOp := s.gitOpForRepo(c, "abort", req.Repo)
	if gitOp == nil {
		return
	}
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

	gitOp := s.gitOpForRepo(c, "commit", req.Repo)
	if gitOp == nil {
		return
	}
	result, err := gitOp.Commit(c.Request.Context(), req.Message, req.StageAll, req.Amend)
	if err != nil {
		s.handleGitError(c, "commit", err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// handleGitRenameBranch handles POST /api/v1/git/rename-branch
func (s *Server) handleGitRenameBranch(c *gin.Context) {
	var req GitRenameBranchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, process.GitOperationResult{
			Success:   false,
			Operation: "rename_branch",
			Error:     "invalid request: " + err.Error(),
		})
		return
	}

	if req.NewName == "" {
		c.JSON(http.StatusBadRequest, process.GitOperationResult{
			Success:   false,
			Operation: "rename_branch",
			Error:     "new_name is required",
		})
		return
	}

	gitOp := s.gitOpForRepo(c, "rename_branch", req.Repo)
	if gitOp == nil {
		return
	}
	result, err := gitOp.RenameBranch(c.Request.Context(), req.NewName)
	if err != nil {
		s.handleGitError(c, "rename_branch", err)
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

	gitOp := s.gitOpForRepo(c, "stage", req.Repo)
	if gitOp == nil {
		return
	}
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

	gitOp := s.gitOpForRepo(c, "unstage", req.Repo)
	if gitOp == nil {
		return
	}
	result, err := gitOp.Unstage(c.Request.Context(), req.Paths)
	if err != nil {
		s.handleGitError(c, "unstage", err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// handleGitDiscard handles POST /api/v1/git/discard
func (s *Server) handleGitDiscard(c *gin.Context) {
	var req GitDiscardRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, process.GitOperationResult{
			Success:   false,
			Operation: "discard",
			Error:     "invalid request: " + err.Error(),
		})
		return
	}

	if len(req.Paths) == 0 {
		c.JSON(http.StatusBadRequest, process.GitOperationResult{
			Success:   false,
			Operation: "discard",
			Error:     "paths are required",
		})
		return
	}

	gitOp := s.gitOpForRepo(c, "discard", req.Repo)
	if gitOp == nil {
		return
	}
	result, err := gitOp.Discard(c.Request.Context(), req.Paths)
	if err != nil {
		s.handleGitError(c, "discard", err)
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

	gitOp, gitOpErr := s.procMgr.GitOperatorFor(req.Repo)
	if gitOpErr != nil {
		c.JSON(http.StatusBadRequest, process.PRCreateResult{
			Success: false,
			Error:   gitOpErr.Error(),
		})
		return
	}
	result, err := gitOp.CreatePR(c.Request.Context(), req.Title, req.Body, req.BaseBranch, req.Draft)
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

// handleGitRevertCommit handles POST /api/v1/git/revert-commit
func (s *Server) handleGitRevertCommit(c *gin.Context) {
	var req GitRevertCommitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, process.GitOperationResult{
			Success:   false,
			Operation: "revert_commit",
			Error:     "invalid request: " + err.Error(),
		})
		return
	}

	if req.CommitSHA == "" {
		c.JSON(http.StatusBadRequest, process.GitOperationResult{
			Success:   false,
			Operation: "revert_commit",
			Error:     "commit_sha is required",
		})
		return
	}

	gitOp := s.gitOpForRepo(c, "revert_commit", req.Repo)
	if gitOp == nil {
		return
	}
	result, err := gitOp.RevertCommit(c.Request.Context(), req.CommitSHA)
	if err != nil {
		s.handleGitError(c, "revert_commit", err)
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

	// ShowCommit accepts an optional ?repo= query param.
	gitOp, gitOpErr := s.procMgr.GitOperatorFor(c.Query("repo"))
	if gitOpErr != nil {
		c.JSON(http.StatusBadRequest, process.CommitDiffResult{
			Success:   false,
			CommitSHA: req.CommitSHA,
			Error:     gitOpErr.Error(),
		})
		return
	}
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

// handleGitReset handles POST /api/v1/git/reset
func (s *Server) handleGitReset(c *gin.Context) {
	var req GitResetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, process.GitOperationResult{
			Success:   false,
			Operation: "reset",
			Error:     "invalid request: " + err.Error(),
		})
		return
	}

	if req.CommitSHA == "" {
		c.JSON(http.StatusBadRequest, process.GitOperationResult{
			Success:   false,
			Operation: "reset",
			Error:     "commit_sha is required",
		})
		return
	}

	if req.Mode == "" {
		req.Mode = "mixed"
	}
	validModes := map[string]bool{"soft": true, "mixed": true, "hard": true}
	if !validModes[req.Mode] {
		c.JSON(http.StatusBadRequest, process.GitOperationResult{
			Success:   false,
			Operation: "reset",
			Error:     fmt.Sprintf("invalid reset mode: %s (must be soft, mixed, or hard)", req.Mode),
		})
		return
	}

	gitOp := s.gitOpForRepo(c, "reset", req.Repo)
	if gitOp == nil {
		return
	}
	result, err := gitOp.Reset(c.Request.Context(), req.CommitSHA, req.Mode)
	if err != nil {
		s.handleGitError(c, "reset", err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// handleGitLog handles GET /api/v1/git/log.
//
// Multi-repo: when the caller doesn't pin a repo (?repo= empty) and the
// workspace contains per-repo subdirs, fan out one log per repo and merge
// the results, stamping RepositoryName on each commit so the frontend can
// group them. Without this, the call would land on the workspace root which
// for multi-repo task workspaces isn't a git repo and returns nothing.
func (s *Server) handleGitLog(c *gin.Context) {
	var req GitLogRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, process.GitLogResult{
			Success: false,
			Error:   "invalid request: " + err.Error(),
		})
		return
	}

	// Bound limit to prevent expensive history scans
	const (
		defaultLogLimit = 100
		maxLogLimit     = 500
	)
	limit := req.Limit
	if limit <= 0 {
		limit = defaultLogLimit
	} else if limit > maxLogLimit {
		c.JSON(http.StatusBadRequest, process.GitLogResult{
			Success: false,
			Error:   fmt.Sprintf("limit must be between 1 and %d", maxLogLimit),
		})
		return
	}

	if req.Repo == "" {
		if subs := s.procMgr.RepoSubpaths(); len(subs) > 0 {
			s.handleGitLogMultiRepo(c, req, subs, limit)
			return
		}
	}

	result, err := s.runGitLogForRepo(c, req, limit, req.Repo)
	if err != nil {
		s.handleGitError(c, "log", err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// runGitLogForRepo runs git log against a single repo subpath. Returns a
// result-with-error or a non-nil error for transport failures.
func (s *Server) runGitLogForRepo(
	c *gin.Context,
	req GitLogRequest,
	limit int,
	repo string,
) (*process.GitLogResult, error) {
	gitOp, gitOpErr := s.procMgr.GitOperatorFor(repo)
	if gitOpErr != nil {
		return nil, gitOpErr
	}

	baseCommit := req.Since
	if req.TargetBranch != "" {
		mergeBase, err := gitOp.GetMergeBase(c.Request.Context(), "HEAD", req.TargetBranch)
		if err != nil {
			s.logger.Warn("failed to compute merge-base, falling back to since",
				zap.String("target_branch", req.TargetBranch),
				zap.String("repo", repo),
				zap.Error(err))
		} else if mergeBase != "" {
			baseCommit = mergeBase
		}
	}

	return gitOp.GetLog(c.Request.Context(), baseCommit, limit)
}

// perRepoLogOutcome captures the outcome of running git log against a single
// repo subpath. Exactly one of result/err is set; result.Success may still be
// false if the per-repo command itself reported a failure.
type perRepoLogOutcome struct {
	subpath string
	result  *process.GitLogResult
	err     error
}

// mergeGitLogResults combines per-repo log outcomes into a single response
// suitable for the multi-repo log endpoint. It is extracted from
// handleGitLogMultiRepo so the merge/error-aggregation logic is unit-testable
// without spinning up a real Manager.
//
// Behavior:
//   - Successful repos contribute their commits (each tagged with RepositoryName).
//   - Failed repos (transport error or result.Success=false) contribute an
//     entry to PerRepoErrors so the frontend can render an "incomplete" warning.
//   - Success=true iff at least one repo succeeded. If every repo failed,
//     Success=false with a summary in Error.
//   - Commits are sorted newest-first across repos and truncated to limit.
func mergeGitLogResults(outcomes []perRepoLogOutcome, limit int) process.GitLogResult {
	merged := make([]*process.GitCommitInfo, 0)
	perRepoErrors := make([]process.GitLogRepoError, 0)
	for _, o := range outcomes {
		if o.err != nil {
			perRepoErrors = append(perRepoErrors, process.GitLogRepoError{
				RepositoryName: o.subpath,
				Error:          o.err.Error(),
			})
			continue
		}
		if o.result == nil || !o.result.Success {
			errMsg := ""
			if o.result != nil {
				errMsg = o.result.Error
			}
			perRepoErrors = append(perRepoErrors, process.GitLogRepoError{
				RepositoryName: o.subpath,
				Error:          errMsg,
			})
			continue
		}
		for _, commit := range o.result.Commits {
			commit.RepositoryName = o.subpath
			merged = append(merged, commit)
		}
	}

	// Sort newest-first across repos. Falls back to insertion order if either
	// timestamp is unparseable.
	sortCommitsByCommittedAtDesc(merged)
	if limit > 0 && len(merged) > limit {
		merged = merged[:limit]
	}

	resp := process.GitLogResult{Commits: merged}
	if len(perRepoErrors) > 0 {
		resp.PerRepoErrors = perRepoErrors
	}
	// Success iff at least one repo succeeded. When every repo failed we mark
	// the response failed and put a one-line summary in Error so callers that
	// only check Success/Error keep working without inspecting PerRepoErrors.
	switch {
	case len(outcomes) > 0 && len(perRepoErrors) == len(outcomes):
		resp.Success = false
		resp.Error = fmt.Sprintf("git log failed for all %d repositories", len(outcomes))
	default:
		resp.Success = true
	}
	return resp
}

// handleGitLogMultiRepo fans the request out across every per-repo tracker,
// stamps RepositoryName on each returned commit, and merges into a single
// log sorted by committed_at descending. Per-repo limits are applied
// individually; the merged result is then truncated to the request limit.
//
// Per-repo failures are surfaced via PerRepoErrors rather than silently
// dropped, so the frontend can render an "incomplete" warning instead of
// pretending the missing repo simply has no history. The merged response
// stays Success=true as long as at least one repo succeeded; if every repo
// failed, Success=false with a summary in Error.
func (s *Server) handleGitLogMultiRepo(
	c *gin.Context,
	req GitLogRequest,
	subpaths []string,
	limit int,
) {
	outcomes := make([]perRepoLogOutcome, 0, len(subpaths))
	for _, sub := range subpaths {
		// Multi-repo: the caller-supplied `since` is a SHA that only exists in
		// the primary repo, and `target_branch` is the primary repo's base
		// branch (e.g. "main"). Both can be wrong for sibling repos — running
		// `git log <foreign-sha>..HEAD` in lvc fails outright and the repo's
		// commits silently disappear from the merged response. Compute a
		// per-repo base via merge-base against `origin/main` (with
		// `origin/master` fallback) so each repo's commits show up.
		perRepoReq := req
		perRepoReq.Since = ""
		perRepoReq.TargetBranch = ""
		if base := s.resolvePerRepoBase(c, sub); base != "" {
			perRepoReq.Since = base
		}
		result, err := s.runGitLogForRepo(c, perRepoReq, limit, sub)
		if err != nil {
			s.logger.Warn("git log for repo failed",
				zap.String("repo", sub), zap.Error(err))
		} else if !result.Success {
			s.logger.Warn("git log for repo returned failure",
				zap.String("repo", sub), zap.String("error", result.Error))
		}
		outcomes = append(outcomes, perRepoLogOutcome{
			subpath: sub,
			result:  result,
			err:     err,
		})
	}

	c.JSON(http.StatusOK, mergeGitLogResults(outcomes, limit))
}

// handleGitCumulativeDiff handles GET /api/v1/git/cumulative-diff.
//
// Multi-repo: when the caller doesn't pin a repo (?repo= empty) and the
// workspace contains per-repo subdirs, fan out one cumulative diff per repo
// and merge results, prefixing each file key with the repo subpath and
// stamping repository_name onto each file's payload. Without this the call
// would land on the workspace root which for multi-repo task workspaces
// isn't a git repo — and `git diff` would silently ascend to whatever .git
// encloses the task root (e.g. the user's outer kandev checkout) and return
// hundreds of unrelated files.
func (s *Server) handleGitCumulativeDiff(c *gin.Context) {
	var req GitCumulativeDiffRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, process.CumulativeDiffResult{
			Success: false,
			Error:   "invalid request: " + err.Error(),
		})
		return
	}

	if req.Repo == "" {
		if subs := s.procMgr.RepoSubpaths(); len(subs) > 0 {
			s.handleGitCumulativeDiffMultiRepo(c, req, subs)
			return
		}
	}

	result := s.runGitCumulativeDiffForRepo(c, req.Base, req.Repo)
	if result == nil {
		return // gitOp lookup error already wrote the response
	}
	c.JSON(http.StatusOK, result)
}

// runGitCumulativeDiffForRepo runs cumulative diff against a single repo
// subpath. Returns nil after writing an error response on lookup failures;
// otherwise returns the (possibly success=false) result for the caller.
func (s *Server) runGitCumulativeDiffForRepo(
	c *gin.Context,
	base, repo string,
) *process.CumulativeDiffResult {
	gitOp, gitOpErr := s.procMgr.GitOperatorFor(repo)
	if gitOpErr != nil {
		c.JSON(http.StatusBadRequest, process.CumulativeDiffResult{
			Success: false,
			Error:   gitOpErr.Error(),
		})
		return nil
	}
	result, err := gitOp.GetCumulativeDiff(c.Request.Context(), base)
	if err != nil {
		s.logger.Error("git cumulative diff failed",
			zap.String("base", base),
			zap.String("repo", repo),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, process.CumulativeDiffResult{
			Success: false,
			Error:   err.Error(),
		})
		return nil
	}
	return result
}

// handleGitCumulativeDiffMultiRepo fans cumulative diff out across every
// per-repo tracker. Each repo's base commit is computed locally via
// merge-base against the integration branch — the caller-supplied base only
// makes sense for one repo at a time, since each repo has its own commit
// graph. Files from each repo are merged into a single map, keyed by
// `<repoSubpath>/<path>` so paths that exist in multiple repos don't clash,
// and tagged with `repository_name` so the frontend can group them.
func (s *Server) handleGitCumulativeDiffMultiRepo(
	c *gin.Context,
	req GitCumulativeDiffRequest,
	subpaths []string,
) {
	merged := process.CumulativeDiffResult{
		Files:   make(map[string]interface{}),
		Success: true,
	}
	anyOK := false
	for _, sub := range subpaths {
		base := s.resolvePerRepoBase(c, sub)
		if base == "" {
			s.logger.Warn("cumulative diff: no per-repo base, skipping",
				zap.String("repo", sub))
			continue
		}
		result := s.runGitCumulativeDiffForRepo(c, base, sub)
		if result == nil {
			return // response already written
		}
		if !result.Success {
			s.logger.Warn("cumulative diff for repo returned failure",
				zap.String("repo", sub),
				zap.String("error", result.Error))
			continue
		}
		anyOK = true
		merged.TotalCommits += result.TotalCommits
		mergeCumulativeFiles(merged.Files, result.Files, sub)
	}
	if !anyOK {
		merged.Success = false
		merged.Error = fmt.Sprintf("cumulative diff failed for all %d repositories", len(subpaths))
	}
	c.JSON(http.StatusOK, merged)
}

// resolvePerRepoBase returns the merge-base of HEAD against the integration
// branch (origin/main, with origin/master fallback). Multi-repo tasks share
// no single base commit across repos, so each repo computes its own.
func (s *Server) resolvePerRepoBase(c *gin.Context, repo string) string {
	gitOp, err := s.procMgr.GitOperatorFor(repo)
	if err != nil {
		return ""
	}
	for _, candidate := range []string{"origin/main", "origin/master"} {
		base, mbErr := gitOp.GetMergeBase(c.Request.Context(), "HEAD", candidate)
		if mbErr == nil && base != "" {
			return base
		}
	}
	return ""
}

// mergeCumulativeFiles copies per-repo files into the merged map under a
// `<repo> <path>` key (NUL-separated) and decorates each file payload
// with `repository_name` + a `path` field carrying the repo-relative path.
// The composite key keeps `README.md` in two repos from clashing in the map;
// the frontend reads `path` and `repository_name` off the payload so the
// file tree groups under the repo header without the prefix bleeding into
// the displayed path. NUL is impossible in real paths, so the key is
// always uniquely splittable and the displayed path is unaffected.
func mergeCumulativeFiles(dst, src map[string]interface{}, repo string) {
	for path, payload := range src {
		m, ok := payload.(map[string]interface{})
		if !ok {
			// Defensive: only known shape from parseCommitDiff is map[string]interface{}.
			// If a future change emits something else, route it through unchanged
			// rather than silently dropping the path/repo metadata.
			dst[fmt.Sprintf("%s\x00%s", repo, path)] = payload
			continue
		}
		// Shallow-copy the per-file map before stamping repository_name +
		// path so the caller's source map isn't mutated. Earlier code wrote
		// directly to `m`, which permanently rewrote the per-repo result
		// before it could be reused (e.g. emitted to a second consumer).
		copied := make(map[string]interface{}, len(m)+2)
		for k, v := range m {
			copied[k] = v
		}
		copied["repository_name"] = repo
		copied["path"] = path
		dst[fmt.Sprintf("%s\x00%s", repo, path)] = copied
	}
}

// GitStatusResult represents the result of a git status query.
type GitStatusResult struct {
	Success         bool                   `json:"success"`
	Branch          string                 `json:"branch"`
	RemoteBranch    string                 `json:"remote_branch"`
	HeadCommit      string                 `json:"head_commit"`
	BaseCommit      string                 `json:"base_commit"` // Merge-base with origin branch
	Ahead           int                    `json:"ahead"`
	Behind          int                    `json:"behind"`
	Modified        []string               `json:"modified"`
	Added           []string               `json:"added"`
	Deleted         []string               `json:"deleted"`
	Untracked       []string               `json:"untracked"`
	Renamed         []string               `json:"renamed"`
	Files           map[string]interface{} `json:"files"`
	Timestamp       string                 `json:"timestamp"`
	BranchAdditions int                    `json:"branch_additions,omitempty"`
	BranchDeletions int                    `json:"branch_deletions,omitempty"`
	Error           string                 `json:"error,omitempty"`
}

// PerRepoGitStatus pairs a repository_name with its current status. Used by
// the multi-repo status fan-out so callers (notably the gateway's session
// subscribe handler) can deliver one stamped GitStatusUpdate per repo on
// reconnect — without it the frontend would only see the workspace-root
// (untagged) status and miss the per-repo grouping.
type PerRepoGitStatus struct {
	RepositoryName string          `json:"repository_name"`
	Status         GitStatusResult `json:"status"`
}

// MultiRepoGitStatusResult is the response shape for /api/v1/git/status/multi.
// For multi-repo task workspaces the response carries one entry per repo,
// stamped with its name. Single-repo workspaces return a single entry with an
// empty repository_name so callers can treat both shapes uniformly.
type MultiRepoGitStatusResult struct {
	Success bool               `json:"success"`
	Repos   []PerRepoGitStatus `json:"repos"`
	Error   string             `json:"error,omitempty"`
}

// handleGitStatusMulti returns one git status entry per repo for multi-repo
// task workspaces (or one untagged entry for single-repo). Used by the
// session-subscribe handler in the main backend to seed per-repo state on
// page reload — the legacy single GET /api/v1/git/status endpoint returns
// only the workspace-root status, which is empty for multi-repo task roots
// (the root isn't itself a git repo).
func (s *Server) handleGitStatusMulti(c *gin.Context) {
	subpaths := s.procMgr.RepoSubpaths()
	// Single-repo: fall back to the workspace-root status with an empty repo
	// name so the response shape stays uniform.
	if len(subpaths) == 0 {
		subpaths = []string{""}
	}
	result := MultiRepoGitStatusResult{Success: true, Repos: make([]PerRepoGitStatus, 0, len(subpaths))}
	for _, sub := range subpaths {
		entry := s.collectStatusForRepo(c, sub)
		result.Repos = append(result.Repos, entry)
	}
	c.JSON(http.StatusOK, result)
}

// collectStatusForRepo runs the status query for a single subpath and packs
// it into a PerRepoGitStatus. Failures land in Status.Error / Status.Success
// so the caller can render partial results instead of erroring out the whole
// fan-out when one repo is misconfigured.
func (s *Server) collectStatusForRepo(c *gin.Context, sub string) PerRepoGitStatus {
	wt, wtErr := s.procMgr.GetWorkspaceTrackerFor(sub)
	if wtErr != nil {
		return PerRepoGitStatus{
			RepositoryName: sub,
			Status:         GitStatusResult{Success: false, Error: wtErr.Error()},
		}
	}
	if wt == nil {
		return PerRepoGitStatus{
			RepositoryName: sub,
			Status:         GitStatusResult{Success: false, Error: "workspace tracker not available"},
		}
	}
	status, err := wt.GetCurrentGitStatus(c.Request.Context())
	if err != nil {
		return PerRepoGitStatus{
			RepositoryName: sub,
			Status:         GitStatusResult{Success: false, Error: err.Error()},
		}
	}
	filesMap := make(map[string]interface{}, len(status.Files))
	for k, v := range status.Files {
		filesMap[k] = v
	}
	return PerRepoGitStatus{
		RepositoryName: sub,
		Status: GitStatusResult{
			Success:         true,
			Branch:          status.Branch,
			RemoteBranch:    status.RemoteBranch,
			HeadCommit:      status.HeadCommit,
			BaseCommit:      status.BaseCommit,
			Ahead:           status.Ahead,
			Behind:          status.Behind,
			Modified:        status.Modified,
			Added:           status.Added,
			Deleted:         status.Deleted,
			Untracked:       status.Untracked,
			Renamed:         status.Renamed,
			Files:           filesMap,
			Timestamp:       status.Timestamp.Format("2006-01-02T15:04:05.000Z07:00"),
			BranchAdditions: status.BranchAdditions,
			BranchDeletions: status.BranchDeletions,
		},
	}
}

// handleGitStatus handles GET /api/v1/git/status. Accepts an optional
// ?repo=<subpath> query param that selects a sub-directory of the workspace
// for multi-repo task roots; empty = workspace root (single-repo behavior).
func (s *Server) handleGitStatus(c *gin.Context) {
	wt, wtErr := s.procMgr.GetWorkspaceTrackerFor(c.Query("repo"))
	if wtErr != nil {
		c.JSON(http.StatusBadRequest, GitStatusResult{
			Success: false,
			Error:   wtErr.Error(),
		})
		return
	}
	if wt == nil {
		c.JSON(http.StatusInternalServerError, GitStatusResult{
			Success: false,
			Error:   "workspace tracker not available",
		})
		return
	}

	status, err := wt.GetCurrentGitStatus(c.Request.Context())
	if err != nil {
		s.logger.Error("git status failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, GitStatusResult{
			Success: false,
			Error:   err.Error(),
		})
		return
	}

	// Convert Files map to interface{} map for JSON serialization
	filesMap := make(map[string]interface{}, len(status.Files))
	for k, v := range status.Files {
		filesMap[k] = v
	}

	c.JSON(http.StatusOK, GitStatusResult{
		Success:         true,
		Branch:          status.Branch,
		RemoteBranch:    status.RemoteBranch,
		HeadCommit:      status.HeadCommit,
		BaseCommit:      status.BaseCommit,
		Ahead:           status.Ahead,
		Behind:          status.Behind,
		Modified:        status.Modified,
		Added:           status.Added,
		Deleted:         status.Deleted,
		Untracked:       status.Untracked,
		Renamed:         status.Renamed,
		Files:           filesMap,
		Timestamp:       status.Timestamp.Format("2006-01-02T15:04:05.000Z07:00"),
		BranchAdditions: status.BranchAdditions,
		BranchDeletions: status.BranchDeletions,
	})
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
