package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
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
	if err := repo.RebindWorkspaceAgentPrincipal(ctx, principal.ID, "coordinator-a", "session-c", now.Add(3*time.Minute)); !errors.Is(err, repoerrors.ErrWorkspaceAgentPrincipalNotFound) {
		t.Fatalf("RebindWorkspaceAgentPrincipal revoked error = %v, want ErrWorkspaceAgentPrincipalNotFound", err)
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

// The vectors below are the test contract the host plugin-integration adapter
// (task 9e67c426) must be able to rely on: typed sentinel errors for
// not-found and conflict races, revocation that takes effect on the next
// read, and an audit projection that carries reason codes only.

// @covers AC-COORDINATOR-AUTHORITY-004
func TestWorkspaceAgentPrincipalTypedErrorsForAdapterSurface(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Coordinator authority"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}

	if _, err := repo.GetWorkspaceAgentPrincipal(ctx, "missing"); !errors.Is(err, repoerrors.ErrWorkspaceAgentPrincipalNotFound) {
		t.Fatalf("GetWorkspaceAgentPrincipal miss error = %v, want ErrWorkspaceAgentPrincipalNotFound", err)
	}
	if _, err := repo.GetWorkspaceAgentPrincipalByContext(ctx, "ws-1", "plugin-1", "missing"); !errors.Is(err, repoerrors.ErrWorkspaceAgentPrincipalNotFound) {
		t.Fatalf("GetWorkspaceAgentPrincipalByContext miss error = %v, want ErrWorkspaceAgentPrincipalNotFound", err)
	}

	now := time.Now().UTC()
	principal := &models.WorkspaceAgentPrincipal{
		ID: "principal-1", WorkspaceID: "ws-1", PluginInstallationID: "plugin-1", LogicalKey: "coordinator",
		BackingTaskID: "coordinator-a", CreatedAt: now,
	}
	if err := repo.CreateWorkspaceAgentPrincipal(ctx, principal); err != nil {
		t.Fatalf("CreateWorkspaceAgentPrincipal: %v", err)
	}
	if err := repo.CreateWorkspaceAgentPrincipal(ctx, &models.WorkspaceAgentPrincipal{
		ID: "principal-2", WorkspaceID: "ws-1", PluginInstallationID: "plugin-1", LogicalKey: "coordinator",
		BackingTaskID: "coordinator-b", CreatedAt: now,
	}); !errors.Is(err, repoerrors.ErrWorkspaceAgentPrincipalConflict) {
		t.Fatalf("duplicate context error = %v, want ErrWorkspaceAgentPrincipalConflict", err)
	}

	if err := repo.RevokeWorkspaceAgentPrincipal(ctx, principal.ID, now.Add(time.Minute)); err != nil {
		t.Fatalf("RevokeWorkspaceAgentPrincipal: %v", err)
	}
	if err := repo.RevokeWorkspaceAgentPrincipal(ctx, principal.ID, now.Add(2*time.Minute)); !errors.Is(err, repoerrors.ErrWorkspaceAgentPrincipalNotFound) {
		t.Fatalf("second revoke error = %v, want ErrWorkspaceAgentPrincipalNotFound", err)
	}
	revoked, err := repo.GetWorkspaceAgentPrincipal(ctx, principal.ID)
	if err != nil || revoked.RevokedAt == nil {
		t.Fatalf("GetWorkspaceAgentPrincipal after revoke = %#v, %v; want revoked row readable for status projection", revoked, err)
	}
	if active, err := repo.GetActiveWorkspaceAgentPrincipalForTask(ctx, "ws-1", "coordinator-a"); err != nil || active != nil {
		t.Fatalf("revoked principal active lookup = %#v, %v; want nil,nil", active, err)
	}
}

// @covers AC-COORDINATOR-AUTHORITY-005
func TestCoordinatorGrantTypedErrorsForAdapterSurface(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Coordinator authority"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	createUsageEventsTestTask(t, repo, "coordinator")
	now := time.Now().UTC()

	if _, err := repo.GetCoordinatorGrant(ctx, "missing"); !errors.Is(err, repoerrors.ErrCoordinatorGrantNotFound) {
		t.Fatalf("GetCoordinatorGrant miss error = %v, want ErrCoordinatorGrantNotFound", err)
	}

	principal := &models.WorkspaceAgentPrincipal{
		ID: "principal-1", WorkspaceID: "ws-1", PluginInstallationID: "plugin-1", LogicalKey: "coordinator",
		BackingTaskID: "coordinator", CreatedAt: now,
	}
	if err := repo.CreateWorkspaceAgentPrincipal(ctx, principal); err != nil {
		t.Fatalf("CreateWorkspaceAgentPrincipal: %v", err)
	}
	grant := &models.CoordinatorGrant{
		ID: "grant-1", CoordinatorTaskID: "coordinator", PrincipalID: principal.ID, WorkspaceID: "ws-1",
		ScopeKind: "workspace", ScopeID: "ws-1", Capabilities: "orchestrate", GrantedAt: now,
	}
	if err := repo.CreateCoordinatorGrant(ctx, grant); err != nil {
		t.Fatalf("CreateCoordinatorGrant: %v", err)
	}
	if err := repo.CreateCoordinatorGrant(ctx, &models.CoordinatorGrant{
		ID: "grant-2", CoordinatorTaskID: "coordinator", PrincipalID: principal.ID, WorkspaceID: "ws-1",
		ScopeKind: "workspace", ScopeID: "ws-1", Capabilities: "inspect", GrantedAt: now,
	}); !errors.Is(err, repoerrors.ErrCoordinatorGrantConflict) {
		t.Fatalf("duplicate principal scope grant error = %v, want ErrCoordinatorGrantConflict", err)
	}

	if err := repo.RevokeCoordinatorGrant(ctx, grant.ID, "operator", now.Add(time.Minute)); err != nil {
		t.Fatalf("RevokeCoordinatorGrant: %v", err)
	}
	if err := repo.RevokeCoordinatorGrant(ctx, grant.ID, "operator", now.Add(2*time.Minute)); !errors.Is(err, repoerrors.ErrCoordinatorGrantNotFound) {
		t.Fatalf("second revoke error = %v, want ErrCoordinatorGrantNotFound", err)
	}
	revoked, err := repo.GetCoordinatorGrant(ctx, grant.ID)
	if err != nil || revoked.RevokedAt == nil || revoked.RevokedByUserID != "operator" {
		t.Fatalf("revoked grant lookup = %#v, %v; want readable revoked row with revoker", revoked, err)
	}
}

// @covers AC-COORDINATOR-AUTHORITY-006
func TestCoordinatorAuditProjectionCarriesReasonCodesOnly(t *testing.T) {
	repo := newUsageEventsTestRepo(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "Coordinator authority"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	now := time.Now().UTC()

	denied := &models.CoordinatorAuditEvent{
		ID: "audit-denied", OccurredAt: now, ActorTaskID: "actor", TargetTaskID: "target",
		WorkspaceID: "ws-1", Action: "stop_task", Capability: "orchestrate", Decision: "denied",
		DenyReason: "scope_or_capability", Result: "ok", Detail: "scope_or_capability",
	}
	allowed := &models.CoordinatorAuditEvent{
		ID: "audit-allowed", OccurredAt: now.Add(time.Second), ActorTaskID: "actor", ActorSessionID: "session-1",
		TargetTaskID: "target", WorkspaceID: "ws-1", Action: "message_interrupt", Capability: "orchestrate",
		Decision: "allowed", GrantID: "grant-1", Result: "pending", Detail: "grant",
	}
	for _, event := range []*models.CoordinatorAuditEvent{denied, allowed} {
		if err := repo.CreateCoordinatorAuditEvent(ctx, event); err != nil {
			t.Fatalf("CreateCoordinatorAuditEvent %s: %v", event.ID, err)
		}
	}
	if err := repo.FinishCoordinatorAuditEvent(ctx, allowed.ID, "error", "operation_error"); err != nil {
		t.Fatalf("FinishCoordinatorAuditEvent: %v", err)
	}

	projected, err := repo.ListCoordinatorAuditEvents(ctx, "ws-1", "target", 10)
	if err != nil {
		t.Fatalf("ListCoordinatorAuditEvents: %v", err)
	}
	if len(projected) != 2 {
		t.Fatalf("projected events = %#v, want denied + allowed visible as target participant", projected)
	}
	if projected[0].ID != allowed.ID || projected[0].Result != "error" || projected[0].Detail != "operation_error" {
		t.Fatalf("allowed event = %#v, want resolved error with operation_error detail", projected[0])
	}
	if projected[1].ID != denied.ID || projected[1].Decision != "denied" || projected[1].DenyReason != "scope_or_capability" {
		t.Fatalf("denied event = %#v, want scope_or_capability reason", projected[1])
	}
	for _, event := range projected {
		if event.Detail != "grant" && event.Detail != "operation_error" && event.Detail != "scope_or_capability" {
			t.Fatalf("event detail %q escapes the reason-code vocabulary", event.Detail)
		}
	}

	actorOnly, err := repo.ListCoordinatorAuditEvents(ctx, "ws-1", "unrelated", 10)
	if err != nil {
		t.Fatalf("ListCoordinatorAuditEvents unrelated: %v", err)
	}
	if len(actorOnly) != 0 {
		t.Fatalf("events for unrelated task = %#v, want none", actorOnly)
	}
}
