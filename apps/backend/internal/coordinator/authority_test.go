package coordinator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

type memoryStore struct {
	principal *models.WorkspaceAgentPrincipal
	grants    []*models.CoordinatorGrant
	audits    []*models.CoordinatorAuditEvent
}

func (s *memoryStore) GetActiveWorkspaceAgentPrincipalForTask(_ context.Context, workspaceID, taskID string) (*models.WorkspaceAgentPrincipal, error) {
	if s.principal != nil && s.principal.WorkspaceID == workspaceID && s.principal.BackingTaskID == taskID && s.principal.RevokedAt == nil {
		return s.principal, nil
	}
	return nil, nil
}

func (s *memoryStore) ListActiveWorkspaceAgentPrincipalGrants(_ context.Context, principalID, workspaceID string) ([]*models.CoordinatorGrant, error) {
	grants := make([]*models.CoordinatorGrant, 0)
	for _, grant := range s.grants {
		if grant.PrincipalID == principalID && grant.WorkspaceID == workspaceID && grant.RevokedAt == nil {
			grants = append(grants, grant)
		}
	}
	return grants, nil
}

func (s *memoryStore) CreateCoordinatorAuditEvent(_ context.Context, event *models.CoordinatorAuditEvent) error {
	s.audits = append(s.audits, event)
	return nil
}

func (s *memoryStore) FinishCoordinatorAuditEvent(_ context.Context, id, result, detail string) error {
	for _, event := range s.audits {
		if event.ID == id {
			event.Result, event.Detail = result, detail
		}
	}
	return nil
}

// @covers AC-COORDINATOR-AUTHORITY-003
func TestAuthorityAllowsInScopeCapabilityAndAuditsGrantUse(t *testing.T) {
	store := &memoryStore{principal: &models.WorkspaceAgentPrincipal{ID: "principal-1", WorkspaceID: "workspace", PluginInstallationID: "plugin", LogicalKey: "agent", BackingTaskID: "actor", BackingSessionID: "session"}, grants: []*models.CoordinatorGrant{{
		ID: "grant-1", PrincipalID: "principal-1", WorkspaceID: "workspace",
		ScopeKind: ScopeWorkspace, ScopeID: "workspace", Capabilities: "inspect,orchestrate",
	}}}
	authority := New(store, func() bool { return true })
	decision, err := authority.Authorize(context.Background(), Request{
		ActorTask:      &models.Task{ID: "actor", WorkspaceID: "workspace"},
		TargetTask:     &models.Task{ID: "target", WorkspaceID: "workspace", WorkflowID: "workflow"},
		ActorSessionID: "session", Action: "stop", Capability: CapabilityOrchestrate,
	})
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if !decision.Allowed || decision.Basis != BasisGrant || decision.GrantID != "grant-1" || !decision.Auditable() {
		t.Fatalf("decision = %#v, want allowed grant decision with audit", decision)
	}
	if err := authority.Finish(context.Background(), decision, nil); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if len(store.audits) != 1 || store.audits[0].Result != "ok" {
		t.Fatalf("audits = %#v, want resolved allowed audit", store.audits)
	}
	if audit := store.audits[0]; audit.PrincipalID != "principal-1" || audit.ActorTaskID != "actor" || audit.ActorSessionID != "session" {
		t.Fatalf("audit provenance = %#v, want principal and exact task/session", audit)
	}
}

func TestDecisionAuditableOnlyWhenAnAuditEventExists(t *testing.T) {
	if (Decision{}).Auditable() {
		t.Fatal("empty decision must not be auditable")
	}
	if !(Decision{AuditID: "audit-1"}).Auditable() {
		t.Fatal("decision with an audit ID must be auditable")
	}
}

func TestAuthorityRejectsAReplacedSessionWithoutAnAuditSideEffect(t *testing.T) {
	store := &memoryStore{principal: &models.WorkspaceAgentPrincipal{
		ID: "principal-1", WorkspaceID: "workspace", PluginInstallationID: "plugin", LogicalKey: "agent",
		BackingTaskID: "actor", BackingSessionID: "current-session",
	}, grants: []*models.CoordinatorGrant{{
		ID: "grant-1", PrincipalID: "principal-1", WorkspaceID: "workspace",
		ScopeKind: ScopeWorkspace, ScopeID: "workspace", Capabilities: "orchestrate",
	}}}
	decision, err := New(store, func() bool { return true }).Authorize(context.Background(), Request{
		ActorTask: &models.Task{ID: "actor", WorkspaceID: "workspace"}, TargetTask: &models.Task{ID: "target", WorkspaceID: "workspace"},
		ActorSessionID: "stale-session", Action: "task.stop", Capability: CapabilityOrchestrate,
	})
	if err != nil || decision.Allowed || decision.Auditable() || len(store.audits) != 0 {
		t.Fatalf("replaced session decision = %#v, err = %v, audits = %#v", decision, err, store.audits)
	}
}

func TestAuthorityDeniesCrossWorkspaceWithoutExposingReason(t *testing.T) {
	store := &memoryStore{principal: &models.WorkspaceAgentPrincipal{ID: "principal-1", WorkspaceID: "workspace-a", PluginInstallationID: "plugin", LogicalKey: "agent", BackingTaskID: "actor", BackingSessionID: "session"}, grants: []*models.CoordinatorGrant{{
		ID: "grant-1", PrincipalID: "principal-1", WorkspaceID: "workspace-a",
		ScopeKind: ScopeWorkspace, ScopeID: "workspace-a", Capabilities: "orchestrate", GrantedAt: time.Now(),
	}}}
	decision, err := New(store, func() bool { return true }).Authorize(context.Background(), Request{
		ActorTask:      &models.Task{ID: "actor", WorkspaceID: "workspace-a"},
		TargetTask:     &models.Task{ID: "target", WorkspaceID: "workspace-b"},
		ActorSessionID: "session", Action: "stop", Capability: CapabilityOrchestrate,
	})
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if decision.Allowed || decision.Basis != BasisDenied || decision.DenyReason != "" {
		t.Fatalf("decision = %#v, want opaque denial", decision)
	}
	if len(store.audits) != 1 || store.audits[0].DenyReason != "cross_workspace" {
		t.Fatalf("audit = %#v, want cross-workspace reason", store.audits)
	}
	if store.audits[0].PrincipalID != "principal-1" || store.audits[0].ActorSessionID != "session" {
		t.Fatalf("denial audit provenance = %#v, want current principal/session", store.audits[0])
	}
}

func TestAuthorityDeniesRevokedOrLegacyTaskBoundPrincipal(t *testing.T) {
	revokedAt := time.Now().UTC()
	store := &memoryStore{
		principal: &models.WorkspaceAgentPrincipal{ID: "principal-1", WorkspaceID: "workspace", PluginInstallationID: "plugin", LogicalKey: "agent", BackingTaskID: "actor", BackingSessionID: "session", RevokedAt: &revokedAt},
		grants: []*models.CoordinatorGrant{{
			ID: "legacy-grant", CoordinatorTaskID: "actor", WorkspaceID: "workspace",
			ScopeKind: ScopeWorkspace, ScopeID: "workspace", Capabilities: "orchestrate",
		}},
	}
	decision, err := New(store, func() bool { return true }).Authorize(context.Background(), Request{
		ActorTask:  &models.Task{ID: "actor", WorkspaceID: "workspace"},
		TargetTask: &models.Task{ID: "target", WorkspaceID: "workspace"},
		Action:     "stop", Capability: CapabilityOrchestrate,
	})
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if decision.Allowed || decision.Basis != BasisDenied {
		t.Fatalf("decision = %#v, want denied revoked principal", decision)
	}
	if len(store.audits) != 0 {
		t.Fatalf("audits = %#v, want none without an active principal grant", store.audits)
	}
}

func TestAuthorityKeepsDirectParentAccessWhenFlagIsOff(t *testing.T) {
	decision, err := New(&memoryStore{}, func() bool { return false }).Authorize(context.Background(), Request{
		ActorTask:  &models.Task{ID: "parent", WorkspaceID: "workspace"},
		TargetTask: &models.Task{ID: "child", ParentID: "parent", WorkspaceID: "workspace"},
		Action:     "stop", Capability: CapabilityOrchestrate,
	})
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if !decision.Allowed || decision.Basis != BasisDirectParent {
		t.Fatalf("decision = %#v, want direct parent access", decision)
	}
}

// An adapter resolves a principal from its own authenticated context and asks
// the authority fail-closed questions with host-loaded actor/target tasks.
// When no active principal binding exists for the acting task - disabled
// plugin install, revoked binding, or plain absence - the authority must deny
// silently and write no audit row: an ordinary task's denial is not auditable.
func TestAuthorityDeniesWhenNoActivePrincipalBinding(t *testing.T) {
	store := &memoryStore{}
	authority := New(store, func() bool { return true })
	decision, err := authority.Authorize(context.Background(), Request{
		ActorTask:  &models.Task{ID: "actor", WorkspaceID: "workspace"},
		TargetTask: &models.Task{ID: "target", WorkspaceID: "workspace"},
		Action:     "stop", Capability: CapabilityOrchestrate,
	})
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if decision.Allowed || decision.Basis != BasisDenied {
		t.Fatalf("decision = %#v, want silent denial without binding", decision)
	}
	if len(store.audits) != 0 {
		t.Fatalf("audits = %#v, want none without an active binding", store.audits)
	}
}

// A store failure must deny the action and surface the error; the caller maps
// it to the same opaque denial message as any other non-parent refusal.
func TestAuthorityFailsClosedOnStoreError(t *testing.T) {
	decision, err := New(&errorStore{}, func() bool { return true }).Authorize(context.Background(), Request{
		ActorTask:  &models.Task{ID: "actor", WorkspaceID: "workspace"},
		TargetTask: &models.Task{ID: "target", WorkspaceID: "workspace"},
		Action:     "stop", Capability: CapabilityOrchestrate,
	})
	if err == nil {
		t.Fatal("Authorize succeeded on store error")
	}
	if decision.Allowed || decision.Basis != BasisDenied {
		t.Fatalf("decision = %#v, want fail-closed denial", decision)
	}
}

func TestAuthorityAllowsWorkflowScopedGrant(t *testing.T) {
	store := &memoryStore{principal: &models.WorkspaceAgentPrincipal{ID: "principal-1", WorkspaceID: "workspace", PluginInstallationID: "plugin", LogicalKey: "agent", BackingTaskID: "actor", BackingSessionID: "session"}, grants: []*models.CoordinatorGrant{{
		ID: "grant-1", PrincipalID: "principal-1", WorkspaceID: "workspace",
		ScopeKind: ScopeWorkflow, ScopeID: "workflow-1", Capabilities: "inspect",
	}}}
	authority := New(store, func() bool { return true })
	matching, err := authority.Authorize(context.Background(), Request{
		ActorTask:      &models.Task{ID: "actor", WorkspaceID: "workspace"},
		TargetTask:     &models.Task{ID: "target-in-flow", WorkspaceID: "workspace", WorkflowID: "workflow-1"},
		ActorSessionID: "session", Action: "list_related", Capability: CapabilityInspect,
	})
	if err != nil || !matching.Allowed || matching.Basis != BasisGrant || matching.GrantID != "grant-1" {
		t.Fatalf("workflow-scope decision = %#v, %v; want allowed by grant-1", matching, err)
	}
	outside, err := authority.Authorize(context.Background(), Request{
		ActorTask:      &models.Task{ID: "actor", WorkspaceID: "workspace"},
		TargetTask:     &models.Task{ID: "target-elsewhere", WorkspaceID: "workspace", WorkflowID: "workflow-2"},
		ActorSessionID: "session", Action: "list_related", Capability: CapabilityInspect,
	})
	if err != nil || outside.Allowed || outside.Basis != BasisDenied {
		t.Fatalf("out-of-workflow decision = %#v, %v; want denied", outside, err)
	}
}

type errorStore struct{}

func (s *errorStore) GetActiveWorkspaceAgentPrincipalForTask(_ context.Context, _, _ string) (*models.WorkspaceAgentPrincipal, error) {
	return nil, context.DeadlineExceeded
}
func (s *errorStore) ListActiveWorkspaceAgentPrincipalGrants(_ context.Context, _, _ string) ([]*models.CoordinatorGrant, error) {
	return nil, context.DeadlineExceeded
}
func (s *errorStore) CreateCoordinatorAuditEvent(_ context.Context, _ *models.CoordinatorAuditEvent) error {
	return context.DeadlineExceeded
}
func (s *errorStore) FinishCoordinatorAuditEvent(_ context.Context, _, _, _ string) error {
	return context.DeadlineExceeded
}
