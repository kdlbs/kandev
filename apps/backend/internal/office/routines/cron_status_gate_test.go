package routines_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routines"
	"github.com/kandev/kandev/internal/office/shared"
)

// newObservedRoutineService builds a RoutineService backed by in-memory
// SQLite whose logger's entries can be inspected — used by the tests below
// to assert the bounded ("at most once per trigger per process") log
// entries the status-gating design requires.
func newObservedRoutineService(t *testing.T) (*routines.RoutineService, *observer.ObservedLogs) {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
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
	return routines.NewRoutineService(repo, log, &noopActivity{}), observed
}

// createStatusGatedRoutine creates a routine with an every-minute cron
// trigger and returns it plus the trigger's initially-computed
// (auto-assigned) next_run_at.
func createStatusGatedRoutine(
	t *testing.T, svc *routines.RoutineService, status string,
) (*models.Routine, time.Time) {
	t.Helper()
	ctx := context.Background()
	routine := &models.Routine{
		WorkspaceID:            "ws-1",
		Name:                   "Status Gate " + status,
		TaskTemplate:           "",
		AssigneeAgentProfileID: "agent-1",
		Status:                 status,
		ConcurrencyPolicy:      "always_create",
	}
	if err := svc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	if err := svc.CreateRoutineTrigger(ctx, &models.RoutineTrigger{
		RoutineID:      routine.ID,
		Kind:           "cron",
		CronExpression: "* * * * *",
		Timezone:       "UTC",
		Enabled:        true,
	}); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	triggers, err := svc.ListRoutineTriggers(ctx, routine.ID)
	if err != nil || len(triggers) != 1 {
		t.Fatalf("list triggers: triggers=%v err=%v", triggers, err)
	}
	return routine, *triggers[0].NextRunAt
}

// assertSuppressed drives one tick past the trigger's due time and asserts
// the AC-OFFICE-ROUTINE-STATUS-001 contract: no run, no wakeup, no fire
// evidence, and the cursor moved to the first slot strictly after the tick
// time (never banked, per REQ-002).
func assertSuppressed(t *testing.T, svc *routines.RoutineService, routine *models.Routine, oldNext time.Time) {
	t.Helper()
	ctx := context.Background()
	enq := &fakeWakeupEnqueuer{}
	svc.SetWakeupEnqueuer(enq)

	tickNow := oldNext.Add(2 * time.Minute)
	if err := svc.TickScheduledTriggers(ctx, tickNow); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if len(enq.created) != 0 {
		t.Errorf("expected no wakeup request, got %d", len(enq.created))
	}
	runs, err := svc.ListRoutineRuns(ctx, routine.ID, 10, 0)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Errorf("expected no run row for a suppressed slot, got %d", len(runs))
	}

	triggers, err := svc.ListRoutineTriggers(ctx, routine.ID)
	if err != nil || len(triggers) != 1 {
		t.Fatalf("list triggers: triggers=%v err=%v", triggers, err)
	}
	got := triggers[0]
	if got.LastFiredAt != nil {
		t.Errorf("last_fired_at = %v, want nil — suppression must leave no fire evidence", got.LastFiredAt)
	}
	wantNext, err := shared.NextCronTime(got.CronExpression, got.Timezone, tickNow)
	if err != nil {
		t.Fatalf("compute expected next: %v", err)
	}
	if got.NextRunAt == nil || !got.NextRunAt.Equal(wantNext) {
		t.Errorf("next_run_at = %v, want %v (first slot strictly after the tick, not banked)", got.NextRunAt, wantNext)
	}
}

func TestTickScheduledTriggers_PausedRoutineSuppresses(t *testing.T) {
	svc, _ := newObservedRoutineService(t)
	routine, oldNext := createStatusGatedRoutine(t, svc, "paused")
	assertSuppressed(t, svc, routine, oldNext)
}

func TestTickScheduledTriggers_ArchivedRoutineSuppresses(t *testing.T) {
	svc, _ := newObservedRoutineService(t)
	routine, oldNext := createStatusGatedRoutine(t, svc, "archived")
	assertSuppressed(t, svc, routine, oldNext)
}

func TestTickScheduledTriggers_UnrecognizedStatusSuppresses(t *testing.T) {
	svc, _ := newObservedRoutineService(t)
	routine, oldNext := createStatusGatedRoutine(t, svc, "on_hold")
	assertSuppressed(t, svc, routine, oldNext)
}

// TestTickScheduledTriggers_EmptyStatusFires covers
// AC-OFFICE-ROUTINE-STATUS-001.4 and -006.5: an empty stored status means
// "no writer set one" and must keep firing exactly as "active" does.
func TestTickScheduledTriggers_EmptyStatusFires(t *testing.T) {
	svc, _ := newObservedRoutineService(t)
	routine, oldNext := createStatusGatedRoutine(t, svc, "")
	ctx := context.Background()
	enq := &fakeWakeupEnqueuer{}
	svc.SetWakeupEnqueuer(enq)

	if err := svc.TickScheduledTriggers(ctx, oldNext.Add(2*time.Minute)); err != nil {
		t.Fatalf("tick: %v", err)
	}
	if len(enq.created) != 1 {
		t.Fatalf("expected 1 wakeup request for an empty-status routine, got %d", len(enq.created))
	}
	runs, err := svc.ListRoutineRuns(ctx, routine.ID, 10, 0)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run row, got %d", len(runs))
	}
}

// TestTickScheduledTriggers_ResumeAfterSuppression_ReportsZeroMissedTicks
// covers AC-OFFICE-ROUTINE-STATUS-002.2/-002.3/-002.4: a routine paused
// through an entire suppression window (here, one evaluation collapsing a
// ~10-slot backlog in a single tick, simulating an outage inside the pause)
// resumes at the cursor the suppression left — strictly in the future — and
// its first fire reports zero missed ticks, never the backlog the
// suppression window spanned. The payload only carries "missed_ticks" when
// it is greater than zero (marshalRoutinePayload), so its absence here is
// the assertion.
func TestTickScheduledTriggers_ResumeAfterSuppression_ReportsZeroMissedTicks(t *testing.T) {
	svc, _ := newObservedRoutineService(t)
	routine, oldNext := createStatusGatedRoutine(t, svc, "paused")
	ctx := context.Background()
	enq := &fakeWakeupEnqueuer{}
	svc.SetWakeupEnqueuer(enq)

	// One suppressing evaluation, ten slots (minutes) after the original
	// due time: the outage-inside-a-pause case in AC-002.4.
	suppressAt := oldNext.Add(10 * time.Minute)
	if err := svc.TickScheduledTriggers(ctx, suppressAt); err != nil {
		t.Fatalf("suppressing tick: %v", err)
	}
	triggers, err := svc.ListRoutineTriggers(ctx, routine.ID)
	if err != nil || len(triggers) != 1 {
		t.Fatalf("list triggers: triggers=%v err=%v", triggers, err)
	}
	resumeCursor := *triggers[0].NextRunAt
	if !resumeCursor.After(suppressAt) {
		t.Fatalf("cursor = %v, want strictly after the suppressing evaluation %v", resumeCursor, suppressAt)
	}

	routine.Status = "active"
	if err := svc.UpdateRoutine(ctx, routine); err != nil {
		t.Fatalf("resume routine: %v", err)
	}

	// Tick exactly at the cursor the suppression left: the trigger is due
	// and every intervening slot is in the future relative to the pause, so
	// per -002.3 the fixed-set-of-cursors-in-the-future condition holds.
	if err := svc.TickScheduledTriggers(ctx, resumeCursor); err != nil {
		t.Fatalf("resuming tick: %v", err)
	}
	if len(enq.created) != 1 {
		t.Fatalf("expected exactly 1 wakeup request on resume, got %d", len(enq.created))
	}
	if strings.Contains(enq.created[0].Payload, "missed_ticks") {
		t.Errorf("payload = %q, must not carry missed_ticks (want 0 missed, absent from payload)",
			enq.created[0].Payload)
	}
	runs, err := svc.ListRoutineRuns(ctx, routine.ID, 10, 0)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected exactly 1 run row on resume, got %d", len(runs))
	}
}

// TestTickScheduledTriggers_NonUTCScheduleSuppressionAdvancesCursor is
// Decision 8's regression: the match check must convert into the trigger's
// timezone before comparing, or every non-UTC schedule freezes its cursor
// on every suppressed slot. A daily 9am America/New_York cron is ~13:00
// UTC; comparing the UTC instant against the spec's wall-clock "9" would
// never match.
func TestTickScheduledTriggers_NonUTCScheduleSuppressionAdvancesCursor(t *testing.T) {
	svc, observed := newObservedRoutineService(t)
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID:            "ws-1",
		Name:                   "Non-UTC Paused",
		TaskTemplate:           "",
		AssigneeAgentProfileID: "agent-1",
		Status:                 "paused",
		ConcurrencyPolicy:      "always_create",
	}
	if err := svc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	if err := svc.CreateRoutineTrigger(ctx, &models.RoutineTrigger{
		RoutineID:      routine.ID,
		Kind:           "cron",
		CronExpression: "0 9 * * *",
		Timezone:       "America/New_York",
		Enabled:        true,
	}); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	triggers, err := svc.ListRoutineTriggers(ctx, routine.ID)
	if err != nil || len(triggers) != 1 {
		t.Fatalf("list triggers: triggers=%v err=%v", triggers, err)
	}
	oldNext := *triggers[0].NextRunAt

	tickNow := oldNext.Add(time.Minute)
	if err := svc.TickScheduledTriggers(ctx, tickNow); err != nil {
		t.Fatalf("tick: %v", err)
	}

	wantNext, err := shared.NextCronTime("0 9 * * *", "America/New_York", tickNow)
	if err != nil {
		t.Fatalf("compute expected next: %v", err)
	}
	triggers, err = svc.ListRoutineTriggers(ctx, routine.ID)
	if err != nil || len(triggers) != 1 {
		t.Fatalf("list triggers: triggers=%v err=%v", triggers, err)
	}
	got := triggers[0].NextRunAt
	if got == nil || got.Equal(oldNext) {
		t.Fatalf("next_run_at did not advance (got %v, old %v) — the match check is rejecting a "+
			"valid non-UTC slot, freezing the cursor", got, oldNext)
	}
	if !got.Equal(wantNext) {
		t.Errorf("next_run_at = %v, want %v", got, wantNext)
	}
	if n := observed.FilterMessage("routine cursor not advanced").Len(); n != 0 {
		t.Errorf("expected no cursor-not-advanced log entries for a valid non-UTC slot, got %d", n)
	}
}

// TestTickScheduledTriggers_NeverMatchingExpression_CursorUnchangedAndLoggedOnce
// covers AC-OFFICE-ROUTINE-STATUS-002.1/-002.8: findNextMatch's silent
// `after + 24h` fallback for an expression naming no slot (Feb 30 never
// occurs) must be rejected by the match check rather than written as if it
// were a real slot, and the failure log is bounded to once per trigger per
// process (AC-OFFICE-ROUTINE-STATUS-003.6) even across repeated ticks.
func TestTickScheduledTriggers_NeverMatchingExpression_CursorUnchangedAndLoggedOnce(t *testing.T) {
	svc, observed := newObservedRoutineService(t)
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID:            "ws-1",
		Name:                   "Never Matching",
		TaskTemplate:           "",
		AssigneeAgentProfileID: "agent-1",
		Status:                 "paused",
		ConcurrencyPolicy:      "always_create",
	}
	if err := svc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	if err := svc.CreateRoutineTrigger(ctx, &models.RoutineTrigger{
		RoutineID:      routine.ID,
		Kind:           "cron",
		CronExpression: "0 0 30 2 *", // Feb 30 never exists
		Timezone:       "UTC",
		Enabled:        true,
	}); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	triggers, err := svc.ListRoutineTriggers(ctx, routine.ID)
	if err != nil || len(triggers) != 1 {
		t.Fatalf("list triggers: triggers=%v err=%v", triggers, err)
	}
	// CreateRoutineTrigger computes next_run_at the same way the firing
	// path does (no match check on create — out of scope), so it already
	// holds findNextMatch's after+24h fallback. Ticking just past it makes
	// the trigger due for the suppression path under test.
	staleNext := *triggers[0].NextRunAt

	for i := 0; i < 3; i++ {
		tickNow := staleNext.Add(time.Duration(i+1) * time.Minute)
		if err := svc.TickScheduledTriggers(ctx, tickNow); err != nil {
			t.Fatalf("tick %d: %v", i, err)
		}
	}

	triggers, err = svc.ListRoutineTriggers(ctx, routine.ID)
	if err != nil || len(triggers) != 1 {
		t.Fatalf("list triggers: triggers=%v err=%v", triggers, err)
	}
	if got := triggers[0].NextRunAt; got == nil || !got.Equal(staleNext) {
		t.Errorf("next_run_at = %v, want unchanged %v — an unsatisfiable expression must not "+
			"silently advance the cursor", got, staleNext)
	}
	if n := observed.FilterMessage("routine cursor not advanced").Len(); n != 1 {
		t.Errorf("cursor-not-advanced logged %d times across 3 ticks, want 1 (AC-003.6 bound)", n)
	}
}

// TestTickScheduledTriggers_MissingRoutine_NotDisarmedAndLoggedOnce covers
// AC-OFFICE-ROUTINE-STATUS-003.2/-003.3: an orphaned trigger (its routine
// deleted) is never disarmed and the read failure is logged at most once
// per process. This fixture only exists because the in-memory SQLite test
// harness opens without `_foreign_keys=on` (production enables it, so
// DeleteRoutine's cascade normally takes the trigger with it) — see
// apps/backend/CLAUDE.md and the routine-status-gating plan.
func TestTickScheduledTriggers_MissingRoutine_NotDisarmedAndLoggedOnce(t *testing.T) {
	svc, observed := newObservedRoutineService(t)
	routine, oldNext := createStatusGatedRoutine(t, svc, "active")
	ctx := context.Background()

	if err := svc.DeleteRoutine(ctx, routine.ID); err != nil {
		t.Fatalf("delete routine: %v", err)
	}

	for i := 0; i < 3; i++ {
		tickNow := oldNext.Add(time.Duration(i+2) * time.Minute)
		if err := svc.TickScheduledTriggers(ctx, tickNow); err != nil {
			t.Fatalf("tick %d: %v", i, err)
		}
	}

	// ListTriggersByRoutineID is a plain SELECT by routine_id, so it still
	// finds the orphaned row even though the routine is gone.
	triggers, err := svc.ListRoutineTriggers(ctx, routine.ID)
	if err != nil || len(triggers) != 1 {
		t.Fatalf("list triggers: triggers=%v err=%v", triggers, err)
	}
	if got := triggers[0].NextRunAt; got == nil || !got.Equal(oldNext) {
		t.Errorf("next_run_at = %v, want unchanged %v — an unreadable routine must never be disarmed", got, oldNext)
	}
	if n := observed.FilterMessage("routine unreadable for due trigger").Len(); n != 1 {
		t.Errorf("unreadable-routine logged %d times across 3 ticks, want 1 (AC-003.6 bound)", n)
	}
}

func TestFireManual_NonFiringRoutine_ReturnsRoutineNotFiringError(t *testing.T) {
	for _, status := range []string{"paused", "archived", "disabled"} {
		t.Run(status, func(t *testing.T) {
			svc := newTestRoutineService(t)
			ctx := context.Background()
			routine := createTestRoutine(t, svc, "Refuse "+status, "always_create")
			routine.Status = status
			if err := svc.UpdateRoutine(ctx, routine); err != nil {
				t.Fatalf("update routine status: %v", err)
			}

			_, err := svc.FireManual(ctx, routine.ID, nil)
			var notFiring *routines.RoutineNotFiringError
			if !errors.As(err, &notFiring) {
				t.Fatalf("FireManual err = %v, want *RoutineNotFiringError", err)
			}
			if notFiring.Status != status {
				t.Errorf("notFiring.Status = %q, want %q", notFiring.Status, status)
			}

			runs, err := svc.ListRoutineRuns(ctx, routine.ID, 10, 0)
			if err != nil {
				t.Fatalf("list runs: %v", err)
			}
			if len(runs) != 0 {
				t.Errorf("expected no run row on a refused manual fire, got %d", len(runs))
			}
		})
	}
}

// TestFireManual_OtherError_NotWrappedAsNotFiring proves the errors.As
// branch in the handler is type-discriminated: a routine that does not
// exist keeps its ordinary error (and 500 mapping in the handler), it does
// not surface as a status refusal.
func TestFireManual_OtherError_NotWrappedAsNotFiring(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()

	_, err := svc.FireManual(ctx, "does-not-exist", nil)
	if err == nil {
		t.Fatal("expected an error for a missing routine")
	}
	var notFiring *routines.RoutineNotFiringError
	if errors.As(err, &notFiring) {
		t.Errorf("a missing-routine error must not be a *RoutineNotFiringError, got %v", err)
	}
}
