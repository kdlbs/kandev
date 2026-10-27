package models

import "time"

// AgentDeliveryRetentionBatchOptions bounds one repository-owned retention
// transaction. Zero values select the default limit; requested limits are
// capped at the platform maximum.
type AgentDeliveryRetentionBatchOptions struct {
	MaxCandidates   int
	MaxPayloadBytes int64
	WorkTimeout     time.Duration
	PayloadGrace    time.Duration
	ReplayAge       time.Duration
	ReplayEvents    int
}

// AgentDeliveryRetentionDiscovery contains private committed scan positions.
// It is scheduling evidence only, never deletion or recovery authority.
type AgentDeliveryRetentionDiscovery struct {
	CompactionAt       time.Time
	CompactionValid    bool
	CompactionStreamID string
	CompactionSequence int64
	ReceiptStreamID    string
	ReceiptSequence    int64
	SubmissionID       string
}

// AgentDeliveryRetentionBatchResult reports the durable work committed by one
// bounded retention transaction.
type AgentDeliveryRetentionBatchResult struct {
	DiscoveryBefore AgentDeliveryRetentionDiscovery
	DiscoveryAfter  AgentDeliveryRetentionDiscovery
	// DiscoveryAdvanced reports committed forward movement, excluding frontier wrap.
	DiscoveryAdvanced        bool
	Candidates               int
	EligibleCandidates       int
	OldestEligibleAgeSeconds int64
	EventPayloadsPruned      int
	EventRowsRetired         int
	PromptPayloadsPruned     int
	PayloadBytesRead         int64
	PayloadBytesPruned       int64
	PromptPayloadBytesPruned int64
}

// AgentDeliveryRetirementBatchOptions bounds one transaction that purges a
// deleted session identity. Zero values use the repository defaults.
type AgentDeliveryRetirementBatchOptions struct {
	MaxCandidates int
	WorkTimeout   time.Duration
}

// AgentDeliveryRetirementBatchResult reports one committed retirement batch.
type AgentDeliveryRetirementBatchResult struct {
	RowsPurged int
	Completed  int
	Suspended  int
	Pending    bool
}

// AgentDeliveryOrphanBatchOptions bounds authoritative orphan discovery.
type AgentDeliveryOrphanBatchOptions struct {
	MaxCandidates int
	WorkTimeout   time.Duration
}

// AgentDeliveryOrphanBatchResult reports orphan jobs scheduled by one scan.
type AgentDeliveryOrphanBatchResult struct {
	Candidates          int
	JobsScheduled       int
	UnboundWorkflowRows int
}

const (
	// DeliveryEffectPending means an effect has been recorded but its
	// authoritative consumer has not committed the state transition yet.
	DeliveryEffectPending = "pending"
	// DeliveryEffectBlockResolved means the terminal effect and matching
	// recovery block were settled; queue claim cleanup remains retryable.
	DeliveryEffectBlockResolved = "block_resolved"
	// DeliveryEffectCompleted means the effect key was committed together with
	// the authoritative state transition.
	DeliveryEffectCompleted    = "completed"
	DeliveryTerminalEffectType = "agent_delivery.terminal_settlement"
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
	PayloadPrunedAt   *time.Time              `json:"payload_pruned_at,omitempty"`
	PayloadBytes      int64                   `json:"payload_bytes"`
	FullySettledAt    *time.Time              `json:"fully_settled_at,omitempty"`
	TerminalStreamID  string                  `json:"terminal_stream_id,omitempty"`
	TerminalSequence  int64                   `json:"terminal_sequence,omitempty"`
	TerminalTurnID    string                  `json:"terminal_turn_id,omitempty"`
	CreatedAt         time.Time               `json:"created_at"`
	UpdatedAt         time.Time               `json:"updated_at"`
}

// AgentDeliveryEvent is the normalized event persisted in the backend inbox
// before agentctl receives an acknowledgment.
type AgentDeliveryEvent struct {
	SessionID          string     `json:"session_id"`
	IncarnationID      string     `json:"incarnation_id"`
	HarnessGeneration  int64      `json:"harness_generation"`
	TaskID             string     `json:"-"`
	ExecutionID        string     `json:"-"`
	OwnerGeneration    int64      `json:"-"`
	ExpectedStreamID   string     `json:"-"`
	StreamID           string     `json:"stream_id"`
	Sequence           int64      `json:"sequence"`
	SubmissionID       string     `json:"submission_id,omitempty"`
	EventType          string     `json:"event_type"`
	Payload            []byte     `json:"payload"`
	ContentHash        string     `json:"content_hash,omitempty"`
	HashVersion        int        `json:"hash_version,omitempty"`
	PayloadPrunedAt    *time.Time `json:"payload_pruned_at,omitempty"`
	PayloadBytes       int64      `json:"payload_bytes"`
	CanonicalMessageID string     `json:"-"`
	Terminal           bool       `json:"terminal,omitempty"`
	ReceivedAt         time.Time  `json:"received_at"`
	ProjectedAt        *time.Time `json:"projected_at,omitempty"`
}

// AgentDeliveryCursor stores the two independent backend watermarks. A
// received event is safe to acknowledge after its inbox transaction commits;
// projection may lag and must never move the received cursor backward.
type AgentDeliveryCursor struct {
	SessionID                      string    `json:"session_id"`
	IncarnationID                  string    `json:"incarnation_id"`
	HarnessGeneration              int64     `json:"harness_generation"`
	StreamID                       string    `json:"stream_id"`
	ReceivedSequence               int64     `json:"received_sequence"`
	ProjectedSequence              int64     `json:"projected_sequence"`
	MessageSegmentBoundarySequence *int64    `json:"message_segment_boundary_sequence,omitempty"`
	RemoteHighWater                int64     `json:"remote_high_water"`
	RetiredSequence                int64     `json:"retired_sequence"`
	UpdatedAt                      time.Time `json:"updated_at"`
}

// CanonicalAgentDeliveryProjectionResult identifies the message committed for
// an event and whether projection appended to an existing message.
type CanonicalAgentDeliveryProjectionResult struct {
	CanonicalMessageID string
	Appended           bool
}

// AgentDeliveryEffect is a durable idempotency key for a projected workflow
// or turn intent. Consumers claim it at their authoritative state transition.
type AgentDeliveryEffect struct {
	EffectKey         string     `json:"effect_key"`
	StreamID          string     `json:"stream_id"`
	Sequence          int64      `json:"sequence"`
	EffectType        string     `json:"effect_type"`
	State             string     `json:"state"`
	SessionID         string     `json:"session_id,omitempty"`
	IncarnationID     string     `json:"incarnation_id,omitempty"`
	HarnessGeneration int64      `json:"harness_generation,omitempty"`
	SubmissionID      string     `json:"submission_id,omitempty"`
	TurnID            string     `json:"turn_id,omitempty"`
	Outcome           string     `json:"outcome,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	CompletedAt       *time.Time `json:"completed_at,omitempty"`
	// Workflow policy fields are an in-process compare-and-swap guard. They are
	// checked while the workflow step row is locked and are not persisted.
	TaskID                  string     `json:"-"`
	WorkflowID              string     `json:"-"`
	WorkflowStepID          string     `json:"-"`
	WorkflowPolicyUpdatedAt *time.Time `json:"-"`
}
