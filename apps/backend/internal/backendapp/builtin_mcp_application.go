package backendapp

import (
	"context"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/auth"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	userstore "github.com/kandev/kandev/internal/user/store"
)

func (a *lifecycleAdapter) WithProviderMCPReplacement(ctx context.Context, sessionID, executionID string, fn func(runtime.ProviderMCPControl) error) error {
	return a.mgr.WithProviderMCPReplacement(ctx, sessionID, executionID, fn)
}

type builtinApplicationAccounts interface {
	Mode() auth.Mode
	ResolveUserIdentity(context.Context, string) (authn.Identity, bool, error)
}

func builtinApplicationAuthorizer(accounts builtinApplicationAccounts, authorize mcpconfig.BuiltinPolicyAuthorizer) mcpconfig.BuiltinPolicyAuthorizer {
	return func(ctx context.Context, workspaceID string, requested authz.Scope) error {
		saved, ok := authn.IdentityFromContext(ctx)
		if !ok || saved.UserID == "" || accounts == nil || authorize == nil {
			return mcpconfig.ErrBuiltinHumanRequired
		}
		var current authn.Identity
		if accounts.Mode() == auth.ModeDisabled {
			if saved.UserID != userstore.DefaultUserID {
				return mcpconfig.ErrBuiltinHumanRequired
			}
			current = authn.Identity{UserID: saved.UserID, Role: authn.RoleAdmin, Synthetic: true}
		} else {
			resolved, active, err := accounts.ResolveUserIdentity(ctx, saved.UserID)
			if err != nil {
				return err
			}
			if !active || resolved.UserID != saved.UserID {
				return mcpconfig.ErrBuiltinHumanRequired
			}
			current = resolved
		}
		return authorize(authn.WithIdentity(ctx, current), workspaceID, requested)
	}
}
