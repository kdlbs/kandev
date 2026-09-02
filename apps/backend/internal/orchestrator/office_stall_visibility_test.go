package orchestrator

import (
	"context"
	"expvar"
	"testing"
	"time"

	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// officeStallStepGetter builds a two-step workflow whose first step would
// auto-advance on turn completion. If the Office exclusion ever regressed, the
// task would move to step2 and the not-acted-on assertions would fail.
func officeStallStepGetter() *mockStepGetter {
	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		AutoAdvanceRequiresSignal: true,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToNext},
			},
		},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
	}
	return stepGetter
}

// The Office exclusion at both watchdog gates is load-bearing: passing the
// reconcileWaitingStuckSignalSessionIfDue gate falls through to
// reconcileStepCompletionSignalLocked, and passing the stuckSignalCandidate
// gate leads to reclaimStuckSignalSessionOwned. Either one applies the step
// transition the agent asked for, which for an Office task would record it
// without the decisions its quorum gate requires. These tests pin the
// disposition that keeps that from happening.
func TestStuckSignalDispositionFor(t *testing.T) {
	const stepID = "step1"
	signal := models.PendingStepCompletionSignal{StepID: stepID}

	tests := []struct {
		name    string
		task    *models.Task
		session *models.TaskSession
		want    stuckSignalDisposition
	}{
		{
			name:    "ordinary task at the signalled step is reclaimable",
			task:    &models.Task{ID: "t1", WorkflowStepID: stepID},
			session: &models.TaskSession{ID: "s1"},
			want:    stuckSignalReclaimable,
		},
		{
			name:    "office task is surface-only, never reclaimable",
			task:    &models.Task{ID: "t1", WorkflowStepID: stepID, IsFromOffice: true},
			session: &models.TaskSession{ID: "s1"},
			want:    stuckSignalSurfaceOnly,
		},
		{
			name:    "passthrough session is excluded even when the task is office",
			task:    &models.Task{ID: "t1", WorkflowStepID: stepID, IsFromOffice: true},
			session: &models.TaskSession{ID: "s1", IsPassthrough: true},
			want:    stuckSignalNotCandidate,
		},
		{
			name:    "passthrough session is excluded on an ordinary task",
			task:    &models.Task{ID: "t1", WorkflowStepID: stepID},
			session: &models.TaskSession{ID: "s1", IsPassthrough: true},
			want:    stuckSignalNotCandidate,
		},
		{
			name:    "stale signal on an office task is not reported as a stall",
			task:    &models.Task{ID: "t1", WorkflowStepID: "step2", IsFromOffice: true},
			session: &models.TaskSession{ID: "s1"},
			want:    stuckSignalNotCandidate,
		},
		{
			name:    "stale signal on an ordinary task is left to the signal-consuming paths",
			task:    &models.Task{ID: "t1", WorkflowStepID: "step2"},
			session: &models.TaskSession{ID: "s1"},
			want:    stuckSignalNotCandidate,
		},
		{
			name:    "nil task",
			task:    nil,
			session: &models.TaskSession{ID: "s1"},
			want:    stuckSignalNotCandidate,
		},
		{
			name:    "nil session",
			task:    &models.Task{ID: "t1", WorkflowStepID: stepID},
			session: nil,
			want:    stuckSignalNotCandidate,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := stuckSignalDispositionFor(tc.task, tc.session, signal); got != tc.want {
				t.Fatalf("disposition = %v, want %v", got, tc.want)
			}
		})
	}
}

// An Office task must never be classified reclaimable, whatever else is true
// of it. This is the invariant the whole unit rests on; if it ever fails, the
// watchdog can forge a quorum decision.
func TestStuckSignalDispositionFor_OfficeIsNeverReclaimable(t *testing.T) {
	const stepID = "step1"
	signal := models.PendingStepCompletionSignal{
		StepID:     stepID,
		SignaledAt: time.Now().UTC().Add(-30 * time.Minute),
	}
	for _, passthrough := range []bool{false, true} {
		task := &models.Task{ID: "t1", WorkflowStepID: stepID, IsFromOffice: true}
		session := &models.TaskSession{ID: "s1", IsPassthrough: passthrough}
		if got := stuckSignalDispositionFor(task, session, signal); got == stuckSignalReclaimable {
			t.Fatalf("office task classified reclaimable (passthrough=%v)", passthrough)
		}
	}
}

// seedOfficeStrandedSignal creates an Office task holding a step-completion
// signal older than the watchdog threshold, in the given session state.
func seedOfficeStrandedSignal(
	t *testing.T, svc *Service, repo *sqliterepo.Repository,
	taskID, sessionID, stepID string, state models.TaskSessionState,
) models.PendingStepCompletionSignal {
	t.Helper()
	ctx := context.Background()

	// models.Task.IsFromOffice is a derived projection, not a stored column
	// (isFromOfficeProjection in the task repository): a task is Office when it
	// has a non-empty project_id, or its workflow is the workspace's canonical
	// office workflow. Setting the struct field would be silently discarded, so
	// take the project_id branch.
	if _, err := repo.DB().ExecContext(ctx,
		`UPDATE tasks SET project_id = 'proj1' WHERE id = ?`, taskID); err != nil {
		t.Fatalf("mark task as office: %v", err)
	}
	task, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if !task.IsFromOffice {
		t.Fatalf("precondition: task %s should read as an Office task", taskID)
	}

	signal := models.PendingStepCompletionSignal{
		StepID:     stepID,
		Source:     models.StepCompletionSourceAgent,
		Summary:    "all done",
		SignaledAt: time.Now().UTC().Add(-30 * time.Minute),
	}
	if err := repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
		t.Fatalf("seed pending signal: %v", err)
	}
	if state != models.TaskSessionStateRunning {
		if err := repo.UpdateTaskSessionState(ctx, sessionID, state, ""); err != nil {
			t.Fatalf("set session state %q: %v", state, err)
		}
	}
	return signal
}

func officeStrandedCount(gate stuckSignalGate) int64 {
	v := officeStallStrandedSignalTotal.Get(officeStallLabel("gate", string(gate)))
	iv, ok := v.(*expvar.Int)
	if !ok {
		return 0
	}
	return iv.Value()
}

// AC-001.2: an Office task entering through the stuckSignalCandidate gate is
// surfaced, and is not reclaimed.
func TestReclaimStuckSignalSessionsOnce_SurfacesOfficeTaskAtCandidateGate(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	svc := createTestService(repo, officeStallStepGetter(), newMockTaskRepo())
	seedOfficeStrandedSignal(t, svc, repo, "t1", "s1", "step1", models.TaskSessionStateRunning)

	before := officeStrandedCount(stuckSignalGateCandidate)
	svc.reclaimStuckSignalSessionsOnce(ctx)

	if got := officeStrandedCount(stuckSignalGateCandidate) - before; got != 1 {
		t.Fatalf("stranded-signal counter delta = %d, want 1", got)
	}
	assertOfficeStallNotActedOn(t, repo, "t1", "s1", "step1")
}

// Signal age alone is not inactivity (stuck_signal_watchdog.go's
// stuckSignalWatchdogThreshold comment). An Office agent still producing
// events within the inactivity window — a long tool call, a provider
// retry/backoff — must not be surfaced just because its signal cleared the
// age gate. Only the candidate gate needs this: the waiting gate's session is
// already WAITING_FOR_INPUT, not running.
func TestReclaimStuckSignalSessionsOnce_ActiveOfficeAgentNotSurfacedAtCandidateGate(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	agentMgr := &mockAgentManager{
		currentPromptExecutionID: "exec-1",
	}
	agentMgr.currentPromptLastActivityAt = time.Now().UTC().Add(-time.Minute)
	svc := createTestServiceWithAgent(repo, officeStallStepGetter(), newMockTaskRepo(), agentMgr)
	seedOfficeStrandedSignal(t, svc, repo, "t1", "s1", "step1", models.TaskSessionStateRunning)

	before := officeStrandedCount(stuckSignalGateCandidate)
	svc.reclaimStuckSignalSessionsOnce(ctx)

	if got := officeStrandedCount(stuckSignalGateCandidate) - before; got != 0 {
		t.Fatalf("stranded-signal counter delta = %d, want 0 — the agent is still active", got)
	}
	assertOfficeStallNotActedOn(t, repo, "t1", "s1", "step1")
}

// AC-001.3: an Office task entering through the
// reconcileWaitingStuckSignalSessionIfDue gate is surfaced from that gate.
// The two gates are near-identical predicates, so each needs its own proof:
// covering only one leaves the other as a silent second gate.
func TestReclaimStuckSignalSessionsOnce_SurfacesOfficeTaskAtWaitingGate(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	svc := createTestService(repo, officeStallStepGetter(), newMockTaskRepo())
	seedOfficeStrandedSignal(t, svc, repo, "t1", "s1", "step1", models.TaskSessionStateWaitingForInput)

	before := officeStrandedCount(stuckSignalGateWaiting)
	svc.reclaimStuckSignalSessionsOnce(ctx)

	if got := officeStrandedCount(stuckSignalGateWaiting) - before; got != 1 {
		t.Fatalf("stranded-signal counter delta = %d, want 1", got)
	}
	assertOfficeStallNotActedOn(t, repo, "t1", "s1", "step1")
}

// The waiting gate receives a session snapshot from the active-session scan.
// A replacement signal on the same step must not inherit the old signal's age
// while the detector is between that scan and its report.
func TestReconcileWaitingStuckSignalSessionIfDue_SkipsReplacedSignal(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	svc := createTestService(repo, officeStallStepGetter(), newMockTaskRepo())
	oldSignal := seedOfficeStrandedSignal(t, svc, repo, "t1", "s1", "step1", models.TaskSessionStateWaitingForInput)
	newSignal := models.PendingStepCompletionSignal{
		StepID:     oldSignal.StepID,
		Source:     models.StepCompletionSourceAgent,
		SignaledAt: time.Now().UTC(),
	}
	if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, newSignal); err != nil {
		t.Fatalf("replace pending signal: %v", err)
	}

	snapshot := &models.TaskSession{
		ID:     "s1",
		TaskID: "t1",
		State:  models.TaskSessionStateWaitingForInput,
		Metadata: map[string]interface{}{
			models.SessionMetaKeyPendingStepCompletion: oldSignal,
		},
	}
	before := officeStrandedCount(stuckSignalGateWaiting)
	svc.reconcileWaitingStuckSignalSessionIfDue(ctx, snapshot, time.Now().UTC())
	if got := officeStrandedCount(stuckSignalGateWaiting) - before; got != 0 {
		t.Fatalf("stranded-signal counter delta = %d, want 0", got)
	}
}

// AC-001.5: a still-stranded signal is reported once, not on every tick.
func TestReclaimStuckSignalSessionsOnce_SurfacesOfficeSignalOncePerSignal(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	svc := createTestService(repo, officeStallStepGetter(), newMockTaskRepo())
	seedOfficeStrandedSignal(t, svc, repo, "t1", "s1", "step1", models.TaskSessionStateRunning)

	before := officeStrandedCount(stuckSignalGateCandidate)
	for range 3 {
		svc.reclaimStuckSignalSessionsOnce(ctx)
	}
	if got := officeStrandedCount(stuckSignalGateCandidate) - before; got != 1 {
		t.Fatalf("counter delta across three scans = %d, want 1", got)
	}

	// A genuinely new signal on the same session and step reports again.
	newer := models.PendingStepCompletionSignal{
		StepID:     "step1",
		Source:     models.StepCompletionSourceAgent,
		SignaledAt: time.Now().UTC().Add(-20 * time.Minute),
	}
	if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, newer); err != nil {
		t.Fatalf("reseed signal: %v", err)
	}
	svc.reclaimStuckSignalSessionsOnce(ctx)
	if got := officeStrandedCount(stuckSignalGateCandidate) - before; got != 2 {
		t.Fatalf("counter delta after a new signal = %d, want 2", got)
	}
}

// AC-003.1 through AC-003.4: surfacing is the entire action.
func assertOfficeStallNotActedOn(t *testing.T, repo *sqliterepo.Repository, taskID, sessionID, stepID string) {
	t.Helper()
	ctx := context.Background()

	task, err := repo.GetTask(ctx, taskID)
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.WorkflowStepID != stepID {
		t.Fatalf("workflow step moved to %q; the watchdog applied a transition an Office quorum never approved", task.WorkflowStepID)
	}
	session, err := repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if _, stillPending := models.LoadPendingStepSignal(session.Metadata); !stillPending {
		t.Error("pending signal was consumed; surfacing must leave it for a human")
	}
}
