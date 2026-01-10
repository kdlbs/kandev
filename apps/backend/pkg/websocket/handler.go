package websocket

import "context"

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

// Dispatcher routes messages to appropriate handlers based on action
type Dispatcher struct {
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
	d.handlers[action] = handler
}

// RegisterFunc registers a handler function for an action
func (d *Dispatcher) RegisterFunc(action string, handler HandlerFunc) {
	d.handlers[action] = handler
}

// Dispatch routes a message to the appropriate handler
func (d *Dispatcher) Dispatch(ctx context.Context, msg *Message) (*Message, error) {
	handler, ok := d.handlers[msg.Action]
	if !ok {
		return NewError(msg.ID, msg.Action, ErrorCodeUnknownAction,
			"Unknown action: "+msg.Action, nil)
	}
	return handler.Handle(ctx, msg)
}

// HasHandler returns true if a handler is registered for the action
func (d *Dispatcher) HasHandler(action string) bool {
	_, ok := d.handlers[action]
	return ok
}

