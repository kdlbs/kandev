package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// seedCronTrigger creates a routine and one cron trigger for it,
// returning the trigger id. next_run_at/last_fired_at are set with a
// raw UPDATE afterward since CreateRoutineTrigger only accepts a
// caller-supplied NextRunAt at creation and never a NULL/backdated
// LastFiredAt.
func seedCronTrigger(t *testing.T, repo interface {
	CreateRoutine(ctx context.Context, r *models.Routine) error
	CreateRoutineTrigger(ctx context.Context, tr *models.RoutineTrigger) error
}, workspaceID, name string) *models.Routine {
	t.Helper()
	routine := &models.Routine{
		WorkspaceID:       workspaceID,
		Name:              name,
		TaskTemplate:      "",
		Status:            "active",
		ConcurrencyPolicy: "always_create",
	}
	if err := repo.CreateRoutine(context.Background(), routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	return routine
}

func TestListOverdueOrStrandedTriggers_ReturnsOverdueArmedTrigger(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	routine := seedCronTrigger(t, repo, "ws-a", "Overdue")
	overdueAt := now.Add(-4 * time.Minute)
	trigger := &models.RoutineTrigger{
		RoutineID: routine.ID, Kind: "cron", CronExpression: "* * * * *",
		Timezone: "UTC", Enabled: true, NextRunAt: &overdueAt,
	}
	if err := repo.CreateRoutineTrigger(ctx, trigger); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	rows, total, err := repo.ListOverdueOrStrandedTriggers(
		ctx, "ws-a", now, 3*time.Minute, 5*time.Minute, 50,
	)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("got %d rows, total %d, want 1/1", len(rows), total)
	}
	if rows[0].Condition != "overdue" {
		t.Errorf("condition = %q, want overdue", rows[0].Condition)
	}
	if rows[0].RoutineName != "Overdue" {
		t.Errorf("routine_name = %q, want Overdue", rows[0].RoutineName)
	}
	_ = db
}

func TestListOverdueOrStrandedTriggers_ReturnsStrandedClaimedTrigger(t *testing.T) {
	repo, _ := newTestRepoWithDB(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	routine := seedCronTrigger(t, repo, "ws-a", "Stranded")
	future := now.Add(time.Minute)
	trigger := &models.RoutineTrigger{
		RoutineID: routine.ID, Kind: "cron", CronExpression: "* * * * *",
		Timezone: "UTC", Enabled: true, NextRunAt: &future,
	}
	if err := repo.CreateRoutineTrigger(ctx, trigger); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	// Claim it: next_run_at -> NULL, last_fired_at -> 6 minutes ago,
	// past the 5-minute stranded grace.
	claimedAt := now.Add(-6 * time.Minute)
	if _, err := repo.ExecRaw(ctx,
		`UPDATE office_routine_triggers SET next_run_at = NULL, last_fired_at = ? WHERE id = ?`,
		claimedAt, trigger.ID); err != nil {
		t.Fatalf("seed stranded state: %v", err)
	}

	rows, total, err := repo.ListOverdueOrStrandedTriggers(
		ctx, "ws-a", now, 3*time.Minute, 5*time.Minute, 50,
	)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("got %d rows, total %d, want 1/1", len(rows), total)
	}
	if rows[0].Condition != "stranded" {
		t.Errorf("condition = %q, want stranded", rows[0].Condition)
	}
}

// AC-004.15: a trigger claimed less than strandedGrace ago is a
// claiming trigger — excluded from the evidence list entirely, not
// given a third label.
func TestListOverdueOrStrandedTriggers_ExcludesClaimingTrigger(t *testing.T) {
	repo, _ := newTestRepoWithDB(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	routine := seedCronTrigger(t, repo, "ws-a", "Claiming")
	future := now.Add(time.Minute)
	trigger := &models.RoutineTrigger{
		RoutineID: routine.ID, Kind: "cron", CronExpression: "* * * * *",
		Timezone: "UTC", Enabled: true, NextRunAt: &future,
	}
	if err := repo.CreateRoutineTrigger(ctx, trigger); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	claimedAt := now.Add(-1 * time.Minute)
	if _, err := repo.ExecRaw(ctx,
		`UPDATE office_routine_triggers SET next_run_at = NULL, last_fired_at = ? WHERE id = ?`,
		claimedAt, trigger.ID); err != nil {
		t.Fatalf("seed claiming state: %v", err)
	}

	rows, total, err := repo.ListOverdueOrStrandedTriggers(
		ctx, "ws-a", now, 3*time.Minute, 5*time.Minute, 50,
	)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 0 || len(rows) != 0 {
		t.Fatalf("got %d rows, total %d, want 0/0 (claiming trigger must be excluded)", len(rows), total)
	}

	eligible, err := repo.CountEligibleTriggers(ctx, "ws-a")
	if err != nil {
		t.Fatalf("count eligible: %v", err)
	}
	if eligible != 1 {
		t.Errorf("eligible count = %d, want 1 (still an eligible trigger, just not overdue/stranded)", eligible)
	}
}

// Ordering: stranded (NULL next_run_at) sorts before overdue.
func TestListOverdueOrStrandedTriggers_StrandedSortsBeforeOverdue(t *testing.T) {
	repo, _ := newTestRepoWithDB(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	overdueRoutine := seedCronTrigger(t, repo, "ws-a", "Overdue")
	overdueAt := now.Add(-10 * time.Minute)
	overdueTrigger := &models.RoutineTrigger{
		RoutineID: overdueRoutine.ID, Kind: "cron", CronExpression: "* * * * *",
		Timezone: "UTC", Enabled: true, NextRunAt: &overdueAt,
	}
	if err := repo.CreateRoutineTrigger(ctx, overdueTrigger); err != nil {
		t.Fatalf("create overdue trigger: %v", err)
	}

	strandedRoutine := seedCronTrigger(t, repo, "ws-a", "Stranded")
	future := now.Add(time.Minute)
	strandedTrigger := &models.RoutineTrigger{
		RoutineID: strandedRoutine.ID, Kind: "cron", CronExpression: "* * * * *",
		Timezone: "UTC", Enabled: true, NextRunAt: &future,
	}
	if err := repo.CreateRoutineTrigger(ctx, strandedTrigger); err != nil {
		t.Fatalf("create stranded trigger: %v", err)
	}
	if _, err := repo.ExecRaw(ctx,
		`UPDATE office_routine_triggers SET next_run_at = NULL, last_fired_at = ? WHERE id = ?`,
		now.Add(-10*time.Minute), strandedTrigger.ID); err != nil {
		t.Fatalf("seed stranded state: %v", err)
	}

	rows, _, err := repo.ListOverdueOrStrandedTriggers(ctx, "ws-a", now, 3*time.Minute, 5*time.Minute, 50)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2", len(rows))
	}
	if rows[0].Condition != "stranded" || rows[1].Condition != "overdue" {
		t.Errorf("order = [%s, %s], want [stranded, overdue]", rows[0].Condition, rows[1].Condition)
	}
}

func TestListOverdueOrStrandedTriggers_CapsAndReportsTotal(t *testing.T) {
	repo, _ := newTestRepoWithDB(t)
	ctx := context.Background()
	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		routine := seedCronTrigger(t, repo, "ws-a", "R")
		overdueAt := now.Add(-10 * time.Minute)
		trigger := &models.RoutineTrigger{
			RoutineID: routine.ID, Kind: "cron", CronExpression: "* * * * *",
			Timezone: "UTC", Enabled: true, NextRunAt: &overdueAt,
		}
		if err := repo.CreateRoutineTrigger(ctx, trigger); err != nil {
			t.Fatalf("create trigger %d: %v", i, err)
		}
	}

	rows, total, err := repo.ListOverdueOrStrandedTriggers(ctx, "ws-a", now, 3*time.Minute, 5*time.Minute, 2)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want capped at 2", len(rows))
	}
	if total != 3 {
		t.Fatalf("total = %d, want 3 (untruncated)", total)
	}
}

func TestCountEligibleTriggers_ZeroForEmptyWorkspace(t *testing.T) {
	repo, _ := newTestRepoWithDB(t)
	count, err := repo.CountEligibleTriggers(context.Background(), "ws-empty")
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}
