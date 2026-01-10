// Package bus provides event bus abstractions for Kandev.
package bus

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Event represents a message on the event bus
type Event struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Source    string                 `json:"source"` // Service that produced the event
	Timestamp time.Time              `json:"timestamp"`
	Data      map[string]interface{} `json:"data"`
}

// NewEvent creates a new event with a UUID and current timestamp
func NewEvent(eventType, source string, data map[string]interface{}) *Event {
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
	// Publish sends an event to a subject
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

