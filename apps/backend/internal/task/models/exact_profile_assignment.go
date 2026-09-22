package models

import (
	"errors"
	"time"
)

var (
	ErrExactProfileAssignmentGeneration   = errors.New("exact profile assignment generation rejected")
	ErrExactProfileAssignmentInvalidInput = errors.New("invalid exact profile assignment input")
	ErrCoordinatorHandoffConflict         = errors.New("coordinator handoff state changed")
)

// Exact-profile launch receipt outcomes.
const (
	ExactProfileLaunchOutcomeApplied      = "applied"
	ExactProfileLaunchOutcomeFailedClosed = "failed_closed"
)

// ExactProfileAssignment is the durable, task-owned selection of one concrete
// agent profile. ProfileRevision is the profile's immutable UpdatedAt snapshot.
type ExactProfileAssignment struct {
	TaskID               string
	WorkspaceID          string
	AgentProfileID       string
	ProfileRevision      time.Time
	Generation           int64
	SourceWorkflowID     string
	SourceWorkflowStepID string
	SourceTaskState      string
	Active               bool
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// ExactProfileLaunchReceipt is the durable record that one launch applied (or
// failed closed against) the exact-profile assignment at a generation. The
// receipt is written once per launch attempt; it is never rewritten by a later
// generation so the linkage between a session and the assignment that created
// it stays auditable.
type ExactProfileLaunchReceipt struct {
	TaskID           string
	SessionID        string
	AgentProfileID   string
	Generation       int64
	ProfileRevision  time.Time
	Model            string
	Outcome          string
	FailureReason    string
	InferenceStarted bool
	SubstitutionDone bool
	CreatedAt        time.Time
}

// ExactProfileLaunchAttemptBinding is the immutable internal identity admitted
// before process start. Legacy sessions have no row until a new exact launch.
type ExactProfileLaunchAttemptBinding struct {
	TaskID, SessionID, ExecutionID, AttemptID, SessionIncarnationID, AgentProfileID string
	ProfileRevision                                                                 time.Time
	Generation                                                                      int64
	CreatedAt                                                                       time.Time
}
