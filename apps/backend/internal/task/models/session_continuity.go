package models

import "time"

// HarnessSessionGeneration identifies one native conversation within a
// durable Kandev session incarnation.
type HarnessSessionGeneration struct {
	SessionID             string    `json:"session_id"`
	IncarnationID         string    `json:"incarnation_id"`
	Generation            int64     `json:"generation"`
	PredecessorGeneration int64     `json:"predecessor_generation,omitempty"`
	NativeSessionID       string    `json:"native_session_id"`
	AgentType             string    `json:"agent_type"`
	AdapterVersion        string    `json:"adapter_version,omitempty"`
	OriginalWorkspace     string    `json:"original_workspace,omitempty"`
	CurrentWorkspace      string    `json:"current_workspace,omitempty"`
	NativeStateReference  string    `json:"native_state_reference,omitempty"`
	CreationReason        string    `json:"creation_reason"`
	CreatedAt             time.Time `json:"created_at"`
	CommittedAt           time.Time `json:"committed_at"`
}

// RestoreAttempt records the decision and result of one native restore or
// explicitly authorized continuation action.
type RestoreAttempt struct {
	ID                 string     `json:"id"`
	SessionID          string     `json:"session_id"`
	IncarnationID      string     `json:"incarnation_id"`
	ExpectedGeneration int64      `json:"expected_generation"`
	Action             string     `json:"action"`
	Outcome            string     `json:"outcome"`
	Reason             string     `json:"reason"`
	TargetWorkspace    string     `json:"target_workspace,omitempty"`
	Authorized         bool       `json:"authorized"`
	CreatedAt          time.Time  `json:"created_at"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
}

// ContinuationSnapshot is bounded, untrusted context composed from canonical
// Kandev task data. It is not a native harness checkpoint.
type ContinuationSnapshot struct {
	ID               string     `json:"id"`
	AttemptID        string     `json:"attempt_id"`
	SessionID        string     `json:"session_id"`
	TargetGeneration int64      `json:"target_generation"`
	SubmissionID     string     `json:"submission_id,omitempty"`
	SourceMessageID  string     `json:"source_message_id,omitempty"`
	Content          string     `json:"content"`
	ByteCount        int        `json:"byte_count"`
	OmittedMessages  int        `json:"omitted_messages"`
	Truncated        bool       `json:"truncated"`
	ContentHash      string     `json:"content_hash"`
	Status           string     `json:"status"`
	CreatedAt        time.Time  `json:"created_at"`
	ResolvedAt       *time.Time `json:"resolved_at,omitempty"`
}

// SessionRecoveryBlock prevents automatic work admission while a session
// needs operator settlement.
type SessionRecoveryBlock struct {
	ID                 string     `json:"id"`
	SessionID          string     `json:"session_id"`
	IncarnationID      string     `json:"incarnation_id"`
	ExpectedGeneration int64      `json:"expected_generation"`
	Reason             string     `json:"reason"`
	State              string     `json:"state"`
	ConsumerReference  string     `json:"consumer_reference,omitempty"`
	AuthorizedAction   string     `json:"authorized_action,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	ResolvedAt         *time.Time `json:"resolved_at,omitempty"`
}

const (
	ContinuitySnapshotPrepared  = "prepared"
	ContinuitySnapshotConsumed  = "consumed"
	ContinuitySnapshotUncertain = "uncertain"
	ContinuitySnapshotResolved  = "resolved"

	RecoveryBlockOpen     = "open"
	RecoveryBlockResolved = "resolved"
)
