package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// AC-004.15 / Claim-instant terminology (Review round 3, R3-3): a
// trigger that has never fired (last_fired_at IS NULL, e.g. a
// malformed cron_expression that never got a next_run_at) is still
// claiming, not stranded, until strandedGrace has elapsed since its
// own created_at — the spec's claim instant is
// COALESCE(last_fired_at, created_at), not last_fired_at alone. The
// old predicate treated any never-fired trigger as immediately
// eligible for stranded, regardless of how recently it was created.
func TestListOverdueOrStrandedTriggers_NeverFiredTriggerNotStrandedBeforeCreatedGrace(t *testing.T) {
	repo, _ := newTestRepoWithDB(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	routine := seedCronTrigger(t, repo, "ws-a", "NeverFired")
	trigger := &models.RoutineTrigger{
		RoutineID: routine.ID, Kind: "cron", CronExpression: "",
		Timezone: "UTC", Enabled: true, NextRunAt: nil,
	}
	if err := repo.CreateRoutineTrigger(ctx, trigger); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	// Created 1 minute before now: well inside the 5-minute stranded
	// grace, never fired, never armed with a next_run_at.
	createdAt := now.Add(-1 * time.Minute)
	if _, err := repo.ExecRaw(ctx,
		`UPDATE office_routine_triggers SET next_run_at = NULL, last_fired_at = NULL, created_at = ? WHERE id = ?`,
		createdAt, trigger.ID); err != nil {
		t.Fatalf("seed never-fired state: %v", err)
	}

	rows, total, err := repo.ListOverdueOrStrandedTriggers(
		ctx, "ws-a", now, 3*time.Minute, 5*time.Minute, 50,
	)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 0 || len(rows) != 0 {
		t.Fatalf("got %d rows, total %d, want 0/0 (a trigger created 1m ago is still claiming, not stranded)", len(rows), total)
	}
}

func TestListOverdueOrStrandedTriggers_NeverFiredTriggerStrandedAfterCreatedGrace(t *testing.T) {
	repo, _ := newTestRepoWithDB(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	routine := seedCronTrigger(t, repo, "ws-a", "NeverFiredStale")
	trigger := &models.RoutineTrigger{
		RoutineID: routine.ID, Kind: "cron", CronExpression: "",
		Timezone: "UTC", Enabled: true, NextRunAt: nil,
	}
	if err := repo.CreateRoutineTrigger(ctx, trigger); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	// Created 6 minutes before now: past the 5-minute stranded grace,
	// never fired, never armed.
	createdAt := now.Add(-6 * time.Minute)
	if _, err := repo.ExecRaw(ctx,
		`UPDATE office_routine_triggers SET next_run_at = NULL, last_fired_at = NULL, created_at = ? WHERE id = ?`,
		createdAt, trigger.ID); err != nil {
		t.Fatalf("seed never-fired stale state: %v", err)
	}

	rows, total, err := repo.ListOverdueOrStrandedTriggers(
		ctx, "ws-a", now, 3*time.Minute, 5*time.Minute, 50,
	)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("got %d rows, total %d, want 1/1 (a trigger created 6m ago that never fired must be stranded)", len(rows), total)
	}
	if rows[0].Condition != "stranded" {
		t.Errorf("condition = %q, want stranded", rows[0].Condition)
	}
}
