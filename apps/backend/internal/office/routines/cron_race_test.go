package routines

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

type raceNoopActivity struct{}

func (raceNoopActivity) LogActivity(context.Context, string, string, string, string, string, string, string) {
}
func (raceNoopActivity) LogActivityWithRun(
	context.Context, string, string, string, string, string, string, string, string, string,
) {
}

// newRaceTestService builds a RoutineService with SetMaxOpenConns(1), the
// convention this codebase's other SQLite-backed race tests use to
// serialize the DB connection while leaving Go-level goroutine scheduling
// free to interleave.
func newRaceTestService(t *testing.T) (*RoutineService, *observer.ObservedLogs) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	core, observed := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	return NewRoutineService(repo, log, raceNoopActivity{}), observed
}

// raceTestTrigger creates a routine plus a cron trigger with an explicit
// past next_run_at, returning both. Using the repo directly (rather than
// the CreateRoutineTrigger convenience, which computes next_run_at from
// "now") gives the test a known, already-due cursor to race on.
func raceTestTrigger(t *testing.T, svc *RoutineService, status string) (*Routine, *RoutineTrigger) {
	t.Helper()
	ctx := context.Background()
	routine := &Routine{
		WorkspaceID:            "ws-1",
		Name:                   "Race " + status,
		TaskTemplate:           "",
		AssigneeAgentProfileID: "agent-1",
		Status:                 status,
		ConcurrencyPolicy:      "always_create",
	}
	if err := svc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	oldNext := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	trigger := &RoutineTrigger{
		RoutineID:      routine.ID,
		Kind:           "cron",
		CronExpression: "* * * * *",
		Timezone:       "UTC",
		NextRunAt:      &oldNext,
		Enabled:        true,
	}
	if err := svc.repo.CreateRoutineTrigger(ctx, trigger); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	return routine, trigger
}

// TestProcessCronTrigger_ConcurrentSuppression_AdvancesAtMostOnceAndLogsOnce
// covers AC-OFFICE-ROUTINE-STATUS-002.6 and Decision 11: two evaluations
// racing on the same due slot of a non-firing routine advance the cursor
// at most once, fire nothing, and log the suppression at most once — never
// twice, which is what an unconditional log (rather than one gated on the
// compare-and-set actually succeeding) would produce.
//
// This interleaving cannot happen through the production cron loop —
// internal/scheduler/cron's fanOut waits on every handler goroutine before
// its next tick, so TickScheduledTriggers passes cannot overlap — hence
// calling the unexported processCronTrigger directly with two goroutines
// sharing one pre-race trigger snapshot, exactly as the routine-status-
// gating plan's suggested test coverage prescribes.
func TestProcessCronTrigger_ConcurrentSuppression_AdvancesAtMostOnceAndLogsOnce(t *testing.T) {
	svc, observed := newRaceTestService(t)
	ctx := context.Background()
	routine, trigger := raceTestTrigger(t, svc, "paused")
	now := trigger.NextRunAt.Add(time.Minute)

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			snapshot := *trigger
			_ = svc.processCronTrigger(ctx, &snapshot, now)
		}()
	}
	wg.Wait()

	runs, err := svc.ListRoutineRuns(ctx, routine.ID, 10, 0)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("expected no run rows from a suppressed race, got %d", len(runs))
	}

	triggers, err := svc.ListRoutineTriggers(ctx, routine.ID)
	if err != nil || len(triggers) != 1 {
		t.Fatalf("list triggers: triggers=%v err=%v", triggers, err)
	}
	if got := triggers[0].NextRunAt; got == nil || !got.After(now) {
		t.Errorf("next_run_at = %v, want a single advance to a slot after %v", got, now)
	}
	if triggers[0].LastFiredAt != nil {
		t.Errorf("last_fired_at = %v, want nil", triggers[0].LastFiredAt)
	}

	if n := observed.FilterMessage("routine slot suppressed").Len(); n != 1 {
		t.Errorf("suppression logged %d times, want exactly 1 (Decision 11: only a successful "+
			"advance logs; the race loser must log nothing)", n)
	}
}

// TestProcessCronTrigger_ConcurrentFiring_ExactlyOneFires covers
// AC-OFFICE-ROUTINE-STATUS-002.10: two evaluations racing on the same due
// slot of a firing routine dispatch exactly one run. Unchanged from
// today's ClaimTrigger CAS — this test pins that the reordered control
// flow (routine read before claim) did not regress it.
func TestProcessCronTrigger_ConcurrentFiring_ExactlyOneFires(t *testing.T) {
	svc, _ := newRaceTestService(t)
	ctx := context.Background()
	routine, trigger := raceTestTrigger(t, svc, "active")
	now := trigger.NextRunAt.Add(time.Minute)

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			snapshot := *trigger
			_ = svc.processCronTrigger(ctx, &snapshot, now)
		}()
	}
	wg.Wait()

	runs, err := svc.ListRoutineRuns(ctx, routine.ID, 10, 0)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Errorf("expected exactly 1 run row from a firing race, got %d", len(runs))
	}
}
