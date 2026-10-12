package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

// atomicRoutine builds a valid routine fixture for the atomic create tests.
func atomicRoutine(workspaceID, name string) *models.Routine {
	return &models.Routine{
		WorkspaceID:       workspaceID,
		Name:              name,
		TaskTemplate:      "{}",
		Status:            "active",
		ConcurrencyPolicy: "skip_if_active",
		Variables:         "{}",
	}
}

// CreateRoutineWithTrigger commits the routine and its trigger together.
func TestCreateRoutineWithTrigger_CommitsBoth(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	routine := atomicRoutine("ws-atomic", "Atomic")
	trigger := &models.RoutineTrigger{
		Kind:           "cron",
		CronExpression: "0 9 * * *",
		Timezone:       "UTC",
		Enabled:        true,
	}
	if err := repo.CreateRoutineWithTrigger(ctx, routine, trigger); err != nil {
		t.Fatalf("create routine with trigger: %v", err)
	}
	if trigger.RoutineID != routine.ID {
		t.Fatalf("trigger routine id = %q, want %q", trigger.RoutineID, routine.ID)
	}

	routines, err := repo.ListRoutines(ctx, "ws-atomic")
	if err != nil {
		t.Fatalf("list routines: %v", err)
	}
	if len(routines) != 1 {
		t.Fatalf("routine count = %d, want 1", len(routines))
	}
	triggers, err := repo.ListTriggersByRoutineID(ctx, routine.ID)
	if err != nil {
		t.Fatalf("list triggers: %v", err)
	}
	if len(triggers) != 1 {
		t.Fatalf("trigger count = %d, want 1", len(triggers))
	}
}

// A failed trigger insert rolls the routine back: no schedule-less routine
// survives (REQ-OFFICE-ROUTINE-WIRE-002.10, atomic create).
func TestCreateRoutineWithTrigger_RollsBackRoutineWhenTriggerFails(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// Occupy a trigger id so the next insert collides on the primary key.
	existing := atomicRoutine("ws-other", "Existing")
	if err := repo.CreateRoutine(ctx, existing); err != nil {
		t.Fatalf("create existing routine: %v", err)
	}
	dup := &models.RoutineTrigger{
		RoutineID: existing.ID,
		Kind:      "cron",
		Enabled:   true,
	}
	if err := repo.CreateRoutineTrigger(ctx, dup); err != nil {
		t.Fatalf("create existing trigger: %v", err)
	}

	routine := atomicRoutine("ws-atomic", "Atomic")
	trigger := &models.RoutineTrigger{
		ID:             dup.ID,
		Kind:           "cron",
		CronExpression: "0 9 * * *",
		Timezone:       "UTC",
		Enabled:        true,
	}
	if err := repo.CreateRoutineWithTrigger(ctx, routine, trigger); err == nil {
		t.Fatal("expected trigger insert to fail on duplicate id")
	}

	routines, err := repo.ListRoutines(ctx, "ws-atomic")
	if err != nil {
		t.Fatalf("list routines: %v", err)
	}
	if len(routines) != 0 {
		t.Fatalf("routine survived rollback: count = %d, want 0", len(routines))
	}
}
