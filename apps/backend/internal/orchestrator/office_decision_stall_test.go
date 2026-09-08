package orchestrator

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"sync"
	"testing"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/workflow/engine"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Doubles for the decision-waiting detector. Each can be told to fail, because
// "the input could not be read" is a behaviour the detector must get right
// (fail closed, count the reason) and not merely a plumbing accident.

type stallParticipantStore struct {
	participants []engine.ParticipantInfo
	err          error
}

func (f *stallParticipantStore) ListStepParticipants(_ context.Context, _, _ string) ([]engine.ParticipantInfo, error) {
	return f.participants, f.err
}

func (f *stallParticipantStore) ListTaskParticipants(_ context.Context, _ string) ([]engine.ParticipantInfo, error) {
	return f.participants, f.err
}

type stallDecisionStore struct {
	decisions []engine.DecisionInfo
	err       error
	// recorded and cleared count writes. The detector must never perform
	// either: recording a decision would forge the quorum the Office gates
	// exist to require, and clearing one would discard a real reviewer's.
	recorded int
	cleared  int
}

func (f *stallDecisionStore) ListStepDecisions(_ context.Context, _, _ string) ([]engine.DecisionInfo, error) {
	return f.decisions, f.err
}

func (f *stallDecisionStore) RecordStepDecision(_ context.Context, _ engine.DecisionInfo) error {
	f.recorded++
	return nil
}

func (f *stallDecisionStore) ClearStepDecisions(_ context.Context, _, _ string) (int64, error) {
	f.cleared++
	return 0, nil
}

// stallRunQueue counts QueueRun calls so "the detector queues no run"
// (AC-003.2) is an assertion rather than an appeal to reading the source.
type stallRunQueue struct {
	calls int
}

func (q *stallRunQueue) QueueRun(_ context.Context, _ engine.QueueRunRequest) (engine.QueueOutcome, error) {
	q.calls++
	return engine.QueueOutcomeQueued, nil
}

type stallRunReader struct {
	inFlight bool
	err      error
	calls    int
}

func (f *stallRunReader) HasInFlightRunForTask(_ context.Context, _ string) (bool, error) {
	f.calls++
	return f.inFlight, f.err
}

func decidingSeat() []engine.ParticipantInfo {
	return []engine.ParticipantInfo{{
		ID:               "seat-1",
		StepID:           "step1",
		Role:             string(wfmodels.ParticipantRoleReviewer),
		AgentProfileID:   "agent-1",
		DecisionRequired: true,
	}}
}

func officeDecisionWaitingCount() int64 {
	v := officeStallDecisionWaitingTotal.Get(officeStallLabel("detector", "decision_waiting"))
	iv, ok := v.(*expvar.Int)
	if !ok {
		return 0
	}
	return iv.Value()
}

func officeStallSkipCount(reason string) int64 {
	v := officeStallDetectorSkippedTotal.Get(officeStallLabel("reason", reason))
	iv, ok := v.(*expvar.Int)
	if !ok {
		return 0
	}
	return iv.Value()
}

// seedDecisionWaitingTask makes an existing task look Office-owned, seats a
// decision-required reviewer at its step, and backdates the task past the
// threshold — the three facts the candidate query selects on.
func seedDecisionWaitingTask(t *testing.T, repo *sqliterepo.Repository, taskID, stepID string) {
	t.Helper()
	ctx := context.Background()

	// IsFromOffice is a derived projection, not a stored column, so take the
	// project_id branch rather than setting the struct field (which the write
	// path discards). Same reason as seedOfficeStrandedSignal.
	if _, err := repo.DB().ExecContext(ctx,
		`UPDATE tasks SET project_id = 'proj1' WHERE id = ?`, taskID); err != nil {
		t.Fatalf("mark task as office: %v", err)
	}
	if _, err := repo.DB().ExecContext(ctx, `
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES ('seat-'||?, ?, '', 'reviewer', 'agent-1', 1, 0)
	`, taskID, stepID); err != nil {
		t.Fatalf("seed decision seat: %v", err)
	}
	quiet := time.Now().UTC().Add(-2 * officeDecisionWaitingThreshold)
	if _, err := repo.DB().ExecContext(ctx,
		`UPDATE tasks SET updated_at = ? WHERE id = ?`, quiet, taskID); err != nil {
		t.Fatalf("backdate task: %v", err)
	}

	task, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if !task.IsFromOffice {
		t.Fatalf("precondition: task %s should read as an Office task", taskID)
	}
}

// decisionWaitingService builds a Service with the detector's three
// dependencies wired to the supplied doubles.
func decisionWaitingService(
	t *testing.T, repo *sqliterepo.Repository,
	participants *stallParticipantStore, decisions *stallDecisionStore, runs *stallRunReader,
) *Service {
	t.Helper()
	svc := createTestService(repo, officeStallStepGetter(), newMockTaskRepo())
	if participants != nil {
		svc.SetEngineParticipantStore(participants)
	}
	if decisions != nil {
		svc.SetEngineDecisionStore(decisions)
	}
	if runs != nil {
		svc.SetOfficeRunInFlightReader(runs)
	}
	return svc
}

// AC-002.1: an Office task past the threshold with an undecided seat and no
// run in flight is surfaced.
func TestDetectOfficeDecisionWaitingOnce_SurfacesWaitingTask(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedDecisionWaitingTask(t, repo, "t1", "step1")
	runs := &stallRunReader{}
	decisions := &stallDecisionStore{}
	queue := &stallRunQueue{}
	svc := decisionWaitingService(t, repo,
		&stallParticipantStore{participants: decidingSeat()}, decisions, runs)
	svc.SetEngineRunQueue(queue)

	before := officeDecisionWaitingCount()
	svc.detectOfficeDecisionWaitingOnce(context.Background())

	if got := officeDecisionWaitingCount() - before; got != 1 {
		t.Fatalf("decision-waiting counter delta = %d, want 1", got)
	}
	if runs.calls != 1 {
		t.Fatalf("run reader called %d times, want 1", runs.calls)
	}
	assertOfficeDecisionNotActedOn(t, repo, "t1", "step1", decisions, queue)
}

// A bounded page must not make the oldest candidates a permanent exclusion
// list. Every candidate past the first page still needs one evaluation during
// a scan pass.
func TestDetectOfficeDecisionWaitingOnce_ProcessesCandidatesBeyondFirstPage(t *testing.T) {
	const candidateCount = 201
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-000", "s-seed", "step1")
	seedDecisionWaitingTask(t, repo, "task-000", "step1")
	for i := 1; i < candidateCount; i++ {
		taskID := fmt.Sprintf("task-%03d", i)
		now := time.Now().UTC()
		if err := repo.CreateTask(context.Background(), &taskmodels.Task{
			ID:             taskID,
			WorkspaceID:    "ws1",
			WorkflowID:     "wf1",
			WorkflowStepID: "step1",
			Title:          taskID,
			State:          v1.TaskStateInProgress,
			CreatedAt:      now,
			UpdatedAt:      now,
		}); err != nil {
			t.Fatalf("create task %s: %v", taskID, err)
		}
		seedDecisionWaitingTask(t, repo, taskID, "step1")
	}
	svc := decisionWaitingService(t, repo,
		&stallParticipantStore{participants: decidingSeat()}, &stallDecisionStore{},
		&stallRunReader{})

	before := officeDecisionWaitingCount()
	svc.detectOfficeDecisionWaitingOnce(context.Background())

	if got := officeDecisionWaitingCount() - before; got != candidateCount {
		t.Fatalf("decision-waiting counter delta = %d, want %d", got, candidateCount)
	}
	if _, seen := svc.officeDecisionWaiting.Load(officeDecisionWaitingKey("task-200", "step1")); !seen {
		t.Fatal("candidate beyond the first page was not evaluated")
	}
}

// The Office dependency readers are wired after Service.Start. The detector
// can run during that interval, so late wiring and detector reads must be race
// free.
func TestOfficeDecisionWaitingLateRunReaderWiringIsRaceFree(t *testing.T) {
	repo := setupTestRepo(t)
	seedTaskWithoutSession(t, repo, "t1", "step1")
	seedDecisionWaitingTask(t, repo, "t1", "step1")
	svc := decisionWaitingService(t, repo,
		&stallParticipantStore{participants: decidingSeat()}, &stallDecisionStore{},
		&stallRunReader{})
	ctx := context.Background()

	const iterations = 1000
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		for i := 0; i < iterations; i++ {
			svc.detectOfficeDecisionWaitingOnce(ctx)
		}
	}()
	go func() {
		defer wg.Done()
		close(start)
		for i := 0; i < iterations; i++ {
			svc.SetOfficeRunInFlightReader(&stallRunReader{inFlight: i%2 == 0})
		}
	}()
	wg.Wait()
}

// AC-002.2: the false-positive guard. A task with a claimed run is being
// worked on, so it must not be reported however long its seat has gone
// undecided. This is the case named in the definition of done.
func TestDetectOfficeDecisionWaitingOnce_ClaimedRunIsNotSurfaced(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedDecisionWaitingTask(t, repo, "t1", "step1")
	svc := decisionWaitingService(t, repo,
		&stallParticipantStore{participants: decidingSeat()}, &stallDecisionStore{},
		&stallRunReader{inFlight: true})

	before := officeDecisionWaitingCount()
	svc.detectOfficeDecisionWaitingOnce(context.Background())

	if got := officeDecisionWaitingCount() - before; got != 0 {
		t.Fatalf("counter delta = %d, want 0 — a task with a run in flight is healthy", got)
	}
}

// AC-002.3: a decision already recorded for the current step suppresses
// surfacing.
func TestDetectOfficeDecisionWaitingOnce_RecordedDecisionSuppresses(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedDecisionWaitingTask(t, repo, "t1", "step1")
	runs := &stallRunReader{}
	svc := decisionWaitingService(t, repo,
		&stallParticipantStore{participants: decidingSeat()},
		&stallDecisionStore{decisions: []engine.DecisionInfo{{
			ID: "d1", TaskID: "t1", StepID: "step1", Decision: "approve",
		}}},
		runs)

	before := officeDecisionWaitingCount()
	svc.detectOfficeDecisionWaitingOnce(context.Background())

	if got := officeDecisionWaitingCount() - before; got != 0 {
		t.Fatalf("counter delta = %d, want 0 — the step already has a decision", got)
	}
	if runs.calls != 0 {
		t.Fatalf("run reader called %d times; the decision check must reject first", runs.calls)
	}
}

// AC-002.3 (superseded case): ListStepDecisions returns superseded rows
// alongside active ones (its own documented contract), so a step whose only
// decision is superseded must still read as undecided. maybeSupersedeOnRework
// leaves the row in place with superseded_at set rather than deleting it, so
// treating any non-empty result as "decided" would permanently suppress the
// repeat stall this detector exists to find.
func TestDetectOfficeDecisionWaitingOnce_SupersededDecisionDoesNotSuppress(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedDecisionWaitingTask(t, repo, "t1", "step1")
	runs := &stallRunReader{}
	supersededAt := time.Now().UTC().Add(-time.Hour)
	svc := decisionWaitingService(t, repo,
		&stallParticipantStore{participants: decidingSeat()},
		&stallDecisionStore{decisions: []engine.DecisionInfo{{
			ID: "d1", TaskID: "t1", StepID: "step1", Decision: "approve",
			SupersededAt: &supersededAt,
		}}},
		runs)

	before := officeDecisionWaitingCount()
	svc.detectOfficeDecisionWaitingOnce(context.Background())

	if got := officeDecisionWaitingCount() - before; got != 1 {
		t.Fatalf("counter delta = %d, want 1 — a superseded-only decision set is undecided", got)
	}
	if runs.calls != 1 {
		t.Fatalf("run reader called %d times, want 1 — the superseded row must not short-circuit the check", runs.calls)
	}
}

// AC-002.4: seats that owe no decision cannot strand a task.
func TestDetectOfficeDecisionWaitingOnce_NonDecidingSeatsAreNotSurfaced(t *testing.T) {
	cases := []struct {
		name string
		seat engine.ParticipantInfo
	}{
		{"watcher", engine.ParticipantInfo{Role: string(wfmodels.ParticipantRoleWatcher), DecisionRequired: true}},
		{"collaborator", engine.ParticipantInfo{Role: string(wfmodels.ParticipantRoleCollaborator), DecisionRequired: true}},
		{"runner", engine.ParticipantInfo{Role: string(wfmodels.ParticipantRoleRunner), DecisionRequired: true}},
		{"reviewer not required", engine.ParticipantInfo{Role: string(wfmodels.ParticipantRoleReviewer), DecisionRequired: false}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "step1")
			seedDecisionWaitingTask(t, repo, "t1", "step1")
			runs := &stallRunReader{}
			svc := decisionWaitingService(t, repo,
				&stallParticipantStore{participants: []engine.ParticipantInfo{tc.seat}},
				&stallDecisionStore{}, runs)

			before := officeDecisionWaitingCount()
			svc.detectOfficeDecisionWaitingOnce(context.Background())

			if got := officeDecisionWaitingCount() - before; got != 0 {
				t.Fatalf("counter delta = %d, want 0 — a %s seat owes no decision", got, tc.name)
			}
			if runs.calls != 0 {
				t.Fatalf("run reader called %d times; the seat check must reject first", runs.calls)
			}
		})
	}
}

// AC-002.5: every unreadable input fails closed and is counted, so a silently
// degraded detector is distinguishable from a quiet system.
func TestDetectOfficeDecisionWaitingOnce_UnreadableInputsFailClosed(t *testing.T) {
	readErr := errors.New("boom")
	cases := []struct {
		name         string
		participants *stallParticipantStore
		decisions    *stallDecisionStore
		runs         *stallRunReader
		reason       string
	}{
		{
			name:   "participant store unwired",
			runs:   &stallRunReader{},
			reason: officeStallSkipParticipantStore,
		},
		{
			name:         "decision store unwired",
			participants: &stallParticipantStore{participants: decidingSeat()},
			runs:         &stallRunReader{},
			reason:       officeStallSkipDecisionStore,
		},
		{
			name:         "participant read failed",
			participants: &stallParticipantStore{err: readErr},
			decisions:    &stallDecisionStore{},
			runs:         &stallRunReader{},
			reason:       officeStallSkipParticipantReadFailed,
		},
		{
			name:         "decision read failed",
			participants: &stallParticipantStore{participants: decidingSeat()},
			decisions:    &stallDecisionStore{err: readErr},
			runs:         &stallRunReader{},
			reason:       officeStallSkipDecisionReadFailed,
		},
		{
			name:         "run reader unwired",
			participants: &stallParticipantStore{participants: decidingSeat()},
			decisions:    &stallDecisionStore{},
			reason:       officeStallSkipRunReaderUnwired,
		},
		{
			name:         "run reader errored",
			participants: &stallParticipantStore{participants: decidingSeat()},
			decisions:    &stallDecisionStore{},
			runs:         &stallRunReader{err: readErr},
			reason:       officeStallSkipRunReaderError,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := setupTestRepo(t)
			seedSession(t, repo, "t1", "s1", "step1")
			seedDecisionWaitingTask(t, repo, "t1", "step1")
			svc := decisionWaitingService(t, repo, tc.participants, tc.decisions, tc.runs)

			beforeSurfaced := officeDecisionWaitingCount()
			beforeSkipped := officeStallSkipCount(tc.reason)
			svc.detectOfficeDecisionWaitingOnce(context.Background())

			if got := officeDecisionWaitingCount() - beforeSurfaced; got != 0 {
				t.Errorf("counter delta = %d, want 0 — an unreadable input must not surface", got)
			}
			if got := officeStallSkipCount(tc.reason) - beforeSkipped; got != 1 {
				t.Errorf("skip counter %q delta = %d, want 1", tc.reason, got)
			}
		})
	}
}

// AC-002.6: a non-Office task is never evaluated. The exclusion here is the
// mirror image of the stranded-signal path's: there, Office must not be
// excluded from detection; here, non-Office must not be included.
func TestDetectOfficeDecisionWaitingOnce_NonOfficeTaskIsNeverEvaluated(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	ctx := context.Background()
	// Everything a candidate needs except Office ownership.
	if _, err := repo.DB().ExecContext(ctx, `
		INSERT INTO workflow_step_participants
			(id, step_id, task_id, role, agent_profile_id, decision_required, position)
		VALUES ('seat-kanban', 'step1', '', 'reviewer', 'agent-1', 1, 0)
	`); err != nil {
		t.Fatalf("seed decision seat: %v", err)
	}
	quiet := time.Now().UTC().Add(-2 * officeDecisionWaitingThreshold)
	if _, err := repo.DB().ExecContext(ctx,
		`UPDATE tasks SET updated_at = ? WHERE id = 't1'`, quiet); err != nil {
		t.Fatalf("backdate task: %v", err)
	}
	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.IsFromOffice {
		t.Fatalf("precondition: t1 must read as a kanban task")
	}

	runs := &stallRunReader{}
	svc := decisionWaitingService(t, repo,
		&stallParticipantStore{participants: decidingSeat()}, &stallDecisionStore{}, runs)

	before := officeDecisionWaitingCount()
	svc.detectOfficeDecisionWaitingOnce(ctx)

	if got := officeDecisionWaitingCount() - before; got != 0 {
		t.Fatalf("counter delta = %d, want 0 — a kanban task is out of scope", got)
	}
	if runs.calls != 0 {
		t.Fatalf("run reader called %d times for a kanban task", runs.calls)
	}
}

// A still-waiting task is reported once, not on every 30-second tick.
func TestDetectOfficeDecisionWaitingOnce_ReportsOncePerTaskAndStep(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedDecisionWaitingTask(t, repo, "t1", "step1")
	svc := decisionWaitingService(t, repo,
		&stallParticipantStore{participants: decidingSeat()}, &stallDecisionStore{},
		&stallRunReader{})

	before := officeDecisionWaitingCount()
	for range 3 {
		svc.detectOfficeDecisionWaitingOnce(context.Background())
	}
	if got := officeDecisionWaitingCount() - before; got != 1 {
		t.Fatalf("counter delta across three scans = %d, want 1", got)
	}
}

// A task that stops being a candidate is pruned from the dedupe map, so if it
// stalls again later it is reported again rather than silently swallowed.
func TestDetectOfficeDecisionWaitingOnce_PrunesResolvedTasks(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedDecisionWaitingTask(t, repo, "t1", "step1")
	svc := decisionWaitingService(t, repo,
		&stallParticipantStore{participants: decidingSeat()}, &stallDecisionStore{},
		&stallRunReader{})
	ctx := context.Background()

	before := officeDecisionWaitingCount()
	svc.detectOfficeDecisionWaitingOnce(ctx)

	// The reviewer finally acts: the task row is touched, so it is no longer
	// quiet and drops out of the candidate set.
	if _, err := repo.DB().ExecContext(ctx,
		`UPDATE tasks SET updated_at = ? WHERE id = 't1'`, time.Now().UTC()); err != nil {
		t.Fatalf("touch task: %v", err)
	}
	svc.detectOfficeDecisionWaitingOnce(ctx)
	if _, seen := svc.officeDecisionWaiting.Load(officeDecisionWaitingKey("t1", "step1")); seen {
		t.Fatal("dedupe entry survived a scan in which the task was no longer a candidate")
	}

	// It stalls again: a new report, not a swallowed repeat.
	quiet := time.Now().UTC().Add(-2 * officeDecisionWaitingThreshold)
	if _, err := repo.DB().ExecContext(ctx,
		`UPDATE tasks SET updated_at = ? WHERE id = 't1'`, quiet); err != nil {
		t.Fatalf("re-backdate task: %v", err)
	}
	svc.detectOfficeDecisionWaitingOnce(ctx)
	if got := officeDecisionWaitingCount() - before; got != 2 {
		t.Fatalf("counter delta = %d, want 2 — a fresh stall is a fresh report", got)
	}
}

// A task whose decision is recorded, then superseded by a fresh requirement,
// must be reported again even though tasks.updated_at never moves and the
// task never leaves the cheap SQL candidate window: recording a decision
// must clear the dedupe key, not merely suppress the tick it was recorded on
// (regression for the "live" set tracking the SQL prefilter instead of the
// full predicate).
func TestDetectOfficeDecisionWaitingOnce_RestallAfterDecisionIsReportedAgain(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedDecisionWaitingTask(t, repo, "t1", "step1")
	decisions := &stallDecisionStore{}
	svc := decisionWaitingService(t, repo,
		&stallParticipantStore{participants: decidingSeat()}, decisions,
		&stallRunReader{})
	ctx := context.Background()

	before := officeDecisionWaitingCount()
	svc.detectOfficeDecisionWaitingOnce(ctx)
	if got := officeDecisionWaitingCount() - before; got != 1 {
		t.Fatalf("first scan counter delta = %d, want 1", got)
	}

	// The reviewer decides. tasks.updated_at is untouched, so the task stays
	// in the cheap SQL candidate window even though the full predicate no
	// longer holds.
	decisions.decisions = []engine.DecisionInfo{{
		ID: "d1", TaskID: "t1", StepID: "step1", Decision: "reject",
	}}
	svc.detectOfficeDecisionWaitingOnce(ctx)
	if _, seen := svc.officeDecisionWaiting.Load(officeDecisionWaitingKey("t1", "step1")); seen {
		t.Fatal("dedupe entry survived a scan in which the decision was recorded")
	}

	// Rework: a fresh requirement supersedes the old decision, still at the
	// same step, still with the same stale tasks.updated_at.
	supersededAt := time.Now().UTC()
	decisions.decisions = []engine.DecisionInfo{{
		ID: "d1", TaskID: "t1", StepID: "step1", Decision: "reject",
		SupersededAt: &supersededAt,
	}}
	svc.detectOfficeDecisionWaitingOnce(ctx)
	if got := officeDecisionWaitingCount() - before; got != 2 {
		t.Fatalf("counter delta after rework = %d, want 2 — the rework stall must be reported again", got)
	}
}

// AC-003.1 through AC-003.3: surfacing is the entire action. No decision row,
// no queued run, no step transition — forging any of those would manufacture
// the quorum the Office gates exist to require.
func assertOfficeDecisionNotActedOn(
	t *testing.T, repo *sqliterepo.Repository, taskID, stepID string,
	decisions *stallDecisionStore, queue *stallRunQueue,
) {
	t.Helper()

	task, err := repo.GetTask(context.Background(), taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.WorkflowStepID != stepID {
		t.Fatalf("workflow step moved to %q; the detector applied a transition", task.WorkflowStepID)
	}
	if decisions.recorded != 0 {
		t.Fatalf("detector recorded %d decisions; it must never manufacture a quorum", decisions.recorded)
	}
	if decisions.cleared != 0 {
		t.Fatalf("detector cleared decisions %d times; it must never discard a real one", decisions.cleared)
	}
	if queue.calls != 0 {
		t.Fatalf("detector queued %d runs; surfacing is the whole action", queue.calls)
	}
}
