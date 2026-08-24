package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

func TestWorkspaceAgentPrincipalRepository_UsesDurablePrincipalForGrantsAndAudit(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-principal")
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task-backing", WorkspaceID: "ws-principal", Title: "Backing execution"}))

	principal := &models.WorkspaceAgentPrincipal{
		ID: "principal-1", WorkspaceID: "ws-principal", PluginInstallationID: "plugin-owned", LogicalKey: "agent",
		BackingTaskID: "task-backing",
	}
	require.NoError(t, repo.CreateWorkspaceAgentPrincipal(ctx, principal))
	require.NoError(t, repo.CreateCoordinatorGrant(ctx, &models.CoordinatorGrant{
		ID: "grant-1", CoordinatorTaskID: "task-backing", PrincipalID: principal.ID, WorkspaceID: "ws-principal",
		ScopeKind: "workspace", ScopeID: "ws-principal", Capabilities: "orchestrate",
	}))

	grants, err := repo.ListActiveWorkspaceAgentPrincipalGrants(ctx, principal.ID, "ws-principal")
	require.NoError(t, err)
	require.Len(t, grants, 1)
	require.Equal(t, "grant-1", grants[0].ID)

	require.NoError(t, repo.CreateCoordinatorAuditEvent(ctx, &models.CoordinatorAuditEvent{
		ID: "audit-1", PrincipalID: principal.ID, WorkspaceID: "ws-principal", ActorTaskID: "task-backing",
		TargetTaskID: "task-backing", Action: "task.stop", Capability: "orchestrate", Decision: "allowed", Result: "ok", Detail: "grant",
	}))
	events, err := repo.ListWorkspaceAgentPrincipalAuditEvents(ctx, "ws-principal", principal.ID, 10)
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, principal.ID, events[0].PrincipalID)

	require.NoError(t, repo.RevokeWorkspaceAgentPrincipal(ctx, principal.ID, time.Now().UTC()))
	active, err := repo.GetActiveWorkspaceAgentPrincipalForTask(ctx, "ws-principal", "task-backing")
	require.NoError(t, err)
	require.Nil(t, active, "revocation must take effect immediately for actor resolution")
}

func TestWorkspaceAgentPrincipalRepository_LegacyTaskGrantCannotMatchPrincipal(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-legacy")
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task-legacy", WorkspaceID: "ws-legacy", Title: "Legacy task"}))
	principal := &models.WorkspaceAgentPrincipal{ID: "principal-new", WorkspaceID: "ws-legacy", PluginInstallationID: "plugin-owned", LogicalKey: "agent", BackingTaskID: "task-legacy"}
	require.NoError(t, repo.CreateWorkspaceAgentPrincipal(ctx, principal))
	// Empty PrincipalID is the non-transferable legacy task grant shape.
	require.NoError(t, repo.CreateCoordinatorGrant(ctx, &models.CoordinatorGrant{ID: "legacy", CoordinatorTaskID: "task-legacy", WorkspaceID: "ws-legacy", ScopeKind: "workspace", ScopeID: "ws-legacy", Capabilities: "orchestrate"}))

	grants, err := repo.ListActiveWorkspaceAgentPrincipalGrants(ctx, principal.ID, "ws-legacy")
	require.NoError(t, err)
	require.Empty(t, grants)
}
