package models

import "time"

const (
	// DeliveryEffectPending means an effect has been recorded but its
	// authoritative consumer has not committed the state transition yet.
	DeliveryEffectPending = "pending"
	// DeliveryEffectCompleted means the effect key was committed together with
	// the authoritative state transition.
	DeliveryEffectCompleted = "completed"
)

// DeliverySubmissionState is the durable state of one immutable prompt
// submission. interrupted_unknown is intentionally terminal for automatic
// dispatch: the caller must reconcile or create a new authorized submission.
type DeliverySubmissionState string

const (
	DeliverySubmissionPrepared           DeliverySubmissionState = "prepared"
	DeliverySubmissionAccepted           DeliverySubmissionState = "accepted"
	DeliverySubmissionDispatching        DeliverySubmissionState = "dispatching"
	DeliverySubmissionCompleted          DeliverySubmissionState = "completed"
	DeliverySubmissionFailed             DeliverySubmissionState = "failed"
	DeliverySubmissionCancelled          DeliverySubmissionState = "cancelled"
	DeliverySubmissionInterruptedUnknown DeliverySubmissionState = "interrupted_unknown"
)

// AgentDeliverySubmission is the backend-owned immutable submission record.
// PayloadHash covers the complete composed prompt, attachments, configuration,
// and target identity used for admission.
type AgentDeliverySubmission struct {
	ID                string                  `json:"id"`
	SessionID         string                  `json:"session_id"`
	IncarnationID     string                  `json:"incarnation_id"`
	HarnessGeneration int64                   `json:"harness_generation"`
	OwnerGeneration   int64                   `json:"owner_generation"`
	DispatchAttemptID string                  `json:"dispatch_attempt_id,omitempty"`
	PayloadHash       string                  `json:"payload_hash"`
	Payload           []byte                  `json:"payload"`
	State             DeliverySubmissionState `json:"state"`
	Outcome           string                  `json:"outcome,omitempty"`
	CreatedAt         time.Time               `json:"created_at"`
	UpdatedAt         time.Time               `json:"updated_at"`
}

// AgentDeliveryEvent is the normalized event persisted in the backend inbox
// before agentctl receives an acknowledgment.
type AgentDeliveryEvent struct {
	SessionID         string     `json:"session_id"`
	IncarnationID     string     `json:"incarnation_id"`
	HarnessGeneration int64      `json:"harness_generation"`
	StreamID          string     `json:"stream_id"`
	Sequence          int64      `json:"sequence"`
	SubmissionID      string     `json:"submission_id,omitempty"`
	EventType         string     `json:"event_type"`
	Payload           []byte     `json:"payload"`
	Terminal          bool       `json:"terminal,omitempty"`
	ReceivedAt        time.Time  `json:"received_at"`
	ProjectedAt       *time.Time `json:"projected_at,omitempty"`
}

// AgentDeliveryCursor stores the two independent backend watermarks. A
// received event is safe to acknowledge after its inbox transaction commits;
// projection may lag and must never move the received cursor backward.
type AgentDeliveryCursor struct {
	SessionID         string    `json:"session_id"`
	IncarnationID     string    `json:"incarnation_id"`
	HarnessGeneration int64     `json:"harness_generation"`
	StreamID          string    `json:"stream_id"`
	ReceivedSequence  int64     `json:"received_sequence"`
	ProjectedSequence int64     `json:"projected_sequence"`
	RemoteHighWater   int64     `json:"remote_high_water"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// AgentDeliveryEffect is a durable idempotency key for a projected workflow
// or turn intent. Consumers claim it at their authoritative state transition.
type AgentDeliveryEffect struct {
	EffectKey   string     `json:"effect_key"`
	StreamID    string     `json:"stream_id"`
	Sequence    int64      `json:"sequence"`
	EffectType  string     `json:"effect_type"`
	State       string     `json:"state"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}
