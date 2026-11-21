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
	mcporigin "github.com/kandev/kandev/internal/mcp/origin"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// ErrEmptyBackendPayload identifies a response with no payload bytes when a result sink was provided.
var ErrEmptyBackendPayload = errors.New("mcp backend response payload was empty")

// ErrKandevCallOutcomeUnknown is returned for a call that was bound to a
// backend stream that ended before the response arrived.
var ErrKandevCallOutcomeUnknown = errors.New("outcome unknown: the request may have reached Kandev; check current state before retrying")

// ErrOfflineBudgetExhausted is returned for a call made while the instance is
// detached after its offline budget expired.
var ErrOfflineBudgetExhausted = errors.New("offline budget reached: Kandev is unreachable; stop and end your turn without polling or sleeping")

// ErrRequestWithdrawn is returned by BindRequestToStream when the call has no
// pending entry (the caller abandoned it or it already finished).
var ErrRequestWithdrawn = errors.New("mcp request withdrawn")

const defaultSendTimeout = 5 * time.Second

// AttachmentSnapshot is the instance's backend-stream state at one instant.
// ChangeCh is closed when Attached, Ready or Exhausted changes.
type AttachmentSnapshot struct {
	Attached  bool
	Ready     bool
	Exhausted bool
	Epoch     uint64
	ChangeCh  <-chan struct{}
}

type noWaitKey struct{}

// WithNoWait marks a call as a best-effort notification that keeps the fixed
// send timeout instead of waiting for a backend stream.
func WithNoWait(ctx context.Context) context.Context {
	return context.WithValue(ctx, noWaitKey{}, true)
}

func isNoWait(ctx context.Context) bool {
	v, _ := ctx.Value(noWaitKey{}).(bool)
	return v
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
	notSent   chan struct{}
	streamID  string
	sessionID string
}

// ChannelBackendClient implements BackendClient using channels.
// It sends MCP requests through a channel that will be read by the agent stream handler,
// and receives responses through a callback mechanism.
type ChannelBackendClient struct {
	requestCh chan *ws.Message
	pending   map[string]*pendingRequest
	pendingMu sync.Mutex
	sessionID string
	done      chan struct{}
	closeOnce sync.Once
	closeMu   sync.Mutex
	closed    bool
	publishWG sync.WaitGroup
	logger    *logger.Logger

	snapshotter func() AttachmentSnapshot
	sendTimeout time.Duration
}

// NewChannelBackendClient creates a new channel-based backend client.
func NewChannelBackendClient(log *logger.Logger) *ChannelBackendClient {
	clientLogger := logger.Default()
	if log != nil {
		clientLogger = log
	}
	clientLogger = clientLogger.WithFields(zap.String("component", "mcp-backend-client"))
	return &ChannelBackendClient{
		requestCh:   make(chan *ws.Message),
		pending:     make(map[string]*pendingRequest),
		done:        make(chan struct{}),
		logger:      clientLogger,
		sendTimeout: defaultSendTimeout,
	}
}

// SetAttachmentSnapshotter makes calls wait for a writer-ready backend stream
// instead of failing after the fixed send timeout. Call it before the first
// request.
func (c *ChannelBackendClient) SetAttachmentSnapshotter(snapshot func() AttachmentSnapshot) {
	c.snapshotter = snapshot
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

// BindRequestToStream binds a pending request to the stream whose writer is
// about to write it. It fails with ErrRequestWithdrawn when the request is no
// longer pending, in which case the writer must not write it.
func (c *ChannelBackendClient) BindRequestToStream(requestID, streamID string) error {
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	pending, ok := c.pending[requestID]
	if !ok {
		return ErrRequestWithdrawn
	}
	pending.streamID = streamID
	return nil
}

// FailRequestNotSent returns a request that was received by a stream writer
// whose stream had already ended to the waiting state. The request stays
// registered and its caller republishes it.
func (c *ChannelBackendClient) FailRequestNotSent(requestID string) {
	c.pendingMu.Lock()
	pending, ok := c.pending[requestID]
	c.pendingMu.Unlock()
	if !ok {
		return
	}
	select {
	case pending.notSent <- struct{}{}:
	default:
	}
}

// FailRequest releases one pending request after a transport failure.
func (c *ChannelBackendClient) FailRequest(requestID string, err error) {
	c.completeRequest(requestID, backendResponse{err: err})
}

// FailStreamRequests releases requests delivered by one disconnected stream.
func (c *ChannelBackendClient) FailStreamRequests(streamID string, err error) {
	c.pendingMu.Lock()
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
// With an attachment snapshotter set, the call waits while the instance has no
// writer-ready backend stream.
func (c *ChannelBackendClient) RequestPayload(ctx context.Context, action string, payload, result interface{}) error {
	id := uuid.New().String()
	start := time.Now()

	msg, err := ws.NewRequest(id, action, payload)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	if mcporigin.IsTrustedInternalCall(ctx) {
		mcporigin.AttachTrustedInternalCall(msg)
	}

	respChan := make(chan backendResponse, 1)
	c.pendingMu.Lock()
	sessionID := c.sessionID
	c.pending[id] = &pendingRequest{result: respChan, notSent: make(chan struct{}, 1), sessionID: sessionID}
	notSent := c.pending[id].notSent
	c.pendingMu.Unlock()

	c.logger.Debug("sending MCP request through agent stream",
		zap.String("request_id", id),
		zap.String("action", action),
		zap.String("session_id", sessionID),
		zap.Any("payload", backendPayloadForLog(action, payload)))

	defer func() {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
	}()

	for {
		early, err := c.publish(ctx, msg, respChan, isNoWait(ctx), start)
		if err != nil {
			return err
		}
		if early == nil {
			early, err = c.awaitResponse(ctx, respChan, notSent, id, action, start)
			if err != nil {
				return err
			}
		}
		if early != nil {
			return c.decodeResponse(*early, id, action, result, start)
		}
		// The writer received the request after its stream had ended: wait again.
	}
}

// publish hands the request to a stream writer. It returns a response when the
// call ended before publication (session reset), and nil once published.
func (c *ChannelBackendClient) publish(ctx context.Context, msg *ws.Message, respChan chan backendResponse, noWait bool, start time.Time) (*backendResponse, error) {
	if !c.beginPublish() {
		return nil, fmt.Errorf("MCP backend client is closed")
	}
	defer c.publishWG.Done()

	waits := c.snapshotter != nil && !noWait
	for {
		var snap AttachmentSnapshot
		if waits {
			snap = c.snapshotter()
			if snap.Exhausted && !snap.Attached {
				c.logger.Warn("MCP request refused: offline budget exhausted", zap.String("reason", "budget_exhausted"))
				return nil, ErrOfflineBudgetExhausted
			}
			if !snap.Ready {
				select {
				case <-snap.ChangeCh:
					continue
				case response := <-respChan:
					return &response, nil
				case <-c.done:
					return nil, fmt.Errorf("MCP backend client is closed")
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
		}
		sent, early, err := c.sendWindow(ctx, msg, respChan, snap, waits, start)
		if err != nil || early != nil {
			return early, err
		}
		if sent {
			return nil, nil
		}
	}
}

// sendWindow tries to hand msg to a stream writer for one send timeout. It
// returns sent=false with no error when the attach epoch changed, so the
// caller starts a new window at the next writer-ready.
func (c *ChannelBackendClient) sendWindow(ctx context.Context, msg *ws.Message, respChan chan backendResponse, snap AttachmentSnapshot, waits bool, start time.Time) (bool, *backendResponse, error) {
	timer := time.NewTimer(c.sendTimeout)
	defer timer.Stop()
	changeCh := snap.ChangeCh
	if !waits {
		changeCh = nil
	}
	sameWindow := func() (AttachmentSnapshot, bool) {
		cur := c.snapshotter()
		return cur, cur.Ready && cur.Epoch == snap.Epoch
	}
	for {
		select {
		case c.requestCh <- msg:
			return true, nil, nil
		case <-c.done:
			return false, nil, fmt.Errorf("MCP backend client is closed")
		case <-ctx.Done():
			c.logger.Debug("MCP request cancelled before send",
				zap.String("request_id", msg.ID),
				zap.String("action", msg.Action),
				zap.Duration("duration", time.Since(start)),
				zap.Error(ctx.Err()))
			return false, nil, ctx.Err()
		case response := <-respChan:
			return false, &response, nil
		case <-changeCh:
			cur, same := sameWindow()
			if !same {
				return false, nil, nil
			}
			changeCh = cur.ChangeCh
		case <-timer.C:
			if waits {
				if _, same := sameWindow(); !same {
					return false, nil, nil
				}
			}
			c.logger.Warn("timed out sending MCP request to agent stream",
				zap.String("request_id", msg.ID),
				zap.String("action", msg.Action),
				zap.Duration("duration", time.Since(start)))
			return false, nil, fmt.Errorf("timeout sending request to agent stream")
		}
	}
}

// awaitResponse waits for the backend's answer. It returns nil, nil when the
// writer reports the request was never sent and it must be published again.
func (c *ChannelBackendClient) awaitResponse(ctx context.Context, respChan chan backendResponse, notSent chan struct{}, id, action string, start time.Time) (*backendResponse, error) {
	select {
	case response := <-respChan:
		return &response, nil
	case <-notSent:
		return nil, nil
	case <-ctx.Done():
		c.logger.Warn("MCP request context cancelled while waiting for response",
			zap.String("request_id", id),
			zap.String("action", action),
			zap.Duration("duration", time.Since(start)),
			zap.Error(ctx.Err()))
		return nil, ctx.Err()
	case <-c.done:
		return nil, fmt.Errorf("MCP backend client is closed")
	}
}

func (c *ChannelBackendClient) decodeResponse(response backendResponse, id, action string, result interface{}, start time.Time) error {
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
