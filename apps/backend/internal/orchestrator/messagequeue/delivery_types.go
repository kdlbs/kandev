package messagequeue

import (
	"errors"
	"time"
)

// DeliveryState is the durable lifecycle of a cross-task prompt receipt.
type DeliveryState string

const (
	DeliveryPendingCapacity DeliveryState = "pending_capacity"
	DeliveryQueued          DeliveryState = "queued"
	DeliveryReserved        DeliveryState = "reserved"
	DeliveryDelivered       DeliveryState = "delivered"
	DeliveryRetryWait       DeliveryState = "retry_wait"
	DeliveryRecoverable     DeliveryState = "recoverable"
	DeliveryTerminalFailed  DeliveryState = "terminal_failed"
	DeliveryCancelled       DeliveryState = "cancelled"
)

// ErrDeliveryLeaseLost means another worker owns, released, or reclaimed a
// delivery reservation. Callers must not mutate the receipt after this error.
var ErrDeliveryLeaseLost = errors.New("delivery lease lost")

// Delivery stores one cross-task prompt independently of the source agent
// turn. Content is retained until the state reaches a terminal outcome so an
// exhausted delivery remains recoverable instead of silently disappearing.
type Delivery struct {
	ID              string                 `json:"id"`
	SenderTaskID    string                 `json:"sender_task_id"`
	SenderSessionID string                 `json:"sender_session_id"`
	SourceTurnID    string                 `json:"source_turn_id"`
	IdempotencyKey  string                 `json:"idempotency_key"`
	TargetTaskID    string                 `json:"target_task_id"`
	TargetSessionID string                 `json:"target_session_id"`
	DeliveryMode    string                 `json:"delivery_mode"`
	Content         string                 `json:"content"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
	State           DeliveryState          `json:"state"`
	QueueEntryID    string                 `json:"queue_entry_id,omitempty"`
	Attempts        int                    `json:"attempts"`
	NextAttemptAt   time.Time              `json:"next_attempt_at"`
	LeaseOwner      string                 `json:"-"`
	LeaseExpiresAt  time.Time              `json:"-"`
	LastError       string                 `json:"last_error,omitempty"`
	CreatedAt       time.Time              `json:"created_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	DeliveredAt     time.Time              `json:"delivered_at,omitempty"`
}
