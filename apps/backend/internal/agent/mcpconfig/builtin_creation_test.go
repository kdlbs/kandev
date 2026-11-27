package mcpconfig

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/mcp/scope"
	"github.com/stretchr/testify/require"
)

func TestBuiltinCreationRequiresHumanConsentAndCurrentPolicy(t *testing.T) {
	repo, service := builtinPolicyTestService()
	repo.snapshot.Policy = DefaultBuiltinWorkspacePolicy("workspace")
	validator, ok := any(service).(interface {
		ValidateTaskCreation(context.Context, string, *BuiltinCreationPreferences) error
	})
	require.True(t, ok, "creation must validate consent before persisting a task")
	revision := int64(0)
	input := &BuiltinCreationPreferences{Version: 1, ExpectedPolicyRevision: &revision, Packages: BuiltinPackageChoices{p.PackageConfiguration: "on"}}
	human := authn.WithIdentity(t.Context(), authn.Identity{UserID: "human"})
	require.NoError(t, validator.ValidateTaskCreation(human, "workspace", input))
	require.ErrorIs(t, validator.ValidateTaskCreation(t.Context(), "workspace", input), ErrBuiltinHumanRequired)
	agent := scope.WithPrincipal(human, scope.Principal{CallerTaskID: "task", CallerSessionID: "session", Surface: p.SurfaceConfiguration})
	require.ErrorIs(t, validator.ValidateTaskCreation(agent, "workspace", input), ErrBuiltinHumanRequired)
	repo.snapshot.Policy.Revision = 1
	require.ErrorIs(t, validator.ValidateTaskCreation(human, "workspace", input), ErrBuiltinPolicyConflict)
	revision = 1
	repo.snapshot.Policy.Packages[p.PackageConfiguration] = BuiltinPolicyBlocked
	require.ErrorIs(t, validator.ValidateTaskCreation(human, "workspace", input), ErrBuiltinPackageBlocked)
	input.ExpectedPolicyRevision = nil
	require.ErrorIs(t, validator.ValidateTaskCreation(human, "workspace", input), ErrBuiltinPolicyInvalid)
}

func TestBuiltinCreationAcceptsTrustedAutomationTaskCreation(t *testing.T) {
	repo, service := builtinPolicyTestService()
	repo.snapshot.Policy = DefaultBuiltinWorkspacePolicy("workspace")
	validator := any(service).(interface {
		ValidateTaskCreation(context.Context, string, *BuiltinCreationPreferences) error
	})
	revision := int64(0)
	input := &BuiltinCreationPreferences{
		Version: 1, ExpectedPolicyRevision: &revision,
		Packages: BuiltinPackageChoices{p.PackageConfiguration: "on"},
	}

	require.ErrorIs(t, validator.ValidateTaskCreation(t.Context(), "workspace", input), ErrBuiltinHumanRequired)
	ctx := WithAutomationTaskCreation(t.Context(), "run-1")
	require.NoError(t, validator.ValidateTaskCreation(ctx, "workspace", input))
	snapshot, err := service.UpdateTask(ctx, "task", 0, 0, input.Packages)
	require.NoError(t, err)
	require.Equal(t, "automation-run:run-1", snapshot.Preferences.UpdatedBy)
	agent := scope.WithPrincipal(ctx, scope.Principal{CallerTaskID: "task", CallerSessionID: "session", Surface: p.SurfaceConfiguration})
	require.ErrorIs(t, validator.ValidateTaskCreation(agent, "workspace", input), ErrBuiltinHumanRequired)
}
