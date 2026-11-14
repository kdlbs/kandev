package handlers

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/auth/authn"
	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/review"
	ws "github.com/kandev/kandev/pkg/websocket"
)

var errReviewSourceNotAdmitted = errors.New("worktree_path is outside the task's local repositories and worktrees")

// admitAgentReviewSource limits MCP sources to the task's local repositories
// and worktrees. A browser operator can explicitly select an external checkout
// using the existing workspace-management authority.
func (h *Handlers) admitAgentReviewSource(ctx context.Context, msg *ws.Message, taskID, worktreePath string) *ws.Message {
	_, denied := h.reviewSourceOperator(ctx, msg, taskID, worktreePath)
	return denied
}

// reviewSourceOperator returns the authenticated operator for an explicitly
// selected external checkout, or an empty identity for a task-owned source.
func (h *Handlers) reviewSourceOperator(ctx context.Context, msg *ws.Message, taskID, worktreePath string) (string, *ws.Message) {
	if worktreePath == "" {
		return "", nil
	}
	allowed, err := review.TaskSourceAdmitted(ctx, h.taskSvc, taskID, worktreePath)
	if allowed {
		return "", nil
	}
	_, agent := mcpscope.PrincipalFromContext(ctx)
	identity, authenticated := authn.IdentityFromContext(ctx)
	if err == nil && !agent && !mcporigin.IsTrustedExternalTransport(ctx) &&
		authenticated && !identity.Synthetic && identity.UserID != "" && identity.SessionID != "" &&
		h.taskSvc.AuthorizeReviewSourceOperator(ctx, taskID) == nil {
		return identity.UserID, nil
	}
	if err == nil {
		err = errReviewSourceNotAdmitted
	}
	resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden, err.Error(), nil)
	return "", resp
}
