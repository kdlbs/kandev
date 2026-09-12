package routines_test

import (
	"context"
	"errors"
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
	"github.com/kandev/kandev/internal/office/wakeup"
)

// wrapRepo wraps a real routines.Repository (the sqlite implementation) and
// lets individual tests force specific methods to fail, to exercise error
// paths that are otherwise unreachable through the public service API
// against a healthy in-memory database.
type wrapRepo struct {
	routines.Repository
	failUpdateTriggerNextRun bool
	failCreateRoutineRun     bool
}

func (w *wrapRepo) UpdateTriggerNextRun(ctx context.Context, triggerID string, nextRunAt *time.Time) error {
	if w.failUpdateTriggerNextRun {
		return errors.New("simulated arm-write failure")
	}
	return w.Repository.UpdateTriggerNextRun(ctx, triggerID, nextRunAt)
}

func (w *wrapRepo) CreateRoutineRun(ctx context.Context, run *models.RoutineRun) error {
	if w.failCreateRoutineRun {
		return errors.New("simulated create-run failure")
	}
	return w.Repository.CreateRoutineRun(ctx, run)
}

// newWrappedTestRoutineService is newTestRoutineService plus access to the
// wrapRepo (to flip failure switches) and the raw *sqlx.DB (to backdate
// rows directly, as no service method can).
func newWrappedTestRoutineService(t *testing.T) (*routines.RoutineService, *wrapRepo, *sqlx.DB) {
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
	wrapped := &wrapRepo{Repository: repo}
	return routines.NewRoutineService(wrapped, logger.Default(), &noopActivity{}), wrapped, db
}

func mustCreateCronRoutineAndTrigger(t *testing.T, svc *routines.RoutineService, taskTemplate string) *models.Routine {
	t.Helper()
	ctx := context.Background()
	routine := &models.Routine{
		WorkspaceID:            "ws-1",
		Name:                   "Cron routine",
		TaskTemplate:           taskTemplate,
		AssigneeAgentProfileID: "agent-1",
		Status:                 "active",
		ConcurrencyPolicy:      models.ConcurrencyPolicyAlwaysCreate,
	}
	if err := svc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	if err := svc.CreateRoutineTrigger(ctx, &models.RoutineTrigger{
		RoutineID: routine.ID, Kind: "cron", CronExpression: "* * * * *", Timezone: "UTC", Enabled: true,
	}); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	return routine
}

// TestProcessCronTrigger_ArmFailureAbortsDispatch covers AC-OFFICE-ROUTINE-CATCHUP-001.2:
// when the re-arm write itself fails, the tick dispatches nothing for that
// claim and no routine run is created.
func TestProcessCronTrigger_ArmFailureAbortsDispatch(t *testing.T) {
	svc, wrapped, _ := newWrappedTestRoutineService(t)
	ctx := context.Background()
	routine := mustCreateCronRoutineAndTrigger(t, svc, "")

	wrapped.failUpdateTriggerNextRun = true
	if err := svc.TickScheduledTriggers(ctx, time.Now().UTC().Add(2*time.Minute)); err != nil {
		t.Fatalf("tick: %v", err)
	}

	runs, err := svc.ListRoutineRuns(ctx, routine.ID, 10, 0)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("routine runs = %d, want 0 (arm failure must abort dispatch entirely)", len(runs))
	}
}

// TestDispatchRoutineRun_CreateRunFailureRecordsNothing covers AC-OFFICE-ROUTINE-CATCHUP-001.12:
// when the run row itself cannot be created, dispatch returns an error and
// no run row exists — there is nothing to mark failed.
func TestDispatchRoutineRun_CreateRunFailureRecordsNothing(t *testing.T) {
	svc, wrapped, _ := newWrappedTestRoutineService(t)
	ctx := context.Background()
	routine := mustCreateCronRoutineAndTrigger(t, svc, "")

	wrapped.failCreateRoutineRun = true
	if _, err := svc.FireManual(ctx, routine.ID, nil); err == nil {
		t.Fatal("expected error from FireManual when CreateRoutineRun fails")
	}

	wrapped.failCreateRoutineRun = false
	runs, err := svc.ListRoutineRuns(ctx, routine.ID, 10, 0)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("routine runs = %d, want 0", len(runs))
	}
}

// failingCreateWakeupEnqueuer forces CreateWakeupRequest to fail with a
// non-idempotency error, to exercise the lightweight path's materialisation
// failure (AC-OFFICE-ROUTINE-CATCHUP-001.10).
type failingCreateWakeupEnqueuer struct {
	createErr error
}

func (f *failingCreateWakeupEnqueuer) CreateWakeupRequest(_ context.Context, _ *routines.WakeupRequest) error {
	return f.createErr
}

func (f *failingCreateWakeupEnqueuer) Dispatch(_ context.Context, _ string) error { return nil }

func (f *failingCreateWakeupEnqueuer) FailWakeupRequest(_ context.Context, _, _ string) error {
	return nil
}

// TestProcessCronTrigger_LightweightMaterialiseFailureMarksRunFailed covers
// AC-OFFICE-ROUTINE-CATCHUP-001.10: a lightweight routine's
// CreateWakeupRequest failure (not an idempotency conflict) marks the
// claim's run failed.
func TestProcessCronTrigger_LightweightMaterialiseFailureMarksRunFailed(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()
	routine := newLightweightTestRoutine(t, svc)
	if err := svc.CreateRoutineTrigger(ctx, &models.RoutineTrigger{
		RoutineID: routine.ID, Kind: "cron", CronExpression: "* * * * *", Timezone: "UTC", Enabled: true,
	}); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	svc.SetWakeupEnqueuer(&failingCreateWakeupEnqueuer{createErr: errors.New("simulated dispatcher failure")})

	if err := svc.TickScheduledTriggers(ctx, time.Now().UTC().Add(2*time.Minute)); err != nil {
		t.Fatalf("tick: %v", err)
	}

	runs, err := svc.ListRoutineRuns(ctx, routine.ID, 10, 0)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("routine runs = %d, want 1", len(runs))
	}
	if runs[0].Status != models.RoutineRunStatusFailed {
		t.Errorf("status = %q, want failed", runs[0].Status)
	}
}

// TestProcessCronTrigger_IdempotencyConflictDoesNotMarkRunFailed covers the
// second sentence of AC-OFFICE-ROUTINE-CATCHUP-001.10: a wakeup request
// refused as an idempotency-key duplicate is a successful dedup, not a
// materialisation failure, and must not flip the run to failed.
func TestProcessCronTrigger_IdempotencyConflictDoesNotMarkRunFailed(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()
	routine := newLightweightTestRoutine(t, svc)
	if err := svc.CreateRoutineTrigger(ctx, &models.RoutineTrigger{
		RoutineID: routine.ID, Kind: "cron", CronExpression: "* * * * *", Timezone: "UTC", Enabled: true,
	}); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	svc.SetWakeupEnqueuer(&failingCreateWakeupEnqueuer{createErr: routines.ErrWakeupAlreadyRequested})

	if err := svc.TickScheduledTriggers(ctx, time.Now().UTC().Add(2*time.Minute)); err != nil {
		t.Fatalf("tick: %v", err)
	}

	runs, err := svc.ListRoutineRuns(ctx, routine.ID, 10, 0)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("routine runs = %d, want 1", len(runs))
	}
	// The concurrency-policy outcome (task_created, since nothing else was
	// active) must survive — an idempotency dedup is not a failure.
	if runs[0].Status == models.RoutineRunStatusFailed {
		t.Error("status = failed, want the concurrency-policy outcome preserved (idempotency dedup is not a failure)")
	}
}

// TestProcessCronTrigger_HeavyMaterialiseFailureMarksRunFailed covers the
// heavy-path half of AC-OFFICE-ROUTINE-CATCHUP-001.10: a task-creation
// failure marks the run failed instead of leaving it stuck at "received".
func TestProcessCronTrigger_HeavyMaterialiseFailureMarksRunFailed(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()
	routine := mustCreateCronRoutineAndTrigger(t, svc, `{"title":"T","description":"D"}`)

	svc.SetWorkflowEnsurer(&fakeWorkflowEnsurer{})
	svc.SetTaskCreator(&failingTaskCreator{})

	if err := svc.TickScheduledTriggers(ctx, time.Now().UTC().Add(2*time.Minute)); err != nil {
		t.Fatalf("tick: %v", err)
	}

	runs, err := svc.ListRoutineRuns(ctx, routine.ID, 10, 0)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("routine runs = %d, want 1", len(runs))
	}
	if runs[0].Status != models.RoutineRunStatusFailed {
		t.Errorf("status = %q, want failed (was stuck at received before this change)", runs[0].Status)
	}
}

type failingTaskCreator struct{}

func (f *failingTaskCreator) CreateOfficeTaskInWorkflow(
	_ context.Context, _, _, _, _, _, _ string,
) (string, error) {
	return "", errors.New("simulated task creation failure")
}

// TestCreateRoutineTrigger_RejectsEmptyCronExpression covers the first half
// of AC-OFFICE-ROUTINE-CATCHUP-001.8: a cron trigger must never be
// persisted with a null next_run_at, so an empty expression is rejected
// outright rather than silently created unarmed.
func TestCreateRoutineTrigger_RejectsEmptyCronExpression(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()
	routine := createTestRoutine(t, svc, "AC-001.8 empty", "always_create")

	err := svc.CreateRoutineTrigger(ctx, &models.RoutineTrigger{
		RoutineID: routine.ID, Kind: "cron", CronExpression: "", Timezone: "UTC", Enabled: true,
	})
	if err == nil {
		t.Fatal("expected an error creating a cron trigger with an empty expression")
	}

	triggers, listErr := svc.ListRoutineTriggers(ctx, routine.ID)
	if listErr != nil {
		t.Fatalf("list triggers: %v", listErr)
	}
	if len(triggers) != 0 {
		t.Fatalf("triggers = %d, want 0 (rejected trigger must not be persisted)", len(triggers))
	}
}

// TestCreateRoutineTrigger_RejectsUnparseableCronExpression covers the
// second half of AC-OFFICE-ROUTINE-CATCHUP-001.8: an expression that
// shared.NextCronTime cannot parse is rejected at creation time rather than
// persisted with a null next_run_at.
func TestCreateRoutineTrigger_RejectsUnparseableCronExpression(t *testing.T) {
	svc := newTestRoutineService(t)
	ctx := context.Background()
	routine := createTestRoutine(t, svc, "AC-001.8 unparseable", "always_create")

	err := svc.CreateRoutineTrigger(ctx, &models.RoutineTrigger{
		RoutineID: routine.ID, Kind: "cron", CronExpression: "not a cron expression", Timezone: "UTC", Enabled: true,
	})
	if err == nil {
		t.Fatal("expected an error creating a cron trigger with an unparseable expression")
	}

	triggers, listErr := svc.ListRoutineTriggers(ctx, routine.ID)
	if listErr != nil {
		t.Fatalf("list triggers: %v", listErr)
	}
	if len(triggers) != 0 {
		t.Fatalf("triggers = %d, want 0 (rejected trigger must not be persisted)", len(triggers))
	}
}

// TestTickScheduledTriggers_ReconciliationArmsStrandedTriggerWithoutDispatch
// covers AC-OFFICE-ROUTINE-CATCHUP-001.9 at the service level: a trigger
// claimed (next_run_at set to NULL) by a process that crashed before the
// re-arm write landed gets armed by the next tick's reconciliation pass,
// without dispatching a spurious run for the abandoned claim.
func TestTickScheduledTriggers_ReconciliationArmsStrandedTriggerWithoutDispatch(t *testing.T) {
	svc, wrapped, db := newWrappedTestRoutineService(t)
	ctx := context.Background()
	routine := mustCreateCronRoutineAndTrigger(t, svc, "")

	triggers, err := svc.ListRoutineTriggers(ctx, routine.ID)
	if err != nil || len(triggers) != 1 {
		t.Fatalf("list triggers: %v (len=%d)", err, len(triggers))
	}
	triggerID := triggers[0].ID

	// Simulate a claim that landed (next_run_at -> NULL) but whose re-arm
	// write never happened, aged well past catchUpReclaimAfter.
	staleUpdatedAt := time.Now().UTC().Add(-10 * time.Minute)
	if _, err := wrapped.ClaimTrigger(ctx, triggerID, *triggers[0].NextRunAt); err != nil {
		t.Fatalf("claim trigger: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		"UPDATE office_routine_triggers SET updated_at = ? WHERE id = ?", staleUpdatedAt, triggerID,
	); err != nil {
		t.Fatalf("backdate updated_at: %v", err)
	}

	if err := svc.TickScheduledTriggers(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("tick: %v", err)
	}

	runs, err := svc.ListRoutineRuns(ctx, routine.ID, 10, 0)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("routine runs = %d, want 0 (reconciliation must never dispatch)", len(runs))
	}

	reArmed, err := svc.ListRoutineTriggers(ctx, routine.ID)
	if err != nil {
		t.Fatalf("list triggers: %v", err)
	}
	if reArmed[0].NextRunAt == nil {
		t.Fatal("NextRunAt still nil after reconciliation, want armed")
	}
	if !reArmed[0].NextRunAt.After(time.Now().UTC().Add(-time.Minute)) {
		t.Errorf("NextRunAt = %v, want a time close to/after now (re-armed from `now`, not from the stale claim)", *reArmed[0].NextRunAt)
	}
}

// TestCreateRoutineTrigger_MalformedExpressionFallback covers AC-001.11 end
// to end through the service: a trigger whose cron expression can never
// parse re-arms 24 hours out and dispatches at most once per day, not
// repeatedly.
func TestTickScheduledTriggers_MalformedExpressionDispatchesOncePerDay(t *testing.T) {
	svc, wrapped, _ := newWrappedTestRoutineService(t)
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID: "ws-1", Name: "Bad cron", AssigneeAgentProfileID: "agent-1",
		Status: "active", ConcurrencyPolicy: models.ConcurrencyPolicyAlwaysCreate,
	}
	if err := svc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	// Bypass CreateRoutineTrigger's own AC-001.8 rejection: seed a
	// malformed-but-armed trigger directly through the repo, as a legacy
	// row (created before AC-001.8) would look. Arm it in the past so it's
	// due.
	past := time.Now().UTC().Add(-time.Hour)
	if err := wrapped.CreateRoutineTrigger(ctx, &models.RoutineTrigger{
		RoutineID: routine.ID, Kind: "cron", CronExpression: "garbage", Timezone: "UTC",
		Enabled: true, NextRunAt: &past,
	}); err != nil {
		t.Fatalf("seed malformed trigger: %v", err)
	}

	now := time.Now().UTC()
	if err := svc.TickScheduledTriggers(ctx, now); err != nil {
		t.Fatalf("first tick: %v", err)
	}
	runsAfterFirst, err := svc.ListRoutineRuns(ctx, routine.ID, 10, 0)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runsAfterFirst) != 1 {
		t.Fatalf("routine runs after first tick = %d, want 1", len(runsAfterFirst))
	}

	// A tick 23 hours later must NOT dispatch again — the fallback interval
	// hasn't elapsed.
	if err := svc.TickScheduledTriggers(ctx, now.Add(23*time.Hour)); err != nil {
		t.Fatalf("23h tick: %v", err)
	}
	runsAfter23h, err := svc.ListRoutineRuns(ctx, routine.ID, 10, 0)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runsAfter23h) != 1 {
		t.Fatalf("routine runs after 23h = %d, want still 1 (not due yet)", len(runsAfter23h))
	}

	// A tick just past 24 hours dispatches exactly once more.
	if err := svc.TickScheduledTriggers(ctx, now.Add(24*time.Hour+time.Minute)); err != nil {
		t.Fatalf("24h tick: %v", err)
	}
	runsAfter24h, err := svc.ListRoutineRuns(ctx, routine.ID, 10, 0)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runsAfter24h) != 2 {
		t.Fatalf("routine runs after 24h = %d, want 2", len(runsAfter24h))
	}
}

// TestTickScheduledTriggers_MalformedExpressionLogsUnderlyingError covers
// AC-OFFICE-ROUTINE-CATCHUP-001.11: the warning emitted when the elapsed-tick
// computation fails must name the underlying cron-parse error, not just the
// trigger ID and expression.
func TestTickScheduledTriggers_MalformedExpressionLogsUnderlyingError(t *testing.T) {
	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("create observer logger: %v", err)
	}

	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	svc := routines.NewRoutineService(repo, log, &noopActivity{})

	ctx := context.Background()
	routine := &models.Routine{
		WorkspaceID: "ws-1", Name: "Bad cron", AssigneeAgentProfileID: "agent-1",
		Status: "active", ConcurrencyPolicy: models.ConcurrencyPolicyAlwaysCreate,
	}
	if err := svc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	past := time.Now().UTC().Add(-time.Hour)
	if err := repo.CreateRoutineTrigger(ctx, &models.RoutineTrigger{
		RoutineID: routine.ID, Kind: "cron", CronExpression: "garbage", Timezone: "UTC",
		Enabled: true, NextRunAt: &past,
	}); err != nil {
		t.Fatalf("seed malformed trigger: %v", err)
	}

	if err := svc.TickScheduledTriggers(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("tick: %v", err)
	}

	entries := logs.FilterMessage("compute routine catch-up failed").All()
	if len(entries) == 0 {
		t.Fatal("expected a 'compute routine catch-up failed' warning")
	}
	if errVal, ok := entries[0].ContextMap()["error"]; !ok || errVal == "" {
		t.Fatalf("warning did not name the underlying error, context = %#v", entries[0].ContextMap())
	}
}

// TestTickScheduledTriggers_GapExceedingCapProducesExactlyOneRun covers
// AC-OFFICE-ROUTINE-CATCHUP-001.1 and 001.3 together at the service level:
// a gap spanning far more ticks than catch_up_max produces exactly one
// run and one wakeup dispatch on resume — catch_up_max bounds only the
// counted tick total (here capped at 3), never the number of runs created.
func TestTickScheduledTriggers_GapExceedingCapProducesExactlyOneRun(t *testing.T) {
	svc, _, db := newWrappedTestRoutineService(t)
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID: "ws-1", Name: "Gapped", AssigneeAgentProfileID: "agent-1",
		Status: "active", ConcurrencyPolicy: models.ConcurrencyPolicyAlwaysCreate,
		CatchUpPolicy: models.CatchUpPolicySummarizeMissed, CatchUpMax: 3,
	}
	if err := svc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	if err := svc.CreateRoutineTrigger(ctx, &models.RoutineTrigger{
		RoutineID: routine.ID, Kind: "cron", CronExpression: "* * * * *", Timezone: "UTC", Enabled: true,
	}); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	// The backend was "down" far longer than catch_up_max ticks can count.
	longDown := time.Now().UTC().Add(-100 * time.Minute)
	triggers, err := svc.ListRoutineTriggers(ctx, routine.ID)
	if err != nil || len(triggers) != 1 {
		t.Fatalf("list triggers: %v (len=%d)", err, len(triggers))
	}
	if _, err := db.ExecContext(ctx,
		"UPDATE office_routine_triggers SET next_run_at = ? WHERE id = ?", longDown, triggers[0].ID,
	); err != nil {
		t.Fatalf("backdate next_run_at: %v", err)
	}

	enq := &fakeWakeupEnqueuer{}
	svc.SetWakeupEnqueuer(enq)

	if err := svc.TickScheduledTriggers(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("tick: %v", err)
	}

	runs, err := svc.ListRoutineRuns(ctx, routine.ID, 10, 0)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("routine runs = %d, want exactly 1 regardless of gap size", len(runs))
	}
	if len(enq.dispatched) != 1 {
		t.Fatalf("wakeup dispatches = %d, want exactly 1", len(enq.dispatched))
	}
	run := runs[0]
	if run.CatchUpMissedTicks == nil {
		t.Fatal("CatchUpMissedTicks = nil, want set")
	}
	if *run.CatchUpMissedTicks != routine.CatchUpMax-1 {
		t.Errorf("CatchUpMissedTicks = %d, want %d (catch_up_max - 1)", *run.CatchUpMissedTicks, routine.CatchUpMax-1)
	}
	if !run.CatchUpTruncated {
		t.Error("CatchUpTruncated = false, want true")
	}
	if run.CatchUpFirstMissedAt == nil || !run.CatchUpFirstMissedAt.Equal(longDown) {
		t.Errorf("CatchUpFirstMissedAt = %v, want %v", run.CatchUpFirstMissedAt, longDown)
	}

	// AC-OFFICE-ROUTINE-CATCHUP-002.5: the gap must also be readable from
	// the actual wakeup-request payload dispatched to the agent, not only
	// from the run's DB columns.
	if len(enq.created) != 1 {
		t.Fatalf("wakeup requests created = %d, want exactly 1", len(enq.created))
	}
	var payload wakeup.RoutinePayload
	if err := wakeup.UnmarshalPayload(enq.created[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal wakeup payload: %v", err)
	}
	if payload.MissedTicks != routine.CatchUpMax-1 {
		t.Errorf("payload MissedTicks = %d, want %d", payload.MissedTicks, routine.CatchUpMax-1)
	}
	if !payload.MissedTruncated {
		t.Error("payload MissedTruncated = false, want true")
	}
	if payload.MissedSince != longDown.UTC().Format(time.RFC3339) {
		t.Errorf("payload MissedSince = %q, want %q", payload.MissedSince, longDown.UTC().Format(time.RFC3339))
	}
}

// TestTickScheduledTriggers_HeavyRoutineGapLeavesTaskUnmodified covers
// AC-OFFICE-ROUTINE-CATCHUP-002.6: a heavy routine dispatched across a
// measured gap must render its task title/description exactly as the
// template does, with the gap readable only through the routine run, not
// injected into the task the agent sees.
func TestTickScheduledTriggers_HeavyRoutineGapLeavesTaskUnmodified(t *testing.T) {
	svc, _, db := newWrappedTestRoutineService(t)
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID: "ws-1", Name: "Heavy gapped", AssigneeAgentProfileID: "agent-1",
		TaskTemplate:      `{"title":"Daily review","description":"Fixed description"}`,
		Status:            "active",
		ConcurrencyPolicy: models.ConcurrencyPolicyAlwaysCreate,
		CatchUpPolicy:     models.CatchUpPolicySummarizeMissed, CatchUpMax: 25,
	}
	if err := svc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	if err := svc.CreateRoutineTrigger(ctx, &models.RoutineTrigger{
		RoutineID: routine.ID, Kind: "cron", CronExpression: "* * * * *", Timezone: "UTC", Enabled: true,
	}); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	longDown := time.Now().UTC().Add(-10 * time.Minute)
	triggers, err := svc.ListRoutineTriggers(ctx, routine.ID)
	if err != nil || len(triggers) != 1 {
		t.Fatalf("list triggers: %v (len=%d)", err, len(triggers))
	}
	if _, err := db.ExecContext(ctx,
		"UPDATE office_routine_triggers SET next_run_at = ? WHERE id = ?", longDown, triggers[0].ID,
	); err != nil {
		t.Fatalf("backdate next_run_at: %v", err)
	}

	wf := &fakeWorkflowEnsurer{}
	tc := &fakeTaskCreator{}
	svc.SetWorkflowEnsurer(wf)
	svc.SetTaskCreator(tc)

	if err := svc.TickScheduledTriggers(ctx, time.Now().UTC()); err != nil {
		t.Fatalf("tick: %v", err)
	}

	if tc.captured.title != "Daily review" {
		t.Errorf("task title = %q, want exactly %q (unmodified by the gap)", tc.captured.title, "Daily review")
	}
	if tc.captured.description != "Fixed description" {
		t.Errorf("task description = %q, want exactly %q (unmodified by the gap)",
			tc.captured.description, "Fixed description")
	}

	runs, err := svc.ListRoutineRuns(ctx, routine.ID, 10, 0)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("routine runs = %d, want 1", len(runs))
	}
	run := runs[0]
	if run.CatchUpMissedTicks == nil || *run.CatchUpMissedTicks < 1 {
		t.Fatalf("CatchUpMissedTicks = %v, want set and >= 1 (gap summary readable via the run)", run.CatchUpMissedTicks)
	}
	if run.CatchUpFirstMissedAt == nil || !run.CatchUpFirstMissedAt.Equal(longDown) {
		t.Errorf("CatchUpFirstMissedAt = %v, want %v", run.CatchUpFirstMissedAt, longDown)
	}
	if run.LinkedTaskID != "task-routine-1" {
		t.Errorf("LinkedTaskID = %q, want task-routine-1 (heavy dispatch happened)", run.LinkedTaskID)
	}
}

// TestFireManual_RecordsNoGapSummaryDespitePendingTriggerGap covers
// AC-OFFICE-ROUTINE-CATCHUP-002.12: gap measurement applies only to a cron
// trigger's own armed claim. A manual fire on a routine whose cron trigger
// has a large backdated next_run_at (i.e. would produce a gap on its own
// tick) must still record no gap summary at all.
func TestFireManual_RecordsNoGapSummaryDespitePendingTriggerGap(t *testing.T) {
	svc, _, db := newWrappedTestRoutineService(t)
	ctx := context.Background()

	routine := &models.Routine{
		WorkspaceID: "ws-1", Name: "Manual despite gap", AssigneeAgentProfileID: "agent-1",
		Status: "active", ConcurrencyPolicy: models.ConcurrencyPolicyAlwaysCreate,
		CatchUpPolicy: models.CatchUpPolicySummarizeMissed, CatchUpMax: 25,
	}
	if err := svc.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	if err := svc.CreateRoutineTrigger(ctx, &models.RoutineTrigger{
		RoutineID: routine.ID, Kind: "cron", CronExpression: "* * * * *", Timezone: "UTC", Enabled: true,
	}); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	triggers, err := svc.ListRoutineTriggers(ctx, routine.ID)
	if err != nil || len(triggers) != 1 {
		t.Fatalf("list triggers: %v (len=%d)", err, len(triggers))
	}
	longDown := time.Now().UTC().Add(-100 * time.Minute)
	if _, err := db.ExecContext(ctx,
		"UPDATE office_routine_triggers SET next_run_at = ? WHERE id = ?", longDown, triggers[0].ID,
	); err != nil {
		t.Fatalf("backdate next_run_at: %v", err)
	}

	run, err := svc.FireManual(ctx, routine.ID, nil)
	if err != nil {
		t.Fatalf("fire manual: %v", err)
	}
	if run.CatchUpMissedTicks != nil {
		t.Errorf("CatchUpMissedTicks = %v, want nil (manual fire records no gap)", *run.CatchUpMissedTicks)
	}
	if run.CatchUpFirstMissedAt != nil {
		t.Errorf("CatchUpFirstMissedAt = %v, want nil", *run.CatchUpFirstMissedAt)
	}
	if run.CatchUpTruncated {
		t.Error("CatchUpTruncated = true, want false")
	}
}
