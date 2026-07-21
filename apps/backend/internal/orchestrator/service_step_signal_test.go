package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// TestStepRequiresCompletionSignal covers every silent-false branch and the
// positive path of Service.StepRequiresCompletionSignal (ADR 0015 gate).
// All four "unknown ⇒ false" cases must hold: a flaky workflow lookup must
// never silently expose the step_complete_kandev tool.
func TestStepRequiresCompletionSignal(t *testing.T) {
	ctx := context.Background()

	seedTaskWithStep := func(t *testing.T) (*Service, *mockStepGetter) {
		t.Helper()
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		ws := &models.Workspace{ID: "ws1", Name: "WS", CreatedAt: now, UpdatedAt: now}
		if err := repo.CreateWorkspace(ctx, ws); err != nil {
			t.Fatalf("seed workspace: %v", err)
		}
		wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
		if err := repo.CreateWorkflow(ctx, wf); err != nil {
			t.Fatalf("seed workflow: %v", err)
		}
		task := &models.Task{ID: "task1", WorkflowID: "wf1", WorkflowStepID: "step1", Title: "T", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now}
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("seed task: %v", err)
		}
		stepGetter := newMockStepGetter()
		svc := createTestService(repo, stepGetter, newMockTaskRepo())
		return svc, stepGetter
	}

	t.Run("nil getter returns false", func(t *testing.T) {
		svc := &Service{} // workflowStepGetter unset
		if got := svc.StepRequiresCompletionSignal(ctx, "any-task"); got {
			t.Fatalf("expected false when workflowStepGetter is nil, got true")
		}
	})

	t.Run("missing task returns false", func(t *testing.T) {
		repo := setupTestRepo(t)
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		if got := svc.StepRequiresCompletionSignal(ctx, "does-not-exist"); got {
			t.Fatalf("expected false for missing task, got true")
		}
	})

	t.Run("task without workflow step returns false", func(t *testing.T) {
		repo := setupTestRepo(t)
		now := time.Now().UTC()
		ws := &models.Workspace{ID: "ws1", Name: "WS", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkspace(ctx, ws)
		wf := &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now}
		_ = repo.CreateWorkflow(ctx, wf)
		task := &models.Task{ID: "task1", WorkflowID: "wf1", Title: "T", State: v1.TaskStateInProgress, CreatedAt: now, UpdatedAt: now}
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("seed task: %v", err)
		}
		svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
		if got := svc.StepRequiresCompletionSignal(ctx, "task1"); got {
			t.Fatalf("expected false when task has empty WorkflowStepID, got true")
		}
	})

	t.Run("step lookup miss returns false", func(t *testing.T) {
		svc, stepGetter := seedTaskWithStep(t)
		// stepGetter has no step registered for "step1" — GetStep returns (nil, nil)
		_ = stepGetter
		if got := svc.StepRequiresCompletionSignal(ctx, "task1"); got {
			t.Fatalf("expected false when step lookup returns nil, got true")
		}
	})

	t.Run("step without signal requirement returns false", func(t *testing.T) {
		svc, stepGetter := seedTaskWithStep(t)
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID:                        "step1",
			WorkflowID:                "wf1",
			AutoAdvanceRequiresSignal: false,
		}
		if got := svc.StepRequiresCompletionSignal(ctx, "task1"); got {
			t.Fatalf("expected false when AutoAdvanceRequiresSignal is false, got true")
		}
	})

	t.Run("step with signal requirement returns true", func(t *testing.T) {
		svc, stepGetter := seedTaskWithStep(t)
		stepGetter.steps["step1"] = &wfmodels.WorkflowStep{
			ID:                        "step1",
			WorkflowID:                "wf1",
			AutoAdvanceRequiresSignal: true,
		}
		if got := svc.StepRequiresCompletionSignal(ctx, "task1"); !got {
			t.Fatalf("expected true when step.AutoAdvanceRequiresSignal is true, got false")
		}
	})
}

// TestWorkflowStepRequiresCompletionSignal covers the step-ID variant used by
// callers that already loaded the task (task_operations.go) — avoids the extra
// GetTask round-trip but must enforce the same "unknown ⇒ false" contract.
func TestWorkflowStepRequiresCompletionSignal(t *testing.T) {
	ctx := context.Background()

	t.Run("nil getter returns false", func(t *testing.T) {
		svc := &Service{}
		if got := svc.WorkflowStepRequiresCompletionSignal(ctx, "step1"); got {
			t.Fatalf("expected false when workflowStepGetter is nil, got true")
		}
	})

	t.Run("empty step ID returns false", func(t *testing.T) {
		svc := &Service{workflowStepGetter: newMockStepGetter()}
		if got := svc.WorkflowStepRequiresCompletionSignal(ctx, ""); got {
			t.Fatalf("expected false for empty step ID, got true")
		}
	})

	t.Run("missing step returns false", func(t *testing.T) {
		svc := &Service{workflowStepGetter: newMockStepGetter()}
		if got := svc.WorkflowStepRequiresCompletionSignal(ctx, "does-not-exist"); got {
			t.Fatalf("expected false when step is not registered, got true")
		}
	})

	t.Run("step flag drives return value", func(t *testing.T) {
		stepGetter := newMockStepGetter()
		stepGetter.steps["off"] = &wfmodels.WorkflowStep{ID: "off", AutoAdvanceRequiresSignal: false}
		stepGetter.steps["on"] = &wfmodels.WorkflowStep{ID: "on", AutoAdvanceRequiresSignal: true}
		svc := &Service{workflowStepGetter: stepGetter}

		if got := svc.WorkflowStepRequiresCompletionSignal(ctx, "off"); got {
			t.Fatalf("expected false for step with AutoAdvanceRequiresSignal=false, got true")
		}
		if got := svc.WorkflowStepRequiresCompletionSignal(ctx, "on"); !got {
			t.Fatalf("expected true for step with AutoAdvanceRequiresSignal=true, got false")
		}
	})
}
