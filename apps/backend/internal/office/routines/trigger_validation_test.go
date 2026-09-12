package routines_test

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/office/routines"
)

// TestCreateRoutineTrigger_EmptyCronExpression_Rejected verifies a cron
// trigger with no expression is rejected at create time instead of
// persisting as a trigger that silently never fires.
func TestCreateRoutineTrigger_EmptyCronExpression_Rejected(t *testing.T) {
	svc := newTestRoutineService(t)
	routine := createTestRoutine(t, svc, "empty-cron", "skip_if_active")

	trigger := &routines.RoutineTrigger{RoutineID: routine.ID, Kind: "cron", CronExpression: ""}
	err := svc.CreateRoutineTrigger(context.Background(), trigger)
	if err == nil {
		t.Fatal("expected error for empty cron expression")
	}
	if !errors.Is(err, routines.ErrInvalidTrigger) {
		t.Errorf("expected ErrInvalidTrigger, got %v", err)
	}
}

// TestCreateRoutineTrigger_UnsatisfiableCron_Rejected verifies an impossible
// expression (Feb 30th) is rejected at create time instead of becoming a
// silent daily-at-midnight job.
func TestCreateRoutineTrigger_UnsatisfiableCron_Rejected(t *testing.T) {
	svc := newTestRoutineService(t)
	routine := createTestRoutine(t, svc, "unsatisfiable-cron", "skip_if_active")

	trigger := &routines.RoutineTrigger{RoutineID: routine.ID, Kind: "cron", CronExpression: "0 0 30 2 *"}
	err := svc.CreateRoutineTrigger(context.Background(), trigger)
	if err == nil {
		t.Fatal("expected error for unsatisfiable cron expression")
	}
	if !errors.Is(err, routines.ErrInvalidTrigger) {
		t.Errorf("expected ErrInvalidTrigger, got %v", err)
	}
}

// TestCreateRoutineTrigger_EmptyTimezone_NormalizedToUTC verifies an empty
// timezone is made explicit at write time rather than persisted as ”.
func TestCreateRoutineTrigger_EmptyTimezone_NormalizedToUTC(t *testing.T) {
	svc := newTestRoutineService(t)
	routine := createTestRoutine(t, svc, "empty-tz", "skip_if_active")

	trigger := &routines.RoutineTrigger{RoutineID: routine.ID, Kind: "cron", CronExpression: "0 9 * * *"}
	if err := svc.CreateRoutineTrigger(context.Background(), trigger); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	if trigger.Timezone != "UTC" {
		t.Errorf("Timezone = %q, want %q", trigger.Timezone, "UTC")
	}

	triggers, err := svc.ListRoutineTriggers(context.Background(), routine.ID)
	if err != nil {
		t.Fatalf("list triggers: %v", err)
	}
	if len(triggers) != 1 || triggers[0].Timezone != "UTC" {
		t.Fatalf("persisted triggers = %+v, want one trigger with Timezone=UTC", triggers)
	}
}

// TestCreateRoutineTrigger_ValidCron_Succeeds is the control case: a
// well-formed cron trigger still creates successfully.
func TestCreateRoutineTrigger_ValidCron_Succeeds(t *testing.T) {
	svc := newTestRoutineService(t)
	routine := createTestRoutine(t, svc, "valid-cron", "skip_if_active")

	trigger := &routines.RoutineTrigger{RoutineID: routine.ID, Kind: "cron", CronExpression: "*/5 * * * *"}
	if err := svc.CreateRoutineTrigger(context.Background(), trigger); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	if trigger.NextRunAt == nil {
		t.Error("expected NextRunAt to be set")
	}
}
