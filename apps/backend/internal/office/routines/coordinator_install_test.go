package routines

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap/zapcore"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

func newCoordinatorInstallRepo(t *testing.T) *sqlite.Repository {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	// A single physical connection, matching internal/db.OpenSQLite's
	// production writer pool: an in-memory database is per-connection, so
	// without this, a second concurrent BeginTxx can be handed a fresh
	// connection to an empty database instead of blocking on the one
	// initSchema() already populated.
	db.SetMaxOpenConns(1)
	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	return repo
}

func newCoordinatorInstallService(t *testing.T, repo Repository) *RoutineService {
	t.Helper()
	log, _ := newObservedLogger(t, zapcore.InfoLevel)
	return NewRoutineService(repo, log, nil)
}

func patchRoutineCreatedAt(t *testing.T, repo *sqlite.Repository, id string, createdAt time.Time) {
	t.Helper()
	if _, err := repo.ExecRaw(context.Background(),
		`UPDATE office_routines SET created_at = ? WHERE id = ?`, createdAt, id); err != nil {
		t.Fatalf("patch created_at for %s: %v", id, err)
	}
}

// coordinatorInstallRepoFailures wraps a real repository, letting a test
// force InstallCoordinatorRoutine to fail outright (before decide ever
// runs) or wrap the tx-scoped writer decide is given so a specific write
// inside it fails instead.
type coordinatorInstallRepoFailures struct {
	*sqlite.Repository
	failLookup error
	// createRoutineErr, when set, is what CreateRoutine returns instead of
	// the generic "forced routine create failure" text — lets a test
	// control decide's own error text (e.g. to collide with the SQLite
	// busy-text heuristic).
	createRoutineErr  error
	failTriggerRead   bool
	failCreateRoutine bool
	failCreateTrigger bool
	// failCommitAfterDecide, when set, is returned instead of nil once the
	// wrapped InstallCoordinatorRoutine call has already succeeded —
	// simulating WithCoordinatorInstallLock's own tx.Commit() failing after
	// decide returned nil, a failure point CoordinatorInstallTx has no way
	// to trigger directly since it never exposes Commit to decide.
	failCommitAfterDecide error
	installDecideCalls    int
	triggerDecideCalls    int
}

func (f *coordinatorInstallRepoFailures) InstallCoordinatorRoutine(
	ctx context.Context,
	workspaceID, agentID, canonicalName string,
	decide func(ctx context.Context, matches []*Routine, tx models.CoordinatorInstallTx) error,
) error {
	if f.failLookup != nil {
		return f.failLookup
	}
	err := f.Repository.InstallCoordinatorRoutine(ctx, workspaceID, agentID, canonicalName,
		func(ctx context.Context, matches []*Routine, tx models.CoordinatorInstallTx) error {
			f.installDecideCalls++
			return decide(ctx, matches, &coordinatorInstallTxFailures{
				CoordinatorInstallTx: tx,
				failCreateRoutine:    f.failCreateRoutine,
				createRoutineErr:     f.createRoutineErr,
			})
		})
	if err == nil && f.failCommitAfterDecide != nil {
		return f.failCommitAfterDecide
	}
	return err
}

func (f *coordinatorInstallRepoFailures) EnsureCoordinatorTrigger(
	ctx context.Context,
	routineID string,
	decide func(ctx context.Context, triggers []*RoutineTrigger, tx models.CoordinatorInstallTx) error,
) error {
	if f.failTriggerRead {
		return errors.New("forced trigger read failure")
	}
	return f.Repository.EnsureCoordinatorTrigger(ctx, routineID,
		func(ctx context.Context, triggers []*RoutineTrigger, tx models.CoordinatorInstallTx) error {
			f.triggerDecideCalls++
			return decide(ctx, triggers, &coordinatorInstallTxFailures{
				CoordinatorInstallTx: tx,
				failCreateTrigger:    f.failCreateTrigger,
			})
		})
}

type coordinatorInstallTxFailures struct {
	models.CoordinatorInstallTx
	failCreateRoutine bool
	createRoutineErr  error
	failCreateTrigger bool
}

func (w *coordinatorInstallTxFailures) CreateRoutine(ctx context.Context, routine *models.Routine) error {
	if w.createRoutineErr != nil {
		return w.createRoutineErr
	}
	if w.failCreateRoutine {
		return errors.New("forced routine create failure")
	}
	return w.CoordinatorInstallTx.CreateRoutine(ctx, routine)
}

func (w *coordinatorInstallTxFailures) CreateRoutineTrigger(ctx context.Context, t *models.RoutineTrigger) error {
	if w.failCreateTrigger {
		return errors.New("forced trigger create failure")
	}
	return w.CoordinatorInstallTx.CreateRoutineTrigger(ctx, t)
}

// AC-OFFICE-COORDINATOR-INSTALL-001.14: empty workspace or assignee is
// rejected before any identity lookup, distinguishable from AC-001.7's
// read failure. A nil Repository proves no repo call happens: any call
// panics with a nil-interface dereference and fails the test loudly.
func TestCreateDefaultCoordinatorRoutine_EmptyIdentityRejected(t *testing.T) {
	log, logs := newObservedLogger(t, zapcore.InfoLevel)
	svc := NewRoutineService(nil, log, nil)
	before := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionEmptyIdentity, "workspace", "", "assignee", "agent-1")))

	routine, err := svc.CreateDefaultCoordinatorRoutine(context.Background(), "", "agent-1")

	if err == nil {
		t.Fatal("expected error for empty workspace_id")
	}
	if routine != nil {
		t.Errorf("expected nil routine, got %+v", routine)
	}
	after := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionEmptyIdentity, "workspace", "", "assignee", "agent-1")))
	if after != before+1 {
		t.Errorf("empty_identity counter = %d, want %d", after, before+1)
	}
	if logs.Len() != 1 || logs.All()[0].Level != zapcore.WarnLevel {
		t.Fatalf("log entries = %+v, want exactly one warn", logs.All())
	}

	if _, err := svc.CreateDefaultCoordinatorRoutine(context.Background(), "ws-1", ""); err == nil {
		t.Fatal("expected error for empty agent_id")
	}
}

// AC-OFFICE-COORDINATOR-INSTALL-001.12: no match creates a routine
// (status=active) plus its canonical trigger, enabled, with the canonical
// cron/timezone and a next_run_at strictly after the instant the install
// captured at its start.
func TestCreateDefaultCoordinatorRoutine_NoMatchCreatesRoutineAndTrigger(t *testing.T) {
	repo := newCoordinatorInstallRepo(t)
	svc := newCoordinatorInstallService(t, repo)

	before := time.Now().UTC()
	routine, err := svc.CreateDefaultCoordinatorRoutine(context.Background(), "ws-1", "agent-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if routine == nil {
		t.Fatal("expected a routine")
	}
	if routine.Status != "active" {
		t.Errorf("status = %q, want active", routine.Status)
	}
	if routine.WorkspaceID != "ws-1" || routine.AssigneeAgentProfileID != "agent-1" || routine.Name != CoordinatorRoutineName {
		t.Errorf("routine identity = %+v", routine)
	}

	triggers, err := repo.ListTriggersByRoutineID(context.Background(), routine.ID)
	if err != nil {
		t.Fatalf("list triggers: %v", err)
	}
	if len(triggers) != 1 {
		t.Fatalf("trigger count = %d, want 1", len(triggers))
	}
	trigger := triggers[0]
	if !trigger.Enabled {
		t.Error("canonical trigger should be enabled")
	}
	if trigger.CronExpression != CoordinatorRoutineCron || trigger.Timezone != "UTC" {
		t.Errorf("trigger schedule = %q/%q", trigger.CronExpression, trigger.Timezone)
	}
	if trigger.NextRunAt == nil || !trigger.NextRunAt.After(before) {
		t.Errorf("next_run_at = %v, want non-nil and after %v", trigger.NextRunAt, before)
	}
}

// AC-OFFICE-COORDINATOR-INSTALL-001.2/.5: a single match with any cron
// trigger (canonical or not, enabled or disabled) is returned unchanged;
// no second trigger is created, and the schedule-state condition fires.
func TestCreateDefaultCoordinatorRoutine_SingleMatchWithCronTriggerUnchanged(t *testing.T) {
	repo := newCoordinatorInstallRepo(t)
	existing := &Routine{
		ID: "r-existing", WorkspaceID: "ws-1", Name: CoordinatorRoutineName, TaskTemplate: "",
		AssigneeAgentProfileID: "agent-1", Status: "active", ConcurrencyPolicy: "coalesce_if_active", Variables: "{}",
	}
	if err := repo.CreateRoutine(context.Background(), existing); err != nil {
		t.Fatalf("seed routine: %v", err)
	}
	if err := repo.CreateRoutineTrigger(context.Background(), &RoutineTrigger{
		ID: "t-existing", RoutineID: existing.ID, Kind: "cron",
		CronExpression: "0 * * * *", Timezone: "America/New_York", Enabled: false,
	}); err != nil {
		t.Fatalf("seed trigger: %v", err)
	}

	svc := newCoordinatorInstallService(t, repo)
	before := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionScheduleState, "workspace", "ws-1", "assignee", "agent-1")))

	routine, err := svc.CreateDefaultCoordinatorRoutine(context.Background(), "ws-1", "agent-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if routine.ID != existing.ID {
		t.Errorf("returned routine id = %q, want %q", routine.ID, existing.ID)
	}
	if routine.Status != "active" {
		t.Errorf("status changed to %q", routine.Status)
	}

	triggers, err := repo.ListTriggersByRoutineID(context.Background(), existing.ID)
	if err != nil {
		t.Fatalf("list triggers: %v", err)
	}
	if len(triggers) != 1 {
		t.Fatalf("trigger count = %d, want 1 (no second trigger created)", len(triggers))
	}
	if triggers[0].Enabled || triggers[0].CronExpression != "0 * * * *" {
		t.Errorf("existing trigger mutated: %+v", triggers[0])
	}

	after := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionScheduleState, "workspace", "ws-1", "assignee", "agent-1")))
	if after != before+1 {
		t.Errorf("schedule_state counter = %d, want %d", after, before+1)
	}
}

// AC-OFFICE-COORDINATOR-INSTALL-001.4: a matched routine with no cron
// trigger at all gets the canonical trigger completed. Re-running the
// install afterwards is then a no-op via the .5 branch (re-entrancy).
func TestCreateDefaultCoordinatorRoutine_MatchWithNoTriggerCompletesInstall(t *testing.T) {
	repo := newCoordinatorInstallRepo(t)
	existing := &Routine{
		ID: "r-half", WorkspaceID: "ws-1", Name: CoordinatorRoutineName, TaskTemplate: "",
		AssigneeAgentProfileID: "agent-1", Status: "active", ConcurrencyPolicy: "coalesce_if_active", Variables: "{}",
	}
	if err := repo.CreateRoutine(context.Background(), existing); err != nil {
		t.Fatalf("seed routine: %v", err)
	}

	svc := newCoordinatorInstallService(t, repo)
	before := time.Now().UTC()
	routine, err := svc.CreateDefaultCoordinatorRoutine(context.Background(), "ws-1", "agent-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if routine.ID != existing.ID {
		t.Errorf("returned routine id = %q, want %q", routine.ID, existing.ID)
	}

	triggers, err := repo.ListTriggersByRoutineID(context.Background(), existing.ID)
	if err != nil {
		t.Fatalf("list triggers: %v", err)
	}
	if len(triggers) != 1 {
		t.Fatalf("trigger count = %d, want 1", len(triggers))
	}
	if !triggers[0].Enabled || triggers[0].NextRunAt == nil || !triggers[0].NextRunAt.After(before) {
		t.Errorf("completed trigger = %+v", triggers[0])
	}

	// Re-entrancy: calling again must not create a second trigger.
	if _, err := svc.CreateDefaultCoordinatorRoutine(context.Background(), "ws-1", "agent-1"); err != nil {
		t.Fatalf("second call unexpected error: %v", err)
	}
	triggers, err = repo.ListTriggersByRoutineID(context.Background(), existing.ID)
	if err != nil {
		t.Fatalf("list triggers after second call: %v", err)
	}
	if len(triggers) != 1 {
		t.Fatalf("trigger count after second call = %d, want 1", len(triggers))
	}
}

// AC-OFFICE-COORDINATOR-INSTALL-001.3: two or more matches select the
// earliest by created_at (ties by id), report the duplicate, and leave the
// non-selected routine and its triggers entirely untouched while the
// selected routine still runs through .4.
func TestCreateDefaultCoordinatorRoutine_DuplicateMatchesSelectsEarliestAndCompletes(t *testing.T) {
	repo := newCoordinatorInstallRepo(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	earlier := &Routine{
		ID: "r-earlier", WorkspaceID: "ws-1", Name: CoordinatorRoutineName, TaskTemplate: "",
		AssigneeAgentProfileID: "agent-1", Status: "active", ConcurrencyPolicy: "coalesce_if_active", Variables: "{}",
	}
	if err := repo.CreateRoutine(context.Background(), earlier); err != nil {
		t.Fatalf("seed earlier routine: %v", err)
	}
	patchRoutineCreatedAt(t, repo, earlier.ID, base)

	later := &Routine{
		ID: "r-later", WorkspaceID: "ws-1", Name: CoordinatorRoutineName, TaskTemplate: "",
		AssigneeAgentProfileID: "agent-1", Status: "active", ConcurrencyPolicy: "coalesce_if_active", Variables: "{}",
	}
	if err := repo.CreateRoutine(context.Background(), later); err != nil {
		t.Fatalf("seed later routine: %v", err)
	}
	patchRoutineCreatedAt(t, repo, later.ID, base.Add(time.Minute))
	if err := repo.CreateRoutineTrigger(context.Background(), &RoutineTrigger{
		ID: "t-later", RoutineID: later.ID, Kind: "cron",
		CronExpression: "0 * * * *", Timezone: "UTC", Enabled: true,
	}); err != nil {
		t.Fatalf("seed later trigger: %v", err)
	}

	svc := newCoordinatorInstallService(t, repo)
	before := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionDuplicateMatches, "workspace", "ws-1", "assignee", "agent-1")))

	routine, err := svc.CreateDefaultCoordinatorRoutine(context.Background(), "ws-1", "agent-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if routine.ID != earlier.ID {
		t.Errorf("selected routine id = %q, want earliest %q", routine.ID, earlier.ID)
	}

	after := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionDuplicateMatches, "workspace", "ws-1", "assignee", "agent-1")))
	if after != before+1 {
		t.Errorf("duplicate_matches counter = %d, want %d", after, before+1)
	}

	earlierTriggers, err := repo.ListTriggersByRoutineID(context.Background(), earlier.ID)
	if err != nil {
		t.Fatalf("list earlier triggers: %v", err)
	}
	if len(earlierTriggers) != 1 {
		t.Fatalf("earlier trigger count = %d, want 1 (completed)", len(earlierTriggers))
	}

	laterTriggers, err := repo.ListTriggersByRoutineID(context.Background(), later.ID)
	if err != nil {
		t.Fatalf("list later triggers: %v", err)
	}
	if len(laterTriggers) != 1 || laterTriggers[0].ID != "t-later" {
		t.Errorf("later routine's trigger was touched: %+v", laterTriggers)
	}

	laterRoutine, err := repo.GetRoutine(context.Background(), later.ID)
	if err != nil {
		t.Fatalf("get later routine: %v", err)
	}
	if laterRoutine.Status != "active" {
		t.Errorf("non-selected routine status changed to %q", laterRoutine.Status)
	}
}

// AC-OFFICE-COORDINATOR-INSTALL-001.3: when two duplicate matches share the
// identical created_at, the tie is broken by id ascending — this is the
// case TestCreateDefaultCoordinatorRoutine_DuplicateMatchesSelectsEarliestAndCompletes
// above cannot exercise, since its two routines differ by created_at alone.
func TestCreateDefaultCoordinatorRoutine_DuplicateMatchesTiesBreakByIDAscending(t *testing.T) {
	repo := newCoordinatorInstallRepo(t)
	tied := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	lowerID := &Routine{
		ID: "r-aaa-lower", WorkspaceID: "ws-1", Name: CoordinatorRoutineName, TaskTemplate: "",
		AssigneeAgentProfileID: "agent-1", Status: "active", ConcurrencyPolicy: "coalesce_if_active", Variables: "{}",
	}
	if err := repo.CreateRoutine(context.Background(), lowerID); err != nil {
		t.Fatalf("seed lower-id routine: %v", err)
	}
	patchRoutineCreatedAt(t, repo, lowerID.ID, tied)

	higherID := &Routine{
		ID: "r-zzz-higher", WorkspaceID: "ws-1", Name: CoordinatorRoutineName, TaskTemplate: "",
		AssigneeAgentProfileID: "agent-1", Status: "active", ConcurrencyPolicy: "coalesce_if_active", Variables: "{}",
	}
	if err := repo.CreateRoutine(context.Background(), higherID); err != nil {
		t.Fatalf("seed higher-id routine: %v", err)
	}
	patchRoutineCreatedAt(t, repo, higherID.ID, tied)
	if err := repo.CreateRoutineTrigger(context.Background(), &RoutineTrigger{
		ID: "t-higher", RoutineID: higherID.ID, Kind: "cron",
		CronExpression: "0 * * * *", Timezone: "UTC", Enabled: true,
	}); err != nil {
		t.Fatalf("seed higher-id trigger: %v", err)
	}

	svc := newCoordinatorInstallService(t, repo)

	routine, err := svc.CreateDefaultCoordinatorRoutine(context.Background(), "ws-1", "agent-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if routine.ID != lowerID.ID {
		t.Errorf("selected routine id = %q, want the lower id %q for the created_at tie", routine.ID, lowerID.ID)
	}

	lowerTriggers, err := repo.ListTriggersByRoutineID(context.Background(), lowerID.ID)
	if err != nil {
		t.Fatalf("list lower-id triggers: %v", err)
	}
	if len(lowerTriggers) != 1 {
		t.Fatalf("lower-id trigger count = %d, want 1 (completed)", len(lowerTriggers))
	}

	higherTriggers, err := repo.ListTriggersByRoutineID(context.Background(), higherID.ID)
	if err != nil {
		t.Fatalf("list higher-id triggers: %v", err)
	}
	if len(higherTriggers) != 1 || higherTriggers[0].ID != "t-higher" {
		t.Errorf("non-selected (higher id) routine's trigger was touched: %+v", higherTriggers)
	}

	higherRoutine, err := repo.GetRoutine(context.Background(), higherID.ID)
	if err != nil {
		t.Fatalf("get higher-id routine: %v", err)
	}
	if higherRoutine.Status != "active" {
		t.Errorf("non-selected (higher id) routine status changed to %q", higherRoutine.Status)
	}
}

// AC-OFFICE-COORDINATOR-INSTALL-001.6: an existing match's status is never
// changed by either the .4 (repair) or .5 (unchanged) branch, even when
// the routine's status is not "active".
func TestCreateDefaultCoordinatorRoutine_NeverChangesStatus(t *testing.T) {
	repo := newCoordinatorInstallRepo(t)
	paused := &Routine{
		ID: "r-paused", WorkspaceID: "ws-1", Name: CoordinatorRoutineName, TaskTemplate: "",
		AssigneeAgentProfileID: "agent-1", Status: "paused", ConcurrencyPolicy: "coalesce_if_active", Variables: "{}",
	}
	if err := repo.CreateRoutine(context.Background(), paused); err != nil {
		t.Fatalf("seed routine: %v", err)
	}

	svc := newCoordinatorInstallService(t, repo)
	routine, err := svc.CreateDefaultCoordinatorRoutine(context.Background(), "ws-1", "agent-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if routine.Status != "paused" {
		t.Errorf("status changed to %q on repair branch, want paused", routine.Status)
	}

	// Second call takes the .5 unchanged branch (a cron trigger now exists).
	routine, err = svc.CreateDefaultCoordinatorRoutine(context.Background(), "ws-1", "agent-1")
	if err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}
	if routine.Status != "paused" {
		t.Errorf("status changed to %q on unchanged branch, want paused", routine.Status)
	}
}

// AC-OFFICE-COORDINATOR-INSTALL-001.7: an identity-lookup failure rejects
// the call, creates nothing, and is reported distinctly (lookup_failed),
// with decide never invoked (also demonstrating AC-001.15's no-retry rule:
// exactly one attempt).
func TestCreateDefaultCoordinatorRoutine_IdentityLookupFailureRejectsAndCreatesNothing(t *testing.T) {
	repo := newCoordinatorInstallRepo(t)
	failing := &coordinatorInstallRepoFailures{Repository: repo, failLookup: errors.New("boom")}
	svc := newCoordinatorInstallService(t, failing)
	before := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionLookupFailed, "workspace", "ws-1", "assignee", "agent-1")))

	routine, err := svc.CreateDefaultCoordinatorRoutine(context.Background(), "ws-1", "agent-1")
	if err == nil {
		t.Fatal("expected error")
	}
	if routine != nil {
		t.Errorf("expected nil routine, got %+v", routine)
	}
	if failing.installDecideCalls != 0 {
		t.Errorf("install decide called %d times, want 0", failing.installDecideCalls)
	}
	after := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionLookupFailed, "workspace", "ws-1", "assignee", "agent-1")))
	if after != before+1 {
		t.Errorf("lookup_failed counter = %d, want %d", after, before+1)
	}

	routines, err := repo.ListRoutines(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("list routines: %v", err)
	}
	if len(routines) != 0 {
		t.Errorf("routines created = %d, want 0", len(routines))
	}
}

// AC-OFFICE-COORDINATOR-INSTALL-001.7: a trigger-read failure on a matched
// routine rejects the call and creates nothing, reported distinctly
// (trigger_read_failed) from an identity-lookup failure.
func TestCreateDefaultCoordinatorRoutine_TriggerReadFailureRejectsAndCreatesNothing(t *testing.T) {
	repo := newCoordinatorInstallRepo(t)
	existing := &Routine{
		ID: "r-existing", WorkspaceID: "ws-1", Name: CoordinatorRoutineName, TaskTemplate: "",
		AssigneeAgentProfileID: "agent-1", Status: "active", ConcurrencyPolicy: "coalesce_if_active", Variables: "{}",
	}
	if err := repo.CreateRoutine(context.Background(), existing); err != nil {
		t.Fatalf("seed routine: %v", err)
	}

	failing := &coordinatorInstallRepoFailures{Repository: repo, failTriggerRead: true}
	svc := newCoordinatorInstallService(t, failing)
	before := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionTriggerReadFailed, "workspace", "ws-1", "assignee", "agent-1")))

	routine, err := svc.CreateDefaultCoordinatorRoutine(context.Background(), "ws-1", "agent-1")
	if err == nil {
		t.Fatal("expected error")
	}
	if routine == nil || routine.ID != existing.ID {
		t.Errorf("expected selected routine returned alongside error, got %+v", routine)
	}
	if failing.installDecideCalls != 1 {
		t.Errorf("install decide called %d times, want 1", failing.installDecideCalls)
	}
	if failing.triggerDecideCalls != 0 {
		t.Errorf("trigger decide called %d times, want 0", failing.triggerDecideCalls)
	}
	after := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionTriggerReadFailed, "workspace", "ws-1", "assignee", "agent-1")))
	if after != before+1 {
		t.Errorf("trigger_read_failed counter = %d, want %d", after, before+1)
	}

	triggers, err := repo.ListTriggersByRoutineID(context.Background(), existing.ID)
	if err != nil {
		t.Fatalf("list triggers: %v", err)
	}
	if len(triggers) != 0 {
		t.Errorf("triggers created = %d, want 0", len(triggers))
	}
}

// AC-OFFICE-COORDINATOR-INSTALL-001.12: a routine-insert failure creates no
// trigger.
func TestCreateDefaultCoordinatorRoutine_RoutineInsertFailureCreatesNoTrigger(t *testing.T) {
	repo := newCoordinatorInstallRepo(t)
	failing := &coordinatorInstallRepoFailures{Repository: repo, failCreateRoutine: true}
	svc := newCoordinatorInstallService(t, failing)
	before := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionRoutineCreateFailed, "workspace", "ws-1", "assignee", "agent-1")))

	routine, err := svc.CreateDefaultCoordinatorRoutine(context.Background(), "ws-1", "agent-1")
	if err == nil {
		t.Fatal("expected error")
	}
	if routine != nil {
		t.Errorf("expected nil routine, got %+v", routine)
	}
	after := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionRoutineCreateFailed, "workspace", "ws-1", "assignee", "agent-1")))
	if after != before+1 {
		t.Errorf("routine_create_failed counter = %d, want %d", after, before+1)
	}

	routines, err := repo.ListRoutines(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("list routines: %v", err)
	}
	if len(routines) != 0 {
		t.Errorf("routines created = %d, want 0", len(routines))
	}
}

// AC-OFFICE-COORDINATOR-INSTALL-001.12: a trigger-creation failure after a
// successful routine insert leaves the routine in place — a half-finished
// install a follow-up call then completes (AC-001.4 re-entrancy).
func TestCreateDefaultCoordinatorRoutine_TriggerCreateFailureLeavesRoutineInPlace(t *testing.T) {
	repo := newCoordinatorInstallRepo(t)
	failing := &coordinatorInstallRepoFailures{Repository: repo, failCreateTrigger: true}
	svc := newCoordinatorInstallService(t, failing)

	routine, err := svc.CreateDefaultCoordinatorRoutine(context.Background(), "ws-1", "agent-1")
	if err == nil {
		t.Fatal("expected error")
	}
	if routine == nil {
		t.Fatal("expected routine returned alongside error")
	}

	routines, err := repo.ListRoutines(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("list routines: %v", err)
	}
	if len(routines) != 1 {
		t.Fatalf("routines created = %d, want 1 (half-finished install preserved)", len(routines))
	}
	triggers, err := repo.ListTriggersByRoutineID(context.Background(), routine.ID)
	if err != nil {
		t.Fatalf("list triggers: %v", err)
	}
	if len(triggers) != 0 {
		t.Fatalf("triggers created = %d, want 0", len(triggers))
	}

	// A follow-up install (no failures injected) completes it.
	plainSvc := newCoordinatorInstallService(t, repo)
	completed, err := plainSvc.CreateDefaultCoordinatorRoutine(context.Background(), "ws-1", "agent-1")
	if err != nil {
		t.Fatalf("follow-up install unexpected error: %v", err)
	}
	if completed.ID != routine.ID {
		t.Errorf("follow-up returned a different routine: %q vs %q", completed.ID, routine.ID)
	}
	triggers, err = repo.ListTriggersByRoutineID(context.Background(), routine.ID)
	if err != nil {
		t.Fatalf("list triggers after follow-up: %v", err)
	}
	if len(triggers) != 1 {
		t.Fatalf("triggers after follow-up = %d, want 1", len(triggers))
	}
}

// AC-OFFICE-COORDINATOR-INSTALL-001.13: contention (the lock's acquisition
// bound was reached) is reported distinctly from an identity-lookup
// failure and from context cancellation.
func TestCreateDefaultCoordinatorRoutine_ContentionReportedDistinctFromOtherFailures(t *testing.T) {
	repo := newCoordinatorInstallRepo(t)
	failing := &coordinatorInstallRepoFailures{Repository: repo, failLookup: models.ErrCoordinatorInstallContention}
	svc := newCoordinatorInstallService(t, failing)

	beforeContention := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionContention, "workspace", "ws-1", "assignee", "agent-1")))
	beforeLookup := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionLookupFailed, "workspace", "ws-1", "assignee", "agent-1")))

	_, err := svc.CreateDefaultCoordinatorRoutine(context.Background(), "ws-1", "agent-1")
	if !errors.Is(err, models.ErrCoordinatorInstallContention) {
		t.Fatalf("expected contention error, got %v", err)
	}

	afterContention := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionContention, "workspace", "ws-1", "assignee", "agent-1")))
	afterLookup := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionLookupFailed, "workspace", "ws-1", "assignee", "agent-1")))
	if afterContention != beforeContention+1 {
		t.Errorf("contention counter = %d, want %d", afterContention, beforeContention+1)
	}
	if afterLookup != beforeLookup {
		t.Errorf("lookup_failed counter changed to %d, want unchanged %d", afterLookup, beforeLookup)
	}
}

// AC-OFFICE-COORDINATOR-INSTALL-001.13: the caller's own context ending
// while waiting is reported distinctly from both contention and a plain
// lookup failure.
func TestCreateDefaultCoordinatorRoutine_ContextCancelledReportedDistinctFromContention(t *testing.T) {
	repo := newCoordinatorInstallRepo(t)
	failing := &coordinatorInstallRepoFailures{Repository: repo, failLookup: context.Canceled}
	svc := newCoordinatorInstallService(t, failing)

	beforeCancelled := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionCancelled, "workspace", "ws-1", "assignee", "agent-1")))
	beforeContention := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionContention, "workspace", "ws-1", "assignee", "agent-1")))

	_, err := svc.CreateDefaultCoordinatorRoutine(context.Background(), "ws-1", "agent-1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	afterCancelled := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionCancelled, "workspace", "ws-1", "assignee", "agent-1")))
	afterContention := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionContention, "workspace", "ws-1", "assignee", "agent-1")))
	if afterCancelled != beforeCancelled+1 {
		t.Errorf("cancelled counter = %d, want %d", afterCancelled, beforeCancelled+1)
	}
	if afterContention != beforeContention {
		t.Errorf("contention counter changed to %d, want unchanged %d", afterContention, beforeContention)
	}
}

// AC-OFFICE-COORDINATOR-INSTALL-001.13: decision.errored must gate before
// the Contention case in reportCoordinatorLockFailure. decide's own
// CreateRoutine failure is reported once, at its own failure site, as
// routine_create_failed; classifyCoordinatorInstallWaitErr's busy-text
// heuristic reclassifies the returned error as ErrCoordinatorInstallContention
// purely because the error text happens to contain "database is locked" —
// but since decide already reported, that must not also increment the
// contention counter a second time.
func TestCreateDefaultCoordinatorRoutine_DecideOwnErrorMatchingBusyTextNotDoubleReported(t *testing.T) {
	repo := newCoordinatorInstallRepo(t)
	failing := &coordinatorInstallRepoFailures{
		Repository:       repo,
		createRoutineErr: errors.New("routine insert failed: database is locked"),
	}
	svc := newCoordinatorInstallService(t, failing)

	beforeContention := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionContention, "workspace", "ws-1", "assignee", "agent-1")))
	beforeRoutineCreateFailed := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionRoutineCreateFailed, "workspace", "ws-1", "assignee", "agent-1")))

	_, err := svc.CreateDefaultCoordinatorRoutine(context.Background(), "ws-1", "agent-1")
	if err == nil {
		t.Fatal("expected error")
	}

	afterContention := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionContention, "workspace", "ws-1", "assignee", "agent-1")))
	afterRoutineCreateFailed := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionRoutineCreateFailed, "workspace", "ws-1", "assignee", "agent-1")))
	if afterContention != beforeContention {
		t.Errorf("contention counter = %d, want unchanged at %d (decide already reported its own failure; no double report)", afterContention, beforeContention)
	}
	if afterRoutineCreateFailed != beforeRoutineCreateFailed+1 {
		t.Errorf("routine_create_failed counter = %d, want %d", afterRoutineCreateFailed, beforeRoutineCreateFailed+1)
	}
}

// AC-OFFICE-COORDINATOR-INSTALL-001.12/.13: a failure surfacing after decide
// itself already ran and returned nil (the enclosing transaction failing to
// commit) is reported as commit_failed, not silently dropped and not folded
// into contention or cancelled. failCommitAfterDecide lets the real decide
// callback run to completion (a real commit happens underneath), then
// substitutes a synthetic, unclassified error for InstallCoordinatorRoutine's
// return value — reproducing the exact decision shape
// (ran=true, errored=false, created=true) a genuine post-decide commit
// failure leaves behind, which is what reportCoordinatorLockFailure's
// default case must classify. It does not exercise the real
// WithCoordinatorInstallLock rollback path; that is covered separately at
// the repository layer (coordinator_install_test.go's
// TestWithCoordinatorInstallLock_FnContentionIsClassified and friends).
func TestCreateDefaultCoordinatorRoutine_CommitFailureAfterDecideSucceedsReportsCommitFailed(t *testing.T) {
	repo := newCoordinatorInstallRepo(t)
	failing := &coordinatorInstallRepoFailures{
		Repository:            repo,
		failCommitAfterDecide: errors.New("forced commit failure: disk I/O error"),
	}
	svc := newCoordinatorInstallService(t, failing)

	beforeCommitFailed := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionCommitFailed, "workspace", "ws-1", "assignee", "agent-1")))
	beforeContention := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionContention, "workspace", "ws-1", "assignee", "agent-1")))
	beforeCreated := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionRoutineCreated, "workspace", "ws-1", "assignee", "agent-1")))

	_, err := svc.CreateDefaultCoordinatorRoutine(context.Background(), "ws-1", "agent-1")
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, models.ErrCoordinatorInstallContention) {
		t.Fatalf("a plain synthetic commit error must not be misclassified as contention, got %v", err)
	}

	afterCommitFailed := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionCommitFailed, "workspace", "ws-1", "assignee", "agent-1")))
	afterContention := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionContention, "workspace", "ws-1", "assignee", "agent-1")))
	afterCreated := counterValue(coordinatorInstallConditionsTotal.Get(
		coordinatorInstallLabel("condition", coordinatorInstallConditionRoutineCreated, "workspace", "ws-1", "assignee", "agent-1")))
	if afterCommitFailed != beforeCommitFailed+1 {
		t.Errorf("commit_failed counter = %d, want %d", afterCommitFailed, beforeCommitFailed+1)
	}
	if afterContention != beforeContention {
		t.Errorf("contention counter changed to %d, want unchanged %d", afterContention, beforeContention)
	}
	if afterCreated != beforeCreated {
		t.Errorf("routine_created counter changed to %d, want unchanged %d (err != nil, so the created report must not fire)", afterCreated, beforeCreated)
	}
}

// AC-OFFICE-COORDINATOR-INSTALL-001.8/.9/.11: two concurrent installs for
// the same identity, sharing one writer connection (the real deployment
// shape: one Kandev backend process, one SQLite writer pool of size 1 —
// see internal/db.OpenSQLite), create at most one routine and at most one
// canonical trigger between them. Go's database/sql pool hands out that
// one physical connection to only one BeginTxx caller at a time, which is
// exactly the mechanism WithCoordinatorInstallLock's SQLite doc comment
// describes.
func TestCreateDefaultCoordinatorRoutine_ConcurrentInstallsCreateAtMostOne(t *testing.T) {
	repo := newCoordinatorInstallRepo(t)
	svc := newCoordinatorInstallService(t, repo)

	type result struct {
		routine *Routine
		err     error
	}
	resultsCh := make(chan result, 2)
	run := func() {
		routine, err := svc.CreateDefaultCoordinatorRoutine(context.Background(), "ws-1", "agent-1")
		resultsCh <- result{routine, err}
	}
	go run()
	go run()

	var results [2]result
	for i := range results {
		results[i] = <-resultsCh
	}
	for i, res := range results {
		if res.err != nil {
			t.Fatalf("call %d unexpected error: %v", i, res.err)
		}
	}
	if results[0].routine.ID != results[1].routine.ID {
		t.Errorf("concurrent installs returned different routines: %q vs %q",
			results[0].routine.ID, results[1].routine.ID)
	}

	routines, err := repo.ListRoutines(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("list routines: %v", err)
	}
	if len(routines) != 1 {
		t.Fatalf("routines created = %d, want 1", len(routines))
	}
	triggers, err := repo.ListTriggersByRoutineID(context.Background(), routines[0].ID)
	if err != nil {
		t.Fatalf("list triggers: %v", err)
	}
	if len(triggers) != 1 {
		t.Fatalf("triggers created = %d, want 1", len(triggers))
	}
}
