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
// ownership guard always runs against a non-empty value, and inter-task
// dispatch hardcodes QueuedByAgent so user clients can never overwrite an
// agent-authored entry.
const (
	QueuedByUser  = "user"
	QueuedByAgent = "agent"
)

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
}

// MessageAttachment represents an attachment (image) in a queued message.
type MessageAttachment struct {
	Type     string `json:"type"`
	Data     string `json:"data"`
	MimeType string `json:"mime_type"`
}

// QueueStatus is the per-session view returned to clients: full ordered list of
// pending entries plus capacity info.
type QueueStatus struct {
	Entries []QueuedMessage `json:"entries"`
	Count   int             `json:"count"`
	Max     int             `json:"max"`
}

// PendingMove represents a workflow step move requested by an agent (via
// move_task_kandev) while its turn is still active. Applied by handleAgentReady
// once the turn ends.
type PendingMove struct {
	TaskID         string    `json:"task_id"`
	WorkflowID     string    `json:"workflow_id"`
	WorkflowStepID string    `json:"workflow_step_id"`
	Position       int       `json:"position"`
	QueuedAt       time.Time `json:"queued_at"`
}
