package routines_test

import (
	"context"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routines"
)

// TestTickScheduledTriggers_UnsatisfiableExpression_DisarmsInsteadOfLooping
// is a regression test for a legacy trigger row whose cron expression can
// never fire (e.g. persisted before create-time validation existed, or
// hand-edited). On a catch-up computation failure the trigger must not
// dispatch and must not re-arm next_run_at to "now": ClaimTrigger already
// clears next_run_at when claiming, and re-arming it to "now" makes the
// trigger due again on the very next 30s cron tick, dispatching a run
// forever.
func TestTickScheduledTriggers_UnsatisfiableExpression_DisarmsInsteadOfLooping(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	svc := routines.NewRoutineService(repo, logger.Default(), &noopActivity{})
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID:            "ws-1",
		Name:                   "Legacy unsatisfiable",
		TaskTemplate:           "", // lightweight
		AssigneeAgentProfileID: "agent-1",
		Status:                 "active",
		ConcurrencyPolicy:      models.ConcurrencyPolicyAlwaysCreate,
	}
	if err := svc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}

	// Bypass service-level create-time validation (routines.ErrInvalidTrigger)
	// by writing straight through the repository, simulating a row that was
	// persisted before that validation existed.
	due := time.Now().UTC().Add(-time.Minute)
	trigger := &models.RoutineTrigger{
		RoutineID:      routine.ID,
		Kind:           "cron",
		CronExpression: "0 0 30 2 *", // February 30th: never matches.
		Timezone:       "UTC",
		Enabled:        true,
		NextRunAt:      &due,
	}
	if err := repo.CreateRoutineTrigger(ctx, trigger); err != nil {
		t.Fatalf("create trigger (bypassing validation): %v", err)
	}

	enq := &fakeWakeupEnqueuer{}
	svc.SetWakeupEnqueuer(enq)

	if err := svc.TickScheduledTriggers(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("tick scheduled triggers: %v", err)
	}
	if len(enq.created) != 0 {
		t.Fatalf("expected no dispatch for an unsatisfiable trigger, got %d", len(enq.created))
	}

	triggers, err := repo.ListTriggersByRoutineID(ctx, routine.ID)
	if err != nil {
		t.Fatalf("list triggers: %v", err)
	}
	if len(triggers) != 1 {
		t.Fatalf("expected 1 trigger, got %d", len(triggers))
	}
	if triggers[0].NextRunAt != nil {
		t.Fatalf("next_run_at = %v, want nil (trigger must stay disarmed, not re-armed to now)", *triggers[0].NextRunAt)
	}

	// A second tick must not dispatch either — proves this isn't a
	// re-arm-to-now loop that would fire again on the very next 30s tick.
	if err := svc.TickScheduledTriggers(ctx, time.Now().UTC().Add(time.Minute)); err != nil {
		t.Fatalf("second tick scheduled triggers: %v", err)
	}
	if len(enq.created) != 0 {
		t.Fatalf("expected still no dispatch after second tick, got %d", len(enq.created))
	}
}

// TestTickScheduledTriggers_RecoverableCatchUpFailure_RearmsForRetry is a
// regression test for a catch-up failure that is NOT expression
// unsatisfiability (e.g. the timezone database is transiently unavailable).
// Unlike the unsatisfiable case (AC-OFFICE-ROUTINE-CATCHUP-001.11 governs
// only "elapsed-tick computation fails", never a syntactically-impossible
// expression), this arms next_run_at to the processing instant plus 24
// hours and still dispatches exactly one run for the claim — REQ-001's
// "one resume, one wake" is unconditional, and AC-001.11 says as much
// ("shall therefore dispatch at most one run per day until it is deleted
// and recreated") — rather than leaving the trigger permanently disarmed
// or silently skipping the claim it already took.
func TestTickScheduledTriggers_RecoverableCatchUpFailure_RearmsForRetry(t *testing.T) {
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	svc := routines.NewRoutineService(repo, logger.Default(), &noopActivity{})
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID:            "ws-1",
		Name:                   "Bad timezone",
		TaskTemplate:           "",
		AssigneeAgentProfileID: "agent-1",
		Status:                 "active",
		ConcurrencyPolicy:      models.ConcurrencyPolicyAlwaysCreate,
	}
	if err := svc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}

	// Bypass service-level create-time validation by writing straight
	// through the repository, simulating a timezone that was valid at
	// create time but whose lookup now fails (e.g. tzdata unavailable).
	due := time.Now().UTC().Add(-time.Minute)
	trigger := &models.RoutineTrigger{
		RoutineID:      routine.ID,
		Kind:           "cron",
		CronExpression: "*/5 * * * *",
		Timezone:       "Not/AZone",
		Enabled:        true,
		NextRunAt:      &due,
	}
	if err := repo.CreateRoutineTrigger(ctx, trigger); err != nil {
		t.Fatalf("create trigger (bypassing validation): %v", err)
	}

	enq := &fakeWakeupEnqueuer{}
	svc.SetWakeupEnqueuer(enq)

	before := time.Now().UTC()
	if err := svc.TickScheduledTriggers(ctx, before); err != nil {
		t.Fatalf("tick scheduled triggers: %v", err)
	}
	after := time.Now().UTC()
	if len(enq.created) != 1 {
		t.Fatalf("expected exactly one dispatch on a recoverable catch-up failure (AC-001.11), got %d", len(enq.created))
	}

	triggers, err := repo.ListTriggersByRoutineID(ctx, routine.ID)
	if err != nil {
		t.Fatalf("list triggers: %v", err)
	}
	if len(triggers) != 1 {
		t.Fatalf("expected 1 trigger, got %d", len(triggers))
	}
	if triggers[0].NextRunAt == nil {
		t.Fatalf("next_run_at = nil, want re-armed to processing instant + 24h")
	}
	got := *triggers[0].NextRunAt
	if got.Before(before.Add(24*time.Hour)) || got.After(after.Add(24*time.Hour)) {
		t.Fatalf("next_run_at = %v, want within [%v, %v] (processing instant + 24h)",
			got, before.Add(24*time.Hour), after.Add(24*time.Hour))
	}
}
