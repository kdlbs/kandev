package utility

import (
	"encoding/json"
	"time"
)

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
	OwnerID     string              `json:"owner_id"`
	InstanceID  string              `json:"instance_id,omitempty"`
	Known       bool                `json:"known"`
	Attempts    []AttemptStatus     `json:"attempts"`
	Diagnostics *AttemptDiagnostics `json:"diagnostics,omitempty"`
}

// AttemptDiagnostics records owner observations without changing inventory trust.
// Initialization and first persistence failure survive subsequent successful writes.
type AttemptDiagnostics struct {
	Version                   int    `json:"version"`
	ContinuityKnown           bool   `json:"continuity_known"`
	InitialReason             string `json:"initial_reason"`
	JournalMode               string `json:"journal_mode"`
	JournalFreshAtOwnerInit   *bool  `json:"journal_fresh_at_owner_init"`
	JournalErrorClass         string `json:"journal_error_class"`
	StorageUnavailable        bool   `json:"storage_unavailable"`
	FirstPersistFailure       string `json:"first_persist_failure"`
	LastPersistResult         string `json:"last_persist_result"`
	ManagerMintedID           bool   `json:"manager_minted_id"`
	DeliveryIncarnationID     string `json:"delivery_incarnation_id,omitempty"`
	DeliveryHarnessGeneration uint64 `json:"delivery_harness_generation"`
	DeliveryStreamPresent     bool   `json:"delivery_stream_present"`
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
	Ownership       *inferenceOwnership      `json:"ownership,omitempty"`
	Version         int                      `json:"version"`
	ContinuityKnown bool                     `json:"continuity_known"`
	Attempts        map[string]*ownedAttempt `json:"attempts"`
}

// UnmarshalJSON keeps optional diagnostics from changing functional observations.
func (s *AttemptSnapshot) UnmarshalJSON(data []byte) error {
	type observation AttemptSnapshot
	decoded := observation(*s)
	decoded.Diagnostics = nil
	wire := struct {
		*observation
		Diagnostics json.RawMessage `json:"diagnostics"`
	}{observation: &decoded}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	var diagnostic AttemptDiagnostics
	if json.Unmarshal(wire.Diagnostics, &diagnostic) == nil && diagnostic.Version == 1 {
		decoded.Diagnostics = &diagnostic
	}
	*s = AttemptSnapshot(decoded)
	return nil
}
