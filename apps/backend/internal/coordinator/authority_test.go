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
	store := &memoryStore{principal: &models.WorkspaceAgentPrincipal{ID: "principal-1", WorkspaceID: "workspace", BackingTaskID: "actor"}, grants: []*models.CoordinatorGrant{{
		ID: "grant-1", PrincipalID: "principal-1", WorkspaceID: "workspace",
		ScopeKind: ScopeWorkspace, ScopeID: "workspace", Capabilities: "inspect,orchestrate",
	}}}
	authority := New(store, func() bool { return true })
	decision, err := authority.Authorize(context.Background(), Request{
		ActorTask:  &models.Task{ID: "actor", WorkspaceID: "workspace"},
		TargetTask: &models.Task{ID: "target", WorkspaceID: "workspace", WorkflowID: "workflow"},
		Action:     "stop", Capability: CapabilityOrchestrate,
	})
	if err != nil {
		t.Fatalf("Authorize: %v", err)
	}
	if !decision.Allowed || decision.Basis != BasisGrant || decision.GrantID != "grant-1" || decision.AuditID == "" {
		t.Fatalf("decision = %#v, want allowed grant decision with audit", decision)
	}
	if err := authority.Finish(context.Background(), decision, nil); err != nil {
		t.Fatalf("Finish: %v", err)
	}
	if len(store.audits) != 1 || store.audits[0].Result != "ok" {
		t.Fatalf("audits = %#v, want resolved allowed audit", store.audits)
	}
}

func TestAuthorityDeniesCrossWorkspaceWithoutExposingReason(t *testing.T) {
	store := &memoryStore{principal: &models.WorkspaceAgentPrincipal{ID: "principal-1", WorkspaceID: "workspace-a", BackingTaskID: "actor"}, grants: []*models.CoordinatorGrant{{
		ID: "grant-1", PrincipalID: "principal-1", WorkspaceID: "workspace-a",
		ScopeKind: ScopeWorkspace, ScopeID: "workspace-a", Capabilities: "orchestrate", GrantedAt: time.Now(),
	}}}
	decision, err := New(store, func() bool { return true }).Authorize(context.Background(), Request{
		ActorTask:  &models.Task{ID: "actor", WorkspaceID: "workspace-a"},
		TargetTask: &models.Task{ID: "target", WorkspaceID: "workspace-b"},
		Action:     "stop", Capability: CapabilityOrchestrate,
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
}

func TestAuthorityDeniesRevokedOrLegacyTaskBoundPrincipal(t *testing.T) {
	revokedAt := time.Now().UTC()
	store := &memoryStore{
		principal: &models.WorkspaceAgentPrincipal{ID: "principal-1", WorkspaceID: "workspace", BackingTaskID: "actor", RevokedAt: &revokedAt},
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
