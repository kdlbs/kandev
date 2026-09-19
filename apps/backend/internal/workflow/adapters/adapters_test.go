package adapters

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/workflow/engine"
	"github.com/kandev/kandev/internal/workflow/models"
)

// fakeRepo is a minimal in-memory WorkflowRepo. The adapters delegate
// straight to the repo so we only need stub responses to verify the
// type translation between models / engine packages.
type fakeRepo struct {
	participants     []*models.WorkflowStepParticipant
	taskParticipants []*models.WorkflowStepParticipant
	decisions        []*models.WorkflowStepDecision
	runner           string
	taskStepID       string

	listErr   error
	recordErr error
	clearErr  error
	runnerErr error
	stepErr   error

	recorded       *models.WorkflowStepDecision
	cleared        bool
	resolvedStepID string
	resolvedTaskID string
	lookedUpTaskID string

	hasRoleSeat       bool
	hasRoleSeatErr    error
	hasRoleSeatCalled bool

	ensuredSeat    *models.WorkflowStepParticipant
	ensureSeatErr  error
	ensureSeatArgs []string
}

func (f *fakeRepo) ListStepParticipantsForTask(_ context.Context, _, _ string) ([]*models.WorkflowStepParticipant, error) {
	return f.participants, f.listErr
}
func (f *fakeRepo) ListParticipantsForTaskAnyStep(_ context.Context, _ string) ([]*models.WorkflowStepParticipant, error) {
	return f.taskParticipants, f.listErr
}
func (f *fakeRepo) ListStepDecisions(_ context.Context, _, _ string) ([]*models.WorkflowStepDecision, error) {
	return f.decisions, f.listErr
}
func (f *fakeRepo) RecordStepDecision(_ context.Context, d *models.WorkflowStepDecision) error {
	if f.recordErr != nil {
		return f.recordErr
	}
	f.recorded = d
	return nil
}
func (f *fakeRepo) ClearStepDecisions(_ context.Context, _, _ string) (int64, error) {
	if f.clearErr != nil {
		return 0, f.clearErr
	}
	f.cleared = true
	return int64(len(f.decisions)), nil
}
func (f *fakeRepo) ResolveCurrentRunner(_ context.Context, stepID, taskID string) (string, error) {
	f.resolvedStepID = stepID
	f.resolvedTaskID = taskID
	return f.runner, f.runnerErr
}
func (f *fakeRepo) GetTaskWorkflowStepID(_ context.Context, taskID string) (string, error) {
	f.lookedUpTaskID = taskID
	return f.taskStepID, f.stepErr
}
func (f *fakeRepo) HasRoleSeatForTaskWorkflow(_ context.Context, _, _, _ string) (bool, error) {
	f.hasRoleSeatCalled = true
	return f.hasRoleSeat, f.hasRoleSeatErr
}
func (f *fakeRepo) EnsureRoleSeat(
	_ context.Context, workflowID, stepID, taskID, role, agentProfileID string,
) (*models.WorkflowStepParticipant, bool, error) {
	f.ensureSeatArgs = []string{workflowID, stepID, taskID, role, agentProfileID}
	if f.ensureSeatErr != nil {
		return nil, false, f.ensureSeatErr
	}
	return f.ensuredSeat, true, nil
}

func TestParticipantAdapter_TranslatesModelToEngineInfo(t *testing.T) {
	repo := &fakeRepo{
		participants: []*models.WorkflowStepParticipant{
			{
				ID: "p-1", StepID: "s-1",
				Role: models.ParticipantRoleApprover, AgentProfileID: "ap-1",
				DecisionRequired: true, Position: 0,
			},
		},
	}
	a := NewParticipantAdapter(repo)
	got, err := a.ListStepParticipants(context.Background(), "s-1", "task-1")
	if err != nil {
		t.Fatalf("ListStepParticipants: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Role != "approver" {
		t.Errorf("role = %q, want approver", got[0].Role)
	}
	if !got[0].DecisionRequired {
		t.Error("decision_required must round-trip true")
	}
}

func TestParticipantAdapter_PropagatesError(t *testing.T) {
	repo := &fakeRepo{listErr: errors.New("boom")}
	a := NewParticipantAdapter(repo)
	if _, err := a.ListStepParticipants(context.Background(), "s", "t"); err == nil {
		t.Fatal("expected error")
	}
}

func TestParticipantAdapter_ListTaskParticipants_TranslatesModelToEngineInfo(t *testing.T) {
	repo := &fakeRepo{
		taskParticipants: []*models.WorkflowStepParticipant{
			{
				ID: "p-1", StepID: "s-other", TaskID: "task-1",
				Role: models.ParticipantRoleReviewer, AgentProfileID: "rev-A",
				DecisionRequired: true, Position: 1,
			},
		},
	}
	a := NewParticipantAdapter(repo)
	got, err := a.ListTaskParticipants(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("ListTaskParticipants: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].StepID != "s-other" || got[0].TaskID != "task-1" || got[0].Role != "reviewer" {
		t.Fatalf("unexpected translation: %+v", got[0])
	}
}

func TestParticipantAdapter_ListTaskParticipants_PropagatesError(t *testing.T) {
	repo := &fakeRepo{listErr: errors.New("boom")}
	a := NewParticipantAdapter(repo)
	if _, err := a.ListTaskParticipants(context.Background(), "task-1"); err == nil {
		t.Fatal("expected error")
	}
}

func TestDecisionAdapter_RecordTranslatesEngineInfoToModel(t *testing.T) {
	repo := &fakeRepo{}
	a := NewDecisionAdapter(repo)
	err := a.RecordStepDecision(context.Background(), engine.DecisionInfo{
		ID: "d-1", TaskID: "t-1", StepID: "s-1",
		ParticipantID: "p-1", Decision: "approved", Note: "lgtm",
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if repo.recorded == nil {
		t.Fatal("repo did not receive the decision")
	}
	if repo.recorded.Decision != "approved" {
		t.Errorf("decision verdict = %q", repo.recorded.Decision)
	}
}

func TestDecisionAdapter_RecordCarriesDeciderIdentityAndComment(t *testing.T) {
	repo := &fakeRepo{}
	a := NewDecisionAdapter(repo)
	err := a.RecordStepDecision(context.Background(), engine.DecisionInfo{
		ID: "d-1", TaskID: "t-1", StepID: "s-1",
		ParticipantID: "p-1", Decision: "approved",
		DeciderType: "agent", DeciderID: "rev-A", Role: "reviewer", Comment: "looks good",
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if repo.recorded.DeciderType != "agent" || repo.recorded.DeciderID != "rev-A" ||
		repo.recorded.Role != "reviewer" || repo.recorded.Comment != "looks good" {
		t.Fatalf("decider identity/comment not carried through: %+v", repo.recorded)
	}
}

func TestDecisionAdapter_List_TranslatesAllRows(t *testing.T) {
	repo := &fakeRepo{
		decisions: []*models.WorkflowStepDecision{
			{ID: "d-1", Decision: "approved"},
			{ID: "d-2", Decision: "rejected"},
		},
	}
	a := NewDecisionAdapter(repo)
	got, err := a.ListStepDecisions(context.Background(), "t", "s")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
}

func TestDecisionAdapter_Record_CarriesDecidedAt(t *testing.T) {
	repo := &fakeRepo{}
	a := NewDecisionAdapter(repo)
	decidedAt := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	err := a.RecordStepDecision(context.Background(), engine.DecisionInfo{
		ID: "d-1", TaskID: "t-1", StepID: "s-1",
		ParticipantID: "p-1", Decision: "approved", DecidedAt: decidedAt,
	})
	if err != nil {
		t.Fatalf("record: %v", err)
	}
	if !repo.recorded.DecidedAt.Equal(decidedAt) {
		t.Fatalf("decided_at not carried through: got %v, want %v", repo.recorded.DecidedAt, decidedAt)
	}
}

func TestDecisionAdapter_List_CarriesDecidedAt(t *testing.T) {
	decidedAt := time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)
	repo := &fakeRepo{
		decisions: []*models.WorkflowStepDecision{
			{ID: "d-1", Decision: "approved", DecidedAt: decidedAt},
		},
	}
	a := NewDecisionAdapter(repo)
	got, err := a.ListStepDecisions(context.Background(), "t", "s")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 || !got[0].DecidedAt.Equal(decidedAt) {
		t.Fatalf("decided_at not carried through: %+v", got)
	}
}

func TestDecisionAdapter_List_CarriesDeciderIdentityAndComment(t *testing.T) {
	repo := &fakeRepo{
		decisions: []*models.WorkflowStepDecision{
			{ID: "d-1", Decision: "rejected", DeciderType: "user", DeciderID: "user-1", Role: "reviewer", Comment: "needs work"},
		},
	}
	a := NewDecisionAdapter(repo)
	got, err := a.ListStepDecisions(context.Background(), "t", "s")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].DeciderType != "user" || got[0].DeciderID != "user-1" || got[0].Role != "reviewer" || got[0].Comment != "needs work" {
		t.Fatalf("decider identity/comment not carried through: %+v", got[0])
	}
}

func TestDecisionAdapter_ClearForwards(t *testing.T) {
	repo := &fakeRepo{decisions: []*models.WorkflowStepDecision{{ID: "d-1"}}}
	a := NewDecisionAdapter(repo)
	n, err := a.ClearStepDecisions(context.Background(), "t", "s")
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if !repo.cleared {
		t.Error("repo.ClearStepDecisions not invoked")
	}
	if n != 1 {
		t.Errorf("rows cleared = %d, want 1", n)
	}
}

func TestPrimaryAgentAdapter_ReturnsCurrentRunner(t *testing.T) {
	repo := &fakeRepo{runner: "ap-runner"}
	a := NewPrimaryAgentAdapter(repo)
	got, err := a.PrimaryAgentProfileID(context.Background(), "s-1", "task-1")
	if err != nil {
		t.Fatalf("primary: %v", err)
	}
	if got != "ap-runner" {
		t.Errorf("primary = %q, want ap-runner", got)
	}
	if repo.resolvedStepID != "s-1" || repo.resolvedTaskID != "task-1" {
		t.Errorf("ResolveCurrentRunner called with step=%q task=%q", repo.resolvedStepID, repo.resolvedTaskID)
	}
}

func TestPrimaryAgentAdapter_PropagatesResolveCurrentRunnerError(t *testing.T) {
	repo := &fakeRepo{runnerErr: errors.New("missing")}
	a := NewPrimaryAgentAdapter(repo)
	if _, err := a.PrimaryAgentProfileID(context.Background(), "s-x", "task-x"); err == nil {
		t.Fatal("expected error")
	}
}
