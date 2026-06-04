package orchestrator

import (
	"context"
	"testing"
	"time"

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
		got, ok := loadPendingStepSignal(meta)
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
		got, ok := loadPendingStepSignal(meta)
		if !ok {
			t.Fatal("expected loadPendingStepSignal to recognise map shape")
		}
		if got.StepID != "step-2" || got.Source != "manual_fallback" || got.Summary != "user marked complete" {
			t.Errorf("map round-trip mismatch: %+v", got)
		}
	})

	t.Run("absent key returns false", func(t *testing.T) {
		_, ok := loadPendingStepSignal(map[string]interface{}{})
		if ok {
			t.Error("expected ok=false on empty metadata")
		}
	})

	t.Run("nil metadata returns false", func(t *testing.T) {
		_, ok := loadPendingStepSignal(nil)
		if ok {
			t.Error("expected ok=false on nil metadata")
		}
	})
}
