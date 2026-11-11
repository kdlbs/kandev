package orchestrator

import (
	"context"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestrator/executor"
)

// withTestExecutorActor models the authenticated context production admission
// restores for a persisted session. Tests for admission behavior install their
// own callback and do not use this helper.
func withTestExecutorActor(exec *executor.Executor) *executor.Executor {
	exec.SetCapabilities(permissiveTestCapabilities{})
	exec.SetExecutorAdmission(func(ctx context.Context, _, _, _ string) (context.Context, error) {
		if _, ok := authn.IdentityFromContext(ctx); ok {
			return ctx, nil
		}
		return authn.WithIdentity(ctx, authn.Identity{UserID: "test-actor", Role: authn.RoleMember}), nil
	})
	return exec
}
