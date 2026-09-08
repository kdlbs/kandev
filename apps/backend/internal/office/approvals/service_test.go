package approvals_test

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/approvals"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// noopActivityLogger implements shared.ActivityLogger without importing shared.
type noopActivityLogger struct{}

func (n *noopActivityLogger) LogActivity(_ context.Context, _, _, _, _, _, _, _ string) {}
func (n *noopActivityLogger) LogActivityWithRun(_ context.Context, _, _, _, _, _, _, _, _, _ string) {
}

// noopRunQueuer implements approvals.RunQueuer as a no-op.
type noopRunQueuer struct{}

func (n *noopRunQueuer) QueueRunWithActor(
	_ context.Context, _, _, _, _ string, _ models.ActorKind, _ string, _ string,
) error {
	return nil
}

// capturingRunQueuer records the arguments of its last QueueRunWithActor
// call so tests can assert the actor threaded all the way from
// DecideApproval into the queued run (AC-OFFICE-RUN-CAUSATION-001.15).
type capturingRunQueuer struct {
	called          bool
	agentInstanceID string
	reason          string
	actorKind       models.ActorKind
	actorID         string
	causingRunID    string
}

func (c *capturingRunQueuer) QueueRunWithActor(
	_ context.Context, agentInstanceID, reason, _, _ string,
	actorKind models.ActorKind, actorID string, causingRunID string,
) error {
	c.called = true
	c.agentInstanceID = agentInstanceID
	c.reason = reason
	c.actorKind = actorKind
	c.actorID = actorID
	c.causingRunID = causingRunID
	return nil
}

type fakeAgentWriter struct {
	statuses map[string]string
	reasons  map[string]string
}

func (f *fakeAgentWriter) UpdateAgentStatusFields(
	_ context.Context,
	agentID, status, pauseReason string,
) error {
	if f.statuses == nil {
		f.statuses = map[string]string{}
	}
	if f.reasons == nil {
		f.reasons = map[string]string{}
	}
	f.statuses[agentID] = status
	f.reasons[agentID] = pauseReason
	return nil
}

// newTestApprovalService creates an ApprovalService backed by an in-memory SQLite repo.
func newTestApprovalService(t *testing.T) (*approvals.ApprovalService, *sqlite.Repository, func(string, ...interface{})) {
	t.Helper()
	return newTestApprovalServiceWithQueuer(t, &noopRunQueuer{})
}

// newTestApprovalServiceWithQueuer is newTestApprovalService with an
// injectable RunQueuer, for tests that need to assert on queued-run
// arguments rather than ignore them.
func newTestApprovalServiceWithQueuer(
	t *testing.T, queuer approvals.RunQueuer,
) (*approvals.ApprovalService, *sqlite.Repository, func(string, ...interface{})) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	log := logger.Default()
	svc := approvals.NewApprovalService(repo, log, &noopActivityLogger{}, queuer)

	execSQL := func(query string, args ...interface{}) {
		t.Helper()
		if _, err := db.Exec(query, args...); err != nil {
			t.Fatalf("exec sql: %v", err)
		}
	}

	return svc, repo, execSQL
}

func TestCreateApprovalWithActivity(t *testing.T) {
	svc, _, _ := newTestApprovalService(t)
	ctx := context.Background()

	approval := &models.Approval{
		WorkspaceID:               "ws-1",
		Type:                      "hire_agent",
		RequestedByAgentProfileID: "agent-1",
		Payload:                   `{"name":"qa-bot"}`,
	}

	if err := svc.CreateApprovalWithActivity(ctx, approval); err != nil {
		t.Fatalf("CreateApprovalWithActivity: %v", err)
	}

	if approval.ID == "" {
		t.Error("approval ID should be set")
	}
	if approval.Status != "pending" {
		t.Errorf("status = %q, want pending", approval.Status)
	}
}

func TestDecideApproval_Approve(t *testing.T) {
	svc, _, _ := newTestApprovalService(t)
	ctx := context.Background()

	approval := &models.Approval{
		WorkspaceID:               "ws-1",
		Type:                      "hire_agent",
		RequestedByAgentProfileID: "agent-1",
		Payload:                   `{"name":"qa-bot"}`,
	}
	if err := svc.CreateApprovalWithActivity(ctx, approval); err != nil {
		t.Fatalf("CreateApprovalWithActivity: %v", err)
	}

	decided, err := svc.DecideApproval(ctx, approval.ID, "approved", "user-1", models.ActorKindUser, "Looks good")
	if err != nil {
		t.Fatalf("DecideApproval: %v", err)
	}
	if decided.Status != "approved" {
		t.Errorf("status = %q, want approved", decided.Status)
	}
	if decided.DecidedBy != "user-1" {
		t.Errorf("decided_by = %q, want user-1", decided.DecidedBy)
	}
	if decided.DecisionNote != "Looks good" {
		t.Errorf("note = %q, want 'Looks good'", decided.DecisionNote)
	}
	if decided.DecidedAt == nil {
		t.Error("decided_at should be set")
	}
}

func TestDecideApproval_ApproveHireAgentActivatesPendingAgent(t *testing.T) {
	svc, _, _ := newTestApprovalService(t)
	ctx := context.Background()
	writer := &fakeAgentWriter{}
	svc.SetAgentWriter(writer)

	approval := &models.Approval{
		WorkspaceID:               "ws-1",
		Type:                      "hire_agent",
		RequestedByAgentProfileID: "creator-1",
		Payload:                   `{"agent_profile_id":"agent-new"}`,
	}
	if err := svc.CreateApprovalWithActivity(ctx, approval); err != nil {
		t.Fatalf("CreateApprovalWithActivity: %v", err)
	}

	if _, err := svc.DecideApproval(ctx, approval.ID, "approved", "user-1", models.ActorKindUser, ""); err != nil {
		t.Fatalf("DecideApproval: %v", err)
	}

	if writer.statuses["agent-new"] != "idle" {
		t.Fatalf("status = %q, want idle", writer.statuses["agent-new"])
	}
	if writer.reasons["agent-new"] != "" {
		t.Fatalf("pause reason = %q, want empty", writer.reasons["agent-new"])
	}
}

func TestDecideApproval_RejectHireAgentStopsPendingAgent(t *testing.T) {
	svc, _, _ := newTestApprovalService(t)
	ctx := context.Background()
	writer := &fakeAgentWriter{}
	svc.SetAgentWriter(writer)

	approval := &models.Approval{
		WorkspaceID:               "ws-1",
		Type:                      "hire_agent",
		RequestedByAgentProfileID: "creator-1",
		Payload:                   `{"agent_profile_id":"agent-new"}`,
	}
	if err := svc.CreateApprovalWithActivity(ctx, approval); err != nil {
		t.Fatalf("CreateApprovalWithActivity: %v", err)
	}

	if _, err := svc.DecideApproval(ctx, approval.ID, "rejected", "user-1", models.ActorKindUser, "not needed"); err != nil {
		t.Fatalf("DecideApproval: %v", err)
	}

	if writer.statuses["agent-new"] != "stopped" {
		t.Fatalf("status = %q, want stopped", writer.statuses["agent-new"])
	}
	if writer.reasons["agent-new"] != "hire rejected" {
		t.Fatalf("pause reason = %q, want hire rejected", writer.reasons["agent-new"])
	}
}

func TestDecideApproval_Reject(t *testing.T) {
	svc, _, _ := newTestApprovalService(t)
	ctx := context.Background()

	approval := &models.Approval{
		WorkspaceID:               "ws-1",
		Type:                      "task_review",
		RequestedByAgentProfileID: "agent-1",
	}
	if err := svc.CreateApprovalWithActivity(ctx, approval); err != nil {
		t.Fatalf("CreateApprovalWithActivity: %v", err)
	}

	decided, err := svc.DecideApproval(ctx, approval.ID, "rejected", "user-1", models.ActorKindUser, "Needs work")
	if err != nil {
		t.Fatalf("DecideApproval: %v", err)
	}
	if decided.Status != "rejected" {
		t.Errorf("status = %q, want rejected", decided.Status)
	}
}

func TestDecideApproval_AlreadyDecided(t *testing.T) {
	svc, _, _ := newTestApprovalService(t)
	ctx := context.Background()

	approval := &models.Approval{
		WorkspaceID:               "ws-1",
		Type:                      "hire_agent",
		RequestedByAgentProfileID: "agent-1",
	}
	if err := svc.CreateApprovalWithActivity(ctx, approval); err != nil {
		t.Fatalf("CreateApprovalWithActivity: %v", err)
	}

	// First decision succeeds.
	if _, err := svc.DecideApproval(ctx, approval.ID, "approved", "user-1", models.ActorKindUser, ""); err != nil {
		t.Fatalf("DecideApproval: %v", err)
	}

	// Second decision fails.
	if _, err := svc.DecideApproval(ctx, approval.ID, "rejected", "user-2", models.ActorKindUser, ""); err == nil {
		t.Error("expected error when deciding already-decided approval")
	}
}

func TestGetPendingApprovals(t *testing.T) {
	svc, _, _ := newTestApprovalService(t)
	ctx := context.Background()

	// Create 3 approvals, decide 1.
	for i := 0; i < 3; i++ {
		a := &models.Approval{
			WorkspaceID:               "ws-1",
			Type:                      "hire_agent",
			RequestedByAgentProfileID: "agent-1",
		}
		if err := svc.CreateApprovalWithActivity(ctx, a); err != nil {
			t.Fatalf("create %d: %v", i, err)
		}
		if i == 0 {
			if _, err := svc.DecideApproval(ctx, a.ID, "approved", "user-1", models.ActorKindUser, ""); err != nil {
				t.Fatalf("decide: %v", err)
			}
		}
	}

	pending, err := svc.GetPendingApprovals(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetPendingApprovals: %v", err)
	}
	if len(pending) != 2 {
		t.Errorf("count = %d, want 2", len(pending))
	}
}

// TestQueueApprovalRun_AgentActorThreadedToQueueRun covers
// AC-OFFICE-RUN-CAUSATION-001.15: queueApprovalRun previously always called
// the actor-blind QueueRun, so an approval decided by an authenticated
// agent (a real caller.ID, per resolveDecider) was silently attributed to
// the system actor. It must now reach QueueRunWithActor with the real
// (ActorKindAgent, decidedBy) pair.
func TestQueueApprovalRun_AgentActorThreadedToQueueRun(t *testing.T) {
	queuer := &capturingRunQueuer{}
	svc, _, _ := newTestApprovalServiceWithQueuer(t, queuer)
	ctx := context.Background()

	approval := &models.Approval{
		WorkspaceID:               "ws-1",
		Type:                      "task_review",
		RequestedByAgentProfileID: "agent-1",
	}
	if err := svc.CreateApprovalWithActivity(ctx, approval); err != nil {
		t.Fatalf("CreateApprovalWithActivity: %v", err)
	}

	if _, err := svc.DecideApproval(
		ctx, approval.ID, "approved", "ceo-1", models.ActorKindAgent, "",
	); err != nil {
		t.Fatalf("DecideApproval: %v", err)
	}

	if !queuer.called {
		t.Fatal("QueueRunWithActor was not called")
	}
	if queuer.agentInstanceID != "agent-1" {
		t.Errorf("agentInstanceID = %q, want agent-1", queuer.agentInstanceID)
	}
	if queuer.reason != "approval_resolved" {
		t.Errorf("reason = %q, want approval_resolved", queuer.reason)
	}
	if queuer.actorKind != models.ActorKindAgent {
		t.Errorf("actorKind = %q, want %q", queuer.actorKind, models.ActorKindAgent)
	}
	if queuer.actorID != "ceo-1" {
		t.Errorf("actorID = %q, want ceo-1", queuer.actorID)
	}
	if queuer.causingRunID != "" {
		t.Errorf("causingRunID = %q, want empty (an approval decision has no live causing run"+
			" to chain from; it always roots a new causation chain)", queuer.causingRunID)
	}
}

// TestQueueApprovalRun_UserActorThreadedToQueueRun is the ActorKindUser
// counterpart of the agent case above — a UI/unauthenticated decider must
// reach QueueRunWithActor as ActorKindUser, not system.
func TestQueueApprovalRun_UserActorThreadedToQueueRun(t *testing.T) {
	queuer := &capturingRunQueuer{}
	svc, _, _ := newTestApprovalServiceWithQueuer(t, queuer)
	ctx := context.Background()

	approval := &models.Approval{
		WorkspaceID:               "ws-1",
		Type:                      "task_review",
		RequestedByAgentProfileID: "agent-1",
	}
	if err := svc.CreateApprovalWithActivity(ctx, approval); err != nil {
		t.Fatalf("CreateApprovalWithActivity: %v", err)
	}

	if _, err := svc.DecideApproval(
		ctx, approval.ID, "approved", "ui", models.ActorKindUser, "",
	); err != nil {
		t.Fatalf("DecideApproval: %v", err)
	}

	if !queuer.called {
		t.Fatal("QueueRunWithActor was not called")
	}
	if queuer.actorKind != models.ActorKindUser {
		t.Errorf("actorKind = %q, want %q", queuer.actorKind, models.ActorKindUser)
	}
	if queuer.actorID != "ui" {
		t.Errorf("actorID = %q, want ui", queuer.actorID)
	}
}
