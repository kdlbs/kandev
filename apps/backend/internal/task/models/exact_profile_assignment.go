package models

import (
	"errors"
	"time"
)

var (
	ErrExactProfileAssignmentGeneration   = errors.New("exact profile assignment generation rejected")
	ErrExactProfileAssignmentInvalidInput = errors.New("invalid exact profile assignment input")
)

// Exact-profile launch receipt outcomes.
const (
	ExactProfileLaunchOutcomeApplied      = "applied"
	ExactProfileLaunchOutcomeFailedClosed = "failed_closed"
)

// ExactProfileAssignment is the durable, task-owned selection of one concrete
// agent profile. ProfileRevision is the database-owned profile mutation token.
type ExactProfileAssignment struct {
	TaskID               string
	WorkspaceID          string
	AgentProfileID       string
	ProfileRevision      int64
	Generation           int64
	SourceWorkflowID     string
	SourceWorkflowStepID string
	SourceTaskState      string
	Active               bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// ExactProfileLaunchReceipt is the immutable audit of the first governed launch
// attempt for a task and session. A later retry can have a different outcome or
// assignment; this historical receipt does not describe the current attempt.
type ExactProfileLaunchReceipt struct {
	TaskID           string
	SessionID        string
	AgentProfileID   string
	Generation       int64
	ProfileRevision  int64
	Model            string
	Outcome          string
	FailureReason    string
	InferenceStarted bool
	SubstitutionDone bool
	CreatedAt        time.Time
}
