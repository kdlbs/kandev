package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/testutil"
)

func TestPostgresCoordinatorAuthorityPrincipalGrantAndAuditParity(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-coordinator", Name: "Coordinator authority"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	now := time.Now().UTC()
	principal := &models.WorkspaceAgentPrincipal{
		ID: "principal-1", WorkspaceID: "ws-coordinator", PluginInstallationID: "plugin-1", LogicalKey: "coordinator",
		BackingTaskID: "replaceable-task", BackingSessionID: "replaceable-session", CreatedAt: now,
	}
	if err := repo.CreateWorkspaceAgentPrincipal(ctx, principal); err != nil {
		t.Fatalf("CreateWorkspaceAgentPrincipal: %v", err)
	}
	grant := &models.CoordinatorGrant{
		ID: "grant-1", PrincipalID: principal.ID, WorkspaceID: principal.WorkspaceID,
		ScopeKind: "workspace", ScopeID: principal.WorkspaceID, Capabilities: "inspect,orchestrate", GrantedAt: now,
	}
	if err := repo.CreateCoordinatorGrant(ctx, grant); err != nil {
		t.Fatalf("CreateCoordinatorGrant: %v", err)
	}
	event := &models.CoordinatorAuditEvent{
		ID: "audit-1", PrincipalID: principal.ID, ActorTaskID: principal.BackingTaskID,
		ActorSessionID: principal.BackingSessionID, TargetTaskID: "target", WorkspaceID: principal.WorkspaceID,
		Action: "stop", Capability: "orchestrate", Decision: "allowed", GrantID: grant.ID, Result: "pending",
	}
	if err := repo.CreateCoordinatorAuditEvent(ctx, event); err != nil {
		t.Fatalf("CreateCoordinatorAuditEvent: %v", err)
	}
	if err := repo.FinishCoordinatorAuditEvent(ctx, event.ID, "ok", ""); err != nil {
		t.Fatalf("FinishCoordinatorAuditEvent: %v", err)
	}
	events, err := repo.ListCoordinatorAuditEvents(ctx, principal.WorkspaceID, principal.BackingTaskID, 10)
	if err != nil {
		t.Fatalf("ListCoordinatorAuditEvents: %v", err)
	}
	if len(events) != 1 || events[0].PrincipalID != principal.ID || events[0].Result != "ok" {
		t.Fatalf("events = %#v, want resolved durable principal audit", events)
	}
}

func TestPostgresCoordinatorGrantMigrationSeparatesPrincipalAndLegacyTaskScopeIndexes(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("init postgres schema: %v", err)
	}
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Coordinator authority"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	now := time.Now().UTC()
	for _, principal := range []*models.WorkspaceAgentPrincipal{
		{ID: "principal-1", WorkspaceID: "ws-1", PluginInstallationID: "plugin-1", LogicalKey: "coordinator-1", CreatedAt: now},
		{ID: "principal-2", WorkspaceID: "ws-1", PluginInstallationID: "plugin-2", LogicalKey: "coordinator-2", CreatedAt: now},
	} {
		if err := repo.CreateWorkspaceAgentPrincipal(ctx, principal); err != nil {
			t.Fatalf("CreateWorkspaceAgentPrincipal %s: %v", principal.ID, err)
		}
	}
	if err := repo.CreateCoordinatorGrant(ctx, &models.CoordinatorGrant{
		ID: "grant-1", CoordinatorTaskID: "coordinator", PrincipalID: "principal-1", WorkspaceID: "ws-1",
		ScopeKind: "workspace", ScopeID: "ws-1", Capabilities: "orchestrate", GrantedAt: now,
	}); err != nil {
		t.Fatalf("CreateCoordinatorGrant first principal: %v", err)
	}
	if _, err := repo.db.Exec(`
		DROP INDEX uniq_active_task_coordinator_grants_scope;
		CREATE UNIQUE INDEX uniq_active_task_coordinator_grants_scope
			ON task_coordinator_grants(coordinator_task_id, scope_kind, scope_id)
			WHERE revoked_at IS NULL;
	`); err != nil {
		t.Fatalf("restore legacy task-scope index: %v", err)
	}
	if err := repo.runMigrations(ctx); err != nil {
		t.Fatalf("runMigrations: %v", err)
	}
	if err := repo.CreateCoordinatorGrant(ctx, &models.CoordinatorGrant{
		ID: "grant-2", CoordinatorTaskID: "coordinator", PrincipalID: "principal-2", WorkspaceID: "ws-1",
		ScopeKind: "workspace", ScopeID: "ws-1", Capabilities: "inspect", GrantedAt: now,
	}); err != nil {
		t.Fatalf("CreateCoordinatorGrant second principal: %v", err)
	}
	if err := repo.CreateCoordinatorGrant(ctx, &models.CoordinatorGrant{
		ID: "grant-3", CoordinatorTaskID: "coordinator", PrincipalID: "principal-1", WorkspaceID: "ws-1",
		ScopeKind: "workspace", ScopeID: "ws-1", Capabilities: "inspect", GrantedAt: now,
	}); !errors.Is(err, repoerrors.ErrCoordinatorGrantConflict) {
		t.Fatalf("duplicate principal scope error = %v, want ErrCoordinatorGrantConflict", err)
	}
}
