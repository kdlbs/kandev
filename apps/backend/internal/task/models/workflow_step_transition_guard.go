package models

import "context"

type workflowStepTransitionGuardKey struct{}

type workflowStepTransitionGuard struct {
	taskID       string
	stepID       string
	transitionID int64
	completion   bool
}

// WithWorkflowStepTransitionGuard binds a guarded move to the observed task
// arrival, so departing and returning to the same step cannot revive it.
func WithWorkflowStepTransitionGuard(ctx context.Context, taskID, stepID string, transitionID int64) context.Context {
	return context.WithValue(ctx, workflowStepTransitionGuardKey{}, workflowStepTransitionGuard{taskID: taskID, stepID: stepID, transitionID: transitionID})
}

// WorkflowStepTransitionGuardFromContext returns the arrival fence for this
// source step. Other tasks and destination entry work keep their admission rules.
func WorkflowStepTransitionGuardFromContext(ctx context.Context, taskID, stepID string) (int64, bool) {
	guard, ok := ctx.Value(workflowStepTransitionGuardKey{}).(workflowStepTransitionGuard)
	return guard.transitionID, ok && guard.taskID == taskID && guard.stepID == stepID
}

// WithWorkflowStepCompletionGuard binds completion and its handoff carry update
// to the source arrival. Other guarded moves do not own the carry slot.
func WithWorkflowStepCompletionGuard(ctx context.Context, taskID, stepID string, transitionID int64) context.Context {
	return context.WithValue(ctx, workflowStepTransitionGuardKey{}, workflowStepTransitionGuard{taskID: taskID, stepID: stepID, transitionID: transitionID, completion: true})
}

// WorkflowStepCompletionOwnsHandoff reports whether this source move carries
// completion-owned handoff intent, including removal of an earlier token.
func WorkflowStepCompletionOwnsHandoff(ctx context.Context, taskID, stepID string) bool {
	guard, ok := ctx.Value(workflowStepTransitionGuardKey{}).(workflowStepTransitionGuard)
	return ok && guard.completion && guard.taskID == taskID && guard.stepID == stepID
}
