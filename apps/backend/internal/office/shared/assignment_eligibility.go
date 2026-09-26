package shared

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"

	"go.uber.org/zap"
)

// AssignmentStepGetter resolves a workflow step by ID. Implemented by
// workflow/service.Service.GetStep and by the scheduler's own
// WorkflowStepGetter (structurally identical), so every assignment-wake
// producer can share one eligibility check without a new dependency.
type AssignmentStepGetter interface {
	GetStep(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error)
}

// AssignmentStepIDLookup resolves the workflow step currently bound to a
// task. Implemented by the office repository's GetTaskWorkflowStepID.
type AssignmentStepIDLookup interface {
	GetTaskWorkflowStepID(ctx context.Context, taskID string) (string, error)
}

// IsAssignmentWakeEligible reports whether a task_assigned wake may be
// queued for taskID, given its current workflow step.
//
// A step is eligible only when it carries an auto_start_agent on_enter
// action (wfmodels.OnEnterAutoStartAgent) — the same predicate the
// orchestrator's SelectAutoStartStep uses to decide which step may launch
// an agent. Waking a task sitting on any other step (Backlog, Review,
// Approval, ...) would run the agent outside the workflow: it does
// real work, calls step_complete_kandev, gets accepted:true even though
// nothing advances, and then the card's later move to an eligible step
// launches the agent a second time.
//
// This intentionally fails OPEN (returns true) whenever eligibility
// cannot be determined:
//   - an empty step ID (task has no workflow step bound) preserves
//     current behavior for non-workflow-driven tasks;
//   - a step-ID lookup error, a nil getter, a step lookup error, or a nil
//     step all mean "we don't know", and refusing to wake would silently
//     strand a task rather than merely misfire it once. Every fail-open
//     branch logs a WARN so the ambiguity is visible instead of silent.
//
// Only the wake decision is gated here. Interrupting a previous
// assignee's running session must still fire regardless of the new
// step's eligibility — callers must not route that through this check.
func IsAssignmentWakeEligible(
	ctx context.Context,
	log *logger.Logger,
	stepIDs AssignmentStepIDLookup,
	steps AssignmentStepGetter,
	taskID string,
	source string,
) bool {
	stepID, err := stepIDs.GetTaskWorkflowStepID(ctx, taskID)
	if err != nil {
		log.Warn("office.assignment_wake.step_lookup_failed",
			zap.String("task_id", taskID), zap.String("source", source), zap.Error(err))
		return true
	}
	if stepID == "" {
		log.Info("office.assignment_wake.step_eligible",
			zap.String("task_id", taskID), zap.String("step_id", stepID), zap.String("source", source))
		return true
	}
	if steps == nil {
		log.Warn("office.assignment_wake.no_step_getter",
			zap.String("task_id", taskID), zap.String("step_id", stepID), zap.String("source", source))
		return true
	}
	step, err := steps.GetStep(ctx, stepID)
	if err != nil {
		log.Warn("office.assignment_wake.step_getter_failed",
			zap.String("task_id", taskID), zap.String("step_id", stepID),
			zap.String("source", source), zap.Error(err))
		return true
	}
	if step == nil {
		log.Warn("office.assignment_wake.step_not_found",
			zap.String("task_id", taskID), zap.String("step_id", stepID), zap.String("source", source))
		return true
	}
	if !step.HasOnEnterAction(wfmodels.OnEnterAutoStartAgent) {
		log.Info("office.assignment_wake.step_ineligible",
			zap.String("task_id", taskID), zap.String("step_id", stepID), zap.String("source", source))
		return false
	}
	log.Info("office.assignment_wake.step_eligible",
		zap.String("task_id", taskID), zap.String("step_id", stepID), zap.String("source", source))
	return true
}
