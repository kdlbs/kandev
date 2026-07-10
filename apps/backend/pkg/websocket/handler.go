package websocket

import (
	"context"
	"sync"
)

// Handler is the interface for WebSocket message handlers
type Handler interface {
	// Handle processes a WebSocket message and returns a response
	Handle(ctx context.Context, msg *Message) (*Message, error)
}

// HandlerFunc is a function type that implements Handler
type HandlerFunc func(ctx context.Context, msg *Message) (*Message, error)

// Handle implements the Handler interface
func (f HandlerFunc) Handle(ctx context.Context, msg *Message) (*Message, error) {
	return f(ctx, msg)
}

// Dispatcher routes messages to appropriate handlers based on action.
//
// Dispatcher must be constructed via NewDispatcher; the zero value has a
// nil handlers map and panics on use. Once constructed it is safe for
// concurrent use: Register/RegisterFunc may be called from any goroutine,
// including while Dispatch or HasHandler are in flight. Typical usage
// registers all handlers at startup, but the API does not require it.
type Dispatcher struct {
	mu       sync.RWMutex
	handlers map[string]Handler
}

// NewDispatcher creates a new message dispatcher
func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		handlers: make(map[string]Handler),
	}
}

// Register registers a handler for an action
func (d *Dispatcher) Register(action string, handler Handler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[action] = handler
}

// RegisterFunc registers a handler function for an action
func (d *Dispatcher) RegisterFunc(action string, handler HandlerFunc) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.handlers[action] = handler
}

// Dispatch routes a message to the appropriate handler.
//
// The handler is looked up under the read lock, then the lock is released
// before invoking Handle: holding it across user-supplied I/O would
// serialise every dispatched message and risk deadlock if a handler
// re-entered Register.
func (d *Dispatcher) Dispatch(ctx context.Context, msg *Message) (*Message, error) {
	d.mu.RLock()
	handler, ok := d.handlers[msg.Action]
	d.mu.RUnlock()
	if !ok {
		return NewError(msg.ID, msg.Action, ErrorCodeUnknownAction,
			"Unknown action: "+msg.Action, nil)
	}
	return handler.Handle(ctx, msg)
}

// HasHandler returns true if a handler is registered for the action
func (d *Dispatcher) HasHandler(action string) bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	_, ok := d.handlers[action]
	return ok
}
