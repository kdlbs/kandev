package pause_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/pause"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	orchestratorexecutor "github.com/kandev/kandev/internal/orchestrator/executor"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// fakeRepo is an in-memory, script-driven double for pause.Repository. It
// lets tests force the exact create/re-read sequence the insert-retry-once
// control flow depends on, which a real SQLite race is not practical to
// reproduce deterministically.
type fakeRepo struct {
	createCalls int
	createErr   []error // consumed in order, one per CreateWorkspacePauseWithActivity call
	activeReads []*models.WorkspacePause
	activeErrs  []error

	releaseResult bool
	releaseErr    error

	activityEntries []*models.ActivityEntry
	activityErr     error

	inflightRuns    []models.InflightRun
	inflightErr     error
	liveRoutineIDs  []string
	liveRoutineErr  error
	cancelRunsCount int64
	cancelRunsErr   error
	releaseCkoutErr error
}

// GetActiveWorkspacePause pops the next scripted (record, error) pair off
// activeReads/activeErrs, in call order.
func (f *fakeRepo) GetActiveWorkspacePause(context.Context, string) (*models.WorkspacePause, error) {
	if len(f.activeReads) == 0 {
		return nil, nil
	}
	p := f.activeReads[0]
	f.activeReads = f.activeReads[1:]
	var err error
	if len(f.activeErrs) > 0 {
		err = f.activeErrs[0]
		f.activeErrs = f.activeErrs[1:]
	}
	return p, err
}

func (f *fakeRepo) CreateWorkspacePauseWithActivity(_ context.Context, _ *models.WorkspacePause, _ *models.ActivityEntry) error {
	var err error
	if f.createCalls < len(f.createErr) {
		err = f.createErr[f.createCalls]
	}
	f.createCalls++
	return err
}

func (f *fakeRepo) ReleaseWorkspacePauseWithActivity(context.Context, string, string, string, string, string) (bool, error) {
	return f.releaseResult, f.releaseErr
}

func (f *fakeRepo) CreateActivityEntry(_ context.Context, entry *models.ActivityEntry) error {
	f.activityEntries = append(f.activityEntries, entry)
	return f.activityErr
}

func (f *fakeRepo) ListInflightRunsForWorkspace(context.Context, string) ([]models.InflightRun, error) {
	return f.inflightRuns, f.inflightErr
}

func (f *fakeRepo) ListLiveRoutineTaskIDsForWorkspace(context.Context, string) ([]string, error) {
	return f.liveRoutineIDs, f.liveRoutineErr
}

func (f *fakeRepo) CancelRunsForWorkspace(context.Context, []string, string) (int64, error) {
	return f.cancelRunsCount, f.cancelRunsErr
}

func (f *fakeRepo) ReleaseCheckoutsForWorkspace(context.Context, []string) error {
	return f.releaseCkoutErr
}

// ctxAwareRepo wraps fakeRepo and surfaces the context's own error from
// every halt-sweep method, letting a test distinguish "the sweep ran on an
// already-cancelled context" from "the sweep ran on a live one" — Pause must
// detach the sweep's context from the caller's request context, since a
// client disconnect must not silently defeat the halt sweep.
type ctxAwareRepo struct {
	*fakeRepo
}

func (r *ctxAwareRepo) ListInflightRunsForWorkspace(ctx context.Context, workspaceID string) ([]models.InflightRun, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return r.fakeRepo.ListInflightRunsForWorkspace(ctx, workspaceID)
}

func (r *ctxAwareRepo) CancelRunsForWorkspace(ctx context.Context, runIDs []string, reason string) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	return r.fakeRepo.CancelRunsForWorkspace(ctx, runIDs, reason)
}

func (r *ctxAwareRepo) ReleaseCheckoutsForWorkspace(ctx context.Context, runIDs []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return r.fakeRepo.ReleaseCheckoutsForWorkspace(ctx, runIDs)
}

func (r *ctxAwareRepo) ListLiveRoutineTaskIDsForWorkspace(ctx context.Context, workspaceID string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return r.fakeRepo.ListLiveRoutineTaskIDsForWorkspace(ctx, workspaceID)
}

// ctxAwareCanceller wraps fakeCanceller and surfaces the context's own
// error, the same way ctxAwareRepo does for the repository methods.
type ctxAwareCanceller struct {
	*fakeCanceller
}

func (c *ctxAwareCanceller) CancelTaskExecution(ctx context.Context, taskID, reason string, force bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.fakeCanceller.CancelTaskExecution(ctx, taskID, reason, force)
}

// fakeCanceller is a script-driven double for pause.TaskCanceller.
type fakeCanceller struct {
	results map[string]error
	calls   []string
}

func (f *fakeCanceller) CancelTaskExecution(_ context.Context, taskID string, _ string, _ bool) error {
	f.calls = append(f.calls, taskID)
	return f.results[taskID]
}

// fakeWorkspaces is a script-driven double for pause.WorkspaceChecker.
// lookupErr, when set, is returned for every id regardless of known —
// simulating a real backend fault (DB down, context cancelled) distinct
// from the task service's own not-found sentinel.
type fakeWorkspaces struct {
	known     map[string]bool
	lookupErr error
}

func (f *fakeWorkspaces) GetWorkspace(_ context.Context, id string) (*taskmodels.Workspace, error) {
	if f.lookupErr != nil {
		return nil, f.lookupErr
	}
	if f.known[id] {
		return &taskmodels.Workspace{ID: id}, nil
	}
	return nil, repoerrors.ErrWorkspaceNotFound
}

func newTestService(repo pause.Repository, canceller pause.TaskCanceller, workspaces pause.WorkspaceChecker) *pause.Service {
	return pause.NewService(repo, canceller, workspaces, logger.Default())
}

// noopDetails decodes a workspace_pause_noop entry's structured Details
// field (docs/specs/office/system-design/workspace-kill-switch-02.md's
// Observability section: "details carries the reason, and for the no-op
// which operation was requested and why it committed nothing").
type noopDetails struct {
	RequestedOp string `json:"requested_op"`
	Reason      string `json:"reason"`
	Cause       string `json:"cause"`
}

func decodeNoopDetails(t *testing.T, raw string) noopDetails {
	t.Helper()
	var d noopDetails
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		t.Fatalf("decode noop details %q: %v", raw, err)
	}
	return d
}

// TestPauseState_NoActiveRecord proves the exported gate predicate reads
// (nil, nil) when the workspace is running.
func TestPauseState_NoActiveRecord(t *testing.T) {
	repo := &fakeRepo{}
	svc := newTestService(repo, &fakeCanceller{}, &fakeWorkspaces{known: map[string]bool{"ws-1": true}})

	got, err := svc.PauseState(context.Background(), "ws-1")
	if err != nil || got != nil {
		t.Fatalf("PauseState = (%v, %v), want (nil, nil)", got, err)
	}
}

// TestPause_WorkspaceNotFound proves AC-006.9: pause on an unknown
// workspace returns ErrWorkspaceNotFound before touching the pause table.
func TestPause_WorkspaceNotFound(t *testing.T) {
	repo := &fakeRepo{}
	svc := newTestService(repo, &fakeCanceller{}, &fakeWorkspaces{known: map[string]bool{}})

	_, err := svc.Pause(context.Background(), "ws-missing", "incident", "user-1", "user")
	if !errors.Is(err, pause.ErrWorkspaceNotFound) {
		t.Fatalf("err = %v, want ErrWorkspaceNotFound", err)
	}
	if repo.createCalls != 0 {
		t.Fatalf("createCalls = %d, want 0 (existence check must run first)", repo.createCalls)
	}
}

// TestPause_WorkspaceLookupFailureIsNotWorkspaceNotFound proves a genuine
// backend fault on the existence check (not the task service's own
// not-found sentinel) must not be rewritten to ErrWorkspaceNotFound —
// otherwise a real 500-class fault is hidden behind "workspace not found".
func TestPause_WorkspaceLookupFailureIsNotWorkspaceNotFound(t *testing.T) {
	lookupErr := errors.New("db unavailable")
	svc := newTestService(&fakeRepo{}, &fakeCanceller{}, &fakeWorkspaces{lookupErr: lookupErr})

	_, err := svc.Pause(context.Background(), "ws-1", "incident", "user-1", "user")
	if errors.Is(err, pause.ErrWorkspaceNotFound) {
		t.Fatalf("err = %v, must not be ErrWorkspaceNotFound for a backend fault", err)
	}
	if !errors.Is(err, lookupErr) {
		t.Fatalf("err = %v, want it to wrap the underlying lookup error", err)
	}
}

// TestPause_RejectsEmptyAndOverLongReason proves the 0/500/501 code-point
// boundary on the trimmed value.
func TestPause_RejectsEmptyAndOverLongReason(t *testing.T) {
	svc := newTestService(&fakeRepo{}, &fakeCanceller{}, &fakeWorkspaces{known: map[string]bool{"ws-1": true}})

	if _, err := svc.Pause(context.Background(), "ws-1", "   ", "user-1", "user"); !errors.Is(err, pause.ErrReasonRequired) {
		t.Fatalf("blank reason err = %v, want ErrReasonRequired", err)
	}

	long501 := make([]rune, 501)
	for i := range long501 {
		long501[i] = '字'
	}
	if _, err := svc.Pause(context.Background(), "ws-1", string(long501), "user-1", "user"); !errors.Is(err, pause.ErrReasonTooLong) {
		t.Fatalf("501-codepoint reason err = %v, want ErrReasonTooLong", err)
	}

	long500 := long501[:500]
	repo := &fakeRepo{}
	svc = newTestService(repo, &fakeCanceller{}, &fakeWorkspaces{known: map[string]bool{"ws-1": true}})
	if _, err := svc.Pause(context.Background(), "ws-1", string(long500), "user-1", "user"); err != nil {
		t.Fatalf("500-codepoint reason rejected: %v", err)
	}
}

// TestPause_RepeatPauseReturnsExistingRecordAndLogsNoop proves the
// ordinary repeat-pause branch (F40/round 5): no race, just an operator
// pressing the button twice. It must return the existing record and still
// write an auditable workspace_pause_noop entry (-005.8).
func TestPause_RepeatPauseReturnsExistingRecordAndLogsNoop(t *testing.T) {
	existing := &models.WorkspacePause{ID: "pause-1", WorkspaceID: "ws-1", Reason: "first"}
	repo := &fakeRepo{
		createErr:   []error{officesqlite.ErrWorkspaceAlreadyPaused},
		activeReads: []*models.WorkspacePause{existing},
	}
	svc := newTestService(repo, &fakeCanceller{}, &fakeWorkspaces{known: map[string]bool{"ws-1": true}})

	result, err := svc.Pause(context.Background(), "ws-1", "second attempt", "user-2", "user")
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if result.Pause != existing {
		t.Fatalf("Pause returned %+v, want the existing record", result.Pause)
	}
	if len(repo.activityEntries) != 1 || repo.activityEntries[0].Action != models.ActivityActionWorkspacePauseNoop {
		t.Fatalf("activity entries = %+v, want one workspace_pause_noop entry", repo.activityEntries)
	}
	details := decodeNoopDetails(t, repo.activityEntries[0].Details)
	if details.RequestedOp != "pause" || details.Reason != "second attempt" || details.Cause != "already_paused" {
		t.Fatalf("noop details = %+v, want requested_op=pause reason=%q cause=already_paused", details, "second attempt")
	}
}

// TestPause_RepeatPauseStillResweepsInFlightWork proves AC-003.6: a repeat
// pause runs the halt sweep again, not just a no-op activity write.
// TestPause_RepeatPauseReturnsExistingRecordAndLogsNoop's fixture seeds no
// in-flight runs, so it would still pass even if a repeat pause skipped
// the sweep entirely — this seeds an in-flight run and a live heavy
// routine so the resweep's own work is actually exercised and asserted.
func TestPause_RepeatPauseStillResweepsInFlightWork(t *testing.T) {
	existing := &models.WorkspacePause{ID: "pause-1", WorkspaceID: "ws-1", Reason: "first"}
	repo := &fakeRepo{
		createErr:       []error{officesqlite.ErrWorkspaceAlreadyPaused},
		activeReads:     []*models.WorkspacePause{existing},
		inflightRuns:    []models.InflightRun{{RunID: "run-1", TaskID: "task-1"}},
		liveRoutineIDs:  []string{"heavy-task-1"},
		cancelRunsCount: 1,
	}
	canceller := &fakeCanceller{results: map[string]error{}}
	svc := newTestService(repo, canceller, &fakeWorkspaces{known: map[string]bool{"ws-1": true}})

	result, err := svc.Pause(context.Background(), "ws-1", "second attempt", "user-2", "user")
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if result.Pause != existing {
		t.Fatalf("Pause returned %+v, want the existing record", result.Pause)
	}
	if result.Sweep.RunsCancelled != 1 {
		t.Fatalf("Sweep.RunsCancelled = %d, want 1 — the resweep must still process in-flight work", result.Sweep.RunsCancelled)
	}
	if result.Sweep.ExecutionsCancelled != 2 {
		t.Fatalf("Sweep.ExecutionsCancelled = %d, want 2 (task-1 and heavy-task-1)", result.Sweep.ExecutionsCancelled)
	}
	if len(canceller.calls) != 2 {
		t.Fatalf("canceller calls = %v, want 2 — the resweep must not be skipped on a repeat pause", canceller.calls)
	}
}

// TestPause_ContendedAfterLosingRaceToResumeTwice proves the insert-retry-
// once contract: an insert that loses to a concurrent resume retries once;
// a second no-row re-read returns ErrPauseContended rather than looping.
// system-design-01.md's Pause step 4 ties the noop write to "the one branch
// that commits nothing and returns an error" — exactly one entry for the
// whole contended sequence, not one per losing attempt.
func TestPause_ContendedAfterLosingRaceToResumeTwice(t *testing.T) {
	repo := &fakeRepo{
		createErr:   []error{officesqlite.ErrWorkspaceAlreadyPaused, officesqlite.ErrWorkspaceAlreadyPaused},
		activeReads: []*models.WorkspacePause{nil, nil}, // both re-reads find no active record
	}
	svc := newTestService(repo, &fakeCanceller{}, &fakeWorkspaces{known: map[string]bool{"ws-1": true}})

	_, err := svc.Pause(context.Background(), "ws-1", "incident", "user-1", "user")
	if !errors.Is(err, pause.ErrPauseContended) {
		t.Fatalf("err = %v, want ErrPauseContended", err)
	}
	if repo.createCalls != 2 {
		t.Fatalf("createCalls = %d, want exactly 2 (retry exactly once)", repo.createCalls)
	}
	if len(repo.activityEntries) != 1 {
		t.Fatalf("activity entries = %d, want exactly 1 (only the branch that returns the error is audited)", len(repo.activityEntries))
	}
	details := decodeNoopDetails(t, repo.activityEntries[0].Details)
	if details.RequestedOp != "pause" || details.Reason != "incident" || details.Cause != "lost_race" {
		t.Fatalf("noop details = %+v, want requested_op=pause reason=incident cause=lost_race", details)
	}
}

// TestPause_RetryWinsAgainstConcurrentResumeLogsNoNoop proves the fixed
// double-audit bug: an insert that loses to a concurrent resume on attempt
// 1, then succeeds outright on attempt 2, must not also leave behind a
// workspace_pause_noop entry for the attempt-1 loss — the request
// succeeded, so only the workspace_paused entry belongs in the log.
func TestPause_RetryWinsAgainstConcurrentResumeLogsNoNoop(t *testing.T) {
	repo := &fakeRepo{
		createErr:   []error{officesqlite.ErrWorkspaceAlreadyPaused, nil},
		activeReads: []*models.WorkspacePause{nil}, // attempt 1's re-read finds no active record (a resume raced it)
	}
	svc := newTestService(repo, &fakeCanceller{}, &fakeWorkspaces{known: map[string]bool{"ws-1": true}})

	result, err := svc.Pause(context.Background(), "ws-1", "incident", "user-1", "user")
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if result.Pause == nil {
		t.Fatalf("Pause returned nil record, want the attempt-2 insert's own candidate")
	}
	if repo.createCalls != 2 {
		t.Fatalf("createCalls = %d, want exactly 2 (attempt 1 lost, attempt 2 succeeded)", repo.createCalls)
	}
	if len(repo.activityEntries) != 0 {
		t.Fatalf("activity entries via CreateActivityEntry = %d, want 0 — a successful retry must not log a lost-race noop", len(repo.activityEntries))
	}
}

// TestPause_RetryWinsAgainstConcurrentPause proves the sibling case: an
// insert that loses to a concurrent PAUSE (not a resume) re-reads and
// returns that record on the first attempt, not a 409 — the pause-versus-
// pause race is distinct from the pause-resume-pause sequence.
func TestPause_RetryWinsAgainstConcurrentPause(t *testing.T) {
	winner := &models.WorkspacePause{ID: "pause-winner", WorkspaceID: "ws-1"}
	repo := &fakeRepo{
		createErr:   []error{officesqlite.ErrWorkspaceAlreadyPaused},
		activeReads: []*models.WorkspacePause{winner},
	}
	svc := newTestService(repo, &fakeCanceller{}, &fakeWorkspaces{known: map[string]bool{"ws-1": true}})

	result, err := svc.Pause(context.Background(), "ws-1", "incident", "user-1", "user")
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if result.Pause != winner {
		t.Fatalf("Pause returned %+v, want the concurrent winner", result.Pause)
	}
}

// TestResume_NotPausedReleasesNothingButLogsNoop proves AC-005.6/F49: a
// resume on an unpaused workspace reports success and still writes its
// audit entry.
func TestResume_NotPausedReleasesNothingButLogsNoop(t *testing.T) {
	repo := &fakeRepo{}
	svc := newTestService(repo, &fakeCanceller{}, &fakeWorkspaces{known: map[string]bool{"ws-1": true}})

	result, err := svc.Resume(context.Background(), "ws-1", "", "user-1", "user")
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if result.Released {
		t.Fatal("expected Released=false for an unpaused workspace")
	}
	if len(repo.activityEntries) != 1 || repo.activityEntries[0].Action != models.ActivityActionWorkspacePauseNoop {
		t.Fatalf("activity entries = %+v, want one workspace_pause_noop entry", repo.activityEntries)
	}
	details := decodeNoopDetails(t, repo.activityEntries[0].Details)
	if details.RequestedOp != "resume" || details.Cause != "not_paused" {
		t.Fatalf("noop details = %+v, want requested_op=resume cause=not_paused", details)
	}
}

// TestResume_ReleasesActivePause proves the ordinary resume path.
func TestResume_ReleasesActivePause(t *testing.T) {
	repo := &fakeRepo{
		activeReads:   []*models.WorkspacePause{{ID: "pause-1", WorkspaceID: "ws-1"}},
		releaseResult: true,
	}
	svc := newTestService(repo, &fakeCanceller{}, &fakeWorkspaces{known: map[string]bool{"ws-1": true}})

	result, err := svc.Resume(context.Background(), "ws-1", "resolved", "user-1", "user")
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	if !result.Released {
		t.Fatal("expected Released=true")
	}
}

// TestHaltSweep_IdleTaskIncrementsNotRunningNotFailures proves the
// three-way count partition (F45): a task with nothing to stop must not
// be reported as a failed cancellation.
func TestHaltSweep_IdleTaskIncrementsNotRunningNotFailures(t *testing.T) {
	repo := &fakeRepo{
		inflightRuns: []models.InflightRun{{RunID: "run-1", TaskID: "task-1"}},
	}
	canceller := &fakeCanceller{results: map[string]error{"task-1": orchestratorexecutor.ErrExecutionNotFound}}
	svc := newTestService(repo, canceller, &fakeWorkspaces{known: map[string]bool{"ws-1": true}})

	result, err := svc.Pause(context.Background(), "ws-1", "incident", "user-1", "user")
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if result.Sweep.ExecutionsNotRunning != 1 {
		t.Fatalf("ExecutionsNotRunning = %d, want 1", result.Sweep.ExecutionsNotRunning)
	}
	if result.Sweep.Failures != 0 {
		t.Fatalf("Failures = %d, want 0", result.Sweep.Failures)
	}
}

// TestHaltSweep_SubStepFailuresFoldIntoFailures proves F48: a failure in
// any of sub-steps (a)/(b)/(c) — not just the per-task cancellation in
// (d) — is counted, so a pause whose run cancellation failed outright does
// not report an indistinguishable "clean pause of an empty workspace".
func TestHaltSweep_SubStepFailuresFoldIntoFailures(t *testing.T) {
	repo := &fakeRepo{
		inflightRuns:   []models.InflightRun{{RunID: "run-1", TaskID: "task-1"}},
		cancelRunsErr:  errors.New("db unavailable"),
		liveRoutineErr: errors.New("db unavailable"),
	}
	svc := newTestService(repo, &fakeCanceller{}, &fakeWorkspaces{known: map[string]bool{"ws-1": true}})

	result, err := svc.Pause(context.Background(), "ws-1", "incident", "user-1", "user")
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if result.Sweep.Failures == 0 {
		t.Fatal("expected a non-zero failure count when sub-steps (b) and the live-routine query fail")
	}
}

// TestHaltSweep_HeavyPathCancelsTasklessLiveRoutine proves the second
// task-discovery source: a live heavy-routine task with no runs row is
// still cancelled.
func TestHaltSweep_HeavyPathCancelsTasklessLiveRoutine(t *testing.T) {
	repo := &fakeRepo{
		liveRoutineIDs: []string{"heavy-task-1"},
	}
	canceller := &fakeCanceller{results: map[string]error{}}
	svc := newTestService(repo, canceller, &fakeWorkspaces{known: map[string]bool{"ws-1": true}})

	result, err := svc.Pause(context.Background(), "ws-1", "incident", "user-1", "user")
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if result.Sweep.ExecutionsCancelled != 1 {
		t.Fatalf("ExecutionsCancelled = %d, want 1", result.Sweep.ExecutionsCancelled)
	}
	if len(canceller.calls) != 1 || canceller.calls[0] != "heavy-task-1" {
		t.Fatalf("canceller calls = %v, want [heavy-task-1]", canceller.calls)
	}
}

// TestPause_HaltSweepSurvivesCallerContextCancellation proves the halt
// sweep's context is detached from the caller's request context: even
// though the ctx passed to Pause is already cancelled (simulating a client
// that disconnected right after the pause record committed), the sweep's
// own repository/canceller calls must not observe that cancellation.
func TestPause_HaltSweepSurvivesCallerContextCancellation(t *testing.T) {
	repo := &ctxAwareRepo{fakeRepo: &fakeRepo{
		inflightRuns:    []models.InflightRun{{RunID: "run-1", TaskID: "task-1"}},
		liveRoutineIDs:  []string{"heavy-task-1"},
		cancelRunsCount: 1,
	}}
	canceller := &ctxAwareCanceller{fakeCanceller: &fakeCanceller{results: map[string]error{}}}
	svc := newTestService(repo, canceller, &fakeWorkspaces{known: map[string]bool{"ws-1": true}})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // simulate a client disconnect before the halt sweep runs

	result, err := svc.Pause(ctx, "ws-1", "incident", "user-1", "user")
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if result.Sweep.Failures != 0 {
		t.Fatalf("Sweep.Failures = %d, want 0 — halt sweep must not inherit the caller's cancelled context", result.Sweep.Failures)
	}
	if result.Sweep.RunsCancelled != 1 {
		t.Fatalf("Sweep.RunsCancelled = %d, want 1", result.Sweep.RunsCancelled)
	}
	if result.Sweep.ExecutionsCancelled != 2 {
		t.Fatalf("Sweep.ExecutionsCancelled = %d, want 2", result.Sweep.ExecutionsCancelled)
	}
}

// TestHaltSweep_UnionDeduplicatesTaskNamedByBothSources proves a task
// named by both the run-derived set and the live-routine set is cancelled
// exactly once.
func TestHaltSweep_UnionDeduplicatesTaskNamedByBothSources(t *testing.T) {
	repo := &fakeRepo{
		inflightRuns:   []models.InflightRun{{RunID: "run-1", TaskID: "task-shared"}},
		liveRoutineIDs: []string{"task-shared"},
	}
	canceller := &fakeCanceller{results: map[string]error{}}
	svc := newTestService(repo, canceller, &fakeWorkspaces{known: map[string]bool{"ws-1": true}})

	result, err := svc.Pause(context.Background(), "ws-1", "incident", "user-1", "user")
	if err != nil {
		t.Fatalf("Pause: %v", err)
	}
	if result.Sweep.ExecutionsCancelled != 1 {
		t.Fatalf("ExecutionsCancelled = %d, want 1 (deduplicated)", result.Sweep.ExecutionsCancelled)
	}
	if len(canceller.calls) != 1 {
		t.Fatalf("canceller calls = %v, want exactly one call for the shared task", canceller.calls)
	}
}
