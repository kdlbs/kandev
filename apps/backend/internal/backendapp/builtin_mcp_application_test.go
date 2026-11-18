package backendapp

import (
	"context"
	"github.com/kandev/kandev/internal/auth"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/stretchr/testify/require"
	"testing"
)

type builtinApplicationAccount struct{ active bool }

func (*builtinApplicationAccount) Mode() auth.Mode { return auth.ModeEnabled }
func (a *builtinApplicationAccount) ResolveUserIdentity(context.Context, string) (authn.Identity, bool, error) {
	return authn.Identity{UserID: "human", Role: authn.RoleGuest, OrgID: "tenant"}, a.active, nil
}

func TestBuiltinApplicationUsesCurrentAccountAuthority(t *testing.T) {
	account := &builtinApplicationAccount{active: true}
	calls := 0
	check := builtinApplicationAuthorizer(account, func(ctx context.Context, workspace string, requested authz.Scope) error {
		identity, ok := authn.IdentityFromContext(ctx)
		require.True(t, ok)
		require.Equal(t, authn.RoleGuest, identity.Role)
		require.Equal(t, "tenant", identity.OrgID)
		require.Equal(t, "workspace", workspace)
		require.Equal(t, authz.ScopeTaskWrite, requested)
		calls++
		return nil
	})
	stale := authn.WithIdentity(context.Background(), authn.Identity{UserID: "human", Role: authn.RoleAdmin})
	require.NoError(t, check(stale, "workspace", authz.ScopeTaskWrite))
	account.active = false
	require.Error(t, check(stale, "workspace", authz.ScopeTaskWrite))
	require.Equal(t, 1, calls, "a saved actor does not retain revoked account authority")
}
