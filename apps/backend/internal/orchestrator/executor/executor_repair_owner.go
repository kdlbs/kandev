package executor

import (
	"context"

	"github.com/kandev/kandev/internal/auth/authn"
)

// managedRuntimeOwnerUserID resolves the trusted application owner for a
// managed runtime installation. An interactive request uses its authenticated
// actor; background launches fall back to the workspace owner. An empty result
// disables reusable repair identity rather than sharing one implicitly.
func (e *Executor) managedRuntimeOwnerUserID(ctx context.Context, workspaceID string) string {
	if identity, ok := authn.IdentityFromContext(ctx); ok && identity.UserID != "" {
		return identity.UserID
	}
	if workspaceID == "" || e.repo == nil {
		return ""
	}
	workspace, err := e.repo.GetWorkspace(ctx, workspaceID)
	if err != nil || workspace == nil {
		return ""
	}
	return workspace.OwnerID
}
