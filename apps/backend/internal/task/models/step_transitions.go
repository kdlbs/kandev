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
