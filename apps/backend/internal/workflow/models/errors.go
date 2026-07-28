package models

import (
	"errors"
	"fmt"
)

// ErrWIPLimitExceeded identifies a workflow-step capacity rejection.
var ErrWIPLimitExceeded = errors.New("workflow step WIP limit exceeded")

// WIPLimitError carries workflow-step capacity details while preserving
// errors.Is(err, ErrWIPLimitExceeded) classification at service boundaries.
type WIPLimitError struct {
	StepID   string
	Limit    int
	Occupied int
}

func (e *WIPLimitError) Error() string {
	return fmt.Sprintf("WIP limit exceeded for workflow step %s: limit %d already occupied (%d)", e.StepID, e.Limit, e.Occupied)
}

func (e *WIPLimitError) Unwrap() error {
	return ErrWIPLimitExceeded
}

// NewWIPLimitError constructs a typed capacity conflict for a workflow step.
func NewWIPLimitError(stepID string, limit, occupied int) error {
	return &WIPLimitError{StepID: stepID, Limit: limit, Occupied: occupied}
}
