package service

import (
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestExecutorActorContextReusesMatchingActiveCaller(t *testing.T) {
	svc, _, _ := createTestService(t)
	caller := authn.WithIdentity(t.Context(), authn.Identity{UserID: "creator", Role: authn.RoleMember})

	prepared, err := svc.ExecutorActorContext(caller, "creator")
	require.NoError(t, err)
	identity, ok := authn.IdentityFromContext(prepared)
	require.True(t, ok)
	require.Equal(t, "creator", identity.UserID)

	_, err = svc.ExecutorActorContext(caller, "another-creator")
	require.Error(t, err, "an authenticated editor must not stand in for a different saved creator")
}

func TestPrepareAutomationCredentialActorAllowsSyntheticAuthOff(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "single-user", Role: authn.RoleAdmin, Synthetic: true})

	executor := &models.Executor{ID: "auth-off-executor", Name: "Shared", Type: models.ExecutorTypeLocal}
	require.NoError(t, repo.CreateExecutor(t.Context(), executor))
	require.NoError(t, repo.CreateExecutorProfile(t.Context(), &models.ExecutorProfile{ID: "auth-off-profile", ExecutorID: executor.ID, Name: "Shared"}))

	prepared, err := svc.PrepareAutomationCredentialActorContext(ctx, "", "", "auth-off-profile")
	require.NoError(t, err)
	identity, ok := authn.IdentityFromContext(prepared)
	require.True(t, ok)
	require.True(t, identity.Synthetic)
}
