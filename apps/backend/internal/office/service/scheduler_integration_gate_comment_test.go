package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/service"
)

// TestSchedulerIntegration_BuildPromptContext_TaskCommentStageFromWorkflowStep
// pins AC-OFFICE-GATE-COMMENT-003.4: the run's own workflow_step_id (the step
// the fan-out queued the run at), not the task's current step, is
// authoritative when it resolves to a non-empty stage type.
func TestSchedulerIntegration_BuildPromptContext_TaskCommentStageFromWorkflowStep(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	insertTaskForPrompt(t, svc, "task-gate-review", "ws-1", "Ship the fix", "Implement the fix", 3)
	svc.ExecSQL(t, `INSERT INTO workflow_steps (id, stage_type) VALUES (?, ?)`, "step-gate-review", "review")

	payload := `{"task_id":"task-gate-review","workflow_step_id":"step-gate-review","stage_type":"work"}`
	pc := service.BuildPromptContextForTest(svc, ctx, service.RunReasonTaskComment, payload)

	if pc.StageType != "review" {
		t.Fatalf("StageType = %q, want authoritative review stage from workflow_step_id", pc.StageType)
	}
}

// TestSchedulerIntegration_BuildPromptContext_TaskCommentFallsBackOnStepReadError
// pins that a step-read error falls back to the payload's own stage_type
// rather than failing the wakeup pipeline.
func TestSchedulerIntegration_BuildPromptContext_TaskCommentFallsBackOnStepReadError(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	insertTaskForPrompt(t, svc, "task-gate-step-error", "ws-1", "Ship the fix", "Implement the fix", 3)
	svc.ExecSQL(t, `DROP TABLE workflow_steps`)

	payload := `{"task_id":"task-gate-step-error","workflow_step_id":"step-missing","stage_type":"approval"}`
	pc := service.BuildPromptContextForTest(svc, ctx, service.RunReasonTaskComment, payload)

	if pc.StageType != "approval" {
		t.Fatalf("StageType = %q, want payload stage_type fallback after step read error", pc.StageType)
	}
}

// TestSchedulerIntegration_BuildPromptContext_TaskCommentFallsBackOnEmptyStepStage
// pins that a step whose stage_type column is empty falls back to the
// payload's own stage_type.
func TestSchedulerIntegration_BuildPromptContext_TaskCommentFallsBackOnEmptyStepStage(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	insertTaskForPrompt(t, svc, "task-gate-empty-stage", "ws-1", "Ship the fix", "Implement the fix", 3)
	svc.ExecSQL(t, `INSERT INTO workflow_steps (id, stage_type) VALUES (?, ?)`, "step-gate-empty", "")

	payload := `{"task_id":"task-gate-empty-stage","workflow_step_id":"step-gate-empty","stage_type":"approval"}`
	pc := service.BuildPromptContextForTest(svc, ctx, service.RunReasonTaskComment, payload)

	if pc.StageType != "approval" {
		t.Fatalf("StageType = %q, want payload stage_type fallback on empty step stage", pc.StageType)
	}
}

// TestSchedulerIntegration_BuildPromptContext_TaskCommentFallsBackOnMissingWorkflowStepID
// pins that an empty/missing workflow_step_id in the payload falls back to
// the payload's own stage_type without attempting a step read.
func TestSchedulerIntegration_BuildPromptContext_TaskCommentFallsBackOnMissingWorkflowStepID(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	insertTaskForPrompt(t, svc, "task-gate-no-step-id", "ws-1", "Ship the fix", "Implement the fix", 3)

	payload := `{"task_id":"task-gate-no-step-id","stage_type":"review"}`
	pc := service.BuildPromptContextForTest(svc, ctx, service.RunReasonTaskComment, payload)

	if pc.StageType != "review" {
		t.Fatalf("StageType = %q, want payload stage_type fallback when workflow_step_id is missing", pc.StageType)
	}
}

// TestSchedulerIntegration_BuildPromptContext_TaskCommentUsesRunStepNotTaskCurrentStep
// pins that when the task has since moved to a different step, the run's own
// step (from the payload) is what gets resolved, not the task's current step.
func TestSchedulerIntegration_BuildPromptContext_TaskCommentUsesRunStepNotTaskCurrentStep(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	insertTaskForPrompt(t, svc, "task-gate-moved", "ws-1", "Ship the fix", "Implement the fix", 3)
	svc.ExecSQL(t, `INSERT INTO workflow_steps (id, stage_type) VALUES (?, ?)`, "step-gate-queued", "approval")
	svc.ExecSQL(t, `INSERT INTO workflow_steps (id, stage_type) VALUES (?, ?)`, "step-gate-current", "work")
	svc.ExecSQL(t, `UPDATE tasks SET workflow_step_id = ? WHERE id = ?`, "step-gate-current", "task-gate-moved")

	payload := `{"task_id":"task-gate-moved","workflow_step_id":"step-gate-queued","stage_type":"work"}`
	pc := service.BuildPromptContextForTest(svc, ctx, service.RunReasonTaskComment, payload)

	if pc.StageType != "approval" {
		t.Fatalf("StageType = %q, want the run's queued step (approval), not the task's current step (work)", pc.StageType)
	}
}

// TestSchedulerIntegration_BuildPromptContext_TaskCommentWithoutStageTypeUnchanged
// pins AC-OFFICE-GATE-COMMENT-003.5: a task_comment run without stage_type in
// its payload never resolves a stage at all, even when workflow_step_id names
// a review step, leaving PromptContext byte-identical to before this feature.
func TestSchedulerIntegration_BuildPromptContext_TaskCommentWithoutStageTypeUnchanged(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	insertTaskForPrompt(t, svc, "task-gate-no-stage-type", "ws-1", "Ship the fix", "Implement the fix", 3)
	svc.ExecSQL(t, `INSERT INTO workflow_steps (id, stage_type) VALUES (?, ?)`, "step-gate-review-only", "review")

	payload := `{"task_id":"task-gate-no-stage-type","workflow_step_id":"step-gate-review-only"}`
	pc := service.BuildPromptContextForTest(svc, ctx, service.RunReasonTaskComment, payload)

	if pc.StageType != "" {
		t.Fatalf("StageType = %q, want empty when payload carries no stage_type", pc.StageType)
	}

	prompt := service.BuildPrompt(pc)
	if !containsIgnoreCase(prompt, "New comment") {
		t.Fatalf("prompt should stay the plain task-comment prompt, got: %s", prompt)
	}
}
