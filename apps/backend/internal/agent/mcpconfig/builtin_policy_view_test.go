package mcpconfig

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/stretchr/testify/require"
)

func TestBuiltinManagementViewSeparatesReadAccessFromChanges(t *testing.T) {
	repo, service := builtinPolicyTestService()
	repo.snapshot = BuiltinPolicySnapshot{Policy: DefaultBuiltinWorkspacePolicy("workspace"), Preferences: &BuiltinTaskPreferences{
		Version: 1, WorkspaceID: "workspace", TaskID: "task", Packages: BuiltinPackageChoices{p.PackageConfiguration: BuiltinPreferenceOn}}}
	human := authn.WithIdentity(context.Background(), authn.Identity{UserID: "viewer"})
	service.authorize = func(_ context.Context, workspaceID string, requested authz.Scope) error {
		if workspaceID == "workspace" && requested == authz.ScopeWorkspaceRead {
			return nil
		}
		return ErrMCPWorkspaceAccess
	}
	view, err := service.ReadTask(human, "task")
	require.NoError(t, err)
	require.NotNil(t, view)
	require.False(t, view.CanEdit)
	require.Contains(t, view.Desired.Packages, p.PackageConfiguration)
	require.Len(t, view.Catalog, 5)
	workspace, err := service.ReadWorkspace(human, "workspace")
	require.NoError(t, err)
	require.False(t, workspace.CanEdit)
	require.Nil(t, workspace.Preferences)
	require.Nil(t, workspace.Desired)
	_, err = service.ReadWorkspace(human, "foreign")
	require.ErrorIs(t, err, ErrMCPWorkspaceAccess)
}

func TestBuiltinManagementViewSeparatesTaskChoiceFromWorkspaceManagement(t *testing.T) {
	repo, service := builtinPolicyTestService()
	repo.snapshot.Policy = DefaultBuiltinWorkspacePolicy("workspace")
	service.authorize = func(_ context.Context, _ string, requested authz.Scope) error {
		if requested == authz.ScopeWorkspaceManage {
			return ErrMCPWorkspaceAccess
		}
		return nil
	}
	human := authn.WithIdentity(t.Context(), authn.Identity{UserID: "collaborator"})
	view, err := service.ReadWorkspace(human, "workspace")
	require.NoError(t, err)
	require.False(t, view.CanEdit)
	encoded, err := json.Marshal(view)
	require.NoError(t, err)
	require.Contains(t, string(encoded), `"can_choose_for_task":true`)
}
