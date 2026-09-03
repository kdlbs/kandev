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
// ownership guard always runs against a non-empty value. Agent, workflow, and
// server identities are reserved for backend dispatch paths.
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
	case QueuedByAgent, QueuedByWorkflow, QueuedByServer:
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

// MetadataLifecycleReserved marks a durable lifecycle entry that ReserveHead
// already handed to a dispatch attempt. The row stays in storage for crash
// recovery, but it is no longer a pending message, so queue status hides it —
// otherwise the delivered prompt and its own reservation row are both visible
// and the reservation looks like a stuck duplicate. Cleared implicitly: a
// requeue rewrites the row's metadata from the in-memory copy, which never
// carries the flag.
const MetadataLifecycleReserved = "lifecycle_reserved_in_flight"

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
	// ErrLifecycleCancelled means an archive/delete purge invalidated a
	// previously accepted lifecycle entry before it could be retried.
	ErrLifecycleCancelled = errors.New("lifecycle queue entry cancelled")
)

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
	// retained this durable row for acknowledgement. It deliberately is not
	// persisted in metadata, where a restart could leak it into a retry.
	reservedLifecycleDelivery bool
}

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
	return origin == "github_pr_automation"
}

// IsReservedInFlight reports whether this durable row was already reserved for
// a dispatch attempt and should not be shown as a pending queue entry.
func (m *QueuedMessage) IsReservedInFlight() bool {
	if m == nil || !m.IsDurableLifecycle() {
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

// markReservedMetadata returns a copy of metadata carrying the in-flight
// reservation marker.
func markReservedMetadata(metadata map[string]interface{}) map[string]interface{} {
	marked := make(map[string]interface{}, len(metadata)+1)
	for k, v := range metadata {
		marked[k] = v
	}
	marked[MetadataLifecycleReserved] = true
	return marked
}

// clearReservedMetadata removes the transient in-process delivery marker from
// copies returned to dispatch or written back for retry.
func clearReservedMetadata(metadata map[string]interface{}) map[string]interface{} {
	cleared := make(map[string]interface{}, len(metadata))
	for k, v := range metadata {
		if k != MetadataLifecycleReserved {
			cleared[k] = v
		}
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

// QueueStatus is the per-session view returned to clients: full ordered list of
// pending entries plus capacity info.
type QueueStatus struct {
	Entries []QueuedMessage `json:"entries"`
	Count   int             `json:"count"`
	Max     int             `json:"max"`
	AutoRun bool            `json:"auto_run"`
	// MergeEnabled mirrors Service.MergeEnabled so clients can hide the
	// "Merge with above" affordance without a separate settings fetch.
	MergeEnabled bool `json:"merge_enabled"`
}

// PendingMove represents a workflow step move requested by an agent (via
// move_task_kandev) while its turn is still active. Applied by handleAgentReady
// once the turn ends.
type PendingMove struct {
	// MoveID identifies one deferred move request across queue snapshots. A
	// rollback can restore a previously consumed snapshot, so the orchestrator
	// needs a durable identity to reject that stale replay.
	MoveID         string    `json:"move_id"`
	TaskID         string    `json:"task_id"`
	WorkflowID     string    `json:"workflow_id"`
	WorkflowStepID string    `json:"workflow_step_id"`
	Position       int       `json:"position"`
	QueuedAt       time.Time `json:"queued_at"`
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
