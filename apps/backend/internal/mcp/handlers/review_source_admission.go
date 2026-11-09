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

// admitAgentReviewSource limits an MCP caller's non-empty worktree_path to the
// target task's registered local repositories, including their other
// worktrees, and its local session worktrees. A person's authenticated
// WebSocket request carries no MCP principal and may name any backend-local
// checkout; an agent, which may run in a container or on another host, must
// not reach an unrelated backend-host repository.
func (h *Handlers) admitAgentReviewSource(ctx context.Context, msg *ws.Message, taskID, worktreePath string) *ws.Message {
	if _, agent := mcpscope.PrincipalFromContext(ctx); !agent || worktreePath == "" {
		return nil
	}
	roots, repositories, err := h.reviewSourceRoots(ctx, taskID)
	if err == nil && review.SourceAdmitted(ctx, worktreePath, roots, repositories) {
		return nil
	}
	if err == nil {
		err = errReviewSourceNotAdmitted
	}
	resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, err.Error(), nil)
	return resp
}

// reviewSourceRoots lists the admitted roots, which are the task's registered
// local repository checkouts and the workspace and worktree paths of its
// sessions on local executors, and separately the registered checkouts whose
// other worktrees are also admitted. A remote session's paths name another
// machine, so they never admit a backend path.
func (h *Handlers) reviewSourceRoots(ctx context.Context, taskID string) ([]string, []string, error) {
	links, err := h.taskSvc.ListTaskRepositories(ctx, taskID)
	if err != nil {
		return nil, nil, err
	}
	var repositories []string
	for _, link := range links {
		if link == nil || link.RepositoryID == "" {
			continue
		}
		repository, err := h.taskSvc.GetRepository(ctx, link.RepositoryID)
		if err != nil {
			return nil, nil, err
		}
		if repository != nil && repository.LocalPath != "" {
			repositories = append(repositories, repository.LocalPath)
		}
	}
	roots := append([]string(nil), repositories...)
	sessions, err := h.taskSvc.ListTaskSessions(ctx, taskID)
	if err != nil {
		return nil, nil, err
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
	return roots, repositories, nil
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
