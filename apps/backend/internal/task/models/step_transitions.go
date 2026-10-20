package models

import "time"

// StepTransition is the public-safe recorded movement of one task.
type StepTransition struct {
	ID                 int64
	FromWorkflowID     *string
	FromWorkflowStepID *string
	ToWorkflowID       *string
	ToWorkflowStepID   *string
	Trigger            string
	OccurredAt         time.Time
}

// TransitionGroup is one route through, into, or out of a workflow.
type TransitionGroup struct {
	Kind       string
	FromStepID *string
	ToStepID   *string
	Count      int64
}

// StepTransitionRecord is one recorded movement of a task with its
// attribution, for callers authorized to read the task's workspace. Step names
// are resolved from current workflow steps and are nil for a removed step.
type StepTransitionRecord struct {
	ID                   int64
	FromWorkflowID       *string
	FromWorkflowStepID   *string
	FromWorkflowStepName *string
	ToWorkflowID         *string
	ToWorkflowStepID     *string
	ToWorkflowStepName   *string
	Trigger              string
	ActorKind            string
	ActorID              *string
	OccurredAt           time.Time
}
