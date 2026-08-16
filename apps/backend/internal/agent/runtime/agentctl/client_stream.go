package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/tracing"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// ErrAgentStreamNotConnected identifies requests that cannot be delivered
// because the agent updates stream has already disconnected. Callers use this
// to distinguish an already-stopped agent from a live transport failure.
var ErrAgentStreamNotConnected = errors.New("agent stream not connected")

// sendStreamRequest sends a request over the agent WebSocket stream and waits for a response.
// It creates a ws.Message with a UUID, registers a pending response channel,
// writes the message to the stream, and blocks until a response arrives or context is cancelled.
func (c *Client) sendStreamRequest(ctx context.Context, action string, payload interface{}) (*ws.Message, error) {
	c.mu.RLock()
	conn := c.agentStreamConn
	c.mu.RUnlock()

	if conn == nil {
		return nil, ErrAgentStreamNotConnected
	}

	reqID := uuid.New().String()

	// Start tracing span for the request/response round-trip
	ctx, span := tracing.TraceWSRequest(ctx, action, reqID, c.executionID, c.sessionID)
	defer span.End()

	msg, err := ws.NewRequest(reqID, action, payload)
	if err != nil {
		tracing.TraceWSResponse(span, "", err)
		return nil, fmt.Errorf("failed to create request message: %w", err)
	}

	// Inject trace context (traceparent) into WS message metadata for cross-process propagation
	otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier(msg.EnsureMetadata()))

	// Register pending request
	respCh := make(chan *ws.Message, 1)
	c.pendingMu.Lock()
	if c.pendingRequests == nil {
		c.pendingRequests = make(map[string]chan *ws.Message)
	}
	if c.pendingRequestConns == nil {
		c.pendingRequestConns = make(map[string]*websocket.Conn)
	}
	c.pendingRequests[reqID] = respCh
	c.pendingRequestConns[reqID] = conn
	c.pendingMu.Unlock()

	// Clean up on exit
	defer func() {
		c.pendingMu.Lock()
		delete(c.pendingRequests, reqID)
		delete(c.pendingRequestConns, reqID)
		c.pendingMu.Unlock()
	}()

	// Serialize write to stream
	data, err := json.Marshal(msg)
	if err != nil {
		tracing.TraceWSResponse(span, "", err)
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	c.streamWriteMu.Lock()
	// Honor a caller deadline for the write itself, not just the response wait:
	// a stalled conn.WriteMessage (full send buffer to a half-open peer) would
	// otherwise block uninterruptibly and, for a steer, pin the lifecycle lock the
	// caller holds across this RPC. streamWriteMu serializes writers, so setting
	// and clearing the shared deadline here cannot race a concurrent write.
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetWriteDeadline(deadline)
	}
	writeErr := conn.WriteMessage(websocket.TextMessage, data)
	_ = conn.SetWriteDeadline(time.Time{}) // clear so later writers are unbounded
	c.streamWriteMu.Unlock()
	if writeErr != nil {
		tracing.TraceWSResponse(span, "", writeErr)
		return nil, fmt.Errorf("failed to write request to stream: %w", writeErr)
	}

	// Wait for response or context cancellation
	select {
	case resp := <-respCh:
		if resp == nil {
			disconnErr := fmt.Errorf("%w while waiting for response", ErrAgentStreamNotConnected)
			tracing.TraceWSResponse(span, "", disconnErr)
			return nil, disconnErr
		}
		tracing.TraceWSResponse(span, string(resp.Type), nil)
		return resp, nil
	case <-ctx.Done():
		tracing.TraceWSResponse(span, "", ctx.Err())
		return nil, ctx.Err()
	}
}

// resolvePendingRequest matches a response message to a pending request by ID.
// Returns true if the message was matched to a pending request.
func (c *Client) resolvePendingRequest(msg *ws.Message) bool {
	if msg.ID == "" {
		return false
	}

	c.pendingMu.Lock()
	ch, ok := c.pendingRequests[msg.ID]
	if ok {
		// Resolve while holding pendingMu so a stream cleanup cannot close the
		// channel between the lookup and this send.
		select {
		case ch <- msg:
		default:
		}
	}
	c.pendingMu.Unlock()

	if !ok {
		return false
	}
	return true
}

// cleanupPendingRequests unblocks pending requests owned by conn with nil
// (signaling disconnect). A nil connection retains the all-stream cleanup
// behavior used by tests and shutdown paths.
func (c *Client) cleanupPendingRequests(conn *websocket.Conn) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()

	for id, ch := range c.pendingRequests {
		if conn != nil && c.pendingRequestConns[id] != conn {
			continue
		}
		close(ch)
		delete(c.pendingRequests, id)
		delete(c.pendingRequestConns, id)
	}
}
