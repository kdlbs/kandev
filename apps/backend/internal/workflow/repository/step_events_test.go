package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/workflow/models"
)

func seatEventsStep(id, name string) *models.WorkflowStep {
	return &models.WorkflowStep{
		ID: id, WorkflowID: "wf-test", Name: name, Position: 0, Color: "yellow",
		Events: models.StepEvents{OnEnter: []models.OnEnterAction{
			{Type: models.OnEnterEnsureParticipantSeat, Config: map[string]interface{}{"role": "reviewer"}},
		}},
	}
}

func excludeOn(step *models.WorkflowStep) models.StepEvents {
	events, _ := models.SetExcludeAuthorProvider(step.Events, "reviewer", true)
	return events
}

func reviewerExcluded(t *testing.T, repo *Repository, id string) bool {
	t.Helper()
	stored, err := repo.GetStep(context.Background(), id)
	if err != nil {
		t.Fatalf("GetStep(%s): %v", id, err)
	}
	return models.ParticipantSeatRoles(stored.Events)["reviewer"]
}

func TestUpdateStepEventsIfUnchanged_WritesEventsOnlyAndReturnsNewVersion(t *testing.T) {
	exerciseStepEventsWrite(t, setupTestRepo(t))
}

func TestUpdateStepEventsIfUnchanged_StaleStepWritesNothing(t *testing.T) {
	exerciseStaleStepEventsWrite(t, setupTestRepo(t))
}

func exerciseStepEventsWrite(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	step := seatEventsStep("step-a", "Review")
	if err := repo.CreateStep(ctx, step); err != nil {
		t.Fatalf("CreateStep: %v", err)
	}
	read, err := repo.GetStep(ctx, "step-a")
	if err != nil {
		t.Fatalf("GetStep: %v", err)
	}

	saved, err := repo.UpdateStepEventsIfUnchanged(ctx, []StepEventsUpdate{
		{StepID: read.ID, WorkflowID: read.WorkflowID, Events: excludeOn(read), ExpectedUpdatedAt: read.UpdatedAt},
	})
	if err != nil {
		t.Fatalf("UpdateStepEventsIfUnchanged: %v", err)
	}
	if !reviewerExcluded(t, repo, "step-a") {
		t.Fatal("events were not written")
	}
	stored, _ := repo.GetStep(ctx, "step-a")
	if stored.Name != "Review" || stored.Position != 0 {
		t.Fatalf("other fields changed: %+v", stored)
	}
	if len(saved) != 1 || !saved[0].UpdatedAt.Equal(stored.UpdatedAt) {
		t.Fatalf("returned version %v, stored %v", saved, stored.UpdatedAt)
	}
}

func exerciseStaleStepEventsWrite(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	for _, s := range []*models.WorkflowStep{seatEventsStep("step-a", "A"), seatEventsStep("step-b", "B")} {
		if err := repo.CreateStep(ctx, s); err != nil {
			t.Fatalf("CreateStep: %v", err)
		}
	}
	a, _ := repo.GetStep(ctx, "step-a")
	b, _ := repo.GetStep(ctx, "step-b")

	// Another writer changes step-b after it was read.
	concurrent := *b
	concurrent.Name = "B renamed"
	if err := repo.UpdateStep(ctx, &concurrent); err != nil {
		t.Fatalf("UpdateStep: %v", err)
	}

	_, err := repo.UpdateStepEventsIfUnchanged(ctx, []StepEventsUpdate{
		{StepID: a.ID, WorkflowID: a.WorkflowID, Events: excludeOn(a), ExpectedUpdatedAt: a.UpdatedAt},
		{StepID: b.ID, WorkflowID: b.WorkflowID, Events: excludeOn(b), ExpectedUpdatedAt: b.UpdatedAt},
	})
	if !errors.Is(err, models.ErrWorkflowStepChanged) {
		t.Fatalf("err = %v, want ErrWorkflowStepChanged", err)
	}
	if reviewerExcluded(t, repo, "step-a") || reviewerExcluded(t, repo, "step-b") {
		t.Fatal("a batch with a stale step must write nothing")
	}
	stored, _ := repo.GetStep(ctx, "step-b")
	if stored.Name != "B renamed" {
		t.Fatalf("concurrent change lost: %q", stored.Name)
	}
}
