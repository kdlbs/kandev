package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/coordinator"
	orchmodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type handoffCoordinatorStore struct {
	principal *models.WorkspaceAgentPrincipal
	grants    []*models.CoordinatorGrant
	audits    []*models.CoordinatorAuditEvent
}

func (s *handoffCoordinatorStore) GetActiveWorkspaceAgentPrincipalForTask(_ context.Context, workspaceID, taskID string) (*models.WorkspaceAgentPrincipal, error) {
	if s.principal != nil && s.principal.WorkspaceID == workspaceID && s.principal.BackingTaskID == taskID && s.principal.RevokedAt == nil {
		return s.principal, nil
	}
	return nil, nil
}

func (s *handoffCoordinatorStore) ListActiveWorkspaceAgentPrincipalGrants(_ context.Context, principalID, workspaceID string) ([]*models.CoordinatorGrant, error) {
	var active []*models.CoordinatorGrant
	for _, grant := range s.grants {
		if grant.PrincipalID == principalID && grant.WorkspaceID == workspaceID && grant.RevokedAt == nil {
			active = append(active, grant)
		}
	}
	return active, nil
}

func (s *handoffCoordinatorStore) CreateCoordinatorAuditEvent(_ context.Context, event *models.CoordinatorAuditEvent) error {
	s.audits = append(s.audits, event)
	return nil
}

func (s *handoffCoordinatorStore) FinishCoordinatorAuditEvent(_ context.Context, id, result, detail string) error {
	for _, event := range s.audits {
		if event.ID == id {
			event.Result = result
			event.Detail = detail
		}
	}
	return nil
}

// setDescription and setState mutate a task the fake repo already holds so a
// test can model a CREATED sibling (no session) that carries dependency
// metadata in its description. They reach into the fake directly because
// addTask only seeds id/parent/workspace.
func (f *fakeTaskRepo) setDescription(id, desc string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t := f.tasks[id]; t != nil {
		t.Description = desc
	}
}

func (f *fakeTaskRepo) setState(id string, state v1.TaskState) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t := f.tasks[id]; t != nil {
		t.State = state
	}
}

// TestListRelated_CreatedSiblingDescription is the core regression for
// issue #1772: a running subtask must be able to read the description of an
// authorized CREATED sibling that has no session yet, through the bounded
// list_related_tasks projection.
func TestListRelated_CreatedSiblingDescription(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("parent", "", "ws-1")
	tasks.addTask("runner", "parent", "ws-1") // the running caller
	tasks.addTask("dep", "parent", "ws-1")    // CREATED sibling, never started
	tasks.setState("runner", v1.TaskStateInProgress)
	tasks.setState("dep", v1.TaskStateCreated)
	tasks.setDescription("dep", "Depends on: 01 [foundation], 03 [canvas]")
	svc := newCascadeService(t, tasks, newCascadeWSGroupRepo())

	out, err := svc.ListRelated(context.Background(), "runner")
	if err != nil {
		t.Fatalf("list related: %v", err)
	}
	if len(out.Siblings) != 1 || out.Siblings[0].ID != "dep" {
		t.Fatalf("siblings = %+v, want single sibling dep", out.Siblings)
	}
	sib := out.Siblings[0]
	if sib.State != string(v1.TaskStateCreated) {
		t.Errorf("sibling state = %q, want CREATED", sib.State)
	}
	if sib.Description != "Depends on: 01 [foundation], 03 [canvas]" {
		t.Errorf("sibling description = %q, want the Depends on line", sib.Description)
	}
}

// TestListRelated_ProjectsDescriptionAcrossRelations verifies the
// description flows onto self, parent, and children too, not just siblings.
func TestListRelated_ProjectsDescriptionAcrossRelations(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("parent", "", "ws-1")
	tasks.addTask("self", "parent", "ws-1")
	tasks.addTask("child", "self", "ws-1")
	tasks.setDescription("parent", "parent desc")
	tasks.setDescription("self", "self desc")
	tasks.setDescription("child", "child desc")
	svc := newCascadeService(t, tasks, newCascadeWSGroupRepo())

	out, err := svc.ListRelated(context.Background(), "self")
	if err != nil {
		t.Fatalf("list related: %v", err)
	}
	if out.Task.Description != "self desc" {
		t.Errorf("self description = %q", out.Task.Description)
	}
	if out.Parent == nil || out.Parent.Description != "parent desc" {
		t.Errorf("parent description = %+v", out.Parent)
	}
	if len(out.Children) != 1 || out.Children[0].Description != "child desc" {
		t.Errorf("children = %+v", out.Children)
	}
}

// TestListRelated_ProjectsBlockerDescriptions exercises the toRelatedByIDs
// path (Blockers / BlockedBy) which only runs when the blockers repo is
// non-nil. It guards against a silent regression in the blocker projection.
func TestListRelated_ProjectsBlockerDescriptions(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("parent", "", "ws-1")
	tasks.addTask("self", "parent", "ws-1")
	tasks.addTask("blocker", "parent", "ws-1")
	tasks.addTask("downstream", "parent", "ws-1")
	tasks.setDescription("blocker", "Depends on: nothing; produces foundation")
	tasks.setDescription("downstream", "Depends on: 02 [self]")
	blockers := &fakeBlockerRepo{}
	// self is blocked by "blocker"; "downstream" is blocked by self.
	_ = blockers.CreateTaskBlocker(context.Background(), &orchmodels.TaskBlocker{TaskID: "self", BlockerTaskID: "blocker"})
	_ = blockers.CreateTaskBlocker(context.Background(), &orchmodels.TaskBlocker{TaskID: "downstream", BlockerTaskID: "self"})
	svc := newPhase4Service(t, tasks, blockers, newCascadeWSGroupRepo())

	out, err := svc.ListRelated(context.Background(), "self")
	if err != nil {
		t.Fatalf("list related: %v", err)
	}
	if len(out.Blockers) != 1 || out.Blockers[0].Description != "Depends on: nothing; produces foundation" {
		t.Errorf("blockers = %+v", out.Blockers)
	}
	if len(out.BlockedBy) != 1 || out.BlockedBy[0].Description != "Depends on: 02 [self]" {
		t.Errorf("blocked_by = %+v", out.BlockedBy)
	}
}

// TestListRelatedForCaller_GatesUnrelatedAndCrossWorkspace covers the access
// guard: a caller may inspect itself and its relations, but not an unrelated
// task or a task in another workspace, so descriptions never leak past the
// caller's relation/workspace scope (issue #1772 acceptance criterion).
func TestListRelatedForCaller_GatesUnrelatedAndCrossWorkspace(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("parent", "", "ws-1")
	tasks.addTask("caller", "parent", "ws-1")
	tasks.addTask("sibling", "parent", "ws-1")
	tasks.addTask("unrelated-parent", "", "ws-1")
	tasks.addTask("unrelated", "unrelated-parent", "ws-1") // same workspace, no relation
	tasks.addTask("other-ws", "", "ws-2")                  // different workspace
	tasks.setDescription("sibling", "Depends on: 01 [foundation]")
	tasks.setDescription("unrelated", "secret plan")
	svc := newCascadeService(t, tasks, newCascadeWSGroupRepo())

	// Self is always allowed.
	if _, err := svc.ListRelatedForCaller(context.Background(), "caller", "caller"); err != nil {
		t.Fatalf("self should be allowed: %v", err)
	}
	// A sibling / parent (relation in same workspace) is allowed.
	if _, err := svc.ListRelatedForCaller(context.Background(), "caller", "sibling"); err != nil {
		t.Fatalf("sibling should be allowed: %v", err)
	}
	if _, err := svc.ListRelatedForCaller(context.Background(), "caller", "parent"); err != nil {
		t.Fatalf("parent should be allowed: %v", err)
	}
	// An unrelated task in the same workspace is denied.
	if _, err := svc.ListRelatedForCaller(context.Background(), "caller", "unrelated"); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("unrelated should be denied, got %v", err)
	}
	// A task in another workspace is denied.
	if _, err := svc.ListRelatedForCaller(context.Background(), "caller", "other-ws"); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("cross-workspace should be denied, got %v", err)
	}
	// An empty caller has no identity to authorize a non-self target.
	if _, err := svc.ListRelatedForCaller(context.Background(), "", "sibling"); !errors.Is(err, ErrAccessDenied) {
		t.Errorf("empty caller should be denied for a non-self target, got %v", err)
	}
}

func TestListRelatedForCallerSession_InspectGrantUsesCurrentBindingAndResolvesAudit(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("caller", "", "ws-1")
	tasks.addTask("unrelated", "", "ws-1")
	store := &handoffCoordinatorStore{
		principal: &models.WorkspaceAgentPrincipal{
			ID: "principal-1", WorkspaceID: "ws-1", PluginInstallationID: "plugin-1", LogicalKey: "coordinator", BackingTaskID: "caller", BackingSessionID: "caller-session",
		},
		grants: []*models.CoordinatorGrant{{
			ID: "grant-1", PrincipalID: "principal-1", WorkspaceID: "ws-1",
			ScopeKind: coordinator.ScopeWorkspace, ScopeID: "ws-1", Capabilities: "inspect",
		}},
	}
	svc := newCascadeService(t, tasks, newCascadeWSGroupRepo())
	svc.SetCoordinatorAuthority(coordinator.New(store, func() bool { return true }))

	if _, err := svc.ListRelatedForCallerSession(context.Background(), "caller", "stale-session", "unrelated"); !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("stale session error = %v, want ErrAccessDenied", err)
	}
	if _, err := svc.ListRelatedForCallerSession(context.Background(), "caller", "caller-session", "unrelated"); err != nil {
		t.Fatalf("current session inspect: %v", err)
	}
	if len(store.audits) != 1 || store.audits[0].Result != "ok" || store.audits[0].PrincipalID != "principal-1" || store.audits[0].ActorSessionID != "caller-session" {
		t.Fatalf("audits = %#v, want one resolved principal/session audit", store.audits)
	}
}

// TestListRelatedForCaller_ReturnsSiblingDescription confirms the gated entry
// point still surfaces an authorized CREATED sibling's description end-to-end.
func TestListRelatedForCaller_ReturnsSiblingDescription(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("parent", "", "ws-1")
	tasks.addTask("runner", "parent", "ws-1")
	tasks.addTask("dep", "parent", "ws-1")
	tasks.setState("dep", v1.TaskStateCreated)
	tasks.setDescription("dep", "Depends on: 01 [foundation]")
	svc := newCascadeService(t, tasks, newCascadeWSGroupRepo())

	out, err := svc.ListRelatedForCaller(context.Background(), "runner", "runner")
	if err != nil {
		t.Fatalf("list related for caller: %v", err)
	}
	if len(out.Siblings) != 1 || out.Siblings[0].Description != "Depends on: 01 [foundation]" {
		t.Errorf("siblings = %+v", out.Siblings)
	}
}

// TestListRelatedForCallerSession_TargetMaterializationFailure audits denial
// without leaking target existence (Finding 2). When the caller holds a valid
// coordinator grant but the target task cannot be resolved, the system must
// produce a denied audit row rather than returning a zero Decision that
// bypasses finishCoordinatorRead.
func TestListRelatedForCallerSession_TargetMaterializationFailure_AuditsDeniedNoLeak(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("caller", "", "ws-1")
	// Do NOT add the target task — it doesn't exist in the DB.
	store := &handoffCoordinatorStore{
		principal: &models.WorkspaceAgentPrincipal{
			ID: "principal-1", WorkspaceID: "ws-1", PluginInstallationID: "plugin-1",
			LogicalKey: "coordinator", BackingTaskID: "caller", BackingSessionID: "caller-session",
		},
		grants: []*models.CoordinatorGrant{{
			ID: "grant-1", PrincipalID: "principal-1", WorkspaceID: "ws-1",
			ScopeKind: coordinator.ScopeWorkspace, ScopeID: "ws-1", Capabilities: "inspect",
		}},
	}
	svc := newCascadeService(t, tasks, newCascadeWSGroupRepo())
	svc.SetCoordinatorAuthority(coordinator.New(store, func() bool { return true }))

	// ListRelatedForCallerSession on a nonexistent target with a valid grant.
	_, err := svc.ListRelatedForCallerSession(context.Background(), "caller", "caller-session", "nonexistent-target")
	if !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("nonexistent target error = %v, want ErrAccessDenied (no leak)", err)
	}
	// Verify a denied audit row was produced for the attempt.
	if len(store.audits) != 1 {
		t.Fatalf("audits = %d rows, want 1 denied audit row", len(store.audits))
	}
	if store.audits[0].Decision != "denied" {
		t.Errorf("audit decision = %q, want denied", store.audits[0].Decision)
	}
	if store.audits[0].ActorTaskID != "caller" {
		t.Errorf("audit actor_task_id = %q, want caller", store.audits[0].ActorTaskID)
	}
	if store.audits[0].TargetTaskID != "nonexistent-target" {
		t.Errorf("audit target_task_id = %q, want nonexistent-target", store.audits[0].TargetTaskID)
	}
	// PrincipalID is empty because target materialization failed before the
	// principal could be resolved — the audit still records the denied attempt.
	if store.audits[0].Detail != "materialization_failure" {
		t.Errorf("audit detail = %q, want materialization_failure", store.audits[0].Detail)
	}
	// Prove the caller error does NOT reveal "nonexistent" or "not found" in
	// its message — only ErrAccessDenied.
	if err != ErrAccessDenied {
		t.Errorf("error message = %q, want only ErrAccessDenied (no existence leak)", err)
	}
}

// TestGetDocumentForCallerSession_TargetMaterializationFailure audits denial
// without leaking target existence (Finding 2 variant for document reads).
func TestGetDocumentForCallerSession_TargetMaterializationFailure_AuditsDeniedNoLeak(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("caller", "", "ws-1")
	store := &handoffCoordinatorStore{
		principal: &models.WorkspaceAgentPrincipal{
			ID: "principal-1", WorkspaceID: "ws-1", PluginInstallationID: "plugin-1",
			LogicalKey: "coordinator", BackingTaskID: "caller", BackingSessionID: "caller-session",
		},
		grants: []*models.CoordinatorGrant{{
			ID: "grant-1", PrincipalID: "principal-1", WorkspaceID: "ws-1",
			ScopeKind: coordinator.ScopeWorkspace, ScopeID: "ws-1", Capabilities: "inspect",
		}},
	}
	svc := newCascadeService(t, tasks, newCascadeWSGroupRepo())
	svc.SetCoordinatorAuthority(coordinator.New(store, func() bool { return true }))

	_, err := svc.GetDocumentForCallerSession(context.Background(), "caller", "caller-session", "nonexistent-target", "some-key")
	if !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("nonexistent target error = %v, want ErrAccessDenied (no leak)", err)
	}
	// Must have a denied audit row.
	if len(store.audits) != 1 {
		t.Fatalf("audits = %d rows, want 1 denied audit row", len(store.audits))
	}
	if store.audits[0].Decision != "denied" {
		t.Errorf("audit decision = %q, want denied", store.audits[0].Decision)
	}
	if store.audits[0].ActorTaskID != "caller" {
		t.Errorf("audit actor_task_id = %q, want caller", store.audits[0].ActorTaskID)
	}
	if store.audits[0].TargetTaskID != "nonexistent-target" {
		t.Errorf("audit target_task_id = %q, want nonexistent-target", store.audits[0].TargetTaskID)
	}
	// Error must be opaque ErrAccessDenied.
	if err != ErrAccessDenied {
		t.Errorf("error message = %q, want only ErrAccessDenied", err)
	}
}

// TestListDocumentsForCallerSession_TargetMaterializationFailure audits
// denial without leaking target existence (Finding 2 variant).
func TestListDocumentsForCallerSession_TargetMaterializationFailure_AuditsDeniedNoLeak(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("caller", "", "ws-1")
	store := &handoffCoordinatorStore{
		principal: &models.WorkspaceAgentPrincipal{
			ID: "principal-1", WorkspaceID: "ws-1", PluginInstallationID: "plugin-1",
			LogicalKey: "coordinator", BackingTaskID: "caller", BackingSessionID: "caller-session",
		},
		grants: []*models.CoordinatorGrant{{
			ID: "grant-1", PrincipalID: "principal-1", WorkspaceID: "ws-1",
			ScopeKind: coordinator.ScopeWorkspace, ScopeID: "ws-1", Capabilities: "inspect",
		}},
	}
	svc := newCascadeService(t, tasks, newCascadeWSGroupRepo())
	svc.SetCoordinatorAuthority(coordinator.New(store, func() bool { return true }))

	_, err := svc.ListDocumentsForCallerSession(context.Background(), "caller", "caller-session", "nonexistent-target")
	if !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("nonexistent target error = %v, want ErrAccessDenied (no leak)", err)
	}
	if len(store.audits) != 1 {
		t.Fatalf("audits = %d rows, want 1 denied audit row", len(store.audits))
	}
	if store.audits[0].Decision != "denied" {
		t.Errorf("audit decision = %q, want denied", store.audits[0].Decision)
	}
}

// TestListRelated_OmitsEmptyDescription confirms the projection stays lean
// when a related task has no description (omitempty keeps the MCP output
// usable on workflows with many historical tasks).
func TestListRelated_OmitsEmptyDescription(t *testing.T) {
	tasks := newFakeTaskRepo()
	tasks.addTask("parent", "", "ws-1")
	tasks.addTask("self", "parent", "ws-1")
	tasks.addTask("sib", "parent", "ws-1")
	svc := newCascadeService(t, tasks, newCascadeWSGroupRepo())

	out, err := svc.ListRelated(context.Background(), "self")
	if err != nil {
		t.Fatalf("list related: %v", err)
	}
	if len(out.Siblings) != 1 || out.Siblings[0].Description != "" {
		t.Errorf("sibling description should be empty, got %+v", out.Siblings)
	}
}
