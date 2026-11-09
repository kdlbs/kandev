package handlers

import (
	"context"
	"errors"

	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/review"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

var errReviewSourceNotAdmitted = errors.New("worktree_path is outside the task's local repositories and worktrees")

// admitAgentReviewSource limits an MCP caller's non-empty worktree_path to the target
// task's own local repositories and worktrees. A person's authenticated
// WebSocket request carries no MCP principal, does not pass through here, and
// may name any backend-local checkout; an agent, which may run in a container
// or on another host, must not reach an unrelated backend-host repository.
func (h *Handlers) admitAgentReviewSource(ctx context.Context, msg *ws.Message, taskID, worktreePath string) *ws.Message {
	if _, agent := mcpscope.PrincipalFromContext(ctx); !agent || worktreePath == "" {
		return nil
	}
	roots, err := h.reviewSourceRoots(ctx, taskID)
	if err == nil && review.SourceWithinRoots(worktreePath, roots) {
		return nil
	}
	if err == nil {
		err = errReviewSourceNotAdmitted
	}
	resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, err.Error(), nil)
	return resp
}

// reviewSourceRoots lists the task's registered local repository checkouts and
// the workspace and worktree paths of its sessions on local executors. A remote
// session's paths name another machine, so they never admit a backend path.
func (h *Handlers) reviewSourceRoots(ctx context.Context, taskID string) ([]string, error) {
	links, err := h.taskSvc.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return nil, err
	}
	roots := make([]string, 0, len(links))
	for _, link := range links {
		if link == nil || link.RepositoryID == "" {
			continue
		}
		repository, err := h.taskSvc.GetRepository(ctx, link.RepositoryID)
		if err != nil {
			return nil, err
		}
		if repository != nil && repository.LocalPath != "" {
			roots = append(roots, repository.LocalPath)
		}
	}
	sessions, err := h.taskSvc.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	for _, session := range sessions {
		if session == nil || !h.localReviewExecutor(ctx, session.ExecutorID) {
			continue
		}
		roots = append(roots, session.WorkspacePath)
		for _, worktree := range session.Worktrees {
			if worktree != nil {
				roots = append(roots, worktree.WorktreePath)
			}
		}
	}
	return roots, nil
}

func (h *Handlers) localReviewExecutor(ctx context.Context, executorID string) bool {
	if executorID == "" {
		return false
	}
	executor, err := h.taskSvc.GetExecutor(ctx, executorID)
	if err != nil || executor == nil {
		return false
	}
	switch executor.Type {
	case models.ExecutorTypeLocal, models.ExecutorTypeWorktree, models.ExecutorType("local_pc"):
		return true
	}
	return false
}
