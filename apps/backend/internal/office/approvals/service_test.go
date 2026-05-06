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

func (n *noopRunQueuer) QueueRun(_ context.Context, _, _, _, _ string) error { return nil }

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
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo, err := sqlite.NewWithDB(db, db)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	log := logger.Default()
	svc := approvals.NewApprovalService(repo, log, &noopActivityLogger{}, &noopRunQueuer{})

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

	decided, err := svc.DecideApproval(ctx, approval.ID, "approved", "user-1", "Looks good")
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

	if _, err := svc.DecideApproval(ctx, approval.ID, "approved", "user-1", ""); err != nil {
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

	if _, err := svc.DecideApproval(ctx, approval.ID, "rejected", "user-1", "not needed"); err != nil {
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

	decided, err := svc.DecideApproval(ctx, approval.ID, "rejected", "user-1", "Needs work")
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
	if _, err := svc.DecideApproval(ctx, approval.ID, "approved", "user-1", ""); err != nil {
		t.Fatalf("DecideApproval: %v", err)
	}

	// Second decision fails.
	if _, err := svc.DecideApproval(ctx, approval.ID, "rejected", "user-2", ""); err == nil {
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
			if _, err := svc.DecideApproval(ctx, a.ID, "approved", "user-1", ""); err != nil {
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
