// Package handlers provides WebSocket and HTTP handlers for agent operations.
package handlers

import (
	"context"
	"fmt"
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

// GitHandlers provides WebSocket handlers for git worktree operations.
// Operations are executed via agentctl which runs in the worktree context.
type GitHandlers struct {
	lifecycleMgr *lifecycle.Manager
	logger       *logger.Logger
	onPRCreated  PRCreatedCallback
}

// NewGitHandlers creates a new GitHandlers instance
func NewGitHandlers(lifecycleMgr *lifecycle.Manager, log *logger.Logger) *GitHandlers {
	return &GitHandlers{
		lifecycleMgr: lifecycleMgr,
		logger:       log.WithFields(zap.String("component", "git_handlers")),
	}
}

// SetOnPRCreated sets a callback invoked after a PR is successfully created.
func (h *GitHandlers) SetOnPRCreated(cb PRCreatedCallback) {
	h.onPRCreated = cb
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
		req.Mode = "mixed" // Default to mixed reset
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
		return nil, err
	}

	result, err := client.GitShowCommit(ctx, req.CommitSHA)
	if err != nil {
		return nil, fmt.Errorf("show commit failed: %w", err)
	}

	return ws.NewResponse(msg.ID, msg.Action, result)
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
