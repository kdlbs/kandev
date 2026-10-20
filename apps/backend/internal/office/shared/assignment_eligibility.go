package shared

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
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

// AssignmentTaskLookup reads the current workflow step and persisted blockers.
type AssignmentTaskLookup interface {
	AssignmentStepIDLookup
	ListTaskBlockers(ctx context.Context, taskID string) ([]*models.TaskBlocker, error)
	IsTaskInTerminalStep(ctx context.Context, taskID string) (bool, error)
}

// IsAssignmentWakeEligible permits assignment wakes only on eligible steps
// without a known pending blocker. Step uncertainty never bypasses blockers.
// Previous-assignee interruption remains outside this wake-only decision.
func IsAssignmentWakeEligible(
	ctx context.Context,
	log *logger.Logger,
	tasks AssignmentTaskLookup,
	steps AssignmentStepGetter,
	taskID string,
	source string,
) bool {
	if !isAssignmentStepEligible(ctx, log, tasks, steps, taskID, source) {
		return false
	}
	blockers, err := tasks.ListTaskBlockers(ctx, taskID)
	if err != nil {
		log.Warn("office.assignment_wake.blockers_lookup_failed",
			zap.String("task_id", taskID), zap.String("source", source), zap.Error(err))
		return true
	}
	for _, blocker := range blockers {
		terminal, err := tasks.IsTaskInTerminalStep(ctx, blocker.BlockerTaskID)
		if err != nil {
			log.Warn("office.assignment_wake.blocker_lookup_failed",
				zap.String("task_id", taskID), zap.String("blocker_task_id", blocker.BlockerTaskID),
				zap.String("source", source), zap.Error(err))
			// Unknown evidence preserves admission, but cannot hide a later pending blocker.
			continue
		}
		if !terminal {
			log.Info("office.assignment_wake.blocker_pending",
				zap.String("task_id", taskID), zap.String("blocker_task_id", blocker.BlockerTaskID), zap.String("source", source))
			return false
		}
	}
	return true
}

// isAssignmentStepEligible preserves assignment admission when a step is unbound
// or cannot be resolved. Known steps require an auto_start_agent action.
func isAssignmentStepEligible(
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
