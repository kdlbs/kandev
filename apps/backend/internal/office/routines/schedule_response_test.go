package routines_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routines"
)

// countingRepo wraps a real repository, counting calls to the two trigger
// read paths ClassifyRoutines uses, and can force either to fail so the
// AC-OFFICE-ROUTINE-ARMING-002.7 query-count bound and the AC-001.13
// fallback path can both be exercised against a real database.
type countingRepo struct {
	routines.Repository
	batchCalls       int
	singleCalls      int
	forceBatchErr    bool
	forceSingleErrOn map[string]bool
}

func (c *countingRepo) ListTriggersByRoutineIDs(
	ctx context.Context, routineIDs []string,
) (map[string][]*models.RoutineTrigger, error) {
	c.batchCalls++
	if c.forceBatchErr {
		return nil, errors.New("forced batch failure")
	}
	return c.Repository.ListTriggersByRoutineIDs(ctx, routineIDs)
}

func (c *countingRepo) ListTriggersByRoutineID(ctx context.Context, routineID string) ([]*models.RoutineTrigger, error) {
	c.singleCalls++
	if c.forceSingleErrOn[routineID] {
		return nil, errors.New("forced single-read failure")
	}
	return c.Repository.ListTriggersByRoutineID(ctx, routineID)
}

func newCountingRoutineService(t *testing.T) (*routines.RoutineService, *countingRepo) {
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
	counting := &countingRepo{Repository: repo}
	svc := routines.NewRoutineService(counting, logger.Default(), &noopActivity{})
	return svc, counting
}

func mustCreateRoutine(t *testing.T, svc *routines.RoutineService, id, name string) *models.Routine {
	t.Helper()
	r := &models.Routine{
		ID: id, WorkspaceID: "ws-1", Name: name, TaskTemplate: "{}",
		Status: "active", ConcurrencyPolicy: "skip_if_active", Variables: "{}",
	}
	if err := svc.CreateRoutine(context.Background(), r); err != nil {
		t.Fatalf("create routine %s: %v", id, err)
	}
	return r
}

func mustCreateTrigger(t *testing.T, svc *routines.RoutineService, tr *models.RoutineTrigger) {
	t.Helper()
	if err := svc.CreateRoutineTrigger(context.Background(), tr); err != nil {
		t.Fatalf("create trigger %s: %v", tr.ID, err)
	}
}

// TestAttachScheduleState_AllStatesFixture covers the routine list response
// carrying the right schedule state and unarmed list per routine
// (AC-OFFICE-ROUTINE-ARMING-002.1) across a fixture spanning several
// schedule states.
func TestAttachScheduleState_AllStatesFixture(t *testing.T) {
	svc, counting := newCountingRoutineService(t)
	ctx := context.Background()

	armed := mustCreateRoutine(t, svc, "r-armed", "Armed")
	invalid := mustCreateRoutine(t, svc, "r-invalid", "Invalid")
	disabled := mustCreateRoutine(t, svc, "r-disabled", "Disabled")
	manualOnly := mustCreateRoutine(t, svc, "r-manual", "ManualOnly")
	noTrigger := mustCreateRoutine(t, svc, "r-none", "NoTrigger")

	future := time.Now().UTC().Add(time.Hour)
	mustCreateTrigger(t, svc, &models.RoutineTrigger{
		ID: "t-armed", RoutineID: armed.ID, Kind: "cron",
		CronExpression: "*/5 * * * *", Timezone: "UTC", Enabled: true, NextRunAt: &future,
	})
	// CreateRoutineTrigger validates the cron expression up front, so an
	// already-invalid trigger (e.g. a timezone later removed from tzdata) is
	// seeded straight through the repository instead.
	if err := counting.CreateRoutineTrigger(ctx, &models.RoutineTrigger{
		ID: "t-invalid", RoutineID: invalid.ID, Kind: "cron",
		CronExpression: "garbage", Timezone: "UTC", Enabled: true,
	}); err != nil {
		t.Fatalf("seed invalid trigger: %v", err)
	}
	mustCreateTrigger(t, svc, &models.RoutineTrigger{
		ID: "t-disabled", RoutineID: disabled.ID, Kind: "cron",
		CronExpression: "*/5 * * * *", Timezone: "UTC", Enabled: false,
	})
	mustCreateTrigger(t, svc, &models.RoutineTrigger{
		ID: "t-manual", RoutineID: manualOnly.ID, Kind: "manual", Enabled: true,
	})

	list, err := svc.ListRoutinesFromConfig(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list routines: %v", err)
	}
	withSchedule, err := svc.AttachScheduleState(ctx, list)
	if err != nil {
		t.Fatalf("attach schedule state: %v", err)
	}

	want := map[string]routines.ScheduleState{
		armed.ID:      routines.ScheduleStateArmed,
		invalid.ID:    routines.ScheduleStateTriggerInvalid,
		disabled.ID:   routines.ScheduleStateTriggerDisabled,
		manualOnly.ID: routines.ScheduleStateUnscheduledManualOnly,
		noTrigger.ID:  routines.ScheduleStateUnscheduledNoTrigger,
	}
	if len(withSchedule) != len(want) {
		t.Fatalf("result count = %d, want %d", len(withSchedule), len(want))
	}
	for _, rw := range withSchedule {
		wantState, ok := want[rw.ID]
		if !ok {
			t.Fatalf("unexpected routine %s in result", rw.ID)
		}
		if rw.ScheduleState != wantState {
			t.Errorf("routine %s: state = %q, want %q", rw.ID, rw.ScheduleState, wantState)
		}
		if rw.Status != "active" {
			t.Errorf("routine %s: intent (Status) = %q, want active", rw.ID, rw.Status)
		}
	}

	disabledResult := findByID(t, withSchedule, disabled.ID)
	if len(disabledResult.UnarmedCronTriggers) != 1 || disabledResult.UnarmedCronTriggers[0].TriggerID != "t-disabled" {
		t.Errorf("disabled routine unarmed list = %+v, want exactly t-disabled", disabledResult.UnarmedCronTriggers)
	}
}

func findByID(t *testing.T, list []*routines.RoutineWithSchedule, id string) *routines.RoutineWithSchedule {
	t.Helper()
	for _, rw := range list {
		if rw.ID == id {
			return rw
		}
	}
	t.Fatalf("routine %s not found in result", id)
	return nil
}

// TestAttachScheduleState_QueryCountBound covers
// AC-OFFICE-ROUTINE-ARMING-002.7: producing the list costs one query for
// routines (asserted by the caller, not here) plus exactly one batch query
// for triggers on the all-succeeds path, regardless of routine count.
func TestAttachScheduleState_QueryCountBound(t *testing.T) {
	svc, counting := newCountingRoutineService(t)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		mustCreateRoutine(t, svc, "r-"+string(rune('a'+i)), "Routine")
	}

	list, err := svc.ListRoutinesFromConfig(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list routines: %v", err)
	}
	if _, err := svc.AttachScheduleState(ctx, list); err != nil {
		t.Fatalf("attach schedule state: %v", err)
	}

	if counting.batchCalls != 1 {
		t.Errorf("batch calls = %d, want 1", counting.batchCalls)
	}
	if counting.singleCalls != 0 {
		t.Errorf("single calls = %d, want 0 on the all-succeeds path", counting.singleCalls)
	}
}

// TestAttachScheduleState_BatchFailureFallsBackPerRoutine covers the
// AC-OFFICE-ROUTINE-ARMING-002.7 documented exception: a batch failure
// re-reads every named routine individually, once each, rather than
// silently growing beyond the bounded path.
func TestAttachScheduleState_BatchFailureFallsBackPerRoutine(t *testing.T) {
	svc, counting := newCountingRoutineService(t)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		mustCreateRoutine(t, svc, "r-"+string(rune('a'+i)), "Routine")
	}
	list, err := svc.ListRoutinesFromConfig(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list routines: %v", err)
	}

	counting.forceBatchErr = true
	if _, err := svc.AttachScheduleState(ctx, list); err != nil {
		t.Fatalf("attach schedule state: %v", err)
	}

	if counting.batchCalls != 1 {
		t.Errorf("batch calls = %d, want 1", counting.batchCalls)
	}
	if counting.singleCalls != len(list) {
		t.Errorf("single calls = %d, want %d (once per routine)", counting.singleCalls, len(list))
	}
}

// TestAttachScheduleState_TriggerReadFailureKeepsIntent covers
// AC-OFFICE-ROUTINE-ARMING-002.12: a routine whose triggers could not be
// read still carries its real intent (Status) alongside schedule state
// unknown, rather than being dropped or blanked.
func TestAttachScheduleState_TriggerReadFailureKeepsIntent(t *testing.T) {
	svc, counting := newCountingRoutineService(t)
	ctx := context.Background()

	ok := mustCreateRoutine(t, svc, "r-ok", "OK")
	bad := mustCreateRoutine(t, svc, "r-bad", "Bad")

	list, err := svc.ListRoutinesFromConfig(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list routines: %v", err)
	}

	counting.forceBatchErr = true
	counting.forceSingleErrOn = map[string]bool{bad.ID: true}

	withSchedule, err := svc.AttachScheduleState(ctx, list)
	if err != nil {
		t.Fatalf("attach schedule state: %v", err)
	}

	okResult := findByID(t, withSchedule, ok.ID)
	if okResult.ScheduleState != routines.ScheduleStateUnscheduledNoTrigger {
		t.Errorf("ok routine state = %q, want unscheduled_no_trigger", okResult.ScheduleState)
	}

	badResult := findByID(t, withSchedule, bad.ID)
	if badResult.ScheduleState != routines.ScheduleStateUnknown {
		t.Errorf("bad routine state = %q, want unknown", badResult.ScheduleState)
	}
	if badResult.Status != "active" {
		t.Errorf("bad routine intent (Status) = %q, want active (preserved despite trigger read failure)", badResult.Status)
	}
}
