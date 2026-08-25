package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// TestProcessOnTurnComplete_ExplicitSignalGating verifies the ADR 0015
// gating: when AutoAdvanceRequiresSignal=true, turn-end without a matching
// pending signal must NOT transition. With the signal present, the
// transition fires as normal.
func TestProcessOnTurnComplete_ExplicitSignalGating(t *testing.T) {
	ctx := context.Background()

	build := func(t *testing.T, withSignal bool, stepRequires bool) (svc *Service, taskID, sessionID string) {
		t.Helper()
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")

		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			AutoAdvanceRequiresSignal: stepRequires,
			Events: wfmodels.StepEvents{
				OnTurnComplete: []wfmodels.OnTurnCompleteAction{
					{Type: wfmodels.OnTurnCompleteMoveToNext},
				},
			},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		}

		svc = createTestService(repo, stepGetter, newMockTaskRepo())

		if withSignal {
			signal := models.PendingStepCompletionSignal{
				StepID:     "step1",
				Source:     models.StepCompletionSourceAgent,
				Summary:    "all done",
				SignaledAt: time.Now().UTC(),
			}
			if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
				t.Fatalf("seed pending signal: %v", err)
			}
		}
		return svc, "t1", "s1"
	}

	t.Run("step requires, no signal → no transition", func(t *testing.T) {
		svc, taskID, sessionID := build(t, false, true)
		task, _ := svc.repo.GetTask(ctx, taskID)
		session, _ := svc.repo.GetTaskSession(ctx, sessionID)
		if got := svc.processOnTurnComplete(ctx, task, session); got {
			t.Errorf("expected gating to BLOCK transition, got transition=true")
		}
		updated, _ := svc.repo.GetTask(ctx, taskID)
		if updated.WorkflowStepID != "step1" {
			t.Errorf("expected to stay on step1, got %q", updated.WorkflowStepID)
		}
	})

	t.Run("step requires, signal present → transition fires", func(t *testing.T) {
		svc, taskID, sessionID := build(t, true, true)
		task, _ := svc.repo.GetTask(ctx, taskID)
		session, _ := svc.repo.GetTaskSession(ctx, sessionID)
		if got := svc.processOnTurnComplete(ctx, task, session); !got {
			t.Errorf("expected transition with pending signal, got transition=false")
		}
		updated, _ := svc.repo.GetTask(ctx, taskID)
		if updated.WorkflowStepID != "step2" {
			t.Errorf("expected to move to step2, got %q", updated.WorkflowStepID)
		}
	})

	t.Run("step does not require → legacy behaviour", func(t *testing.T) {
		svc, taskID, sessionID := build(t, false, false)
		task, _ := svc.repo.GetTask(ctx, taskID)
		session, _ := svc.repo.GetTaskSession(ctx, sessionID)
		if got := svc.processOnTurnComplete(ctx, task, session); !got {
			t.Errorf("expected transition (step does not require signal), got transition=false")
		}
	})

	t.Run("step requires, signal for DIFFERENT step → still blocked", func(t *testing.T) {
		svc, taskID, sessionID := build(t, false, true)
		stale := models.PendingStepCompletionSignal{
			StepID:     "step_old", // stale entry — doesn't match current step
			Source:     models.StepCompletionSourceAgent,
			Summary:    "stale",
			SignaledAt: time.Now().UTC(),
		}
		if err := svc.repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyPendingStepCompletion, stale); err != nil {
			t.Fatalf("seed stale signal: %v", err)
		}
		task, _ := svc.repo.GetTask(ctx, taskID)
		session, _ := svc.repo.GetTaskSession(ctx, sessionID)
		if got := svc.processOnTurnComplete(ctx, task, session); got {
			t.Errorf("expected stale signal to be treated as absent, but got transition=true")
		}
	})
}

// TestProcessOnTurnComplete_OfficeExplicitSignalGating proves the ADR 0015
// signal gate (WO-32) blocks an Office task's turn-end transition exactly
// like it does for kanban: a gated step with no pending signal must not
// transition, and the session must be parked WAITING_FOR_INPUT rather than
// silently advancing — the failure mode WO-38 measured for 6.75 hours.
func TestProcessOnTurnComplete_OfficeExplicitSignalGating(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedOfficeSession(t, repo, "t-office-gate", "s-office-gate", "")

	stepID := "wfs-t-office-gate" // matches seedOfficeSession's stepID convention
	stepGetter := newMockStepGetter()
	stepGetter.steps[stepID] = &wfmodels.WorkflowStep{
		ID: stepID, WorkflowID: "wf-office", Name: "work", Position: 1,
		AutoAdvanceRequiresSignal: true,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToNext},
			},
		},
	}
	svc := createTestServiceWithAgent(repo, stepGetter, newMockTaskRepo(), &mockAgentManager{})

	task, err := repo.GetTask(ctx, "t-office-gate")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "s-office-gate")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}

	if got := svc.processOnTurnComplete(ctx, task, session); got {
		t.Fatalf("expected Office gate to BLOCK transition, got transition=true")
	}

	updatedTask, err := repo.GetTask(ctx, "t-office-gate")
	if err != nil {
		t.Fatalf("re-read task: %v", err)
	}
	if updatedTask.WorkflowStepID != stepID {
		t.Errorf("expected task to stay on %q, got %q", stepID, updatedTask.WorkflowStepID)
	}
	updatedSession, err := repo.GetTaskSession(ctx, "s-office-gate")
	if err != nil {
		t.Fatalf("re-read session: %v", err)
	}
	if updatedSession.State != models.TaskSessionStateWaitingForInput {
		t.Errorf("expected session WAITING_FOR_INPUT, got %q", updatedSession.State)
	}
}

// TestProcessOnTurnComplete_OfficeExplicitSignalGating_AllowsWithSignal proves
// the ADR 0015 gate's ALLOW half for Office: with a pending signal recorded
// for the current step, turn-end transitions as normal. Uses move_to_step
// (not move_to_next) to mirror office-default.yml's shipped Work step action.
func TestProcessOnTurnComplete_OfficeExplicitSignalGating_AllowsWithSignal(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedOfficeSession(t, repo, "t-office-allow", "s-office-allow", "")

	stepID := "wfs-t-office-allow" // matches seedOfficeSession's stepID convention
	reviewStepID := "wfs-office-review"
	stepGetter := newMockStepGetter()
	stepGetter.steps[stepID] = &wfmodels.WorkflowStep{
		ID: stepID, WorkflowID: "wf-office", Name: "work", Position: 1,
		AutoAdvanceRequiresSignal: true,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": reviewStepID}},
			},
		},
	}
	stepGetter.steps[reviewStepID] = &wfmodels.WorkflowStep{
		ID: reviewStepID, WorkflowID: "wf-office", Name: "review", Position: 2,
	}
	svc := createTestServiceWithAgent(repo, stepGetter, newMockTaskRepo(), &mockAgentManager{})

	signal := models.PendingStepCompletionSignal{
		StepID:     stepID,
		Source:     models.StepCompletionSourceAgent,
		Summary:    "work complete",
		SignaledAt: time.Now().UTC(),
	}
	if err := repo.SetSessionMetadataKey(ctx, "s-office-allow", models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
		t.Fatalf("seed pending signal: %v", err)
	}

	task, err := repo.GetTask(ctx, "t-office-allow")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	session, err := repo.GetTaskSession(ctx, "s-office-allow")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}

	if got := svc.processOnTurnComplete(ctx, task, session); !got {
		t.Fatalf("expected Office gate to ALLOW transition with pending signal, got transition=false")
	}

	updatedTask, err := repo.GetTask(ctx, "t-office-allow")
	if err != nil {
		t.Fatalf("re-read task: %v", err)
	}
	if updatedTask.WorkflowStepID != reviewStepID {
		t.Errorf("expected task to move to %q, got %q", reviewStepID, updatedTask.WorkflowStepID)
	}
}

func TestProcessOnTurnComplete_BlocksWhileClarificationPending(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedPendingClarificationMessage(t, repo, "t1", "s1")

	stepGetter := newMockStepGetter()
	stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
		ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{
				{Type: wfmodels.OnTurnCompleteMoveToNext},
			},
		},
	}
	stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
		ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
	}
	svc := createTestService(repo, stepGetter, newMockTaskRepo())

	task, _ := repo.GetTask(ctx, "t1")
	session, _ := repo.GetTaskSession(ctx, "s1")
	if got := svc.processOnTurnComplete(ctx, task, session); got {
		t.Fatal("pending clarification must block legacy on_turn_complete transition")
	}
	updated, _ := repo.GetTask(ctx, "t1")
	if updated.WorkflowStepID != "step1" {
		t.Fatalf("expected workflow step to remain step1, got %q", updated.WorkflowStepID)
	}
}

func TestProcessOnTurnCompleteViaEngine_BlocksWhileClarificationPendingEvenWithSignal(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1")
	seedPendingClarificationMessage(t, repo, "t1", "s1")
	if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, models.PendingStepCompletionSignal{
		StepID:     "step1",
		Source:     models.StepCompletionSourceAgent,
		Summary:    "done without answer",
		SignaledAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed signal: %v", err)
	}

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
	svc := createEngineService(t, repo, stepGetter, &mockAgentManager{})

	session, _ := repo.GetTaskSession(ctx, "s1")
	if got := svc.processOnTurnCompleteViaEngine(ctx, "t1", session); got {
		t.Fatal("pending clarification must block engine on_turn_complete transition even with completion signal")
	}
	session, _ = repo.GetTaskSession(ctx, "s1")
	if _, has := models.LoadPendingStepSignal(session.Metadata); has {
		t.Fatal("pending clarification must clear stale completion signal")
	}
	updated, _ := repo.GetTask(ctx, "t1")
	if updated.WorkflowStepID != "step1" {
		t.Fatalf("expected workflow step to remain step1, got %q", updated.WorkflowStepID)
	}
}

// TestProcessOnTurnCompleteViaEngine_OfficeExplicitSignalGating covers the
// production engine path for Office sessions. The legacy path has separate
// coverage, but production uses the workflow engine when it is configured.
func TestProcessOnTurnCompleteViaEngine_OfficeExplicitSignalGating(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedOfficeSession(t, repo, "t-office-engine", "s-office-engine", "")

	stepID := "wfs-t-office-engine"
	stepGetter := newMockStepGetter()
	stepGetter.steps[stepID] = &wfmodels.WorkflowStep{
		ID: stepID, WorkflowID: "wf-office", Name: "Work", Position: 1,
		AutoAdvanceRequiresSignal: true,
		Events: wfmodels.StepEvents{
			OnTurnComplete: []wfmodels.OnTurnCompleteAction{{Type: wfmodels.OnTurnCompleteMoveToNext}},
		},
	}
	stepGetter.steps["wfs-office-engine-review"] = &wfmodels.WorkflowStep{
		ID: "wfs-office-engine-review", WorkflowID: "wf-office", Name: "Review", Position: 2,
	}

	svc := createEngineService(t, repo, stepGetter, &mockAgentManager{})
	session, err := repo.GetTaskSession(ctx, "s-office-engine")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}

	if transitioned := svc.processOnTurnCompleteViaEngine(ctx, "t-office-engine", session); transitioned {
		t.Fatal("expected the engine path to block an Office turn without a signal")
	}

	task, err := repo.GetTask(ctx, "t-office-engine")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.WorkflowStepID != stepID {
		t.Errorf("workflow step = %q, want %q", task.WorkflowStepID, stepID)
	}
}

// TestLoadPendingStepSignal_RoundTrip verifies the bag survives JSON
// rehydration — important for the backend-restart path where the bag is
// read from the DB as map[string]interface{} rather than the typed struct.
func TestLoadPendingStepSignal_RoundTrip(t *testing.T) {
	t.Run("typed struct", func(t *testing.T) {
		now := time.Now().UTC().Truncate(time.Nanosecond)
		want := models.PendingStepCompletionSignal{
			StepID: "step-1", Source: "agent", Summary: "ok", SignaledAt: now,
		}
		meta := map[string]interface{}{
			models.SessionMetaKeyPendingStepCompletion: want,
		}
		got, ok := models.LoadPendingStepSignal(meta)
		if !ok || got.StepID != "step-1" || got.Source != "agent" {
			t.Errorf("typed struct round-trip failed: ok=%v got=%+v", ok, got)
		}
	})

	t.Run("json-rehydrated map", func(t *testing.T) {
		meta := map[string]interface{}{
			models.SessionMetaKeyPendingStepCompletion: map[string]interface{}{
				"step_id":     "step-2",
				"source":      "manual_fallback",
				"summary":     "user marked complete",
				"signaled_at": "2026-06-04T12:00:00Z",
			},
		}
		got, ok := models.LoadPendingStepSignal(meta)
		if !ok {
			t.Fatal("expected models.LoadPendingStepSignal to recognise map shape")
		}
		if got.StepID != "step-2" || got.Source != "manual_fallback" || got.Summary != "user marked complete" {
			t.Errorf("map round-trip mismatch: %+v", got)
		}
	})

	t.Run("absent key returns false", func(t *testing.T) {
		_, ok := models.LoadPendingStepSignal(map[string]interface{}{})
		if ok {
			t.Error("expected ok=false on empty metadata")
		}
	})

	t.Run("nil metadata returns false", func(t *testing.T) {
		_, ok := models.LoadPendingStepSignal(nil)
		if ok {
			t.Error("expected ok=false on nil metadata")
		}
	})
}

// TestOnStepCompletionSignaled covers the out-of-band subscriber that
// drives a step transition when a `step_complete_kandev` signal arrives
// AFTER the turn has already ended. The three branches:
//
//   - session still RUNNING (turn in flight): no-op, inline path will handle it.
//   - session WAITING + step matches + step gated: re-runs transition pipeline.
//   - signal stale (step has changed under us): clear the bag, no transition.
//   - step not signal-gated: do not advance (signal is not a manual-advance trigger).
func TestOnStepCompletionSignaled(t *testing.T) {
	ctx := context.Background()

	buildEvent := func(taskID, sessionID, stepID string) *bus.Event {
		return bus.NewEvent("workflow.step_completion_signaled", "test", map[string]interface{}{
			"task_id":    taskID,
			"session_id": sessionID,
			"step_id":    stepID,
		})
	}

	t.Run("session still RUNNING — subscriber is a no-op", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		// seedSession leaves the session in RUNNING; that's what we want.

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
		svc := createTestService(repo, stepGetter, newMockTaskRepo())

		svc.onStepCompletionSignaled(ctx, buildEvent("t1", "s1", "step1"))

		updated, _ := repo.GetTask(ctx, "t1")
		if updated.WorkflowStepID != "step1" {
			t.Errorf("expected to stay on step1 (turn in flight), got %q", updated.WorkflowStepID)
		}
	})

	t.Run("WAITING + matching step + gated → transition fires", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		// Flip session to WAITING_FOR_INPUT (the only state the subscriber acts on).
		if err := repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateWaitingForInput, ""); err != nil {
			t.Fatalf("flip session waiting: %v", err)
		}
		// Pre-write the signal in the bag — the subscriber re-runs the
		// inline turn-end path, which reads the bag for gating.
		signal := models.PendingStepCompletionSignal{
			StepID:     "step1",
			Source:     models.StepCompletionSourceAgent,
			Summary:    "ok",
			SignaledAt: time.Now().UTC(),
		}
		if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
			t.Fatalf("seed bag: %v", err)
		}

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
		svc := createTestService(repo, stepGetter, newMockTaskRepo())

		svc.onStepCompletionSignaled(ctx, buildEvent("t1", "s1", "step1"))

		updated, _ := repo.GetTask(ctx, "t1")
		if updated.WorkflowStepID != "step2" {
			t.Errorf("expected transition to step2, got %q", updated.WorkflowStepID)
		}
	})

	t.Run("WAITING + pending clarification → no transition", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		seedPendingClarificationMessage(t, repo, "t1", "s1")
		if err := repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateWaitingForInput, ""); err != nil {
			t.Fatalf("flip session waiting: %v", err)
		}
		signal := models.PendingStepCompletionSignal{
			StepID:     "step1",
			Source:     models.StepCompletionSourceAgent,
			Summary:    "ok",
			SignaledAt: time.Now().UTC(),
		}
		if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
			t.Fatalf("seed bag: %v", err)
		}

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
		svc := createTestService(repo, stepGetter, newMockTaskRepo())

		svc.onStepCompletionSignaled(ctx, buildEvent("t1", "s1", "step1"))

		updated, _ := repo.GetTask(ctx, "t1")
		if updated.WorkflowStepID != "step1" {
			t.Errorf("expected pending clarification to keep task on step1, got %q", updated.WorkflowStepID)
		}
	})

	t.Run("stale step → bag cleared, no transition", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step_current")
		if err := repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateWaitingForInput, ""); err != nil {
			t.Fatalf("flip session waiting: %v", err)
		}
		// Stale signal: written when step was "step_old", but the task has
		// already moved on to "step_current" via some other path.
		stale := models.PendingStepCompletionSignal{
			StepID:     "step_old",
			Source:     models.StepCompletionSourceAgent,
			Summary:    "stale",
			SignaledAt: time.Now().UTC(),
		}
		if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, stale); err != nil {
			t.Fatalf("seed stale signal: %v", err)
		}

		stepGetter := newMockStepGetter()
		stepGetter.steps["step_current"] = &wfmodels.WorkflowStep{
			ID: "step_current", WorkflowID: "wf1", Name: "Current", Position: 5,
		}
		svc := createTestService(repo, stepGetter, newMockTaskRepo())

		svc.onStepCompletionSignaled(ctx, buildEvent("t1", "s1", "step_old"))

		updatedSession, _ := repo.GetTaskSession(ctx, "s1")
		if _, hasBag := models.LoadPendingStepSignal(updatedSession.Metadata); hasBag {
			t.Error("expected stale bag entry to be cleared")
		}
		updatedTask, _ := repo.GetTask(ctx, "t1")
		if updatedTask.WorkflowStepID != "step_current" {
			t.Errorf("expected no transition (stale signal), got %q", updatedTask.WorkflowStepID)
		}
	})

	t.Run("stale event → valid bag for CURRENT step is preserved", func(t *testing.T) {
		// Pins the negative side of the StepID guard in the subscriber's
		// stale-step branch: a late step-A event must not erase a
		// freshly-written step-B bag (which can happen when the session
		// is reused across steps without auto_start_agent). A regression
		// here would silently leave signal-gated steps stuck waiting.
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step_current")
		if err := repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateWaitingForInput, ""); err != nil {
			t.Fatalf("flip session waiting: %v", err)
		}
		// Bag holds a VALID signal for the current step (step_current).
		valid := models.PendingStepCompletionSignal{
			StepID:     "step_current",
			Source:     models.StepCompletionSourceAgent,
			Summary:    "valid current-step signal",
			SignaledAt: time.Now().UTC(),
		}
		if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, valid); err != nil {
			t.Fatalf("seed valid signal: %v", err)
		}

		stepGetter := newMockStepGetter()
		stepGetter.steps["step_current"] = &wfmodels.WorkflowStep{
			ID: "step_current", WorkflowID: "wf1", Name: "Current", Position: 5,
		}
		svc := createTestService(repo, stepGetter, newMockTaskRepo())

		// Fire a STALE event (step_old != current step_current). The
		// guard must see that the bag's StepID is "step_current" (not
		// "step_old") and leave it alone.
		svc.onStepCompletionSignaled(ctx, buildEvent("t1", "s1", "step_old"))

		updatedSession, _ := repo.GetTaskSession(ctx, "s1")
		bag, hasBag := models.LoadPendingStepSignal(updatedSession.Metadata)
		if !hasBag {
			t.Fatal("expected valid bag to survive stale event")
		}
		if bag.StepID != "step_current" {
			t.Errorf("expected bag StepID=step_current, got %q", bag.StepID)
		}
	})

	t.Run("step not signal-gated → subscriber ignores", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		if err := repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateWaitingForInput, ""); err != nil {
			t.Fatalf("flip session waiting: %v", err)
		}

		// Step explicitly NOT gated on the signal — even though one was
		// written and matches, the subscriber must not advance.
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			AutoAdvanceRequiresSignal: false,
			Events: wfmodels.StepEvents{
				OnTurnComplete: []wfmodels.OnTurnCompleteAction{
					{Type: wfmodels.OnTurnCompleteMoveToNext},
				},
			},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		}
		svc := createTestService(repo, stepGetter, newMockTaskRepo())

		svc.onStepCompletionSignaled(ctx, buildEvent("t1", "s1", "step1"))

		updated, _ := repo.GetTask(ctx, "t1")
		if updated.WorkflowStepID != "step1" {
			t.Errorf("expected no transition for un-gated step, got %q", updated.WorkflowStepID)
		}
	})

	t.Run("coordinator cancellation wins while signal subscriber waits", func(t *testing.T) {
		repo := setupTestRepo(t)
		seedSession(t, repo, "t1", "s1", "step1")
		if err := repo.UpdateTaskSessionState(ctx, "s1", models.TaskSessionStateWaitingForInput, ""); err != nil {
			t.Fatalf("flip session waiting: %v", err)
		}
		signal := models.PendingStepCompletionSignal{
			StepID: "step1", Source: models.StepCompletionSourceAgent,
			Summary: "done", SignaledAt: time.Now().UTC(),
		}
		if err := repo.SetSessionMetadataKey(ctx, "s1", models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
			t.Fatalf("seed signal: %v", err)
		}
		stepGetter := newMockStepGetter()
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID: "step1", WorkflowID: "wf1", Name: "Step 1", Position: 0,
			AutoAdvanceRequiresSignal: true,
			Events: wfmodels.StepEvents{OnTurnComplete: []wfmodels.OnTurnCompleteAction{{
				Type: wfmodels.OnTurnCompleteMoveToNext,
			}}},
		}
		stepGetter.steps["step2"] = &wfmodels.WorkflowStep{
			ID: "step2", WorkflowID: "wf1", Name: "Step 2", Position: 1,
		}
		svc := createTestService(repo, stepGetter, newMockTaskRepo())
		guard, release := svc.acquireCancelInFlightGuard("s1")
		guard.Lock()
		done := make(chan struct{})
		go func() {
			svc.onStepCompletionSignaled(ctx, buildEvent("t1", "s1", "step1"))
			close(done)
		}()
		coordinatorStopWaitForGuardRefs(t, svc, "s1", 2)
		changed, _, err := repo.CancelActiveTaskSession(ctx, "s1", coordinatorMCPStopReason)
		if err != nil || !changed {
			t.Fatalf("cancel waiting session: changed=%v err=%v", changed, err)
		}
		guard.Unlock()
		release()
		coordinatorStopAwaitSignal(t, done, "guarded step-completion subscriber")

		updated, err := repo.GetTask(ctx, "t1")
		if err != nil {
			t.Fatalf("get task: %v", err)
		}
		if updated.WorkflowStepID != "step1" {
			t.Fatalf("stale signal advanced workflow after cancellation: %q", updated.WorkflowStepID)
		}
		updatedSession, err := repo.GetTaskSession(ctx, "s1")
		if err != nil {
			t.Fatalf("get session: %v", err)
		}
		if updatedSession.State != models.TaskSessionStateCancelled {
			t.Fatalf("expected cancelled session, got %q", updatedSession.State)
		}
		if _, hasSignal := models.LoadPendingStepSignal(updatedSession.Metadata); !hasSignal {
			t.Fatal("stop-winning subscriber consumed the queued completion signal")
		}
	})
}

// newGatedStepFailureService creates the workflow used by the turn-failure
// signal tests below. The first step advances only when a matching signal is
// present.
func newGatedStepFailureService(t *testing.T) (*Service, *sqliterepo.Repository) {
	t.Helper()
	repo := setupTestRepo(t)
	seedSession(t, repo, "t1", "s1", "step1") // seedSession leaves the session RUNNING.

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

	taskRepo := newMockTaskRepo()
	agentMgr := &mockAgentManager{repoForExecutionLookup: repo}
	return createTestServiceWithScheduler(repo, stepGetter, taskRepo, agentMgr), repo
}

func seedPendingStepCompletionSignal(t *testing.T, repo *sqliterepo.Repository, stepID, summary string) {
	t.Helper()
	signal := models.PendingStepCompletionSignal{
		StepID:     stepID,
		Source:     models.StepCompletionSourceAgent,
		Summary:    summary,
		SignaledAt: time.Now().UTC(),
	}
	if err := repo.SetSessionMetadataKey(context.Background(), "s1", models.SessionMetaKeyPendingStepCompletion, signal); err != nil {
		t.Fatalf("seed pending signal: %v", err)
	}
}

func stepCompletionSignalEvent(taskID, sessionID, stepID string) *bus.Event {
	return bus.NewEvent("workflow.step_completion_signaled", "test", map[string]interface{}{
		"task_id":    taskID,
		"session_id": sessionID,
		"step_id":    stepID,
	})
}

// TestStepCompletionSignalSurvivesTurnFailure is the regression test for the
// dropped-signal bug. A signal written during a running turn must be applied
// when that turn fails and settles the session into WAITING_FOR_INPUT.
func TestStepCompletionSignalSurvivesTurnFailure(t *testing.T) {
	ctx := context.Background()
	svc, repo := newGatedStepFailureService(t)
	seedPendingStepCompletionSignal(t, repo, "step1", "all done")

	// The subscriber sees a running session and correctly defers to the
	// turn-end path.
	svc.onStepCompletionSignaled(ctx, stepCompletionSignalEvent("t1", "s1", "step1"))
	if session, err := repo.GetTaskSession(ctx, "s1"); err != nil || session.State != models.TaskSessionStateRunning {
		t.Fatalf("expected session to remain RUNNING after subscriber no-op, got %+v (err=%v)", session, err)
	}
	if task, err := repo.GetTask(ctx, "t1"); err != nil || task.WorkflowStepID != "step1" {
		t.Fatalf("expected no transition yet, got %+v (err=%v)", task, err)
	}

	svc.handleAgentFailed(ctx, watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s1",
		AgentExecutionID: "exec-1",
		ErrorMessage:     "agent crashed",
	})

	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("expected session WAITING_FOR_INPUT after failure, got %q", session.State)
	}
	if _, hasSignal := models.LoadPendingStepSignal(session.Metadata); hasSignal {
		t.Error("expected the pending signal to be consumed by the transition")
	}

	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.WorkflowStepID != "step2" {
		t.Fatalf("expected task to advance to step2 after the failed turn, got %q", task.WorkflowStepID)
	}
}

func TestStaleStepCompletionSignalDoesNotTransitionAfterTurnFailure(t *testing.T) {
	ctx := context.Background()
	svc, repo := newGatedStepFailureService(t)
	seedPendingStepCompletionSignal(t, repo, "step_old", "stale")

	svc.handleAgentFailed(ctx, watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s1",
		AgentExecutionID: "exec-1",
		ErrorMessage:     "agent crashed",
	})

	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if session.State != models.TaskSessionStateWaitingForInput {
		t.Fatalf("expected session WAITING_FOR_INPUT after failure, got %q", session.State)
	}
	if _, hasSignal := models.LoadPendingStepSignal(session.Metadata); hasSignal {
		t.Error("expected the stale signal to be cleared by the reconciler")
	}

	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.WorkflowStepID != "step1" {
		t.Fatalf("expected task to stay on step1 (stale signal must not transition), got %q", task.WorkflowStepID)
	}
}

// TestOfficeStepCompletionSignalDoesNotAdvanceAfterTurnFailure covers the
// Office-session exclusion. Office failures are terminal for the session and
// must not advance the workflow from a matching pending signal.
func TestOfficeStepCompletionSignalDoesNotAdvanceAfterTurnFailure(t *testing.T) {
	ctx := context.Background()
	svc, repo := newGatedStepFailureService(t)
	task, err := repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	task.ProjectID = "office-project"
	if err := repo.UpdateTask(ctx, task); err != nil {
		t.Fatalf("mark task as Office-owned: %v", err)
	}
	seedPendingStepCompletionSignal(t, repo, "step1", "all done")

	svc.handleAgentFailed(ctx, watcher.AgentEventData{
		TaskID:           "t1",
		SessionID:        "s1",
		AgentExecutionID: "exec-1",
		ErrorMessage:     "agent crashed",
	})

	session, err := repo.GetTaskSession(ctx, "s1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if session.State != models.TaskSessionStateFailed {
		t.Fatalf("expected Office session FAILED after failure, got %q", session.State)
	}

	task, err = repo.GetTask(ctx, "t1")
	if err != nil {
		t.Fatalf("get task: %v", err)
	}
	if task.WorkflowStepID != "step1" {
		t.Fatalf("expected Office task to stay on step1, got %q", task.WorkflowStepID)
	}
}
