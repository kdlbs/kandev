package service

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func TestPullNextTaskOnVacatePreservesSameStepPromotionSource(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-limited": {ID: "step-limited", WorkflowID: "wf-source", Position: 0, WIPLimit: 1},
		"step-target":  {ID: "step-target", WorkflowID: "wf-target", Position: 0},
	}})
	createMoveTask(t, ctx, repo, "task-vacating", "wf-source", "step-limited", nil)
	createMoveTask(t, ctx, repo, "task-queued", "wf-source", "step-limited", nil)

	queued, err := repo.GetTask(ctx, "task-queued")
	if err != nil {
		t.Fatalf("GetTask(task-queued): %v", err)
	}
	now := time.Now().UTC()
	queued.WIPAdmitted = false
	queued.QueuedForStepID = "step-limited"
	queued.QueuedAt = &now
	if err := repo.UpdateTask(ctx, queued); err != nil {
		t.Fatalf("queue task: %v", err)
	}

	if _, err := svc.MoveTask(ctx, "task-vacating", "wf-target", "step-target", 0); err != nil {
		t.Fatalf("MoveTask: %v", err)
	}

	promoted, err := repo.GetTask(ctx, "task-queued")
	if err != nil {
		t.Fatalf("reload promoted task: %v", err)
	}
	assertQueuePromotionSource(t, promoted, "step-limited")
}

func TestPullNextTaskOnVacatePreservesFeederPromotionSource(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	seedMoveWorkflows(t, ctx, repo)
	svc.SetWorkflowStepGetter(&fakeWorkflowStepGetter{steps: map[string]*wfmodels.WorkflowStep{
		"step-limited": {
			ID: "step-limited", WorkflowID: "wf-source", Position: 0,
			WIPLimit: 1, PullFromStepID: "step-feeder",
		},
		"step-feeder": {ID: "step-feeder", WorkflowID: "wf-source", Position: 1},
		"step-target": {ID: "step-target", WorkflowID: "wf-target", Position: 0},
	}})
	createMoveTask(t, ctx, repo, "task-vacating", "wf-source", "step-limited", nil)
	createMoveTask(t, ctx, repo, "task-feeder", "wf-source", "step-feeder", nil)

	if _, err := svc.MoveTask(ctx, "task-vacating", "wf-target", "step-target", 0); err != nil {
		t.Fatalf("MoveTask: %v", err)
	}

	promoted, err := repo.GetTask(ctx, "task-feeder")
	if err != nil {
		t.Fatalf("reload promoted task: %v", err)
	}
	assertQueuePromotionSource(t, promoted, "step-feeder")
}

func assertQueuePromotionSource(t *testing.T, task *models.Task, want string) {
	t.Helper()
	raw, ok := task.Metadata[models.MetaKeyQueuePromotionPending]
	if !ok {
		t.Fatal("promoted task has no queue-promotion token")
	}
	descriptor, ok := raw.(map[string]interface{})
	if !ok {
		t.Fatalf("queue-promotion token type = %T, want descriptor", raw)
	}
	if got, _ := descriptor["from_step_id"].(string); got != want {
		t.Fatalf("queue-promotion source = %q, want %q", got, want)
	}
}
