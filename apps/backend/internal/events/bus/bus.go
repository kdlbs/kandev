// Package bus provides event bus abstractions for Kandev.
package bus

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Event represents a message on the event bus
type Event struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	// Subject is the concrete subject this event was published on, stamped
	// by EventBus.Publish. It is NOT always equal to Type: many per-session
	// (and per-run) events are published on a suffixed subject while Type
	// stays the bare event constant — e.g. Type "shell.output" published on
	// subject "shell.output.<sessionId>". A few carry an unrelated type
	// entirely (three git.ws publish sites in the orchestrator stamp
	// "git.event"). Subscribers that need to know which concrete subject
	// matched (the plugin deliverer, which re-checks the subscriber's
	// manifest pattern) must read Subject, not Type. Use EffectiveSubject to
	// read it with a Type fallback for events that predate the stamping (or
	// were constructed by hand in a test).
	//
	// Because Publish stamps this in place, a single *Event must not be
	// published to more than one subject concurrently — see EventBus.Publish.
	Subject   string      `json:"subject,omitempty"`
	Source    string      `json:"source"` // Service that produced the event
	Timestamp time.Time   `json:"timestamp"`
	Data      interface{} `json:"data"`
}

// EffectiveSubject returns the concrete subject e was published on, falling
// back to e.Type when no subject was stamped (an Event handed straight to a
// handler without going through Publish). For every subject that is
// published unsuffixed the two are identical, so the fallback is exact
// there; only per-session/per-run suffixed subjects depend on Subject.
func (e *Event) EffectiveSubject() string {
	if e == nil {
		return ""
	}
	if e.Subject != "" {
		return e.Subject
	}
	return e.Type
}

// NewEvent creates a new event with a UUID and current timestamp.
// The data parameter can be any type that is JSON-serializable (struct, map, etc.).
func NewEvent(eventType, source string, data interface{}) *Event {
	return &Event{
		ID:        uuid.New().String(),
		Type:      eventType,
		Source:    source,
		Timestamp: time.Now().UTC(),
		Data:      data,
	}
}

// EventHandler is a function that handles an event
type EventHandler func(ctx context.Context, event *Event) error

// Subscription represents an active subscription
type Subscription interface {
	Unsubscribe() error
	IsValid() bool
}

// EventBus interface for event bus operations
type EventBus interface {
	// Publish sends an event to a subject.
	//
	// Implementations stamp subject onto event.Subject in place before
	// dispatching, so callers must not share one *Event across concurrent
	// Publish calls on different subjects — build a fresh Event per publish
	// (which every call site does today). Publishing the same pointer to
	// several subjects sequentially is fine: the memory bus dispatches its
	// handlers synchronously inside Publish, so each handler observes the
	// subject it was published on.
	// Implementations contain panics raised by EventHandler. For synchronous
	// delivery, a recovered panic does not stop later subscribers or become a
	// Publish error. NATS callbacks also contain panics on the client goroutine.
	Publish(ctx context.Context, subject string, event *Event) error

	// Subscribe creates a subscription to a subject pattern
	Subscribe(subject string, handler EventHandler) (Subscription, error)

	// QueueSubscribe creates a queue subscription for load balancing
	QueueSubscribe(subject, queue string, handler EventHandler) (Subscription, error)

	// Request sends a request and waits for a response (with timeout)
	Request(ctx context.Context, subject string, event *Event, timeout time.Duration) (*Event, error)

	// Close closes the connection
	Close()

	// IsConnected returns connection status
	IsConnected() bool
}
