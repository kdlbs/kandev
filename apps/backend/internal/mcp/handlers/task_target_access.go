package handlers

import (
	"context"

	"github.com/kandev/kandev/internal/coordinator"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
)

// canDirectParentAccess is deliberately narrower than workspace membership:
// only a direct parent may mutate its child through task-bound MCP.
func canDirectParentAccess(caller, target *models.Task) bool {
	return caller != nil && target != nil && caller.WorkspaceID != "" &&
		target.WorkspaceID == caller.WorkspaceID && target.ParentID == caller.ID
}

// canonicalMCPCaller prevents an agent-controlled request payload from
// replacing the task and session bound to the inbound MCP connection.
func canonicalMCPCaller(ctx context.Context, taskID, sessionID string) (string, string, bool) {
	principal, scoped := mcpscope.PrincipalFromContext(ctx)
	if !scoped {
		return taskID, sessionID, true
	}
	if (taskID != "" && taskID != principal.CallerTaskID) ||
		(sessionID != "" && sessionID != principal.CallerSessionID) {
		return "", "", false
	}
	return principal.CallerTaskID, principal.CallerSessionID, true
}

func (h *Handlers) authorizeCoordinatorAction(ctx context.Context, caller, target *models.Task, actorSessionID, action string, capability coordinator.Capability) (coordinator.Decision, error) {
	if h.coordinatorAuthority == nil {
		if canDirectParentAccess(caller, target) {
			return coordinator.Decision{Allowed: true, Basis: coordinator.BasisDirectParent}, nil
		}
		return coordinator.Decision{Basis: coordinator.BasisDenied}, nil
	}
	return h.coordinatorAuthority.Authorize(ctx, coordinator.Request{ActorTask: caller, TargetTask: target, ActorSessionID: actorSessionID, Action: action, Capability: capability})
}

func (h *Handlers) finishCoordinatorAction(ctx context.Context, decision coordinator.Decision, err error) error {
	if h.coordinatorAuthority == nil {
		return nil
	}
	return h.coordinatorAuthority.Finish(ctx, decision, err)
}
