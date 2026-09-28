// Package clarification provides types and services for agent clarification requests.
// This allows agents to ask structured questions to users and wait for responses.
package clarification

import (
	"sync"
	"time"

	"github.com/kandev/kandev/internal/clarification/protocol"
)

// Option is the wire type used by clarification requests.
type Option = protocol.Option

// Question is the wire type used by clarification requests.
type Question = protocol.Question

// Request represents a clarification request from an agent. A request bundles
// one or more questions; the agent stays blocked until every question has been
// answered (or the bundle is rejected as a whole).
type Request struct {
	PendingID string     `json:"pending_id"`
	SessionID string     `json:"session_id"`
	TaskID    string     `json:"task_id"`
	Questions []Question `json:"questions"`         // 1-N questions, all required
	Context   string     `json:"context,omitempty"` // Optional shared context for all questions
	CreatedAt time.Time  `json:"created_at"`
}

// Answer is the wire type used by clarification responses.
type Answer = protocol.Answer

// Response is the wire type used by clarification responses.
type Response = protocol.Response

// PendingClarification represents a clarification request waiting for a response.
type PendingClarification struct {
	Request   *Request
	done      chan struct{} // Closed when a response is submitted (broadcast to all waiters)
	resp      *Response
	mu        sync.Mutex
	resolved  bool
	cancelled bool
	// mu guards the deliveryConfirmation* and deliveryAbandoned fields except
	// deliveryConfirmationOnce. deliveryConfirmationDone exists only when a
	// callback was supplied; started and complete distinguish an in-flight
	// callback from its result.
	deliveryConfirmation         func() error
	deliveryConfirmationOnce     sync.Once
	deliveryConfirmationErr      error
	deliveryConfirmationDone     chan struct{}
	deliveryConfirmationStarted  bool
	deliveryConfirmationComplete bool
	deliveryAbandoned            bool
	CancelCh                     chan struct{} // Closed when session's turn completes (agent moved on)
	CreatedAt                    time.Time
}

// Status represents the status of a clarification request.
type Status string

const (
	StatusPending   Status = "pending"
	StatusAnswered  Status = "answered"
	StatusRejected  Status = "rejected"
	StatusExpired   Status = "expired"
	StatusCancelled Status = "cancelled"
)
