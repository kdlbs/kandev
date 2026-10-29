package utility

import "time"

const (
	AttemptPrepared    = "prepared"
	AttemptStarting    = "starting"
	AttemptRunning     = "running"
	AttemptUnknown     = "unknown"
	AttemptNotStarted  = "not_started"
	AttemptTerminated  = "terminated"
	PromptNotSent      = "not_sent"
	PromptPossiblySent = "possibly_sent"
)

// AttemptStatus contains identities and observations, never execution content.
type AttemptStatus struct {
	PreparationID  string    `json:"preparation_id,omitempty"`
	AttemptID      string    `json:"attempt_id"`
	CallID         string    `json:"call_id,omitempty"`
	OwnerID        string    `json:"owner_id"`
	InstanceID     string    `json:"instance_id,omitempty"`
	TaskID         string    `json:"task_id,omitempty"`
	SessionID      string    `json:"session_id,omitempty"`
	State          string    `json:"state"`
	PromptDispatch string    `json:"prompt_dispatch"`
	Known          bool      `json:"known"`
	Settled        bool      `json:"settled"`
	Retained       bool      `json:"retained,omitempty"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// AttemptSnapshot.Known attests inventory continuity, not just this process map.
// A false value with an empty list must never be interpreted as zero population.
type AttemptSnapshot struct {
	OwnerID    string          `json:"owner_id"`
	InstanceID string          `json:"instance_id,omitempty"`
	Known      bool            `json:"known"`
	Attempts   []AttemptStatus `json:"attempts"`
}

type ownedAttempt struct {
	Status   AttemptStatus `json:"status"`
	PID      int           `json:"pid,omitempty"`
	Consumed bool          `json:"consumed,omitempty"`
}

// PrepareAttemptRequest correlates token preparation without inference content.
type PrepareAttemptRequest struct {
	PreparationID string `json:"preparation_id"`
	CallID        string `json:"call_id,omitempty"`
	Retain        bool   `json:"retain,omitempty"`
}

type inferenceOwnerState struct {
	Version         int                      `json:"version"`
	ContinuityKnown bool                     `json:"continuity_known"`
	Attempts        map[string]*ownedAttempt `json:"attempts"`
}
