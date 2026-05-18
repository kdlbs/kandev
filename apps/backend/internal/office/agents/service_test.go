package agents

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/shared"
)

type testActivityLogger struct{}

func (t *testActivityLogger) LogActivity(_ context.Context, _, _, _, _, _, _, _ string) {}
func (t *testActivityLogger) LogActivityWithRun(_ context.Context, _, _, _, _, _, _, _, _, _ string) {
}

type fakeGovernanceSettings struct {
	requireNewAgents bool
}

func (f *fakeGovernanceSettings) GetRequireApprovalForNewAgents(
	_ context.Context,
	_ string,
) (bool, error) {
	return f.requireNewAgents, nil
}

type fakeApprovalCreator struct {
	approvals []*models.Approval
}

func (f *fakeApprovalCreator) CreateApprovalWithActivity(
	_ context.Context,
	approval *models.Approval,
) error {
	f.approvals = append(f.approvals, approval)
	return nil
}

func newTestAgentService(t *testing.T) (*AgentService, *sqlite.Repository) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}

	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	svc := NewAgentService(repo, logger.Default(), &testActivityLogger{})
	return svc, repo
}

func TestCreateAgentInstanceWithCaller_PersistsPendingAgentWhenApprovalRequired(t *testing.T) {
	svc, repo := newTestAgentService(t)
	ctx := context.Background()
	approvalCreator := &fakeApprovalCreator{}
	svc.SetGovernanceSettings(&fakeGovernanceSettings{requireNewAgents: true})
	svc.SetGovernanceApproval(approvalCreator)

	caller := &models.AgentInstance{ID: "creator-1", Name: "CEO", Role: models.AgentRoleCEO}
	agent := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "QA Reviewer",
		Role:        models.AgentRoleQA,
	}

	if err := svc.CreateAgentInstanceWithCaller(ctx, agent, caller, "expand testing"); err != nil {
		t.Fatalf("CreateAgentInstanceWithCaller: %v", err)
	}

	stored, err := repo.GetAgentInstance(ctx, agent.ID)
	if err != nil {
		t.Fatalf("GetAgentInstance: %v", err)
	}
	if stored.Status != models.AgentStatusPendingApproval {
		t.Fatalf("status = %q, want pending_approval", stored.Status)
	}
	if stored.Permissions == "" || stored.Permissions == "{}" {
		t.Fatal("expected default permissions on pending agent")
	}
	if len(approvalCreator.approvals) != 1 {
		t.Fatalf("approvals = %d, want 1", len(approvalCreator.approvals))
	}

	approval := approvalCreator.approvals[0]
	if approval.Type != models.ApprovalTypeHireAgent {
		t.Fatalf("approval type = %q, want hire_agent", approval.Type)
	}
	if approval.RequestedByAgentProfileID != caller.ID {
		t.Fatalf("requested_by = %q, want %q", approval.RequestedByAgentProfileID, caller.ID)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(approval.Payload), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload["agent_profile_id"] != agent.ID {
		t.Errorf("agent_profile_id = %v, want %q", payload["agent_profile_id"], agent.ID)
	}
	if payload["creator_agent_id"] != caller.ID {
		t.Errorf("creator_agent_id = %v, want %q", payload["creator_agent_id"], caller.ID)
	}
	if payload["permissions"] != shared.DefaultPermissions(shared.AgentRoleQA) {
		t.Errorf("permissions = %v, want role defaults", payload["permissions"])
	}
}

func TestCreateAgentInstanceWithCaller_UICreateBypassesGovernance(t *testing.T) {
	svc, _ := newTestAgentService(t)
	ctx := context.Background()
	approvalCreator := &fakeApprovalCreator{}
	svc.SetGovernanceSettings(&fakeGovernanceSettings{requireNewAgents: true})
	svc.SetGovernanceApproval(approvalCreator)

	agent := &models.AgentInstance{
		WorkspaceID: "ws-1",
		Name:        "Frontend Worker",
		Role:        models.AgentRoleWorker,
	}

	if err := svc.CreateAgentInstanceWithCaller(ctx, agent, nil, ""); err != nil {
		t.Fatalf("CreateAgentInstanceWithCaller: %v", err)
	}
	if agent.Status != models.AgentStatusIdle {
		t.Fatalf("status = %q, want idle", agent.Status)
	}
	if len(approvalCreator.approvals) != 0 {
		t.Fatalf("approvals = %d, want 0", len(approvalCreator.approvals))
	}
}
