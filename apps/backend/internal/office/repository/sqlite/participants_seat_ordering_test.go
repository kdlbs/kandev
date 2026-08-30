package sqlite_test

import (
	"context"
	"testing"
)

// Regression coverage for AC-OFFICE-REVIEW-SEATS-001.10: when two seats in
// the same role carry the same position, the tiebreak is agent profile
// identifier ascending, not insertion order (row id). Each case pins an
// explicit row id that sorts *opposite* to the desired agent_profile_id
// order — "id-2" (profile-a) is inserted after, but must sort before,
// "id-1" (profile-z) — so a query that still tiebreaks on id would return
// them in the wrong order and fail deterministically, mirroring
// internal/workflow/repository/participant_seat_ordering_test.go's pattern.

func TestListTaskParticipants_SamePositionTiebreaksOnAgentProfileID(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "sp-1", "step-order")
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES
			('id-1', 'step-order', 'sp-1', 'reviewer', 'profile-z', 1, 0),
			('id-2', 'step-order', 'sp-1', 'reviewer', 'profile-a', 1, 0)
	`); err != nil {
		t.Fatalf("seed participants: %v", err)
	}

	got, err := repo.ListTaskParticipants(ctx, "sp-1", "reviewer")
	if err != nil {
		t.Fatalf("ListTaskParticipants: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 participants, got %d: %+v", len(got), got)
	}
	if got[0].AgentProfileID != "profile-a" || got[1].AgentProfileID != "profile-z" {
		t.Fatalf("expected agent_profile_id ascending order, got %s, %s", got[0].AgentProfileID, got[1].AgentProfileID)
	}
}

func TestListAllTaskParticipants_SamePositionTiebreaksOnAgentProfileID(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	seedParticipantTask(t, repo, "sp-2", "step-order-all")
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES
			('id-1', 'step-order-all', 'sp-2', 'reviewer', 'profile-z', 1, 0),
			('id-2', 'step-order-all', 'sp-2', 'reviewer', 'profile-a', 1, 0)
	`); err != nil {
		t.Fatalf("seed participants: %v", err)
	}

	got, err := repo.ListAllTaskParticipants(ctx, "sp-2")
	if err != nil {
		t.Fatalf("ListAllTaskParticipants: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 participants, got %d: %+v", len(got), got)
	}
	if got[0].AgentProfileID != "profile-a" || got[1].AgentProfileID != "profile-z" {
		t.Fatalf("expected agent_profile_id ascending order, got %s, %s", got[0].AgentProfileID, got[1].AgentProfileID)
	}
}
