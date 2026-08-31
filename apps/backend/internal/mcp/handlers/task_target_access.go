package handlers

import (
	"context"

	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/task/models"
)

func canDirectParentAccess(caller, target *models.Task) bool {
	return caller != nil && target != nil && caller.WorkspaceID != "" &&
		target.WorkspaceID == caller.WorkspaceID && target.ParentID == caller.ID
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

func (h *Handlers) finishCoordinatorAction(ctx context.Context, decision coordinator.Decision, operationErr error) error {
	if h.coordinatorAuthority == nil {
		return nil
	}
	return h.coordinatorAuthority.Finish(ctx, decision, operationErr)
}
