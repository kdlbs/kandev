package models

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrWorkflowScriptRunNotFound is returned when a script run ID or
	// occurrence key does not identify a stored run.
	ErrWorkflowScriptRunNotFound = errors.New("workflow script run not found")
	// ErrWorkflowScriptRunInvalid is returned when a new run does not contain
	// the identity and immutable snapshot required for durable admission.
	ErrWorkflowScriptRunInvalid = errors.New("invalid workflow script run")
	// ErrWorkflowScriptRunInvalidTransition is returned when a caller tries to
	// move a run outside its one-way lifecycle.
	ErrWorkflowScriptRunInvalidTransition = errors.New("invalid workflow script run transition")
)

// WorkflowScriptRunTrigger identifies the lifecycle event that admitted a
// script. The values are persisted so a run can be understood without the
// workflow definition that caused it.
type WorkflowScriptRunTrigger string

const (
	WorkflowScriptRunTriggerOnEnter        WorkflowScriptRunTrigger = "on_enter"
	WorkflowScriptRunTriggerOnTurnComplete WorkflowScriptRunTrigger = "on_turn_complete"
	WorkflowScriptRunTriggerOnExit         WorkflowScriptRunTrigger = "on_exit"
)

// IsValid reports whether the trigger is one of the lifecycle events that can
// own a workflow script action.
func (t WorkflowScriptRunTrigger) IsValid() bool {
	switch t {
	case WorkflowScriptRunTriggerOnEnter, WorkflowScriptRunTriggerOnTurnComplete, WorkflowScriptRunTriggerOnExit:
		return true
	default:
		return false
	}
}

const (
	WorkflowScriptFailurePolicyBlock    = "block"
	WorkflowScriptFailurePolicyContinue = "continue"
)

// IsValid reports whether a failure policy is understood by the durable run
// ledger.
func IsValidWorkflowScriptFailurePolicy(policy string) bool {
	return policy == WorkflowScriptFailurePolicyBlock || policy == WorkflowScriptFailurePolicyContinue
}

// WorkflowScriptRunStatus is the durable lifecycle of one admitted action.
type WorkflowScriptRunStatus string

const (
	WorkflowScriptRunPending     WorkflowScriptRunStatus = "pending"
	WorkflowScriptRunStarting    WorkflowScriptRunStatus = "starting"
	WorkflowScriptRunRunning     WorkflowScriptRunStatus = "running"
	WorkflowScriptRunSucceeded   WorkflowScriptRunStatus = "succeeded"
	WorkflowScriptRunFailed      WorkflowScriptRunStatus = "failed"
	WorkflowScriptRunTimedOut    WorkflowScriptRunStatus = "timed_out"
	WorkflowScriptRunInterrupted WorkflowScriptRunStatus = "interrupted"
)

// IsTerminal reports whether a run has a final result and cannot be admitted
// or completed again.
func (s WorkflowScriptRunStatus) IsTerminal() bool {
	switch s {
	case WorkflowScriptRunSucceeded, WorkflowScriptRunFailed,
		WorkflowScriptRunTimedOut, WorkflowScriptRunInterrupted:
		return true
	default:
		return false
	}
}

// CanTransitionTo reports whether next is a legal one-way lifecycle change.
func (s WorkflowScriptRunStatus) CanTransitionTo(next WorkflowScriptRunStatus) bool {
	if s.IsTerminal() {
		return false
	}
	if next == WorkflowScriptRunRunning {
		return s == WorkflowScriptRunStarting
	}
	if next.IsTerminal() {
		return s == WorkflowScriptRunPending || s == WorkflowScriptRunStarting || s == WorkflowScriptRunRunning
	}
	return next == WorkflowScriptRunStarting && s == WorkflowScriptRunPending
}

// NewWorkflowScriptOccurrenceKey returns the stable deduplication identity for
// one action occurrence. The action position is part of the key because one
// trigger can contain multiple scripts.
func NewWorkflowScriptOccurrenceKey(trigger WorkflowScriptRunTrigger, occurrenceID, workflowStepID string, actionPosition int) string {
	return fmt.Sprintf("%s/%s/%s/%d", trigger, occurrenceID, workflowStepID, actionPosition)
}

// WorkflowScriptRun is the immutable action snapshot and mutable execution
// ledger for one workflow script occurrence.
type WorkflowScriptRun struct {
	ID                   string                   `json:"id"`
	OccurrenceKey        string                   `json:"occurrence_key"`
	TaskID               string                   `json:"task_id"`
	WorkflowID           string                   `json:"workflow_id"`
	WorkflowStepID       string                   `json:"workflow_step_id"`
	WorkflowStepName     string                   `json:"workflow_step_name"`
	Trigger              WorkflowScriptRunTrigger `json:"trigger"`
	ActionPosition       int                      `json:"action_position"`
	SessionID            string                   `json:"session_id"`
	ExecutionID          string                   `json:"execution_id"`
	Command              string                   `json:"command"`
	TimeoutSeconds       int                      `json:"timeout_seconds"`
	FailurePolicy        string                   `json:"failure_policy"`
	ProcessRequestID     string                   `json:"process_request_id"`
	MessageID            string                   `json:"message_id,omitempty"`
	ProcessID            string                   `json:"process_id,omitempty"`
	Status               WorkflowScriptRunStatus  `json:"status"`
	AdmissionAttemptedAt *time.Time               `json:"admission_attempted_at,omitempty"`
	ExitCode             *int                     `json:"exit_code,omitempty"`
	Output               string                   `json:"output,omitempty"`
	OutputTruncated      bool                     `json:"output_truncated,omitempty"`
	FailureReason        string                   `json:"failure_reason,omitempty"`
	CreatedAt            time.Time                `json:"created_at"`
	UpdatedAt            time.Time                `json:"updated_at"`
	StartedAt            *time.Time               `json:"started_at,omitempty"`
	CompletedAt          *time.Time               `json:"completed_at,omitempty"`
}

// WorkflowScriptRunCompletion contains the mutable result written once by a
// terminal transition.
type WorkflowScriptRunCompletion struct {
	Status          WorkflowScriptRunStatus
	ProcessID       string
	ExitCode        *int
	Output          string
	OutputTruncated bool
	FailureReason   string
	CompletedAt     time.Time
}
