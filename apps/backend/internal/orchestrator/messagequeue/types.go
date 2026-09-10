package messagequeue

import (
	"errors"
	"time"
)

// DefaultMaxPerSession is the default cap for queued messages per session
// when the env var KANDEV_QUEUE_MAX_PER_SESSION is unset or invalid.
const DefaultMaxPerSession = 10

// Sender identities written to QueuedMessage.QueuedBy. The handlers default
// any empty user-supplied identity to QueuedByUser so the UpdateMessage
// ownership guard always runs against a non-empty value. Agent, workflow,
// server, and move-task identities are reserved for backend dispatch paths.
const (
	QueuedByUser     = "user"
	QueuedByAgent    = "agent"
	QueuedByWorkflow = "workflow"
	QueuedByServer   = "server"
	QueuedByMoveTask = "mcp-move-task"
)

// IsReservedQueuedBy reports identities owned by backend dispatch paths.
// WebSocket/MCP clients may create and mutate only user-owned entries.
//
// MergeIntoAbove is the single controlled exception: a client with session
// access may fold one agent-owned entry into the agent-owned entry above it
// when both carry the same sender_task_id. That preserves the reserved row's
// provenance (the merged entry keeps the target's identity) while consolidating
// additive prompts from one agent into a single delivery. See ADR 0051.
func IsReservedQueuedBy(queuedBy string) bool {
	switch queuedBy {
	case QueuedByAgent, QueuedByWorkflow, QueuedByServer, QueuedByMoveTask:
		return true
	default:
		return false
	}
}

// MetadataCoalesceKey identifies queued entries that should be replaced rather
// than appended when a newer pending message supersedes an older one.
const MetadataCoalesceKey = "coalesce_key"

// MetadataEntityReferences carries persisted entity-reference context for a
// queued message.
const MetadataEntityReferences = "entity_references"

// MetadataStepHandoff carries a completion-handoff carry token's claimed text
// for a queued workflow auto-start prompt, so a dispatch path that defers
// delivery through the queue (rather than sending it directly) still appends
// the handoff at actual dispatch time, after entity-reference context, rather
// than losing it at claim time or appending it out of order.
const MetadataStepHandoff = "step_handoff"

// MetadataContextFiles carries path/name references and optional directory
// identity for queued user messages.
const MetadataContextFiles = "context_files"

// MetadataLifecycleDurable marks lifecycle entries that remain in persistent
// queue storage until the executor accepts their prompt.
const MetadataLifecycleDurable = "lifecycle_durable_until_accepted"

// MetadataLifecycleGeneration ties a durable lifecycle entry to the task
// archive generation that accepted it. A task purge advances the generation,
// so stale retries cannot revive work after an archive then unarchive.
const MetadataLifecycleGeneration = "lifecycle_queue_generation"

// MetadataLifecycleReserved marks a retained queue entry already handed to a
// dispatch attempt. The historical key name is preserved for durable rows
// written by older builds. Reserved rows stay in storage for crash recovery
// but are hidden from pending queue status until acknowledged or released.
const MetadataLifecycleReserved = "lifecycle_reserved_in_flight"

const metadataLifecycleReservationID = "lifecycle_reservation_id"

// MetadataLifecycleReservationIncarnation binds a retained row to the
// immutable session incarnation that reserved it. A replacement incarnation
// may discard that stale reservation but must never deliver it. The historical
// key name is retained for storage compatibility.
const MetadataLifecycleReservationIncarnation = "lifecycle_reservation_incarnation_id"

// MetadataSenderTaskID identifies the task that produced an agent message. Two
// agent entries may only merge when their sender task ids match, so the merge
// never mixes prompts issued by different agents.
const MetadataSenderTaskID = "sender_task_id"

// MetadataDeferredMoveID identifies the hand-off prompt created for one
// deferred workflow move. The orchestrator uses it to remove only stale move
// prompts after a replay.
const MetadataDeferredMoveID = "deferred_move_id"

// QueueFullErrorCode is the well-known WS / MCP error code surfaced when an
// insert would exceed the per-session cap. Shared between the user-side WS
// handlers and the inter-task MCP handler so the wire contract stays in sync.
const QueueFullErrorCode = "queue_full"

// Errors returned by the queue service / repository.
var (
	// ErrQueueFull is returned when an insert would exceed the per-session cap.
	ErrQueueFull = errors.New("queue full")
	// ErrEntryNotFound is returned when an operation targets an entry that no
	// longer exists (e.g. it was drained between fetch and update).
	ErrEntryNotFound = errors.New("queue entry not found")
	// ErrNoMergeTarget is returned when a merge source exists but has no valid
	// entry above it: the source is the head, the sender kinds differ, agent
	// sender tasks differ, the caller does not own the rows, or the target is a
	// reserved in-flight lifecycle entry.
	ErrNoMergeTarget = errors.New("no mergeable message above")
	// ErrMergeDisabled is returned when a merge is attempted while queued
	// message merging is disabled (see Service.SetMergeEnabled). The setting
	// is admin-controlled and enabled by default.
	ErrMergeDisabled = errors.New("queued message merging is disabled")
	// ErrQueueChanged is returned when a reorder's submitted id set does not
	// match the session's current visible pending entries — an entry was
	// drained, removed, merged, or newly queued since the client's snapshot.
	// The reorder is rejected atomically with no partial position rewrite.
	ErrQueueChanged = errors.New("queue changed during reorder")
	// ErrTaskInactive means a lifecycle prompt could not be accepted because
	// its task was deleted or archived before the queue transaction claimed it.
	ErrTaskInactive = errors.New("queue task is inactive")
	// ErrSessionIdentityMismatch means the supplied immutable session identity
	// no longer names the authoritative task-session row.
	ErrSessionIdentityMismatch = errors.New("queue session identity mismatch")
	// ErrLifecycleCancelled means an archive/delete purge invalidated a
	// previously accepted lifecycle entry before it could be retried.
	ErrLifecycleCancelled = errors.New("lifecycle queue entry cancelled")
	// ErrQueueDispatchClaimChanged means the durable ordinary-dispatch claim
	// was cleared or transferred before its worker attempted to settle it.
	ErrQueueDispatchClaimChanged = errors.New("queue dispatch claim changed")
	// ErrLifecycleReservationChanged means a newer lifecycle delivery attempt
	// replaced the reservation being acknowledged.
	ErrLifecycleReservationChanged = errors.New("lifecycle reservation changed")
	// ErrSessionTransferInProgress prevents a queue mutation from entering a
	// session while its rows and external attachment claims are being rebound.
	ErrSessionTransferInProgress = errors.New("session transfer in progress")
	// ErrSessionTransferOwnershipLost prevents an older transfer attempt from
	// deleting or replacing a newer durable transfer fence.
	ErrSessionTransferOwnershipLost = errors.New("session transfer ownership lost")
	// ErrEditConflict means a queue entry is currently held by another editor.
	ErrEditConflict = errors.New("queue entry edit conflict")
	// ErrEditLeaseNotFound means a lease is missing, expired, or owned by
	// another WebSocket connection.
	ErrEditLeaseNotFound = errors.New("queue edit lease not found")
	// ErrEditRevisionConflict means the target changed since edit.begin.
	ErrEditRevisionConflict = errors.New("queue edit target revision conflict")
	// ErrAutoMergePolicyChanged means admission's immutable policy snapshot no
	// longer matches the durable session override. The caller must re-resolve
	// policy before deciding whether to fold.
	ErrAutoMergePolicyChanged = errors.New("queue Auto-merge policy changed")
)

const QueueEditLeaseTTL = 60 * time.Second

// QueueEditLease is the server-issued, connection-bound hold for one entry.
type QueueEditLease struct {
	SessionID       string    `json:"session_id"`
	EntryID         string    `json:"entry_id"`
	LeaseID         string    `json:"lease_id"`
	TargetRevision  int64     `json:"target_revision"`
	LeaseGeneration int64     `json:"lease_generation,omitempty"`
	ExpiresAt       time.Time `json:"expires_at,omitempty"`
	// connectionID and operation fields are server-side fencing state.
	connectionID           string
	taskID                 string
	lastOperationID        string
	lastOperationHash      string
	lastOperationResult    int64
	lastOperationPrevious  *QueuedMessage
	lastOperationFinalized bool
}

// QueueSessionIdentity is the immutable authority for session-scoped queue work.
type QueueSessionIdentity struct {
	TaskID               string `json:"task_id"`
	SessionID            string `json:"session_id"`
	SessionIncarnationID string `json:"session_incarnation_id"`
}

// QueueAttachmentClaim carries authenticated staged-attachment ownership into
// the queue repository transaction.
type QueueAttachmentClaim struct {
	OwnerID     string
	WorkspaceID string
	IDs         []string
}
type AutoMergeSource string

const (
	AutoMergeSourceGlobal  AutoMergeSource = "global"
	AutoMergeSourceSession AutoMergeSource = "session"
)

// AutoMergePolicy is one immutable effective policy snapshot.
type AutoMergePolicy struct {
	Enabled  bool
	Source   AutoMergeSource
	Revision int64
}

// AutoMergeOverride is the explicit per-session value and revision.
type AutoMergeOverride struct {
	Enabled  bool
	Revision int64
}

// QueuedMessage represents a single FIFO entry queued for a session.
type QueuedMessage struct {
	ID          string                 `json:"id"`
	SessionID   string                 `json:"session_id"`
	TaskID      string                 `json:"task_id"`
	Position    int64                  `json:"position"` // FIFO order (lower = head)
	Content     string                 `json:"content"`
	Model       string                 `json:"model"`
	PlanMode    bool                   `json:"plan_mode"`
	Attachments []MessageAttachment    `json:"attachments"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	QueuedAt    time.Time              `json:"queued_at"`
	QueuedBy    string                 `json:"queued_by"`

	// reservedLifecycleDelivery is process-local evidence that ReserveHead
	// retained this durable row for acknowledgement.
	reservedLifecycleDelivery bool

	// dispatchAttemptID and lifecycleReservationID identify the exact durable
	// delivery attempts this process may settle. They are intentionally absent
	// from the public JSON wire shape.
	dispatchAttemptID      string
	lifecycleReservationID string

	// reservationSessionGeneration and reservationLifecycleGeneration fence a
	// FIFO dispatch source to the generations observed while it was reserved.
	// They are process-local and only used if reservationGenerationsCaptured is
	// true.
	reservationSessionGeneration   int64
	reservationLifecycleGeneration int64
	reservationGenerationsCaptured bool
	reservationIdentity            QueueSessionIdentity
}

// QueueRemovalResult is the atomic outcome of a user-driven queue deletion.
// Retained includes reserved in-flight rows so attachment cleanup cannot remove
// a descriptor still owned by surviving work.
type QueueRemovalResult struct {
	Removed  []QueuedMessage
	Retained []QueuedMessage
}

// ReservationGenerations returns the session and lifecycle generations
// captured when this entry was reserved for FIFO dispatch.
func (m *QueuedMessage) ReservationGenerations() (int64, int64, bool) {
	if m == nil {
		return 0, 0, false
	}
	return m.reservationSessionGeneration, m.reservationLifecycleGeneration, m.reservationGenerationsCaptured
}

const (
	lifecycleOriginGitHubPR = "github_pr_automation"
	lifecycleOriginGitLabMR = "gitlab_mr_automation"
)

// IsDurableLifecycle reports whether this entry uses reserve/ack delivery.
// The origin fallback keeps lifecycle rows queued by older builds safe across
// a rolling restart before the explicit marker was introduced.
func (m *QueuedMessage) IsDurableLifecycle() bool {
	if m == nil {
		return false
	}
	if durable, _ := m.Metadata[MetadataLifecycleDurable].(bool); durable {
		return true
	}
	origin, _ := m.Metadata["origin"].(string)
	return origin == lifecycleOriginGitHubPR || origin == "ci_automation" || origin == lifecycleOriginGitLabMR
}

// IsReservedInFlight reports whether this row was retained for an in-flight
// dispatch and should not be shown as a pending queue entry.
func (m *QueuedMessage) IsReservedInFlight() bool {
	if m == nil {
		return false
	}
	reserved, _ := m.Metadata[MetadataLifecycleReserved].(bool)
	return reserved
}

// IsReservedLifecycleDelivery reports whether this copy came from the
// reserve/ack path rather than a destructive legacy TakeHead call.
func (m *QueuedMessage) IsReservedLifecycleDelivery() bool {
	return m != nil && m.reservedLifecycleDelivery
}

// markReservedMetadata returns a copy carrying the exact lifecycle delivery
// attempt that may acknowledge the durable row.
func markReservedMetadata(metadata map[string]interface{}, reservationIDs ...string) map[string]interface{} {
	marked := copyMessageMetadata(metadata, 3)
	marked[MetadataLifecycleReserved] = true
	if len(reservationIDs) > 0 && reservationIDs[0] != "" {
		marked[metadataLifecycleReservationID] = reservationIDs[0]
	}
	return marked
}

func markReservedMetadataForIncarnation(metadata map[string]interface{}, incarnationID string) map[string]interface{} {
	marked := markReservedMetadata(metadata)
	if incarnationID != "" {
		marked[MetadataLifecycleReservationIncarnation] = incarnationID
	}
	return marked
}

func lifecycleReservationIncarnation(metadata map[string]interface{}) string {
	incarnationID, _ := metadata[MetadataLifecycleReservationIncarnation].(string)
	return incarnationID
}

// clearReservedMetadata removes transient delivery ownership from copies
// returned to dispatch or written back for retry.
func clearReservedMetadata(metadata map[string]interface{}) map[string]interface{} {
	cleared := copyMessageMetadata(metadata, 0)
	if cleared == nil {
		cleared = make(map[string]interface{})
	}
	for k := range cleared {
		if k != MetadataLifecycleReserved && k != metadataLifecycleReservationID {
			if k == MetadataLifecycleReservationIncarnation {
				delete(cleared, k)
				continue
			}
			continue
		}
		delete(cleared, k)
	}
	return cleared
}

// MessageAttachment represents an attachment (image) in a queued message.
type MessageAttachment struct {
	Type         string `json:"type"`
	Data         string `json:"data"`
	AttachmentID string `json:"attachment_id,omitempty"`
	MimeType     string `json:"mime_type"`
	Name         string `json:"name,omitempty"`
	SizeBytes    int64  `json:"size_bytes,omitempty"`
	DeliveryMode string `json:"delivery_mode,omitempty"`
}

// AttachmentCleanup records a durable obligation to release attachment claims
// after a queued message no longer references them.
type AttachmentCleanup struct {
	SessionID        string
	CurrentSessionID string
	EntryID          string
	OperationID      string
	TaskID           string
	OwnerID          string
	LeaseID          string
	RemoveEntry      bool
	Attachments      []MessageAttachment
	EntryFingerprint string
	ClaimPending     bool
	CreatedAt        time.Time
}

type AttachmentCleanupLocator struct {
	SessionID   string `json:"session_id"`
	EntryID     string `json:"entry_id"`
	OperationID string `json:"operation_id"`
}

// SessionTransferCompensation records an attachment binding that must be
// reconciled with the durable queue location after an interrupted transfer.
type SessionTransferCompensation struct {
	OperationID     string
	TaskID          string
	FromSessionID   string
	ToSessionID     string
	EntryIDs        []string
	AttachmentIDs   []string
	CleanupLocators []AttachmentCleanupLocator
	CreatedAt       time.Time
}

// QueueStatus is the per-session view returned to clients: full ordered list of
// pending entries plus capacity info.
type QueueStatus struct {
	Entries              []QueuedMessage `json:"entries"`
	Count                int             `json:"count"`
	Max                  int             `json:"max"`
	TaskID               string          `json:"task_id,omitempty"`
	SessionID            string          `json:"session_id,omitempty"`
	SessionIncarnationID string          `json:"session_incarnation_id,omitempty"`
	StatusEpoch          string          `json:"status_epoch,omitempty"`
	StatusGeneration     int64           `json:"status_generation,omitempty"`
	AutoRun              bool            `json:"auto_run"`
	MergeEnabled         bool            `json:"merge_enabled"`
	AutoMergeAvailable   bool            `json:"auto_merge_available"`
	AutoMergeEnabled     *bool           `json:"auto_merge_enabled,omitempty"`
	AutoMergeSource      AutoMergeSource `json:"auto_merge_source,omitempty"`
	AutoMergeRevision    *int64          `json:"auto_merge_revision,omitempty"`
}

// PendingMove represents a workflow step move requested by an agent (via
// move_task_kandev) while its turn is still active. Applied by handleAgentReady
// once the turn ends.
type PendingMove struct {
	// MoveID is the durable effect token for one deferred move request across
	// queue snapshots. Rollback can restore a consumed snapshot, so replay uses
	// this token to suppress a second workflow effect.
	MoveID               string    `json:"move_id"`
	SessionIncarnationID string    `json:"session_incarnation_id,omitempty"`
	TaskID               string    `json:"task_id"`
	WorkflowID           string    `json:"workflow_id"`
	WorkflowStepID       string    `json:"workflow_step_id"`
	Position             int       `json:"position"`
	QueuedAt             time.Time `json:"queued_at"`
	// Actor records provenance across the deferred move boundary. Agent is the
	// value used by move_task_kandev; it prevents owner identity leakage.
	Actor string `json:"actor,omitempty"`
	// SenderSessionID identifies the session that requested the move. It is
	// distinct from the session owning this queue, which is only the execution
	// context used to apply the deferred move.
	SenderSessionID string `json:"sender_session_id,omitempty"`
}

// PendingMoveTTL bounds how long a deferred move may stay armed before it is
// treated as stale and dropped instead of applied.
//
// A pending move only exists to bridge two moments: the agent called
// move_task_kandev while its turn was still running, and that same turn ended.
// In a healthy system those are seconds to minutes apart. A row that outlives
// this window did not survive a slow turn — its turn never ended cleanly (crash,
// restart, parked session), and the board state it was authored against is gone.
// Replaying it then relocates a card against a board that has moved on.
//
// 24h is deliberately orders of magnitude above the legitimate window, so no
// healthy move is ever caught by it, while being far below the multi-day replay
// that motivated the TTL.
//
// It is a code constant rather than an env var or runtime flag, matching the
// precedent set by the idle-session reaper's thresholds: the orchestrator has no
// other runtime configuration of this shape, and the value is bounded by a
// fail-closed invariant rather than by deployment shape.
const PendingMoveTTL = 24 * time.Hour

// IsStaleAt reports whether the move has been armed longer than ttl.
//
// A zero or future QueuedAt is never stale. An unset timestamp column or a
// clock skew must not be able to mass-expire the table; the same rejection the
// idle-session reaper applies to zero/future UpdatedAt.
func (m *PendingMove) IsStaleAt(now time.Time, ttl time.Duration) bool {
	if m == nil || ttl <= 0 {
		return false
	}
	if m.QueuedAt.IsZero() || m.QueuedAt.After(now) {
		return false
	}
	return now.Sub(m.QueuedAt) > ttl
}

// PendingMoveRecord pairs a deferred move with the session it is keyed to.
// Used by the sweep, which works across sessions rather than looking one up.
type PendingMoveRecord struct {
	SessionID string
	Move      PendingMove
}
