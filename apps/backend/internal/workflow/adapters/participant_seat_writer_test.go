package adapters

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/workflow/models"
)

func TestParticipantSeatWriterAdapter_HasRoleSeatForTaskWorkflow(t *testing.T) {
	repo := &fakeRepo{hasRoleSeat: true}
	a := NewParticipantSeatWriterAdapter(repo)

	got, err := a.HasRoleSeatForTaskWorkflow(context.Background(), "wf-1", "task-1", "reviewer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Fatalf("expected true, got false")
	}
	if !repo.hasRoleSeatCalled {
		t.Fatalf("expected the repo method to be called")
	}
}

func TestParticipantSeatWriterAdapter_HasRoleSeatForTaskWorkflow_WrapsError(t *testing.T) {
	repo := &fakeRepo{hasRoleSeatErr: errors.New("boom")}
	a := NewParticipantSeatWriterAdapter(repo)

	_, err := a.HasRoleSeatForTaskWorkflow(context.Background(), "wf-1", "task-1", "reviewer")
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestParticipantSeatWriterAdapter_EnsureRoleSeat_TranslatesModelToEngineInfo(t *testing.T) {
	repo := &fakeRepo{
		ensuredSeat: &models.WorkflowStepParticipant{
			ID:               "p-1",
			StepID:           "step-1",
			TaskID:           "task-1",
			Role:             models.ParticipantRoleReviewer,
			AgentProfileID:   "agent-1",
			DecisionRequired: true,
			Position:         0,
		},
	}
	a := NewParticipantSeatWriterAdapter(repo)

	got, err := a.EnsureRoleSeat(context.Background(), "wf-1", "step-1", "task-1", "reviewer", "agent-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "p-1" || got.Role != "reviewer" || got.AgentProfileID != "agent-1" || !got.DecisionRequired {
		t.Fatalf("unexpected translation: %+v", got)
	}
	want := []string{"wf-1", "step-1", "task-1", "reviewer", "agent-1"}
	if len(repo.ensureSeatArgs) != len(want) {
		t.Fatalf("unexpected args: %v", repo.ensureSeatArgs)
	}
	for i, v := range want {
		if repo.ensureSeatArgs[i] != v {
			t.Fatalf("arg %d = %q, want %q (all args: %v)", i, repo.ensureSeatArgs[i], v, repo.ensureSeatArgs)
		}
	}
}

func TestParticipantSeatWriterAdapter_EnsureRoleSeat_WrapsError(t *testing.T) {
	repo := &fakeRepo{ensureSeatErr: errors.New("boom")}
	a := NewParticipantSeatWriterAdapter(repo)

	_, err := a.EnsureRoleSeat(context.Background(), "wf-1", "step-1", "task-1", "reviewer", "agent-1")
	if err == nil {
		t.Fatalf("expected error")
	}
}
