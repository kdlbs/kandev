package executor

import (
	"context"

	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/auth/authn"
)

// managedRuntimeOwnerUserID resolves the trusted application owner for a
// managed runtime installation through the shared policy. An interactive
// request uses its authenticated actor; background launches retain an explicit
// workspace owner; disabled authentication supplies the single-user fallback
// from backend composition. An empty result disables reusable repair identity.
func (e *Executor) managedRuntimeOwnerUserID(ctx context.Context, workspaceID string) string {
	input := managedruntime.RepairOwnerInput{}
	if identity, ok := authn.IdentityFromContext(ctx); ok {
		input.ActorUserID = identity.UserID
		input.ActorSynthetic = identity.Synthetic
	}
	if workspaceID != "" && e.repo != nil {
		if workspace, err := e.repo.GetWorkspace(ctx, workspaceID); err == nil && workspace != nil {
			input.WorkspaceOwnerUserID = workspace.OwnerID
		}
	}
	if e.managedRuntimeSingleUserOwner != nil {
		input.SingleUserFallbackUserID, input.SingleUserFallbackAllowed = e.managedRuntimeSingleUserOwner()
	}
	return managedruntime.ResolveRepairOwner(input)
}

// SetManagedRuntimeSingleUserOwner wires the live authentication mode and the
// established single-user identity. The callback must return allowed=false for
// setup, enabled, unknown, or unavailable authentication.
func (e *Executor) SetManagedRuntimeSingleUserOwner(owner func() (string, bool)) {
	e.managedRuntimeSingleUserOwner = owner
}
