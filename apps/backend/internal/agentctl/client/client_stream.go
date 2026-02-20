package client

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agentctl/tracing"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// sendStreamRequest sends a request over the agent WebSocket stream and waits for a response.
// It creates a ws.Message with a UUID, registers a pending response channel,
// writes the message to the stream, and blocks until a response arrives or context is cancelled.
func (c *Client) sendStreamRequest(ctx context.Context, action string, payload interface{}) (*ws.Message, error) {
	c.mu.RLock()
	conn := c.agentStreamConn
	c.mu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("agent stream not connected")
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
	c.pendingRequests[reqID] = respCh
	c.pendingMu.Unlock()

	// Clean up on exit
	defer func() {
		c.pendingMu.Lock()
		delete(c.pendingRequests, reqID)
		c.pendingMu.Unlock()
	}()

	// Serialize write to stream
	data, err := json.Marshal(msg)
	if err != nil {
		tracing.TraceWSResponse(span, "", err)
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	c.streamWriteMu.Lock()
	writeErr := conn.WriteMessage(websocket.TextMessage, data)
	c.streamWriteMu.Unlock()
	if writeErr != nil {
		tracing.TraceWSResponse(span, "", writeErr)
		return nil, fmt.Errorf("failed to write request to stream: %w", writeErr)
	}

	// Wait for response or context cancellation
	select {
	case resp := <-respCh:
		if resp == nil {
			disconnErr := fmt.Errorf("agent stream disconnected while waiting for response")
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
	c.pendingMu.Unlock()

	if !ok {
		return false
	}

	// Send response to waiting caller (non-blocking since channel is buffered)
	select {
	case ch <- msg:
	default:
	}
	return true
}

// cleanupPendingRequests unblocks all pending requests with nil (signaling disconnect).
func (c *Client) cleanupPendingRequests() {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()

	for id, ch := range c.pendingRequests {
		close(ch)
		delete(c.pendingRequests, id)
	}
}
