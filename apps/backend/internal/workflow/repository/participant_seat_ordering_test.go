package repository

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/workflow/models"
)

// Regression coverage for AC-OFFICE-REVIEW-SEATS-001.10: when two seats in
// the same role carry the same position, the tiebreak is agent profile
// identifier ascending, not insertion order (row id). Each case pins an
// explicit row id that sorts *opposite* to the desired agent_profile_id
// order — "id-2" (profile-a) is inserted after, but must sort before,
// "id-1" (profile-z) — so a query that still tiebreaks on id (even
// partially, as a secondary sort after id) would return them in the wrong
// order and fail deterministically. Row ids are normally random UUIDs
// (UpsertStepParticipant only generates one when p.ID is empty), so a test
// relying on natural insertion order would pass or fail by chance.

func mustUpsertSeatOrderingParticipant(
	t *testing.T, repo *Repository, id, stepID, taskID string, role models.ParticipantRole, profile string, pos int,
) {
	t.Helper()
	if err := repo.UpsertStepParticipant(context.Background(), &models.WorkflowStepParticipant{
		ID: id, StepID: stepID, TaskID: taskID, Role: role, AgentProfileID: profile, Position: pos,
	}); err != nil {
		t.Fatalf("upsert participant: %v", err)
	}
}

func TestListStepParticipantsForTask_SamePositionTiebreaksOnAgentProfileID(t *testing.T) {
	repo := setupTestRepo(t)
	step := newPhase2TestStep(t, repo, "Review")

	mustUpsertSeatOrderingParticipant(t, repo, "id-1", step.ID, "task-1", models.ParticipantRoleReviewer, "profile-z", 0)
	mustUpsertSeatOrderingParticipant(t, repo, "id-2", step.ID, "task-1", models.ParticipantRoleReviewer, "profile-a", 0)

	got, err := repo.ListStepParticipantsForTask(context.Background(), step.ID, "task-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 participants, got %d: %+v", len(got), got)
	}
	if got[0].AgentProfileID != "profile-a" || got[1].AgentProfileID != "profile-z" {
		t.Fatalf("expected agent_profile_id ascending order, got %s, %s", got[0].AgentProfileID, got[1].AgentProfileID)
	}
}

func TestListParticipantsForTaskAnyStep_SamePositionTiebreaksOnAgentProfileID(t *testing.T) {
	repo := setupTestRepo(t)
	step := newPhase2TestStep(t, repo, "Review")

	mustUpsertSeatOrderingParticipant(t, repo, "id-1", step.ID, "task-1", models.ParticipantRoleReviewer, "profile-z", 0)
	mustUpsertSeatOrderingParticipant(t, repo, "id-2", step.ID, "task-1", models.ParticipantRoleReviewer, "profile-a", 0)

	got, err := repo.ListParticipantsForTaskAnyStep(context.Background(), "task-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 participants, got %d: %+v", len(got), got)
	}
	if got[0].AgentProfileID != "profile-a" || got[1].AgentProfileID != "profile-z" {
		t.Fatalf("expected agent_profile_id ascending order, got %s, %s", got[0].AgentProfileID, got[1].AgentProfileID)
	}
}

func TestListParticipantsForTaskWorkflow_SamePositionTiebreaksOnAgentProfileID(t *testing.T) {
	repo := setupTestRepo(t)
	step := newPhase2TestStep(t, repo, "Review")

	mustUpsertSeatOrderingParticipant(t, repo, "id-1", step.ID, "task-1", models.ParticipantRoleReviewer, "profile-z", 0)
	mustUpsertSeatOrderingParticipant(t, repo, "id-2", step.ID, "task-1", models.ParticipantRoleReviewer, "profile-a", 0)

	got, err := repo.ListParticipantsForTaskWorkflow(context.Background(), "task-1", "wf-test")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 participants, got %d: %+v", len(got), got)
	}
	if got[0].AgentProfileID != "profile-a" || got[1].AgentProfileID != "profile-z" {
		t.Fatalf("expected agent_profile_id ascending order, got %s, %s", got[0].AgentProfileID, got[1].AgentProfileID)
	}
}

func TestListStepParticipants_SamePositionTiebreaksOnAgentProfileID(t *testing.T) {
	repo := setupTestRepo(t)
	step := newPhase2TestStep(t, repo, "Review")

	// Template-level rows (task_id = "") — this is what ListStepParticipants returns.
	mustUpsertSeatOrderingParticipant(t, repo, "id-1", step.ID, "", models.ParticipantRoleReviewer, "profile-z", 0)
	mustUpsertSeatOrderingParticipant(t, repo, "id-2", step.ID, "", models.ParticipantRoleReviewer, "profile-a", 0)

	got, err := repo.ListStepParticipants(context.Background(), step.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 participants, got %d: %+v", len(got), got)
	}
	if got[0].AgentProfileID != "profile-a" || got[1].AgentProfileID != "profile-z" {
		t.Fatalf("expected agent_profile_id ascending order, got %s, %s", got[0].AgentProfileID, got[1].AgentProfileID)
	}
}
