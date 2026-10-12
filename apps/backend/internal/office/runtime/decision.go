package runtime

import (
	"context"
	"fmt"
	"time"
)

// DecisionRecorder is the composition-boundary dependency for the signed
// Office task decision endpoint.
type DecisionRecorder interface {
	RecordAgentDecision(ctx context.Context, input RecordAgentDecisionInput) (RecordAgentDecisionResult, error)
}

// RecordAgentDecisionInput contains only identity resolved from the signed run
// and the two caller-controlled decision fields.
type RecordAgentDecisionInput struct {
	TaskID         string
	AgentProfileID string
	SessionID      string
	Decision       string
	Reason         string
}

// DecisionGuard is the JSON shape shared by the runtime decision response and
// the Office quorum diagnostic response.
type DecisionGuard struct {
	TargetStepID  string `json:"target_step_id"`
	Role          string `json:"role"`
	Threshold     string `json:"threshold"`
	RequiredCount int    `json:"required_count"`
	ReceivedCount int    `json:"received_count"`
	Satisfied     bool   `json:"satisfied"`
	Reason        string `json:"reason,omitempty"`
	Error         string `json:"error,omitempty"`
}

// RecordAgentDecisionResult is the seven-field task decision contract.
type RecordAgentDecisionResult struct {
	Decision          string          `json:"decision"`
	Role              string          `json:"role"`
	StepID            string          `json:"step_id"`
	DecisionID        string          `json:"decision_id"`
	DecidedAt         time.Time       `json:"decided_at"`
	TransitionApplied bool            `json:"transition_applied"`
	Guards            []DecisionGuard `json:"guards"`
}

// DecisionValidationError identifies a caller-correctable decision request.
type DecisionValidationError struct {
	Err error
}

func (e *DecisionValidationError) Error() string {
	if e == nil || e.Err == nil {
		return "invalid agent decision"
	}
	return e.Err.Error()
}

func (e *DecisionValidationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// NewDecisionValidationError preserves validation classification at the
// runtime/composition boundary without importing the dashboard package here.
func NewDecisionValidationError(err error) error {
	return &DecisionValidationError{Err: err}
}

var errDecisionRecorderMissing = fmt.Errorf("%w: decision recorder", ErrRuntimeDependencyMissing)
