// Package handlers provides WebSocket and HTTP handlers for agent operations.
package handlers

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// GitHandlers provides WebSocket handlers for git worktree operations.
// Operations are executed via agentctl which runs in the worktree context.
type GitHandlers struct {
	lifecycleMgr *lifecycle.Manager
	logger       *logger.Logger
}

// NewGitHandlers creates a new GitHandlers instance
func NewGitHandlers(lifecycleMgr *lifecycle.Manager, log *logger.Logger) *GitHandlers {
	return &GitHandlers{
		lifecycleMgr: lifecycleMgr,
		logger:       log.WithFields(zap.String("component", "git_handlers")),
	}
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
	d.RegisterFunc(ws.ActionWorktreeCreatePR, h.wsCreatePR)
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
}

// GitStageRequest for worktree.stage action
type GitStageRequest struct {
	SessionID string   `json:"session_id"`
	Paths     []string `json:"paths"` // Empty = stage all
}

// GitCreatePRRequest for worktree.create_pr action
type GitCreatePRRequest struct {
	SessionID  string `json:"session_id"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	BaseBranch string `json:"base_branch"`
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

	result, err := client.GitCommit(ctx, req.Message, req.StageAll)
	if err != nil {
		return nil, fmt.Errorf("commit failed: %w", err)
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

	result, err := client.GitCreatePR(ctx, req.Title, req.Body, req.BaseBranch)
	if err != nil {
		return nil, fmt.Errorf("create PR failed: %w", err)
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

