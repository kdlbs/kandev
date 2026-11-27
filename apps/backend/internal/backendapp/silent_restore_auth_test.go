package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/config"
	usermodels "github.com/kandev/kandev/internal/user/models"
	"github.com/stretchr/testify/require"
)

func TestStartupRecoveryOwnerContextUsesCurrentWorkspaceOwnerIdentity(t *testing.T) {
	ctx := context.Background()
	authCfg := &config.Config{}
	authCfg.Features.Auth = true
	authSvc := newEnabledAuthService(t, authCfg)
	owner, err := authSvc.AdminCreateUser(ctx, "restore-owner@example.test", "ownerpass123", "Restore Owner", usermodels.RoleMember)
	require.NoError(t, err)

	ownerCtx, err := startupRecoveryOwnerContext(ctx, authSvc, owner.ID, owner.OrgID)
	require.NoError(t, err)
	identity, ok := authn.IdentityFromContext(ownerCtx)
	require.True(t, ok)
	require.Equal(t, owner.ID, identity.UserID)
	require.Equal(t, owner.OrgID, identity.OrgID)
	require.Equal(t, authn.RoleMember, identity.Role)
	require.False(t, identity.Synthetic)

	_, err = startupRecoveryOwnerContext(ctx, authSvc, owner.ID, "different-org")
	require.Error(t, err, "a workspace organization change must not borrow the owner's identity")

	_, err = authSvc.AdminSetRoleStatus(ctx, owner.ID, usermodels.RoleMember, usermodels.StatusDisabled)
	require.NoError(t, err)
	_, err = startupRecoveryOwnerContext(ctx, authSvc, owner.ID, owner.OrgID)
	require.Error(t, err, "a disabled workspace owner must not authorize startup recovery")
}

func TestStartupRecoveryOwnerContextKeepsAuthDisabledUnscoped(t *testing.T) {
	ctx := context.Background()
	ownerCtx, err := startupRecoveryOwnerContext(ctx, newDisabledAuthService(t), "stored-owner", "stored-org")
	require.NoError(t, err)
	require.Equal(t, ctx, ownerCtx)
	_, ok := authn.IdentityFromContext(ownerCtx)
	require.False(t, ok)
}
