package adapters

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/workflow/engine"
	"github.com/kandev/kandev/internal/workflow/models"
)

// fakeRepo is a minimal in-memory WorkflowRepo. The adapters delegate
// straight to the repo so we only need stub responses to verify the
// type translation between models / engine packages.
type fakeRepo struct {
	participants []*models.WorkflowStepParticipant
	decisions    []*models.WorkflowStepDecision
	step         *models.WorkflowStep

	listErr   error
	recordErr error
	clearErr  error
	getErr    error

	recorded *models.WorkflowStepDecision
	cleared  bool
}

func (f *fakeRepo) ListStepParticipantsForTask(_ context.Context, _, _ string) ([]*models.WorkflowStepParticipant, error) {
	return f.participants, f.listErr
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
func (f *fakeRepo) GetStep(_ context.Context, _ string) (*models.WorkflowStep, error) {
	return f.step, f.getErr
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

func TestPrimaryAgentAdapter_ReturnsStepAgentProfileID(t *testing.T) {
	repo := &fakeRepo{step: &models.WorkflowStep{ID: "s-1", AgentProfileID: "ap-primary"}}
	a := NewPrimaryAgentAdapter(repo)
	got, err := a.PrimaryAgentProfileID(context.Background(), "s-1")
	if err != nil {
		t.Fatalf("primary: %v", err)
	}
	if got != "ap-primary" {
		t.Errorf("primary = %q, want ap-primary", got)
	}
}

func TestPrimaryAgentAdapter_PropagatesGetStepError(t *testing.T) {
	repo := &fakeRepo{getErr: errors.New("missing")}
	a := NewPrimaryAgentAdapter(repo)
	if _, err := a.PrimaryAgentProfileID(context.Background(), "s-x"); err == nil {
		t.Fatal("expected error")
	}
}
