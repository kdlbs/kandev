package mcpconfig

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/mcp/origin"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/mcp/scope"
	"github.com/stretchr/testify/require"
)

type builtinPolicyMemoryStore struct {
	snapshot BuiltinPolicySnapshot
}

func (r *builtinPolicyMemoryStore) GetBuiltinPolicySnapshot(context.Context, string, string) (*BuiltinPolicySnapshot, error) {
	return &r.snapshot, nil
}

func (r *builtinPolicyMemoryStore) PutBuiltinWorkspacePolicy(_ context.Context, policy *BuiltinWorkspacePolicy, _ int64) (*BuiltinWorkspacePolicy, error) {
	r.snapshot.Policy = policy
	return policy, nil
}

func (r *builtinPolicyMemoryStore) PutBuiltinTaskPreferences(_ context.Context, preferences *BuiltinTaskPreferences, _, _ int64, _ p.Surface) (*BuiltinPolicySnapshot, error) {
	r.snapshot.Preferences = preferences
	return &r.snapshot, nil
}

// @covers AC-AGENTS-BUILTIN-MCP-003.1
func TestBuiltinPolicyWritersRejectAgentAuthority(t *testing.T) {
	human := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner", Role: authn.RoleAdmin, Synthetic: true})
	contexts := map[string]context.Context{
		"no identity":         context.Background(),
		"external MCP":        origin.WithTrustedExternalTransport(human),
		"configuration agent": scope.WithPrincipal(human, scope.Principal{CallerTaskID: "task", CallerSessionID: "session", Surface: p.SurfaceConfiguration}),
		"task execution":      streams.WithMCPExecutionContext(human, streams.MCPExecutionContext{ExecutionID: "execution", TaskID: "task", SessionID: "session"}),
	}
	for name, ctx := range contexts {
		t.Run(name, func(t *testing.T) {
			repo, service := builtinPolicyTestService()
			policy, err := service.UpdateWorkspace(human, "workspace", 0, nil)
			require.NoError(t, err, "same service permits the human request")
			require.Equal(t, "owner", policy.UpdatedBy)
			repo.snapshot = BuiltinPolicySnapshot{}
			_, err = service.UpdateWorkspace(ctx, "workspace", 0, nil)
			require.ErrorIs(t, err, ErrBuiltinHumanRequired)
			_, err = service.UpdateTask(ctx, "task", 0, 0, BuiltinPackageChoices{p.PackageConfiguration: "on"})
			require.ErrorIs(t, err, ErrBuiltinHumanRequired)
			require.Nil(t, repo.snapshot.Policy)
			require.Nil(t, repo.snapshot.Preferences)
		})
	}
}

func TestBuiltinTaskChoiceUsesTaskWorkspaceAndHumanActor(t *testing.T) {
	_, service := builtinPolicyTestService()
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "owner"})
	snapshot, err := service.UpdateTask(ctx, "task", 0, 0, BuiltinPackageChoices{p.PackageConfiguration: "on", p.PackageDiagnostics: "inherit"})
	require.NoError(t, err)
	require.Equal(t, "workspace", snapshot.Preferences.WorkspaceID)
	require.Equal(t, "task", snapshot.Preferences.TaskID)
	require.Equal(t, "owner", snapshot.Preferences.UpdatedBy)
	require.Equal(t, BuiltinPackageChoices{p.PackageConfiguration: "on"}, snapshot.Preferences.Packages)
	_, err = service.UpdateWorkspace(ctx, "foreign", 0, nil)
	require.ErrorIs(t, err, ErrMCPWorkspaceAccess)
}

func builtinPolicyTestService() (*builtinPolicyMemoryStore, *BuiltinPolicyService) {
	repo := &builtinPolicyMemoryStore{}
	service := NewBuiltinPolicyService(repo, func(_ context.Context, workspaceID string, _ authz.Scope) error {
		if workspaceID != "workspace" {
			return ErrMCPWorkspaceAccess
		}
		return nil
	}, func(_ context.Context, taskID string) (string, p.Surface, error) {
		return "workspace", p.SurfaceKanbanTask, nil
	})
	return repo, service
}

func TestBuiltinPolicyWritersRequireTheirResourceScope(t *testing.T) {
	repo, service := builtinPolicyTestService()
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "collaborator"})
	service.authorize = func(_ context.Context, _ string, requested authz.Scope) error {
		if authz.WorkspaceScopes(authz.WorkspaceRoleCollaborator).Has(requested) {
			return nil
		}
		return ErrMCPWorkspaceAccess
	}
	_, err := service.UpdateTask(ctx, "task", 0, 0, BuiltinPackageChoices{p.PackageConfiguration: "on"})
	require.NoError(t, err)
	_, err = service.UpdateWorkspace(ctx, "workspace", 0, nil)
	require.ErrorIs(t, err, ErrMCPWorkspaceAccess)
	require.Nil(t, repo.snapshot.Policy)
	service.authorize = func(_ context.Context, _ string, requested authz.Scope) error {
		if authz.WorkspaceScopes(authz.WorkspaceRoleViewer).Has(requested) {
			return nil
		}
		return ErrMCPWorkspaceAccess
	}
	repo.snapshot = BuiltinPolicySnapshot{}
	_, err = service.UpdateTask(ctx, "task", 0, 0, BuiltinPackageChoices{p.PackageConfiguration: "on"})
	require.ErrorIs(t, err, ErrMCPWorkspaceAccess)
	require.Nil(t, repo.snapshot.Preferences)
}
