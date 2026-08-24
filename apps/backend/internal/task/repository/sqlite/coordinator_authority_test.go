package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
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

func TestWorkspaceAgentPrincipalRepository_TypedErrorsRejectLegacyInstallAndPreservePrincipalGrantLookup(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-errors")
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task-errors", WorkspaceID: "ws-errors", Title: "Backing execution"}))

	require.ErrorIs(t, repo.CreateWorkspaceAgentPrincipal(ctx, &models.WorkspaceAgentPrincipal{
		ID: "legacy-empty-install", WorkspaceID: "ws-errors", PluginInstallationID: "", LogicalKey: "agent",
	}), repoerrors.ErrWorkspaceAgentPrincipalNotFound)

	principal := &models.WorkspaceAgentPrincipal{ID: "principal-errors", WorkspaceID: "ws-errors", PluginInstallationID: "plugin-errors", LogicalKey: "agent", BackingTaskID: "task-errors"}
	require.NoError(t, repo.CreateWorkspaceAgentPrincipal(ctx, principal))
	require.ErrorIs(t, repo.CreateWorkspaceAgentPrincipal(ctx, &models.WorkspaceAgentPrincipal{
		ID: "principal-conflict", WorkspaceID: "ws-errors", PluginInstallationID: "plugin-errors", LogicalKey: "agent",
	}), repoerrors.ErrWorkspaceAgentPrincipalConflict)

	grant := &models.CoordinatorGrant{ID: "grant-errors", CoordinatorTaskID: "task-errors", PrincipalID: principal.ID, WorkspaceID: "ws-errors", ScopeKind: "workspace", ScopeID: "ws-errors", Capabilities: "orchestrate"}
	require.NoError(t, repo.CreateCoordinatorGrant(ctx, grant))
	require.ErrorIs(t, repo.CreateCoordinatorGrant(ctx, &models.CoordinatorGrant{
		ID: "grant-conflict", CoordinatorTaskID: "task-errors", PrincipalID: principal.ID, WorkspaceID: "ws-errors", ScopeKind: "workspace", ScopeID: "ws-errors", Capabilities: "inspect",
	}), repoerrors.ErrCoordinatorGrantConflict)

	all, err := repo.ListWorkspaceAgentPrincipalGrants(ctx, "ws-errors", principal.ID, true)
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.Equal(t, grant.ID, all[0].ID)
	require.Empty(t, mustListPrincipalGrants(t, repo, ctx, "ws-errors", "", true), "legacy empty principal IDs never match durable lookup")

	require.NoError(t, repo.RevokeCoordinatorGrant(ctx, grant.ID, "operator", time.Now().UTC()))
	require.ErrorIs(t, repo.RevokeCoordinatorGrant(ctx, grant.ID, "operator", time.Now().UTC()), repoerrors.ErrCoordinatorGrantNotFound)
	require.ErrorIs(t, repo.RebindWorkspaceAgentPrincipal(ctx, "missing", "task-errors", "session-errors", time.Now().UTC()), repoerrors.ErrWorkspaceAgentPrincipalNotFound)
	require.NoError(t, repo.RevokeWorkspaceAgentPrincipal(ctx, principal.ID, time.Now().UTC()))
	require.ErrorIs(t, repo.RevokeWorkspaceAgentPrincipal(ctx, principal.ID, time.Now().UTC()), repoerrors.ErrWorkspaceAgentPrincipalNotFound)
	_, err = repo.GetWorkspaceAgentPrincipalByContext(ctx, "ws-errors", "", "agent")
	require.True(t, errors.Is(err, repoerrors.ErrWorkspaceAgentPrincipalNotFound))
}

func TestWorkspaceAgentPrincipalRepository_FiltersAuditByDurableIdentityAndReasonFields(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-audit-filter")
	now := time.Now().UTC()
	for _, event := range []*models.CoordinatorAuditEvent{
		{ID: "audit-allowed", PrincipalID: "principal-a", WorkspaceID: "ws-audit-filter", ActorTaskID: "actor-a", TargetTaskID: "target-a", Action: "task.stop", Capability: "orchestrate", Decision: "allowed", Result: "ok", Detail: "grant", OccurredAt: now},
		{ID: "audit-denied", PrincipalID: "principal-a", WorkspaceID: "ws-audit-filter", ActorTaskID: "actor-a", TargetTaskID: "target-b", Action: "task.stop", Capability: "orchestrate", Decision: "denied", Result: "ok", Detail: "scope_or_capability", OccurredAt: now.Add(time.Second)},
		{ID: "audit-foreign-principal", PrincipalID: "principal-b", WorkspaceID: "ws-audit-filter", ActorTaskID: "actor-b", TargetTaskID: "target-a", Action: "task.stop", Capability: "orchestrate", Decision: "allowed", Result: "error", Detail: "operation_error", OccurredAt: now.Add(2 * time.Second)},
	} {
		require.NoError(t, repo.CreateCoordinatorAuditEvent(ctx, event))
	}

	filtered, err := repo.ListWorkspaceAgentPrincipalAuditEventsFiltered(ctx, "ws-audit-filter", models.WorkspaceAgentAuditFilter{PrincipalID: "principal-a", Decision: "denied", Result: "ok"}, 10)
	require.NoError(t, err)
	require.Len(t, filtered, 1)
	require.Equal(t, "audit-denied", filtered[0].ID)

	legacyProjection, err := repo.ListWorkspaceAgentPrincipalAuditEvents(ctx, "ws-audit-filter", "principal-a", 10)
	require.NoError(t, err)
	require.Len(t, legacyProjection, 2, "the existing principal audit query remains source-compatible")
}

func mustListPrincipalGrants(t *testing.T, repo *Repository, ctx context.Context, workspaceID, principalID string, includeRevoked bool) []*models.CoordinatorGrant {
	t.Helper()
	grants, err := repo.ListWorkspaceAgentPrincipalGrants(ctx, workspaceID, principalID, includeRevoked)
	require.NoError(t, err)
	return grants
}
