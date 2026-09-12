package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

func seedCatchUpRoutine(t *testing.T, repo interface {
	CreateRoutine(ctx context.Context, r *models.Routine) error
}, id string, catchUpMax int) *models.Routine {
	t.Helper()
	routine := &models.Routine{
		ID:                id,
		WorkspaceID:       "ws-1",
		Name:              "routine-" + id,
		Status:            "active",
		ConcurrencyPolicy: models.ConcurrencyPolicyAlwaysCreate,
		CatchUpMax:        catchUpMax,
		Variables:         "{}",
	}
	if err := repo.CreateRoutine(context.Background(), routine); err != nil {
		t.Fatalf("CreateRoutine: %v", err)
	}
	return routine
}

// TestCreateRoutine_CatchUpMaxNormalized covers AC-OFFICE-ROUTINE-CATCHUP-001.13
// on the create path: 0 clamps to the default floor, 5000 clamps to the
// ceiling, and the mutated struct returned to the caller (not only a
// subsequent GET) already reports the normalized value.
func TestCreateRoutine_CatchUpMaxNormalized(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	zero := &models.Routine{ID: "r-zero", WorkspaceID: "ws-1", Name: "zero", Variables: "{}", CatchUpMax: 0}
	if err := repo.CreateRoutine(ctx, zero); err != nil {
		t.Fatalf("CreateRoutine: %v", err)
	}
	if zero.CatchUpMax != models.CatchUpMaxDefault {
		t.Errorf("in-place CatchUpMax = %d, want %d", zero.CatchUpMax, models.CatchUpMaxDefault)
	}
	got, err := repo.GetRoutine(ctx, "r-zero")
	if err != nil {
		t.Fatalf("GetRoutine: %v", err)
	}
	if got.CatchUpMax != models.CatchUpMaxDefault {
		t.Errorf("read-back CatchUpMax = %d, want %d", got.CatchUpMax, models.CatchUpMaxDefault)
	}

	over := &models.Routine{ID: "r-over", WorkspaceID: "ws-1", Name: "over", Variables: "{}", CatchUpMax: 5000}
	if err := repo.CreateRoutine(ctx, over); err != nil {
		t.Fatalf("CreateRoutine: %v", err)
	}
	if over.CatchUpMax != models.CatchUpMaxCeiling {
		t.Errorf("in-place CatchUpMax = %d, want %d", over.CatchUpMax, models.CatchUpMaxCeiling)
	}
	got, err = repo.GetRoutine(ctx, "r-over")
	if err != nil {
		t.Fatalf("GetRoutine: %v", err)
	}
	if got.CatchUpMax != models.CatchUpMaxCeiling {
		t.Errorf("read-back CatchUpMax = %d, want %d", got.CatchUpMax, models.CatchUpMaxCeiling)
	}
}

// TestCreateRoutineTx_CatchUpMaxNormalized is CreateRoutine's transaction
// sibling — config-sync's own creation path, which bypasses CreateRoutine
// entirely, must clamp identically.
func TestCreateRoutineTx_CatchUpMaxNormalized(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	tx, err := repo.Writer().BeginTxx(ctx, nil)
	if err != nil {
		t.Fatalf("BeginTxx: %v", err)
	}
	routine := &models.Routine{ID: "r-tx-over", WorkspaceID: "ws-1", Name: "over", Variables: "{}", CatchUpMax: 5000}
	if err := repo.CreateRoutineTx(ctx, tx, routine); err != nil {
		t.Fatalf("CreateRoutineTx: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if routine.CatchUpMax != models.CatchUpMaxCeiling {
		t.Errorf("in-place CatchUpMax = %d, want %d", routine.CatchUpMax, models.CatchUpMaxCeiling)
	}
	got, err := repo.GetRoutine(ctx, "r-tx-over")
	if err != nil {
		t.Fatalf("GetRoutine: %v", err)
	}
	if got.CatchUpMax != models.CatchUpMaxCeiling {
		t.Errorf("read-back CatchUpMax = %d, want %d", got.CatchUpMax, models.CatchUpMaxCeiling)
	}
}

// TestUpdateRoutine_CatchUpMaxNormalized covers the update path — the hole
// that existed on both ends before this change (UpdateRoutine wrote
// catch_up_max completely unbound).
func TestUpdateRoutine_CatchUpMaxNormalized(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	routine := seedCatchUpRoutine(t, repo, "r-upd", 25)

	routine.CatchUpMax = 0
	if err := repo.UpdateRoutine(ctx, routine); err != nil {
		t.Fatalf("UpdateRoutine: %v", err)
	}
	if routine.CatchUpMax != models.CatchUpMaxDefault {
		t.Errorf("in-place CatchUpMax after 0 = %d, want %d", routine.CatchUpMax, models.CatchUpMaxDefault)
	}

	routine.CatchUpMax = 5000
	if err := repo.UpdateRoutine(ctx, routine); err != nil {
		t.Fatalf("UpdateRoutine: %v", err)
	}
	if routine.CatchUpMax != models.CatchUpMaxCeiling {
		t.Errorf("in-place CatchUpMax after 5000 = %d, want %d", routine.CatchUpMax, models.CatchUpMaxCeiling)
	}
	got, err := repo.GetRoutine(ctx, routine.ID)
	if err != nil {
		t.Fatalf("GetRoutine: %v", err)
	}
	if got.CatchUpMax != models.CatchUpMaxCeiling {
		t.Errorf("read-back CatchUpMax = %d, want %d", got.CatchUpMax, models.CatchUpMaxCeiling)
	}
}

// TestGetDueTriggers_OrderedByNextRunThenID covers AC-OFFICE-ROUTINE-CATCHUP-001.7:
// triggers due at the same instant are returned in ascending id order, and
// an earlier next_run_at always sorts first regardless of id.
func TestGetDueTriggers_OrderedByNextRunThenID(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	routine := seedCatchUpRoutine(t, repo, "r-order", 25)

	now := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	tied := now.Add(-time.Minute)
	earlier := now.Add(-2 * time.Minute)

	mustCreateTrigger := func(id string, next time.Time) {
		t.Helper()
		if err := repo.CreateRoutineTrigger(ctx, &models.RoutineTrigger{
			ID: id, RoutineID: routine.ID, Kind: "cron", CronExpression: "* * * * *",
			NextRunAt: &next, Enabled: true,
		}); err != nil {
			t.Fatalf("CreateRoutineTrigger(%s): %v", id, err)
		}
	}
	// Two triggers tied on next_run_at, seeded out of id order, plus one
	// due earlier that must sort first despite a "larger" id.
	mustCreateTrigger("trigger-z", tied)
	mustCreateTrigger("trigger-a", tied)
	mustCreateTrigger("trigger-m-earliest", earlier)

	due, err := repo.GetDueTriggers(ctx, now)
	if err != nil {
		t.Fatalf("GetDueTriggers: %v", err)
	}
	if len(due) != 3 {
		t.Fatalf("len(due) = %d, want 3", len(due))
	}
	gotIDs := []string{due[0].ID, due[1].ID, due[2].ID}
	wantIDs := []string{"trigger-m-earliest", "trigger-a", "trigger-z"}
	for i := range wantIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Errorf("due[%d].ID = %q, want %q (got order %v)", i, gotIDs[i], wantIDs[i], gotIDs)
		}
	}
}

// TestListStrandedTriggers_LiveClaimIsNotStale covers the discriminator
// AC-OFFICE-ROUTINE-CATCHUP-001.9 relies on: a claim taken this tick
// (updated_at inside catchUpReclaimAfter) must not be returned as stranded.
func TestListStrandedTriggers_LiveClaimIsNotStale(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	routine := seedCatchUpRoutine(t, repo, "r-live", 25)

	now := time.Now().UTC()
	claimAt := now.Add(-time.Minute)
	if err := repo.CreateRoutineTrigger(ctx, &models.RoutineTrigger{
		ID: "trigger-live", RoutineID: routine.ID, Kind: "cron", CronExpression: "* * * * *",
		NextRunAt: &claimAt, Enabled: true,
	}); err != nil {
		t.Fatalf("CreateRoutineTrigger: %v", err)
	}
	claimed, err := repo.ClaimTrigger(ctx, "trigger-live", claimAt)
	if err != nil || !claimed {
		t.Fatalf("ClaimTrigger: claimed=%v err=%v", claimed, err)
	}

	// A claim taken just now (updated_at very fresh) is well inside any
	// reasonable reclaim window, so a cutoff far in the past finds nothing.
	stranded, err := repo.ListStrandedTriggers(ctx, now.Add(-10*time.Minute))
	if err != nil {
		t.Fatalf("ListStrandedTriggers: %v", err)
	}
	for _, tr := range stranded {
		if tr.ID == "trigger-live" {
			t.Fatalf("live claim returned as stranded: %+v", tr)
		}
	}

	// The same row IS stranded once the cutoff is recent enough to include it.
	stranded, err = repo.ListStrandedTriggers(ctx, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("ListStrandedTriggers: %v", err)
	}
	found := false
	for _, tr := range stranded {
		if tr.ID == "trigger-live" {
			found = true
		}
	}
	if !found {
		t.Fatal("trigger-live not found once cutoff is recent enough")
	}
}

// TestReconcileTriggerNextRun_CASSemantics covers the reconciliation
// write's compare-and-swap: it only affects a row still claimed (next_run_at
// IS NULL, enabled = 1), so a second reconciler racing the first affects
// zero rows and a disabled trigger is left alone.
func TestReconcileTriggerNextRun_CASSemantics(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	routine := seedCatchUpRoutine(t, repo, "r-cas", 25)

	next := time.Now().UTC().Add(-time.Minute)
	if err := repo.CreateRoutineTrigger(ctx, &models.RoutineTrigger{
		ID: "trigger-cas", RoutineID: routine.ID, Kind: "cron", CronExpression: "* * * * *",
		NextRunAt: &next, Enabled: true,
	}); err != nil {
		t.Fatalf("CreateRoutineTrigger: %v", err)
	}
	claimed, err := repo.ClaimTrigger(ctx, "trigger-cas", next)
	if err != nil || !claimed {
		t.Fatalf("ClaimTrigger: claimed=%v err=%v", claimed, err)
	}

	armTo := time.Now().UTC().Add(time.Minute)
	won, err := repo.ReconcileTriggerNextRun(ctx, "trigger-cas", armTo)
	if err != nil {
		t.Fatalf("ReconcileTriggerNextRun: %v", err)
	}
	if !won {
		t.Fatal("first reconcile did not win the CAS")
	}

	// A second reconciler racing the first affects zero rows: next_run_at
	// is no longer null, so the WHERE predicate matches nothing.
	againTo := armTo.Add(time.Minute)
	wonAgain, err := repo.ReconcileTriggerNextRun(ctx, "trigger-cas", againTo)
	if err != nil {
		t.Fatalf("second ReconcileTriggerNextRun: %v", err)
	}
	if wonAgain {
		t.Fatal("second reconcile won the CAS, want it to affect zero rows")
	}
}

// TestReconcileTriggerNextRun_ArmedValueWins asserts the first CAS write
// actually landed the intended next_run_at (not just that it reported
// success), by re-listing due triggers at that instant.
func TestReconcileTriggerNextRun_ArmedValueWins(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	routine := seedCatchUpRoutine(t, repo, "r-cas2", 25)

	next := time.Now().UTC().Add(-time.Minute)
	if err := repo.CreateRoutineTrigger(ctx, &models.RoutineTrigger{
		ID: "trigger-cas2", RoutineID: routine.ID, Kind: "cron", CronExpression: "* * * * *",
		NextRunAt: &next, Enabled: true,
	}); err != nil {
		t.Fatalf("CreateRoutineTrigger: %v", err)
	}
	if claimed, err := repo.ClaimTrigger(ctx, "trigger-cas2", next); err != nil || !claimed {
		t.Fatalf("ClaimTrigger: claimed=%v err=%v", claimed, err)
	}

	armTo := time.Now().UTC().Add(time.Hour)
	if won, err := repo.ReconcileTriggerNextRun(ctx, "trigger-cas2", armTo); err != nil || !won {
		t.Fatalf("ReconcileTriggerNextRun: won=%v err=%v", won, err)
	}

	due, err := repo.GetDueTriggers(ctx, armTo.Add(time.Minute))
	if err != nil {
		t.Fatalf("GetDueTriggers: %v", err)
	}
	found := false
	for _, tr := range due {
		if tr.ID == "trigger-cas2" {
			found = true
			if tr.NextRunAt == nil || !tr.NextRunAt.Equal(armTo) {
				t.Errorf("NextRunAt = %v, want %v", tr.NextRunAt, armTo)
			}
		}
	}
	if !found {
		t.Fatal("reconciled trigger not found among due triggers at the armed instant")
	}
}

// TestCreateRoutineRun_GapSummaryRoundTrip covers the NULL-versus-zero
// distinction (AC-OFFICE-ROUTINE-CATCHUP-002.3): a run created with no gap
// summary must read back with a nil CatchUpMissedTicks, not a stored zero,
// while a run created with one round-trips exactly.
func TestCreateRoutineRun_GapSummaryRoundTrip(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	routine := seedCatchUpRoutine(t, repo, "r-gap", 25)

	noGap := &models.RoutineRun{
		ID: "run-no-gap", RoutineID: routine.ID, Source: "cron",
		Status: models.RoutineRunStatusReceived, TriggerPayload: "{}",
	}
	if err := repo.CreateRoutineRun(ctx, noGap); err != nil {
		t.Fatalf("CreateRoutineRun (no gap): %v", err)
	}

	missed := 5
	firstMissed := time.Date(2026, 5, 10, 11, 55, 0, 0, time.UTC)
	withGap := &models.RoutineRun{
		ID: "run-with-gap", RoutineID: routine.ID, Source: "cron",
		Status: models.RoutineRunStatusReceived, TriggerPayload: "{}",
		CatchUpMissedTicks: &missed, CatchUpFirstMissedAt: &firstMissed, CatchUpTruncated: true,
	}
	if err := repo.CreateRoutineRun(ctx, withGap); err != nil {
		t.Fatalf("CreateRoutineRun (with gap): %v", err)
	}

	runs, err := repo.ListRoutineRuns(ctx, routine.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListRoutineRuns: %v", err)
	}
	byID := map[string]*models.RoutineRun{}
	for _, r := range runs {
		byID[r.ID] = r
	}

	gotNoGap, ok := byID["run-no-gap"]
	if !ok {
		t.Fatal("run-no-gap not found")
	}
	if gotNoGap.CatchUpMissedTicks != nil {
		t.Errorf("no-gap CatchUpMissedTicks = %v, want nil", *gotNoGap.CatchUpMissedTicks)
	}
	if gotNoGap.CatchUpFirstMissedAt != nil {
		t.Errorf("no-gap CatchUpFirstMissedAt = %v, want nil", *gotNoGap.CatchUpFirstMissedAt)
	}
	if gotNoGap.CatchUpTruncated {
		t.Error("no-gap CatchUpTruncated = true, want false")
	}

	gotWithGap, ok := byID["run-with-gap"]
	if !ok {
		t.Fatal("run-with-gap not found")
	}
	if gotWithGap.CatchUpMissedTicks == nil || *gotWithGap.CatchUpMissedTicks != missed {
		t.Errorf("with-gap CatchUpMissedTicks = %v, want %d", gotWithGap.CatchUpMissedTicks, missed)
	}
	if gotWithGap.CatchUpFirstMissedAt == nil || !gotWithGap.CatchUpFirstMissedAt.Equal(firstMissed) {
		t.Errorf("with-gap CatchUpFirstMissedAt = %v, want %v", gotWithGap.CatchUpFirstMissedAt, firstMissed)
	}
	if !gotWithGap.CatchUpTruncated {
		t.Error("with-gap CatchUpTruncated = false, want true")
	}
}

// TestCreateRoutineRun_GapSummaryWriteOnce covers AC-OFFICE-ROUTINE-CATCHUP-002.10:
// the three gap columns are unchanged after a run transitions through
// coalesced and failed.
func TestCreateRoutineRun_GapSummaryWriteOnce(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	routine := seedCatchUpRoutine(t, repo, "r-write-once", 25)

	missed := 3
	firstMissed := time.Date(2026, 5, 10, 11, 55, 0, 0, time.UTC)
	run := &models.RoutineRun{
		ID: "run-once", RoutineID: routine.ID, Source: "cron",
		Status: models.RoutineRunStatusReceived, TriggerPayload: "{}",
		CatchUpMissedTicks: &missed, CatchUpFirstMissedAt: &firstMissed, CatchUpTruncated: false,
	}
	if err := repo.CreateRoutineRun(ctx, run); err != nil {
		t.Fatalf("CreateRoutineRun: %v", err)
	}

	if err := repo.UpdateRunCoalesced(ctx, run.ID, "run-other"); err != nil {
		t.Fatalf("UpdateRunCoalesced: %v", err)
	}
	if err := repo.UpdateRunStatus(ctx, run.ID, models.RoutineRunStatusFailed, ""); err != nil {
		t.Fatalf("UpdateRunStatus: %v", err)
	}

	runs, err := repo.ListRoutineRuns(ctx, routine.ID, 10, 0)
	if err != nil {
		t.Fatalf("ListRoutineRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1", len(runs))
	}
	got := runs[0]
	if got.Status != models.RoutineRunStatusFailed {
		t.Fatalf("Status = %q, want failed", got.Status)
	}
	if got.CatchUpMissedTicks == nil || *got.CatchUpMissedTicks != missed {
		t.Errorf("CatchUpMissedTicks = %v, want %d (unchanged)", got.CatchUpMissedTicks, missed)
	}
	if got.CatchUpFirstMissedAt == nil || !got.CatchUpFirstMissedAt.Equal(firstMissed) {
		t.Errorf("CatchUpFirstMissedAt = %v, want %v (unchanged)", got.CatchUpFirstMissedAt, firstMissed)
	}
}
