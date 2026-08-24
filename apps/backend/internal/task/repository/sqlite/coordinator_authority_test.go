package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-COORDINATOR-AUTHORITY-001
func TestCoordinatorAuthoritySchemaCreatesRevocableGrantAndAuditTables(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	if err := repo.CreateWorkspace(context.Background(), &models.Workspace{ID: "ws-1", Name: "Coordinator authority"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	createUsageEventsTestTask(t, repo, "coordinator")
	now := time.Now().UTC()

	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_coordinator_grants (
			id, coordinator_task_id, workspace_id, scope_kind, scope_id,
			capabilities, note, granted_by_user_id, granted_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), "grant-1", "coordinator", "ws-1", "workspace", "ws-1", "inspect,orchestrate", "", "operator", now); err != nil {
		t.Fatalf("insert coordinator grant: %v", err)
	}

	if _, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO task_coordinator_audit_events (
			id, occurred_at, actor_task_id, actor_session_id, target_task_id,
			workspace_id, action, capability, decision, deny_reason, grant_id, result, detail
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), "audit-1", now, "coordinator", "session-1", "target", "ws-1", "stop", "orchestrate", "allowed", "", "grant-1", "pending", "grant"); err != nil {
		t.Fatalf("insert coordinator audit event: %v", err)
	}
}

// @covers AC-COORDINATOR-AUTHORITY-002
func TestCoordinatorGrantRepositoryRevokesGrantAndResolvesAudit(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Coordinator authority"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	createUsageEventsTestTask(t, repo, "coordinator")
	now := time.Now().UTC()
	grant := &models.CoordinatorGrant{
		ID: "grant-1", CoordinatorTaskID: "coordinator", WorkspaceID: "ws-1",
		ScopeKind: "workspace", ScopeID: "ws-1", Capabilities: "inspect,orchestrate",
		GrantedByUserID: "operator", GrantedAt: now,
	}
	if err := repo.CreateCoordinatorGrant(ctx, grant); err != nil {
		t.Fatalf("CreateCoordinatorGrant: %v", err)
	}
	active, err := repo.ListActiveCoordinatorGrants(ctx, "coordinator", "ws-1")
	if err != nil {
		t.Fatalf("ListActiveCoordinatorGrants: %v", err)
	}
	if len(active) != 1 || active[0].ID != grant.ID {
		t.Fatalf("active grants = %#v, want grant-1", active)
	}
	if err := repo.RevokeCoordinatorGrant(ctx, grant.ID, "operator-2", now.Add(time.Minute)); err != nil {
		t.Fatalf("RevokeCoordinatorGrant: %v", err)
	}
	active, err = repo.ListActiveCoordinatorGrants(ctx, "coordinator", "ws-1")
	if err != nil {
		t.Fatalf("ListActiveCoordinatorGrants after revoke: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("active grants after revoke = %#v, want none", active)
	}

	event := &models.CoordinatorAuditEvent{
		ID: "audit-1", OccurredAt: now, ActorTaskID: "coordinator", TargetTaskID: "target",
		WorkspaceID: "ws-1", Action: "stop", Capability: "orchestrate", Decision: "allowed",
		GrantID: grant.ID, Result: "pending", Detail: "grant",
	}
	if err := repo.CreateCoordinatorAuditEvent(ctx, event); err != nil {
		t.Fatalf("CreateCoordinatorAuditEvent: %v", err)
	}
	if err := repo.FinishCoordinatorAuditEvent(ctx, event.ID, "ok", ""); err != nil {
		t.Fatalf("FinishCoordinatorAuditEvent: %v", err)
	}
	events, err := repo.ListCoordinatorAuditEvents(ctx, "ws-1", "coordinator", 10)
	if err != nil {
		t.Fatalf("ListCoordinatorAuditEvents: %v", err)
	}
	if len(events) != 1 || events[0].Result != "ok" {
		t.Fatalf("audit events = %#v, want resolved event", events)
	}
}

func TestWorkspaceAgentPrincipalRepositoryRebindsAndRevokesImmediately(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Coordinator authority"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	createUsageEventsTestTask(t, repo, "coordinator-a")
	createUsageEventsTestTask(t, repo, "coordinator-b")
	now := time.Now().UTC()
	principal := &models.WorkspaceAgentPrincipal{
		ID: "principal-1", WorkspaceID: "ws-1", PluginInstallationID: "plugin-1", LogicalKey: "coordinator",
		BackingTaskID: "coordinator-a", BackingSessionID: "session-a", CreatedAt: now,
	}
	if err := repo.CreateWorkspaceAgentPrincipal(ctx, principal); err != nil {
		t.Fatalf("CreateWorkspaceAgentPrincipal: %v", err)
	}
	if err := repo.CreateWorkspaceAgentPrincipal(ctx, &models.WorkspaceAgentPrincipal{
		ID: "principal-duplicate", WorkspaceID: "ws-1", PluginInstallationID: "plugin-1", LogicalKey: "coordinator",
		BackingTaskID: "coordinator-b", CreatedAt: now,
	}); err == nil {
		t.Fatal("CreateWorkspaceAgentPrincipal duplicate context succeeded")
	}
	byContext, err := repo.GetWorkspaceAgentPrincipalByContext(ctx, "ws-1", "plugin-1", "coordinator")
	if err != nil || byContext.ID != principal.ID {
		t.Fatalf("GetWorkspaceAgentPrincipalByContext = %#v, %v; want principal-1", byContext, err)
	}
	if err := repo.RebindWorkspaceAgentPrincipal(ctx, principal.ID, "coordinator-b", "session-b", now.Add(time.Minute)); err != nil {
		t.Fatalf("RebindWorkspaceAgentPrincipal: %v", err)
	}
	if active, err := repo.GetActiveWorkspaceAgentPrincipalForTask(ctx, "ws-1", "coordinator-a"); err != nil || active != nil {
		t.Fatalf("old task active principal = %#v, %v; want nil", active, err)
	}
	if active, err := repo.GetActiveWorkspaceAgentPrincipalForTask(ctx, "ws-1", "coordinator-b"); err != nil || active == nil || active.BackingSessionID != "session-b" {
		t.Fatalf("new task active principal = %#v, %v; want rebound principal", active, err)
	}
	if err := repo.RevokeWorkspaceAgentPrincipal(ctx, principal.ID, now.Add(2*time.Minute)); err != nil {
		t.Fatalf("RevokeWorkspaceAgentPrincipal: %v", err)
	}
	if active, err := repo.GetActiveWorkspaceAgentPrincipalForTask(ctx, "ws-1", "coordinator-b"); err != nil || active != nil {
		t.Fatalf("revoked active principal = %#v, %v; want nil", active, err)
	}
	if err := repo.RebindWorkspaceAgentPrincipal(ctx, principal.ID, "coordinator-a", "session-c", now.Add(3*time.Minute)); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("RebindWorkspaceAgentPrincipal revoked error = %v, want sql.ErrNoRows", err)
	}
}

func TestListActiveWorkspaceAgentPrincipalGrantsExcludesLegacyTaskBoundRows(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Coordinator authority"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	createUsageEventsTestTask(t, repo, "coordinator")
	now := time.Now().UTC()
	principal := &models.WorkspaceAgentPrincipal{
		ID: "principal-1", WorkspaceID: "ws-1", PluginInstallationID: "plugin-1", LogicalKey: "coordinator",
		BackingTaskID: "coordinator", CreatedAt: now,
	}
	if err := repo.CreateWorkspaceAgentPrincipal(ctx, principal); err != nil {
		t.Fatalf("CreateWorkspaceAgentPrincipal: %v", err)
	}
	if err := repo.CreateCoordinatorGrant(ctx, &models.CoordinatorGrant{
		ID: "principal-grant", CoordinatorTaskID: "coordinator", PrincipalID: principal.ID, WorkspaceID: "ws-1",
		ScopeKind: "workspace", ScopeID: "ws-1", Capabilities: "orchestrate", GrantedAt: now,
	}); err != nil {
		t.Fatalf("CreateCoordinatorGrant: %v", err)
	}
	if _, err := repo.db.Exec(repo.db.Rebind(`INSERT INTO task_coordinator_grants (id, coordinator_task_id, workspace_id, scope_kind, scope_id, capabilities, granted_at) VALUES (?, ?, ?, ?, ?, ?, ?)`), "legacy-grant", "coordinator", "ws-1", "workflow", "workflow-1", "orchestrate", now); err != nil {
		t.Fatalf("insert legacy grant: %v", err)
	}
	grants, err := repo.ListActiveWorkspaceAgentPrincipalGrants(ctx, principal.ID, "ws-1")
	if err != nil {
		t.Fatalf("ListActiveWorkspaceAgentPrincipalGrants: %v", err)
	}
	if len(grants) != 1 || grants[0].ID != "principal-grant" || grants[0].PrincipalID != principal.ID {
		t.Fatalf("principal grants = %#v, want only principal-grant", grants)
	}
}
