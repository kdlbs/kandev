package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events/bus"
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
}
