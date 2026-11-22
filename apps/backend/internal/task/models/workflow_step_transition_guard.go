package models

import "context"

type workflowStepTransitionGuardKey struct{}

type workflowStepTransitionGuard struct {
	taskID       string
	stepID       string
	transitionID int64
	completion   bool
	sessionID    string
	signalID     string
}

// WithWorkflowStepTransitionGuard binds a guarded move to the observed task
// arrival, so departing and returning to the same step cannot revive it.
func WithWorkflowStepTransitionGuard(ctx context.Context, taskID, stepID string, transitionID int64) context.Context {
	return context.WithValue(ctx, workflowStepTransitionGuardKey{}, workflowStepTransitionGuard{taskID: taskID, stepID: stepID, transitionID: transitionID})
}

// WithWorkflowStepCompletionGuard binds completion and its handoff carry update
// to the source arrival. Other guarded moves do not own the carry slot.
func WithWorkflowStepCompletionGuard(ctx context.Context, taskID, stepID string, transitionID int64) context.Context {
	return context.WithValue(ctx, workflowStepTransitionGuardKey{}, workflowStepTransitionGuard{taskID: taskID, stepID: stepID, transitionID: transitionID, completion: true})
}

// WithWorkflowStepCompletionSignalGuard adds the exact accepted-signal identity
// to a completion guard. A move may commit only while that source still owns
// the pending completion for the observed entry.
func WithWorkflowStepCompletionSignalGuard(
	ctx context.Context, taskID, stepID string, transitionID int64, sessionID, signalID string,
) context.Context {
	return context.WithValue(ctx, workflowStepTransitionGuardKey{}, workflowStepTransitionGuard{
		taskID:       taskID,
		stepID:       stepID,
		transitionID: transitionID,
		completion:   true,
		sessionID:    sessionID,
		signalID:     signalID,
	})
}

// WorkflowStepTransitionGuardFromContext returns the arrival fence for this
// source step. Other tasks and destination entry work keep their admission rules.
func WorkflowStepTransitionGuardFromContext(ctx context.Context, taskID, stepID string) (int64, bool) {
	guard, ok := ctx.Value(workflowStepTransitionGuardKey{}).(workflowStepTransitionGuard)
	return guard.transitionID, ok && guard.taskID == taskID && guard.stepID == stepID
}

// WorkflowStepCompletionGuardFromContext returns the completion-owned guard
// when this move is bound to a step-completion source. signalID and sessionID
// are empty for a legacy completion guard that predates accepted-signal
// identity, preserving its entry-only fence.
func WorkflowStepCompletionGuardFromContext(ctx context.Context, taskID, stepID string) (WorkflowStepCompletionGuard, bool) {
	guard, ok := ctx.Value(workflowStepTransitionGuardKey{}).(workflowStepTransitionGuard)
	if !ok || !guard.completion || guard.taskID != taskID || guard.stepID != stepID {
		return WorkflowStepCompletionGuard{}, false
	}
	return WorkflowStepCompletionGuard{
		SessionID:    guard.sessionID,
		SignalID:     guard.signalID,
		TransitionID: guard.transitionID,
	}, true
}

// WorkflowStepCompletionGuard is the immutable completion identity carried by
// an in-flight transition.
type WorkflowStepCompletionGuard struct {
	SessionID    string
	SignalID     string
	TransitionID int64
}

// WorkflowStepCompletionOwnsHandoff reports whether this source move carries
// completion-owned handoff intent, including removal of an earlier token.
func WorkflowStepCompletionOwnsHandoff(ctx context.Context, taskID, stepID string) bool {
	guard, ok := ctx.Value(workflowStepTransitionGuardKey{}).(workflowStepTransitionGuard)
	return ok && guard.completion && guard.taskID == taskID && guard.stepID == stepID
}
