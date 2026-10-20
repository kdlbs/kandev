package models

import "context"

type workflowStepTransitionGuardKey struct{}

type workflowStepTransitionGuard struct {
	taskID       string
	stepID       string
	transitionID int64
}

// WithWorkflowStepTransitionGuard binds a guarded move to the observed task
// arrival, so departing and returning to the same step cannot revive it.
func WithWorkflowStepTransitionGuard(ctx context.Context, taskID, stepID string, transitionID int64) context.Context {
	return context.WithValue(ctx, workflowStepTransitionGuardKey{}, workflowStepTransitionGuard{taskID, stepID, transitionID})
}

// WorkflowStepTransitionGuardFromContext returns the arrival fence for this
// source step. Other tasks and destination entry work keep their admission rules.
func WorkflowStepTransitionGuardFromContext(ctx context.Context, taskID, stepID string) (int64, bool) {
	guard, ok := ctx.Value(workflowStepTransitionGuardKey{}).(workflowStepTransitionGuard)
	return guard.transitionID, ok && guard.taskID == taskID && guard.stepID == stepID
}
