// Package handlers provides WebSocket and HTTP handlers for agent operations.
package handlers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// PRCreatedCallback is called after a PR is successfully created.
// Parameters: ctx, sessionID, taskID, prURL, branch.
type PRCreatedCallback func(ctx context.Context, sessionID, taskID, prURL, branch string)

// GitOperationFailedCallback is called when a git operation fails.
// Parameters: ctx, sessionID, taskID, operation name, error output.
type GitOperationFailedCallback func(ctx context.Context, sessionID, taskID, operation, errorOutput string)

// ExecutionLookup provides access to running agent executions by session ID.
type ExecutionLookup interface {
	GetExecutionBySessionID(sessionID string) (*lifecycle.AgentExecution, bool)
}

// SessionReader is a minimal interface for reading session metadata.
// This is needed because git operations need to know the session's base commit SHA
// to filter commits to only those made during the session.
type SessionReader interface {
	// GetSessionBaseCommit returns the base commit SHA for a session.
	// Returns empty string if not set or on error.
	GetSessionBaseCommit(ctx context.Context, sessionID string) string
}

// GitHandlers provides WebSocket handlers for git worktree operations.
// Operations are executed via agentctl which runs in the worktree context.
type GitHandlers struct {
	lifecycleMgr         ExecutionLookup
	sessionReader        SessionReader
	logger               *logger.Logger
	onPRCreated          PRCreatedCallback
	onGitOperationFailed GitOperationFailedCallback
}

// NewGitHandlers creates a new GitHandlers instance.
// sessionReader is required to look up session metadata (e.g., base commit SHA).
func NewGitHandlers(lifecycleMgr ExecutionLookup, sessionReader SessionReader, log *logger.Logger) *GitHandlers {
	return &GitHandlers{
		lifecycleMgr:  lifecycleMgr,
		sessionReader: sessionReader,
		logger:        log.WithFields(zap.String("component", "git_handlers")),
	}
}

// SetOnPRCreated sets a callback invoked after a PR is successfully created.
func (h *GitHandlers) SetOnPRCreated(cb PRCreatedCallback) {
	h.onPRCreated = cb
}

// SetOnGitOperationFailed sets a callback invoked when a git operation fails.
func (h *GitHandlers) SetOnGitOperationFailed(cb GitOperationFailedCallback) {
	h.onGitOperationFailed = cb
}

// notifyGitOperationFailed fires the failure callback asynchronously if the result indicates failure.
func (h *GitHandlers) notifyGitOperationFailed(sessionID, operation string, result *client.GitOperationResult) {
	if result == nil || result.Success || h.onGitOperationFailed == nil {
		return
	}
	execution, ok := h.lifecycleMgr.GetExecutionBySessionID(sessionID)
	if !ok || execution.TaskID == "" {
		return
	}
	taskID := execution.TaskID
	errorOutput := result.Error
	if errorOutput == "" {
		errorOutput = result.Output
	}
	go func() {
		callbackCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		h.onGitOperationFailed(callbackCtx, sessionID, taskID, operation, errorOutput)
	}()
}

// RegisterHandlers registers git handlers with the WebSocket dispatcher
func (h *GitHandlers) RegisterHandlers(d *ws.Dispatcher) {
	d.RegisterFunc(ws.ActionWorktreePull, h.wsPull)
	d.RegisterFunc(ws.ActionWorktreePush, h.wsPush)
	d.RegisterFunc(ws.ActionWorktreeRebase, h.wsRebase)
	d.RegisterFunc(ws.ActionWorktreeMerge, h.wsMerge)
	d.RegisterFunc(ws.ActionWorktreeAbort, h.wsAbort)
	d.RegisterFunc(ws.ActionWorktreeCommit, h.wsCommit)
	d.RegisterFunc(ws.ActionWorktreeStage, h.wsStage)
	d.RegisterFunc(ws.ActionWorktreeUnstage, h.wsUnstage)
	d.RegisterFunc(ws.ActionWorktreeDiscard, h.wsDiscard)
	d.RegisterFunc(ws.ActionWorktreeCreatePR, h.wsCreatePR)
	d.RegisterFunc(ws.ActionWorktreeRevertCommit, h.wsRevertCommit)
	d.RegisterFunc(ws.ActionWorktreeRenameBranch, h.wsRenameBranch)
	d.RegisterFunc(ws.ActionWorktreeReset, h.wsReset)
	d.RegisterFunc(ws.ActionSessionCommitDiff, h.wsCommitDiff)
	d.RegisterFunc(ws.ActionSessionGitCommits, h.wsGitCommits)
	d.RegisterFunc(ws.ActionSessionCumulativeDiff, h.wsCumulativeDiff)
}

// GitPullRequest for worktree.pull action
type GitPullRequest struct {
	SessionID string `json:"session_id"`
	Rebase    bool   `json:"rebase"`
}

// GitPushRequest for worktree.push action
type GitPushRequest struct {
	SessionID   string `json:"session_id"`
	Force       bool   `json:"force"`
	SetUpstream bool   `json:"set_upstream"`
}

// GitRebaseRequest for worktree.rebase action
type GitRebaseRequest struct {
	SessionID  string `json:"session_id"`
	BaseBranch string `json:"base_branch"`
}

// GitMergeRequest for worktree.merge action
type GitMergeRequest struct {
	SessionID  string `json:"session_id"`
	BaseBranch string `json:"base_branch"`
}

// GitAbortRequest for worktree.abort action
type GitAbortRequest struct {
	SessionID string `json:"session_id"`
	Operation string `json:"operation"` // "merge" or "rebase"
}

// GitCommitRequest for worktree.commit action
type GitCommitRequest struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message"`
	StageAll  bool   `json:"stage_all"`
	Amend     bool   `json:"amend"`
}

// GitRenameBranchRequest for worktree.rename_branch action
type GitRenameBranchRequest struct {
	SessionID string `json:"session_id"`
	NewName   string `json:"new_name"`
}

// GitStageRequest for worktree.stage action
type GitStageRequest struct {
	SessionID string   `json:"session_id"`
	Paths     []string `json:"paths"` // Empty = stage all
}

// GitUnstageRequest for worktree.unstage action
type GitUnstageRequest struct {
	SessionID string   `json:"session_id"`
	Paths     []string `json:"paths"` // Empty = unstage all
}

// GitDiscardRequest for worktree.discard action
type GitDiscardRequest struct {
	SessionID string   `json:"session_id"`
	Paths     []string `json:"paths"` // Required - files to discard
}

// GitCreatePRRequest for worktree.create_pr action
type GitCreatePRRequest struct {
	SessionID  string `json:"session_id"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	BaseBranch string `json:"base_branch"`
	Draft      bool   `json:"draft"`
}

// GitRevertCommitRequest for worktree.revert_commit action
type GitRevertCommitRequest struct {
	SessionID string `json:"session_id"`
	CommitSHA string `json:"commit_sha"`
}

// GitResetRequest for worktree.reset action
type GitResetRequest struct {
	SessionID string `json:"session_id"`
	CommitSHA string `json:"commit_sha"`
	Mode      string `json:"mode"` // "soft", "mixed", or "hard"
}

// GitShowCommitRequest for session.commit_diff action
type GitShowCommitRequest struct {
	SessionID string `json:"session_id"`
	CommitSHA string `json:"commit_sha"`
}

// wsPull handles worktree.pull action
func (h *GitHandlers) wsPull(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req GitPullRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	client, err := h.getAgentCtlClient(req.SessionID)
	if err != nil {
		return nil, err
	}

	result, err := client.GitPull(ctx, req.Rebase)
	if err != nil {
		return nil, fmt.Errorf("pull failed: %w", err)
	}

	h.notifyGitOperationFailed(req.SessionID, "pull", result)
	return ws.NewResponse(msg.ID, msg.Action, result)
}

// wsPush handles worktree.push action
func (h *GitHandlers) wsPush(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req GitPushRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	client, err := h.getAgentCtlClient(req.SessionID)
	if err != nil {
		return nil, err
	}

	result, err := client.GitPush(ctx, req.Force, req.SetUpstream)
	if err != nil {
		return nil, fmt.Errorf("push failed: %w", err)
	}

	h.notifyGitOperationFailed(req.SessionID, "push", result)
	return ws.NewResponse(msg.ID, msg.Action, result)
}

// wsRebase handles worktree.rebase action
func (h *GitHandlers) wsRebase(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req GitRebaseRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if req.BaseBranch == "" {
		return nil, fmt.Errorf("base_branch is required")
	}

	client, err := h.getAgentCtlClient(req.SessionID)
	if err != nil {
		return nil, err
	}

	result, err := client.GitRebase(ctx, req.BaseBranch)
	if err != nil {
		return nil, fmt.Errorf("rebase failed: %w", err)
	}

	h.notifyGitOperationFailed(req.SessionID, "rebase", result)
	return ws.NewResponse(msg.ID, msg.Action, result)
}

// wsMerge handles worktree.merge action
func (h *GitHandlers) wsMerge(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req GitMergeRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if req.BaseBranch == "" {
		return nil, fmt.Errorf("base_branch is required")
	}

	client, err := h.getAgentCtlClient(req.SessionID)
	if err != nil {
		return nil, err
	}

	result, err := client.GitMerge(ctx, req.BaseBranch)
	if err != nil {
		return nil, fmt.Errorf("merge failed: %w", err)
	}

	h.notifyGitOperationFailed(req.SessionID, "merge", result)
	return ws.NewResponse(msg.ID, msg.Action, result)
}

// wsAbort handles worktree.abort action
func (h *GitHandlers) wsAbort(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req GitAbortRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if req.Operation != "merge" && req.Operation != "rebase" {
		return nil, fmt.Errorf("operation must be 'merge' or 'rebase'")
	}

	client, err := h.getAgentCtlClient(req.SessionID)
	if err != nil {
		return nil, err
	}

	result, err := client.GitAbort(ctx, req.Operation)
	if err != nil {
		return nil, fmt.Errorf("abort failed: %w", err)
	}

	return ws.NewResponse(msg.ID, msg.Action, result)
}

// wsCommit handles worktree.commit action
func (h *GitHandlers) wsCommit(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req GitCommitRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if req.Message == "" {
		return nil, fmt.Errorf("message is required")
	}

	client, err := h.getAgentCtlClient(req.SessionID)
	if err != nil {
		return nil, err
	}

	result, err := client.GitCommit(ctx, req.Message, req.StageAll, req.Amend)
	if err != nil {
		return nil, fmt.Errorf("commit failed: %w", err)
	}

	h.notifyGitOperationFailed(req.SessionID, "commit", result)
	return ws.NewResponse(msg.ID, msg.Action, result)
}

// wsRenameBranch handles worktree.rename_branch action
func (h *GitHandlers) wsRenameBranch(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req GitRenameBranchRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if req.NewName == "" {
		return nil, fmt.Errorf("new_name is required")
	}

	client, err := h.getAgentCtlClient(req.SessionID)
	if err != nil {
		return nil, err
	}

	result, err := client.GitRenameBranch(ctx, req.NewName)
	if err != nil {
		return nil, fmt.Errorf("rename branch failed: %w", err)
	}

	return ws.NewResponse(msg.ID, msg.Action, result)
}

// wsReset handles worktree.reset action
func (h *GitHandlers) wsReset(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req GitResetRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if req.CommitSHA == "" {
		return nil, fmt.Errorf("commit_sha is required")
	}
	if req.Mode == "" {
		req.Mode = "mixed"
	}
	validModes := map[string]bool{"soft": true, "mixed": true, "hard": true}
	if !validModes[req.Mode] {
		return nil, fmt.Errorf("invalid reset mode: %s (must be soft, mixed, or hard)", req.Mode)
	}

	client, err := h.getAgentCtlClient(req.SessionID)
	if err != nil {
		return nil, err
	}

	result, err := client.GitReset(ctx, req.CommitSHA, req.Mode)
	if err != nil {
		return nil, fmt.Errorf("reset failed: %w", err)
	}

	return ws.NewResponse(msg.ID, msg.Action, result)
}

// wsStage handles worktree.stage action
func (h *GitHandlers) wsStage(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req GitStageRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	client, err := h.getAgentCtlClient(req.SessionID)
	if err != nil {
		return nil, err
	}

	result, err := client.GitStage(ctx, req.Paths)
	if err != nil {
		return nil, fmt.Errorf("stage failed: %w", err)
	}

	return ws.NewResponse(msg.ID, msg.Action, result)
}

// wsUnstage handles worktree.unstage action
func (h *GitHandlers) wsUnstage(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req GitUnstageRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	client, err := h.getAgentCtlClient(req.SessionID)
	if err != nil {
		return nil, err
	}

	result, err := client.GitUnstage(ctx, req.Paths)
	if err != nil {
		return nil, fmt.Errorf("unstage failed: %w", err)
	}

	return ws.NewResponse(msg.ID, msg.Action, result)
}

// wsDiscard handles worktree.discard action
func (h *GitHandlers) wsDiscard(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req GitDiscardRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	if len(req.Paths) == 0 {
		return nil, fmt.Errorf("paths are required")
	}

	client, err := h.getAgentCtlClient(req.SessionID)
	if err != nil {
		return nil, err
	}

	result, err := client.GitDiscard(ctx, req.Paths)
	if err != nil {
		return nil, fmt.Errorf("discard failed: %w", err)
	}

	return ws.NewResponse(msg.ID, msg.Action, result)
}

// wsCreatePR handles worktree.create_pr action
func (h *GitHandlers) wsCreatePR(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req GitCreatePRRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if req.Title == "" {
		return nil, fmt.Errorf("title is required")
	}

	client, err := h.getAgentCtlClient(req.SessionID)
	if err != nil {
		return nil, err
	}

	result, err := client.GitCreatePR(ctx, req.Title, req.Body, req.BaseBranch, req.Draft)
	if err != nil {
		return nil, fmt.Errorf("create PR failed: %w", err)
	}

	// On success, notify callback to associate PR with task.
	// Use a timeout-bound context so a stuck callback doesn't leak the goroutine.
	if result.Success && result.PRURL != "" && h.onPRCreated != nil {
		execution, ok := h.lifecycleMgr.GetExecutionBySessionID(req.SessionID)
		if ok && execution.TaskID != "" {
			sessionID := req.SessionID
			taskID := execution.TaskID
			prURL := result.PRURL
			go func() {
				callbackCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				h.onPRCreated(callbackCtx, sessionID, taskID, prURL, "")
			}()
		}
	}

	return ws.NewResponse(msg.ID, msg.Action, result)
}

// wsRevertCommit handles worktree.revert_commit action
func (h *GitHandlers) wsRevertCommit(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req GitRevertCommitRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if req.CommitSHA == "" {
		return nil, fmt.Errorf("commit_sha is required")
	}

	client, err := h.getAgentCtlClient(req.SessionID)
	if err != nil {
		return nil, err
	}

	result, err := client.GitRevertCommit(ctx, req.CommitSHA)
	if err != nil {
		return nil, fmt.Errorf("revert commit failed: %w", err)
	}

	h.notifyGitOperationFailed(req.SessionID, "revert", result)
	return ws.NewResponse(msg.ID, msg.Action, result)
}

// wsCommitDiff handles session.commit_diff action
func (h *GitHandlers) wsCommitDiff(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req GitShowCommitRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}
	if req.CommitSHA == "" {
		return nil, fmt.Errorf("commit_sha is required")
	}

	client, err := h.getAgentCtlClient(req.SessionID)
	if err != nil {
		if isSessionNotReadyError(err) {
			return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
				"success":        false,
				"ready":          false,
				"reason":         "agent_starting",
				"retry_after_ms": 500,
			})
		}
		return nil, err
	}

	result, err := client.GitShowCommit(ctx, req.CommitSHA)
	if err != nil {
		return nil, fmt.Errorf("show commit failed: %w", err)
	}

	return ws.NewResponse(msg.ID, msg.Action, result)
}

func isSessionNotReadyError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no agent running for session") ||
		strings.Contains(msg, "agent client not available for session")
}

// getAgentCtlClient gets the agentctl client for a session
func (h *GitHandlers) getAgentCtlClient(sessionID string) (*client.Client, error) {
	execution, ok := h.lifecycleMgr.GetExecutionBySessionID(sessionID)
	if !ok {
		return nil, fmt.Errorf("no agent running for session %s", sessionID)
	}

	c := execution.GetAgentCtlClient()
	if c == nil {
		return nil, fmt.Errorf("agent client not available for session %s", sessionID)
	}

	return c, nil
}

// GitCommitsRequest for session.git.commits action
type GitCommitsRequest struct {
	SessionID string `json:"session_id"`
	Limit     int    `json:"limit"` // Max commits to return
}

// wsGitCommits handles session.git.commits action
// The base commit SHA is always looked up from the session metadata in the database.
// This ensures commits are filtered to only those made during the session.
func (h *GitHandlers) wsGitCommits(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req GitCommitsRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	agentClient, err := h.getAgentCtlClient(req.SessionID)
	if err != nil {
		return nil, err
	}

	// Look up base commit SHA from the session metadata
	var baseCommit string
	if h.sessionReader != nil {
		baseCommit = h.sessionReader.GetSessionBaseCommit(ctx, req.SessionID)
	}

	// Fallback: if base_commit_sha is not stored in session, use git merge-base
	// from git status. This happens for sessions created before the base commit
	// capture feature or if the capture failed.
	if baseCommit == "" {
		status, statusErr := agentClient.GetGitStatus(ctx)
		if statusErr == nil && status != nil && status.BaseCommit != "" {
			baseCommit = status.BaseCommit
			h.logger.Debug("using git status base commit as fallback",
				zap.String("session_id", req.SessionID),
				zap.String("base_commit", baseCommit))
		}
	}

	result, err := agentClient.GitLog(ctx, baseCommit, req.Limit)
	if err != nil {
		return nil, fmt.Errorf("git log failed: %w", err)
	}

	return ws.NewResponse(msg.ID, msg.Action, result)
}

// CumulativeDiffRequest for session.cumulative_diff action
type CumulativeDiffRequest struct {
	SessionID string `json:"session_id"`
}

// wsCumulativeDiff handles session.cumulative_diff action
// The base commit SHA is always looked up from the session metadata in the database.
func (h *GitHandlers) wsCumulativeDiff(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req CumulativeDiffRequest
	if err := msg.ParsePayload(&req); err != nil {
		return nil, fmt.Errorf("invalid payload: %w", err)
	}

	if req.SessionID == "" {
		return nil, fmt.Errorf("session_id is required")
	}

	agentClient, err := h.getAgentCtlClient(req.SessionID)
	if err != nil {
		return nil, err
	}

	// Look up base commit SHA from the session metadata
	var baseCommit string
	if h.sessionReader != nil {
		baseCommit = h.sessionReader.GetSessionBaseCommit(ctx, req.SessionID)
	}

	// Fallback: if base_commit_sha is not stored, use git merge-base from status
	if baseCommit == "" {
		status, statusErr := agentClient.GetGitStatus(ctx)
		if statusErr == nil && status != nil && status.BaseCommit != "" {
			baseCommit = status.BaseCommit
			h.logger.Debug("using git status base commit as fallback for cumulative diff",
				zap.String("session_id", req.SessionID),
				zap.String("base_commit", baseCommit))
		} else {
			return nil, fmt.Errorf("no base commit available for cumulative diff")
		}
	}

	result, err := agentClient.GetCumulativeDiff(ctx, baseCommit)
	if err != nil {
		return nil, fmt.Errorf("cumulative diff failed: %w", err)
	}

	// Wrap in cumulative_diff key as expected by frontend
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"cumulative_diff": result,
	})
}
