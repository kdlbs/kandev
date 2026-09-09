package routines

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
	"github.com/kandev/kandev/internal/office/shared"
)

// fakePauseGate is a scripted shared.PauseGate double: pop-front semantics
// let a test sequence a gate-error attempt followed by a since-resolved
// clean read, mirroring fakeRepo's convention in office/pause's own tests.
type fakePauseGate struct {
	active []*models.WorkspacePause
	errs   []error
	calls  []string
}

func (f *fakePauseGate) PauseState(_ context.Context, workspaceID string) (*models.WorkspacePause, error) {
	f.calls = append(f.calls, workspaceID)
	var active *models.WorkspacePause
	if len(f.active) > 0 {
		active, f.active = f.active[0], f.active[1:]
	}
	var err error
	if len(f.errs) > 0 {
		err, f.errs = f.errs[0], f.errs[1:]
	}
	return active, err
}

// newGatedTestRoutineService mirrors service_test.go's newTestRoutineService
// but returns the internal *RoutineService type (this file is in package
// routines, not routines_test) so tests can reach unexported members like
// processCronTrigger.
func newGatedTestRoutineService(t *testing.T) (*RoutineService, *sqlite.Repository) {
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
	return NewRoutineService(repo, logger.Default(), &noopGateActivity{}), repo
}

type noopGateActivity struct{}

func (n *noopGateActivity) LogActivity(_ context.Context, _, _, _, _, _, _, _ string) {}
func (n *noopGateActivity) LogActivityWithRun(_ context.Context, _, _, _, _, _, _, _, _, _ string) {
}

func createGatedTestRoutine(t *testing.T, repo *sqlite.Repository, policy string) *Routine {
	t.Helper()
	r := &Routine{
		WorkspaceID:       "ws-1",
		Name:              "Gated Routine",
		TaskTemplate:      `{"title":"{{name}} - {{date}}","description":"Run for {{date}}"}`,
		Status:            "active",
		ConcurrencyPolicy: models.RoutineConcurrencyPolicy(policy),
		Variables:         `{"name":{"default":"Daily Check"}}`,
	}
	if err := repo.CreateRoutine(context.Background(), r); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	return r
}

// createUnattributedTestRoutine mirrors createGatedTestRoutine but leaves
// WorkspaceID empty, the "no workspace attribution" case AC-002.10 and the
// design's "Routine dispatch performs no lookup" paragraph both cover.
func createUnattributedTestRoutine(t *testing.T, repo *sqlite.Repository, policy string) *Routine {
	t.Helper()
	r := &Routine{
		Name:              "Unattributed Routine",
		TaskTemplate:      `{"title":"{{name}} - {{date}}","description":"Run for {{date}}"}`,
		Status:            "active",
		ConcurrencyPolicy: models.RoutineConcurrencyPolicy(policy),
		Variables:         `{"name":{"default":"Daily Check"}}`,
	}
	if err := repo.CreateRoutine(context.Background(), r); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	return r
}

// TestDispatch_BlockedByPause_SkipsRunAndRecordsSkippedRow proves a
// confirmed pause blocks every dispatch entry point (manual fires here)
// without writing an office_routine_runs row via the normal path — the
// only row written is the CreatePauseSkippedRoutineRun skip record.
func TestDispatch_BlockedByPause_SkipsRunAndRecordsSkippedRow(t *testing.T) {
	svc, repo := newGatedTestRoutineService(t)
	routine := createGatedTestRoutine(t, repo, "always_create")

	gate := &fakePauseGate{active: []*models.WorkspacePause{{ID: "pause-1", WorkspaceID: "ws-1"}}}
	svc.SetPauseGate(gate)

	_, err := svc.FireManual(context.Background(), routine.ID, nil)
	if !errors.Is(err, shared.ErrWorkspacePaused) {
		t.Fatalf("err = %v, want shared.ErrWorkspacePaused", err)
	}
	if len(gate.calls) != 1 || gate.calls[0] != "ws-1" {
		t.Fatalf("gate.calls = %v, want [ws-1] — the gate must be asked about the routine's own workspace", gate.calls)
	}

	runs, err := repo.ListAllRuns(context.Background(), "ws-1", 10)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want 1 skip record", len(runs))
	}
	if runs[0].Status != models.RoutineRunStatusSkipped {
		t.Errorf("status = %q, want skipped", runs[0].Status)
	}
	if runs[0].SkipReason != "workspace_paused" {
		t.Errorf("skip_reason = %q, want workspace_paused", runs[0].SkipReason)
	}
	if runs[0].PauseID != "pause-1" {
		t.Errorf("pause_id = %q, want pause-1", runs[0].PauseID)
	}
}

// TestDispatch_SecondBlockedFireSamePause_WritesNoDuplicateRow proves the
// (routine_id, pause_id) partial unique index makes the skip record
// idempotent across repeated blocked fires under the same pause.
func TestDispatch_SecondBlockedFireSamePause_WritesNoDuplicateRow(t *testing.T) {
	svc, repo := newGatedTestRoutineService(t)
	routine := createGatedTestRoutine(t, repo, "always_create")

	gate := &fakePauseGate{active: []*models.WorkspacePause{
		{ID: "pause-1", WorkspaceID: "ws-1"},
		{ID: "pause-1", WorkspaceID: "ws-1"},
	}}
	svc.SetPauseGate(gate)

	ctx := context.Background()
	if _, err := svc.FireManual(ctx, routine.ID, nil); !errors.Is(err, shared.ErrWorkspacePaused) {
		t.Fatalf("first fire err = %v, want shared.ErrWorkspacePaused", err)
	}
	if _, err := svc.FireManual(ctx, routine.ID, nil); !errors.Is(err, shared.ErrWorkspacePaused) {
		t.Fatalf("second fire err = %v, want shared.ErrWorkspacePaused", err)
	}

	runs, err := repo.ListAllRuns(ctx, "ws-1", 10)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("len(runs) = %d, want exactly 1 skip record for repeated blocked fires", len(runs))
	}
}

// TestDispatch_BlockedFiresDifferentPauses_EachWritesOwnSkipRow is the
// other half of AC-002.13's dedup proof: the partial unique index is
// keyed on (routine_id, pause_id), not routine_id alone, so two blocked
// fires under two DIFFERENT pause records (a resume, then a new pause)
// each write their own skip row rather than the second being silently
// swallowed as if it were a repeat under the first pause.
// TestDispatch_SecondBlockedFireSamePause_WritesNoDuplicateRow alone would
// still pass even if the index were mistakenly keyed on routine_id only.
func TestDispatch_BlockedFiresDifferentPauses_EachWritesOwnSkipRow(t *testing.T) {
	svc, repo := newGatedTestRoutineService(t)
	routine := createGatedTestRoutine(t, repo, "always_create")

	gate := &fakePauseGate{active: []*models.WorkspacePause{
		{ID: "pause-1", WorkspaceID: "ws-1"},
		{ID: "pause-2", WorkspaceID: "ws-1"},
	}}
	svc.SetPauseGate(gate)

	ctx := context.Background()
	if _, err := svc.FireManual(ctx, routine.ID, nil); !errors.Is(err, shared.ErrWorkspacePaused) {
		t.Fatalf("first fire err = %v, want shared.ErrWorkspacePaused", err)
	}
	if _, err := svc.FireManual(ctx, routine.ID, nil); !errors.Is(err, shared.ErrWorkspacePaused) {
		t.Fatalf("second fire err = %v, want shared.ErrWorkspacePaused", err)
	}

	runs, err := repo.ListAllRuns(ctx, "ws-1", 10)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("len(runs) = %d, want 2 skip records — one per distinct pause", len(runs))
	}
	pauseIDs := map[string]bool{}
	for _, r := range runs {
		if r.Status != models.RoutineRunStatusSkipped {
			t.Errorf("status = %q, want skipped", r.Status)
		}
		pauseIDs[r.PauseID] = true
	}
	if !pauseIDs["pause-1"] || !pauseIDs["pause-2"] {
		t.Fatalf("pause ids seen = %v, want both pause-1 and pause-2", pauseIDs)
	}
}

// TestDispatch_BlockedByPause_SkipInsertFailsForNonUniqueReason_StillBlocks
// proves checkPauseGate's "best-effort" comment: when
// CreatePauseSkippedRoutineRun fails for a reason OTHER than the partial
// unique index (a genuine write failure, simulated here by dropping the
// table), the fire must still be blocked — the caller gets
// shared.ErrWorkspacePaused either way, since the pause gate itself, not
// the skip-record write, is what decides whether the fire proceeds.
func TestDispatch_BlockedByPause_SkipInsertFailsForNonUniqueReason_StillBlocks(t *testing.T) {
	svc, repo := newGatedTestRoutineService(t)
	routine := createGatedTestRoutine(t, repo, "always_create")

	if _, err := repo.ExecRaw(context.Background(), `DROP TABLE office_routine_runs`); err != nil {
		t.Fatalf("drop office_routine_runs: %v", err)
	}

	gate := &fakePauseGate{active: []*models.WorkspacePause{{ID: "pause-1", WorkspaceID: "ws-1"}}}
	svc.SetPauseGate(gate)

	_, err := svc.FireManual(context.Background(), routine.ID, nil)
	if !errors.Is(err, shared.ErrWorkspacePaused) {
		t.Fatalf("err = %v, want shared.ErrWorkspacePaused despite the failed skip-record write", err)
	}
}

// TestDispatch_PauseGateError_FailsClosedWithNoRunRow proves a gate-read
// error fails dispatch closed (shared.ErrPauseGateUnavailable) and writes
// no row at all — nothing to retry from, so the next fire attempt is a
// clean slate.
func TestDispatch_PauseGateError_FailsClosedWithNoRunRow(t *testing.T) {
	svc, repo := newGatedTestRoutineService(t)
	routine := createGatedTestRoutine(t, repo, "always_create")

	gate := &fakePauseGate{errs: []error{errors.New("db unavailable")}}
	svc.SetPauseGate(gate)

	_, err := svc.FireManual(context.Background(), routine.ID, nil)
	if !errors.Is(err, shared.ErrPauseGateUnavailable) {
		t.Fatalf("err = %v, want shared.ErrPauseGateUnavailable", err)
	}

	runs, err := repo.ListAllRuns(context.Background(), "ws-1", 10)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("len(runs) = %d, want 0 (gate error must write no row)", len(runs))
	}
}

// TestDispatch_NoPauseGateWired_Unaffected proves the nil-gate default
// (existing tests, and any deployment not yet wiring SetPauseGate) keeps
// dispatching exactly as before the kill switch existed.
func TestDispatch_NoPauseGateWired_Unaffected(t *testing.T) {
	svc, repo := newGatedTestRoutineService(t)
	routine := createGatedTestRoutine(t, repo, "always_create")

	run, err := svc.FireManual(context.Background(), routine.ID, nil)
	if err != nil {
		t.Fatalf("fire manual: %v", err)
	}
	if run.Status != models.RoutineRunStatusTaskCreated {
		t.Errorf("status = %q, want task_created", run.Status)
	}
}

// TestDispatch_EmptyWorkspaceRoutine_GateErrorProceedsUngated proves the
// design's "Routine dispatch performs no lookup... An empty value there
// takes the not-found branch... so dispatch proceeds ungated rather than
// failing closed on a routine that carries no workspace"
// (workspace-kill-switch-01.md, Gate points): a routine with no
// WorkspaceID must not consult the pause gate at all, so a gate-read
// error (which would otherwise fail dispatch closed for an attributed
// routine) has no effect on it.
func TestDispatch_EmptyWorkspaceRoutine_GateErrorProceedsUngated(t *testing.T) {
	svc, repo := newGatedTestRoutineService(t)
	routine := createUnattributedTestRoutine(t, repo, "always_create")

	gate := &fakePauseGate{errs: []error{errors.New("db unavailable")}}
	svc.SetPauseGate(gate)

	run, err := svc.FireManual(context.Background(), routine.ID, nil)
	if err != nil {
		t.Fatalf("fire manual: %v, want no error — an unattributed routine must proceed ungated on a gate-read error", err)
	}
	if run.Status != models.RoutineRunStatusTaskCreated {
		t.Errorf("status = %q, want task_created", run.Status)
	}
	if len(gate.calls) != 0 {
		t.Fatalf("gate.calls = %v, want none — the gate must not be consulted for an unattributed routine", gate.calls)
	}

	runs, err := repo.ListAllRuns(context.Background(), "", 10)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Status == models.RoutineRunStatusSkipped {
		t.Fatalf("runs = %+v, want exactly the one dispatched run, no pause-skip row", runs)
	}
}

// TestDispatch_EmptyWorkspaceRoutine_ConfirmedPauseProceedsUngated covers
// the confirmed-pause half of the same rule for completeness: even if the
// gate were consulted and found an (impossible, since no pause record can
// name the empty workspace) active pause, an unattributed routine must
// still dispatch. Documents behavior that was already correct before this
// fix — no pause_id can ever equal "" — but was previously untested.
func TestDispatch_EmptyWorkspaceRoutine_ConfirmedPauseProceedsUngated(t *testing.T) {
	svc, repo := newGatedTestRoutineService(t)
	routine := createUnattributedTestRoutine(t, repo, "always_create")

	gate := &fakePauseGate{active: []*models.WorkspacePause{{ID: "pause-1", WorkspaceID: "ws-1"}}}
	svc.SetPauseGate(gate)

	run, err := svc.FireManual(context.Background(), routine.ID, nil)
	if err != nil {
		t.Fatalf("fire manual: %v, want no error", err)
	}
	if run.Status != models.RoutineRunStatusTaskCreated {
		t.Errorf("status = %q, want task_created", run.Status)
	}
	if len(gate.calls) != 0 {
		t.Fatalf("gate.calls = %v, want none — the gate must not be consulted for an unattributed routine", gate.calls)
	}
}

// TestProcessCronTrigger_BlockedByPause_SwallowsErrorForTickScheduledTriggers
// proves F-cron-swallow: a confirmed pause must not surface as a
// TickScheduledTriggers ERROR log (a paged on-call incident) for expected
// operator behaviour, while the blocked fire is still recorded.
func TestProcessCronTrigger_BlockedByPause_SwallowsErrorForTickScheduledTriggers(t *testing.T) {
	svc, repo := newGatedTestRoutineService(t)
	routine := createGatedTestRoutine(t, repo, "always_create")

	due := time.Now().UTC().Add(-time.Minute)
	trigger := &RoutineTrigger{
		RoutineID:      routine.ID,
		Kind:           "cron",
		CronExpression: "* * * * *",
		Timezone:       "UTC",
		Enabled:        true,
		NextRunAt:      &due,
	}
	if err := repo.CreateRoutineTrigger(context.Background(), trigger); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	gate := &fakePauseGate{active: []*models.WorkspacePause{{ID: "pause-1", WorkspaceID: "ws-1"}}}
	svc.SetPauseGate(gate)

	// Asserting TickScheduledTriggers's own return proves nothing here: its
	// per-trigger loop only logs processCronTrigger's error and always
	// returns nil itself, regardless of whether the swallow below actually
	// ran. Call processCronTrigger directly so a regression (returning err
	// instead of nil on a confirmed pause) fails this test.
	now := time.Now().UTC()
	if err := svc.processCronTrigger(context.Background(), trigger, now); err != nil {
		t.Fatalf("processCronTrigger: %v", err)
	}

	runs, err := repo.ListAllRuns(context.Background(), "ws-1", 10)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != models.RoutineRunStatusSkipped {
		t.Fatalf("runs = %+v, want exactly 1 skipped run", runs)
	}

	// AC-002.2: a blocked tick must still advance the trigger's cursor,
	// exactly like processCronTrigger's ordinary (unblocked) path —
	// otherwise NextRunAt stays stuck at the blocked fire time and every
	// subsequent tick while paused re-fires (and, on resume, bursts every
	// missed interval at once instead of resuming on schedule).
	triggers, err := repo.ListTriggersByRoutineID(context.Background(), routine.ID)
	if err != nil {
		t.Fatalf("list triggers: %v", err)
	}
	if len(triggers) != 1 || triggers[0].NextRunAt == nil {
		t.Fatalf("triggers = %+v, want exactly 1 trigger with NextRunAt set", triggers)
	}
	if !triggers[0].NextRunAt.After(due) {
		t.Fatalf("NextRunAt = %v, want advanced past the blocked tick's due time %v", triggers[0].NextRunAt, due)
	}
}

// TestProcessCronTrigger_PauseGateError_PropagatesAsTickError proves a
// gate-read error (distinct from a confirmed pause) is NOT swallowed —
// TickScheduledTriggers still logs it, since it's a real, retryable
// failure rather than expected operator behaviour.
func TestProcessCronTrigger_PauseGateError_PropagatesAsTickError(t *testing.T) {
	svc, repo := newGatedTestRoutineService(t)
	routine := createGatedTestRoutine(t, repo, "always_create")

	due := time.Now().UTC().Add(-time.Minute)
	trigger := &RoutineTrigger{
		RoutineID:      routine.ID,
		Kind:           "cron",
		CronExpression: "* * * * *",
		Timezone:       "UTC",
		Enabled:        true,
		NextRunAt:      &due,
	}
	if err := repo.CreateRoutineTrigger(context.Background(), trigger); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	gate := &fakePauseGate{errs: []error{errors.New("db unavailable")}}
	svc.SetPauseGate(gate)

	err := svc.processCronTrigger(context.Background(), trigger, time.Now().UTC())
	if !errors.Is(err, shared.ErrPauseGateUnavailable) {
		t.Fatalf("err = %v, want shared.ErrPauseGateUnavailable", err)
	}
}
