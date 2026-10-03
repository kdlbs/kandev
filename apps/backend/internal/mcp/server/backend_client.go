package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// ErrEmptyBackendPayload identifies a response with no payload bytes when a result sink was provided.
var ErrEmptyBackendPayload = errors.New("mcp backend response payload was empty")

// ErrOfflineBudgetExhausted is returned to every Kandev call waiting on a
// detached episode whose offline budget expired (system design part 2 "Send
// loop", "Agent guidance"). Its text is agent-facing guidance, not just a
// log message: it tells the agent to stop rather than poll or retry.
var ErrOfflineBudgetExhausted = errors.New("Kandev is unreachable and the offline budget was reached. Stop now and end your turn. Do not poll, sleep, or retry this call.") //nolint:staticcheck // agent-facing guidance text (system design part 2 "Agent guidance"), not a log message

// ErrKandevCallOutcomeUnknown is returned for a call whose stream ended
// after it was bound (sent) but before an answer arrived: the backend may
// already have applied it (system design part 2 "Sent and not sent", "Agent
// guidance").
var ErrKandevCallOutcomeUnknown = errors.New("The connection to Kandev dropped before this call returned. Its outcome is unknown. Check the current state before retrying.") //nolint:staticcheck // agent-facing guidance text (system design part 2 "Agent guidance"), not a log message

// errRequestNotSent is the internal sentinel a bind failure completes a call
// with (system design part 2 "Sent and not sent" step 3): the call was
// never written, so RequestPayload retries from the top of its send loop
// instead of surfacing anything to the agent.
var errRequestNotSent = errors.New("mcp: request not sent")

// requestSendTimeout bounds one attempt to hand a request to the stream
// writer while attached (system design part 2 "Send loop" step 2). A
// package-level var, like askQuestionKeepAliveInterval, so tests can shrink
// it instead of waiting out the real 5s.
var requestSendTimeout = 5 * time.Second

// AttachmentSnapshot is what RequestPayload's send loop reads to decide
// whether to send now, wait for a reattach, or give up on offline-budget
// expiry (system design part 2 "Kandev tool calls while detached"). Defined
// locally rather than imported from agentctl's process package: this
// package is also linked into the backend binary for its own MCP server
// (see NewExternalDispatcherBackendClient), which must not depend on
// agentctl-only internals.
type AttachmentSnapshot struct {
	// Attached reports whether the current agentctl stream is confirmed.
	Attached bool
	// Episode identifies the detached period AttachedCh/BudgetExhausted
	// belong to.
	Episode uint64
	// AttachedCh is closed when this episode ends by a confirmation.
	AttachedCh <-chan struct{}
	// BudgetExhausted is closed when this episode's offline budget expires.
	BudgetExhausted <-chan struct{}
}

// MCPRequest represents an MCP request to be sent to the backend.
type MCPRequest struct {
	ID      string          `json:"id"`
	Action  string          `json:"action"`
	Payload json.RawMessage `json:"payload"`
}

// MCPResponse represents an MCP response from the backend.
type MCPResponse struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"` // "response" or "error"
	Payload json.RawMessage `json:"payload"`
}

type backendResponse struct {
	message   *ws.Message
	err       error
	sessionID string
}

type pendingRequest struct {
	result    chan backendResponse
	streamID  string
	sessionID string
}

// ChannelBackendClient implements BackendClient using channels.
// It sends MCP requests through a channel that will be read by the agent stream handler,
// and receives responses through a callback mechanism.
type ChannelBackendClient struct {
	requestCh          chan *ws.Message
	pending            map[string]*pendingRequest
	failedStreams      map[string]struct{}
	pendingMu          sync.Mutex
	sessionID          string
	done               chan struct{}
	closeOnce          sync.Once
	closeMu            sync.Mutex
	closed             bool
	publishWG          sync.WaitGroup
	logger             *logger.Logger
	attachmentSnapshot func() AttachmentSnapshot
}

// NewChannelBackendClient creates a new channel-based backend client.
func NewChannelBackendClient(log *logger.Logger) *ChannelBackendClient {
	clientLogger := logger.Default()
	if log != nil {
		clientLogger = log
	}
	clientLogger = clientLogger.WithFields(zap.String("component", "mcp-backend-client"))
	return &ChannelBackendClient{
		requestCh:     make(chan *ws.Message),
		pending:       make(map[string]*pendingRequest),
		failedStreams: make(map[string]struct{}),
		done:          make(chan struct{}),
		logger:        clientLogger,
	}
}

// SetAttachmentSnapshotter wires the agentctl-side attachment state that
// RequestPayload's send loop waits on while detached (system design part 2
// "Send loop"). Unset, every call behaves as always attached, matching the
// client's original single-attempt-then-timeout behavior.
func (c *ChannelBackendClient) SetAttachmentSnapshotter(fn func() AttachmentSnapshot) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	c.attachmentSnapshot = fn
}

func (c *ChannelBackendClient) snapshot() AttachmentSnapshot {
	c.pendingMu.Lock()
	fn := c.attachmentSnapshot
	c.pendingMu.Unlock()
	if fn == nil {
		return AttachmentSnapshot{Attached: true}
	}
	return fn()
}

// GetRequestChannel returns the channel for outgoing MCP requests.
// The agent stream handler should read from this channel and forward to the backend.
func (c *ChannelBackendClient) GetRequestChannel() <-chan *ws.Message {
	return c.requestCh
}

// SetSessionID sets the server-owned session correlation used in bridge logs.
func (c *ChannelBackendClient) SetSessionID(sessionID string) {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	c.sessionID = sessionID
}

// HandleResponse handles an incoming MCP response from the backend.
// This should be called by the agent stream handler when it receives a response.
func (c *ChannelBackendClient) HandleResponse(msg *ws.Message) {
	if c.completeRequest(msg.ID, backendResponse{message: msg}) {
		return
	}
	c.logger.Debug("dropping MCP response with no pending request",
		zap.String("request_id", msg.ID),
		zap.String("type", string(msg.Type)),
		zap.String("action", msg.Action))
}

// BindRequestToStream records which backend stream delivered a request.
// Binding to a stream FailStreamRequests has already marked failed returns
// an error and binds nothing (system design part 2 "Sent and not sent" step
// 2): the write raced a disconnect, so the caller must treat the request as
// not sent and retry rather than leave it bound to a dead stream.
func (c *ChannelBackendClient) BindRequestToStream(requestID, streamID string) error {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	if _, failed := c.failedStreams[streamID]; failed {
		return fmt.Errorf("mcp: stream %s already failed", streamID)
	}
	if pending, ok := c.pending[requestID]; ok {
		pending.streamID = streamID
	}
	return nil
}

// FailRequest releases one pending request after a transport failure.
func (c *ChannelBackendClient) FailRequest(requestID string, err error) {
	c.completeRequest(requestID, backendResponse{err: err})
}

// FailRequestNotSent releases one pending request that was never written to
// the agent stream (system design part 2 "Sent and not sent" step 2, a bind
// failure). RequestPayload's send loop retries on this outcome instead of
// surfacing it, so callers outside this package signal it through this
// method rather than needing the unexported sentinel directly.
func (c *ChannelBackendClient) FailRequestNotSent(requestID string) {
	c.completeRequest(requestID, backendResponse{err: errRequestNotSent})
}

// FailStreamRequests releases requests delivered by one disconnected stream
// and marks the stream failed so a late-arriving bind for it (system design
// part 2 "Sent and not sent" step 2) is rejected instead of silently
// succeeding.
func (c *ChannelBackendClient) FailStreamRequests(streamID string, err error) {
	c.pendingMu.Lock()
	c.failedStreams[streamID] = struct{}{}
	failed := make([]*pendingRequest, 0)
	for id, pending := range c.pending {
		if pending.streamID == streamID {
			delete(c.pending, id)
			failed = append(failed, pending)
		}
	}
	c.pendingMu.Unlock()

	for _, pending := range failed {
		pending.result <- backendResponse{err: err, sessionID: pending.sessionID}
	}
}

func (c *ChannelBackendClient) completeRequest(requestID string, response backendResponse) bool {
	c.pendingMu.Lock()
	pending, ok := c.pending[requestID]
	if ok {
		delete(c.pending, requestID)
	}
	c.pendingMu.Unlock()
	if ok {
		if response.sessionID == "" {
			response.sessionID = pending.sessionID
		}
		pending.result <- response
	}
	return ok
}

// RequestPayload sends a request to the backend and unmarshals the response.
// The request will be cancelled if the context is cancelled or if Reset() is called.
//
// While agentctl is detached from the backend, this blocks on the send loop
// from system design part 2 "Kandev tool calls while detached": wait for
// reattachment or offline-budget exhaustion, then attempt to send once
// attached. A bind failure ("Sent and not sent" step 2, surfaced here as
// errRequestNotSent) means the write never happened, so the loop retries
// from the top instead of surfacing anything to the caller.
func (c *ChannelBackendClient) RequestPayload(ctx context.Context, action string, payload, result interface{}) error {
	if !c.beginPublish() {
		return fmt.Errorf("MCP backend client is closed")
	}
	publishing := true
	defer func() {
		if publishing {
			c.publishWG.Done()
		}
	}()

	id := uuid.New().String()
	start := time.Now()

	msg, err := ws.NewRequest(id, action, payload)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.pendingMu.Lock()
	sessionID := c.sessionID
	c.pendingMu.Unlock()

	c.logger.Debug("sending MCP request through agent stream",
		zap.String("request_id", id),
		zap.String("action", action),
		zap.String("session_id", sessionID),
		zap.Any("payload", backendPayloadForLog(action, payload)))

	// Ensure cleanup on exit
	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
	}()

	respChan := make(chan backendResponse, 1)
	for {
		// A response queued while the call was parked (session reset) is
		// terminal: re-registering and sending would execute a call the agent
		// was told was cancelled.
		select {
		case parked := <-respChan:
			if !errors.Is(parked.err, errRequestNotSent) {
				if parked.err == nil {
					return fmt.Errorf("MCP request cancelled: unexpected parked response")
				}
				return parked.err
			}
		default:
		}
		if err := ctx.Err(); err != nil {
			return err
		}

		c.pendingMu.Lock()
		c.pending[id] = &pendingRequest{result: respChan, sessionID: sessionID}
		c.pendingMu.Unlock()

		snap := c.snapshot()
		if !snap.Attached {
			if err := c.awaitReattach(ctx, snap); err != nil {
				return err
			}
			continue
		}

		sent, err := c.attemptSend(ctx, msg, id, action, sessionID, start)
		if err != nil {
			return err
		}
		if !sent {
			continue
		}
		if publishing {
			publishing = false
			c.publishWG.Done()
		}

		response, retry, err := c.awaitResponse(ctx, id, action, start, respChan)
		if err != nil {
			return err
		}
		if retry {
			continue
		}
		return c.finishResponse(id, action, start, response, result)
	}
}

// awaitReattach blocks while detached (system design part 2 "Send loop" step
// 3): a closed AttachedCh means the caller should retry the send loop from
// the top, a closed BudgetExhausted means the offline budget ran out.
func (c *ChannelBackendClient) awaitReattach(ctx context.Context, snap AttachmentSnapshot) error {
	select {
	case <-snap.AttachedCh:
		return nil
	case <-snap.BudgetExhausted:
		return ErrOfflineBudgetExhausted
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return fmt.Errorf("MCP backend client is closed")
	}
}

// attemptSend hands msg to the stream writer while attached (system design
// part 2 "Send loop" step 2). sent=true means the writer took it and the
// caller should wait for a response. sent=false, err=nil means the send
// timed out and agentctl is now detached, so the caller should retry from
// the top of the send loop; any other outcome is terminal.
func (c *ChannelBackendClient) attemptSend(ctx context.Context, msg *ws.Message, id, action, sessionID string, start time.Time) (bool, error) {
	select {
	case c.requestCh <- msg:
		return true, nil
	case <-c.done:
		return false, fmt.Errorf("MCP backend client is closed")
	case <-ctx.Done():
		c.logger.Debug("MCP request cancelled before send",
			zap.String("request_id", id),
			zap.String("action", action),
			zap.String("session_id", sessionID),
			zap.Duration("duration", time.Since(start)),
			zap.Error(ctx.Err()))
		return false, ctx.Err()
	case <-time.After(requestSendTimeout):
		if !c.snapshot().Attached {
			return false, nil
		}
		c.logger.Warn("timed out sending MCP request to agent stream",
			zap.String("request_id", id),
			zap.String("action", action),
			zap.String("session_id", sessionID),
			zap.Duration("duration", time.Since(start)))
		return false, fmt.Errorf("timeout sending request to agent stream")
	}
}

// awaitResponse waits for the bound request's answer. retry=true means the
// response carried errRequestNotSent (system design part 2 "Sent and not
// sent" step 3, a bind failure): the write never happened, so the caller
// retries the send loop instead of surfacing anything to the agent.
func (c *ChannelBackendClient) awaitResponse(ctx context.Context, id, action string, start time.Time, respChan chan backendResponse) (backendResponse, bool, error) {
	select {
	case response := <-respChan:
		if errors.Is(response.err, errRequestNotSent) {
			return backendResponse{}, true, nil
		}
		return response, false, nil
	case <-ctx.Done():
		c.logger.Warn("MCP request context cancelled while waiting for response",
			zap.String("request_id", id),
			zap.String("action", action),
			zap.Duration("duration", time.Since(start)),
			zap.Error(ctx.Err()))
		return backendResponse{}, false, ctx.Err()
	case <-c.done:
		return backendResponse{}, false, fmt.Errorf("MCP backend client is closed")
	}
}

// finishResponse turns a completed backend response into RequestPayload's result.
func (c *ChannelBackendClient) finishResponse(id, action string, start time.Time, response backendResponse, result interface{}) error {
	if response.err != nil {
		c.logger.Warn("MCP request failed after publication",
			zap.String("request_id", id),
			zap.String("action", action),
			zap.String("session_id", response.sessionID),
			zap.Duration("duration", time.Since(start)),
			zap.Error(response.err))
		return response.err
	}
	resp := response.message
	c.logger.Debug("received MCP response from backend",
		zap.String("request_id", id),
		zap.String("action", action),
		zap.String("type", string(resp.Type)),
		zap.Duration("duration", time.Since(start)))
	if resp.Type == ws.MessageTypeError {
		var ep ws.ErrorPayload
		if json.Unmarshal(resp.Payload, &ep) == nil {
			return &BackendError{Code: ep.Code, Message: ep.Message, Details: ep.Details}
		}
		return fmt.Errorf("backend error: %s", string(resp.Payload))
	}
	if result == nil {
		return nil
	}
	if len(resp.Payload) == 0 {
		c.logger.Warn(ErrEmptyBackendPayload.Error(),
			zap.String("request_id", id),
			zap.String("action", action),
			zap.String("session_id", response.sessionID),
			zap.Duration("duration", time.Since(start)))
		return fmt.Errorf("empty response payload for action %q: %w", action, ErrEmptyBackendPayload)
	}
	if err := json.Unmarshal(resp.Payload, result); err != nil {
		return fmt.Errorf("failed to unmarshal response: %w", err)
	}
	return nil
}

func (c *ChannelBackendClient) beginPublish() bool {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	if c.closed {
		return false
	}
	c.publishWG.Add(1)
	return true
}

func backendPayloadForLog(action string, payload interface{}) interface{} {
	if action != ws.ActionMCPInvokePluginTool {
		return payload
	}
	values, ok := payload.(map[string]any)
	if !ok {
		return "<redacted>"
	}
	safe := make(map[string]any, len(values))
	for key, value := range values {
		if key != pluginToolArgumentsKey {
			safe[key] = value
		}
	}
	return safe
}

// Reset clears all pending MCP requests.
// This should be called when starting a new ACP session to prevent
// stale requests from a previous session from interfering.
func (c *ChannelBackendClient) Reset() {
	c.pendingMu.Lock()
	pending := make([]*pendingRequest, 0, len(c.pending))
	for id, request := range c.pending {
		delete(c.pending, id)
		pending = append(pending, request)
	}
	c.pendingMu.Unlock()

	for _, request := range pending {
		request.result <- backendResponse{
			err:       fmt.Errorf("MCP request cancelled: session reset"),
			sessionID: request.sessionID,
		}
	}
}

// Close prevents new requests and cancels pending requests.
func (c *ChannelBackendClient) Close() {
	c.closeOnce.Do(func() {
		c.closeMu.Lock()
		c.closed = true
		close(c.done)
		c.closeMu.Unlock()
	})
	c.publishWG.Wait()
}
