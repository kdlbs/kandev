package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// TestAdvanceTriggerWithoutFiring_AdvancesCursor covers the repository half
// of REQ-OFFICE-ROUTINE-STATUS-002: a suppressed slot's cursor moves without
// stamping any fire evidence (last_fired_at stays nil, matching
// AC-OFFICE-ROUTINE-STATUS-001.7 — this is why ClaimTrigger cannot be
// reused for suppression).
func TestAdvanceTriggerWithoutFiring_AdvancesCursor(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID:       "ws-1",
		Name:              "Advance Test",
		TaskTemplate:      "{}",
		Status:            "paused",
		ConcurrencyPolicy: "skip_if_active",
		Variables:         "{}",
	}
	if err := repo.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}

	oldNext := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	trigger := &models.RoutineTrigger{
		RoutineID:      routine.ID,
		Kind:           "cron",
		CronExpression: "0 9 * * *",
		Timezone:       "UTC",
		NextRunAt:      &oldNext,
		Enabled:        true,
	}
	if err := repo.CreateRoutineTrigger(ctx, trigger); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	newNext := time.Date(2026, 5, 2, 9, 0, 0, 0, time.UTC)
	advanced, err := repo.AdvanceTriggerWithoutFiring(ctx, trigger.ID, oldNext, newNext)
	if err != nil {
		t.Fatalf("advance: %v", err)
	}
	if !advanced {
		t.Fatal("expected the compare-and-set to succeed")
	}

	due, err := repo.GetDueTriggers(ctx, newNext.Add(time.Minute))
	if err != nil {
		t.Fatalf("get due: %v", err)
	}
	if len(due) != 1 {
		t.Fatalf("expected 1 due trigger after advance, got %d", len(due))
	}
	got := due[0]
	if got.NextRunAt == nil || !got.NextRunAt.Equal(newNext) {
		t.Errorf("next_run_at = %v, want %v", got.NextRunAt, newNext)
	}
	if got.LastFiredAt != nil {
		t.Errorf("last_fired_at = %v, want nil — suppression must not record a fire", got.LastFiredAt)
	}
}

// TestAdvanceTriggerWithoutFiring_LosesRaceWhenCursorAlreadyMoved covers
// AC-OFFICE-ROUTINE-STATUS-002.6/-002.7: the compare-and-set predicate is
// the same shape as ClaimTrigger's, so a stale oldNextRunAt changes no rows.
func TestAdvanceTriggerWithoutFiring_LosesRaceWhenCursorAlreadyMoved(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID:       "ws-1",
		Name:              "Race Test",
		TaskTemplate:      "{}",
		Status:            "paused",
		ConcurrencyPolicy: "skip_if_active",
		Variables:         "{}",
	}
	if err := repo.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}

	oldNext := time.Date(2026, 5, 1, 9, 0, 0, 0, time.UTC)
	trigger := &models.RoutineTrigger{
		RoutineID:      routine.ID,
		Kind:           "cron",
		CronExpression: "0 9 * * *",
		Timezone:       "UTC",
		NextRunAt:      &oldNext,
		Enabled:        true,
	}
	if err := repo.CreateRoutineTrigger(ctx, trigger); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	newNext := time.Date(2026, 5, 2, 9, 0, 0, 0, time.UTC)
	first, err := repo.AdvanceTriggerWithoutFiring(ctx, trigger.ID, oldNext, newNext)
	if err != nil || !first {
		t.Fatalf("first advance: advanced=%v, err=%v", first, err)
	}

	// A second evaluation racing on the stale cursor must lose: no rows
	// change, no error.
	second, err := repo.AdvanceTriggerWithoutFiring(ctx, trigger.ID, oldNext, newNext.AddDate(0, 0, 1))
	if err != nil {
		t.Fatalf("second advance err: %v", err)
	}
	if second {
		t.Error("second advance should have lost the compare-and-set")
	}
}
