package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/user/store"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

const (
	// Time allowed to write a message to the peer
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer
	pongWait = 60 * time.Second

	// Send pings to peer with this period (must be less than pongWait)
	pingPeriod = (pongWait * 9) / 10

	// Maximum message size allowed from peer
	// Increased to support image attachments (base64 encoded images are ~33% larger)
	maxMessageSize = 32 * 1024 * 1024 // 32MB

	// Control frames (RPC responses and errors) use a separate bounded queue so
	// high-volume session notifications cannot fill the queue and make a user
	// action appear to time out.
	controlSendBufferSize   = 256
	orderedConsumerPlugin   = "plugin"
	orderedConsumerCore     = "core"
	responseErrorKey        = "error"
	sessionIDPayloadKey     = "session_id"
	eventTypePayloadKey     = "type"
	eventSequencePayloadKey = "sequence"
	taskIDPayloadKey        = "task_id"
)

// Client represents a single WebSocket connection
type Client struct {
	ID string
	// identity is the authenticated caller behind this connection. Zero for
	// anonymous connections (auth disabled and no synthetic identity set by
	// the HTTP middleware — e.g. direct hub tests); synthetic in disabled
	// mode; a real user when auth is enabled.
	identity                    authn.Identity
	conn                        *websocket.Conn
	hub                         *Hub
	send                        chan []byte
	controlSend                 chan []byte
	subscriptions               map[string]bool // Task IDs this client is subscribed to
	sessionSubscriptions        map[string]bool // Session IDs this client is subscribed to
	sessionFocus                map[string]bool // Session IDs this client has focused (a strict subset of subscriptions, conceptually — see hub_session_mode.go)
	orderedSessionSubscriptions map[string]map[string]plugins.SessionDeliveryCursorKey
	userSubscriptions           map[string]bool // User IDs this client is subscribed to
	runSubscriptions            map[string]bool // Office run IDs this client is subscribed to (for run.event.appended)
	systemMetricsSubscribed     bool
	mu                          sync.RWMutex
	closed                      bool
	logger                      *logger.Logger

	// Replaceable session.message.updated traffic is scheduled separately from
	// semantic notifications so one noisy session cannot fill the shared FIFO.
	// Each session queue contains replaceable segments separated by semantic
	// barriers. This prevents an update published after a barrier from overtaking
	// that barrier while still allowing coalescing within the current segment.
	replaceableByKey           map[queuedReplaceableKey]outboundNotification
	replaceableBySession       map[string][]sessionNotificationQueueItem
	replaceableCurrentByKey    map[replaceableNotificationKey]queuedReplaceableKey
	replaceableSessionOrder    []string
	replaceableRoundRobin      int
	nextReplaceableSequence    uint64
	scheduledSemantic          int
	notificationWake           chan struct{}
	replaceableReplacements    uint64
	replaceableEvictions       uint64
	replaceableRejected        uint64
	droppedSemantic            uint64
	replaceablePerSessionLimit int
	replaceableGlobalLimit     int
}

// NewClient creates a new WebSocket client
func NewClient(id string, identity authn.Identity, conn *websocket.Conn, hub *Hub, log *logger.Logger) *Client {
	return &Client{
		ID:                          id,
		identity:                    identity,
		conn:                        conn,
		hub:                         hub,
		send:                        make(chan []byte, 256),
		controlSend:                 make(chan []byte, controlSendBufferSize),
		subscriptions:               make(map[string]bool),
		sessionSubscriptions:        make(map[string]bool),
		sessionFocus:                make(map[string]bool),
		orderedSessionSubscriptions: make(map[string]map[string]plugins.SessionDeliveryCursorKey),
		userSubscriptions:           make(map[string]bool),
		runSubscriptions:            make(map[string]bool),
		replaceableByKey:            make(map[queuedReplaceableKey]outboundNotification),
		replaceableBySession:        make(map[string][]sessionNotificationQueueItem),
		replaceableCurrentByKey:     make(map[replaceableNotificationKey]queuedReplaceableKey),
		notificationWake:            make(chan struct{}, 1),
		logger:                      log.WithFields(zap.String("client_id", id)),
	}
}

// dispatchContext returns the hub's lifetime context carrying this client's
// identity and server-assigned connection ID. The latter is used by
// connection-bound queue edit leases and never comes from the request payload.
func (c *Client) dispatchContext() context.Context {
	ctx := c.hub.DispatchContext()
	ctx = ws.WithConnectionID(ctx, c.ID)
	if c.identity.UserID != "" {
		ctx = authn.WithIdentity(ctx, c.identity)
	}
	return ctx
}

// ReadPump pumps messages from the WebSocket connection to the hub.
//
// The ctx argument is retained for API stability but is not consulted by the
// pump itself — Gorilla's ReadMessage blocks on the conn only, so teardown
// happens via the conn close path (driven by client disconnect, server
// shutdown closing all conns, or pong timeout). Dispatched handlers use the
// hub's lifetime context instead; see handleMessage.
func (c *Client) ReadPump(_ context.Context) {
	defer func() {
		c.hub.Unregister(c)
		if err := c.conn.Close(); err != nil {
			c.logger.Debug("failed to close websocket connection", zap.Error(err))
		}
	}()

	c.conn.SetReadLimit(maxMessageSize)
	if err := c.conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		c.logger.Debug("failed to set read deadline", zap.Error(err))
	}
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			// CloseNormalClosure (1000): Normal browser close
			// CloseGoingAway (1001): Client navigating away
			// CloseNoStatusReceived (1005): Client closed without status (normal browser close)
			// CloseAbnormalClosure (1006): Abnormal close (network drop)
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived, websocket.CloseAbnormalClosure) {
				c.logger.Error("WebSocket read error", zap.Error(err))
			}
			break
		}

		// Parse the message
		var msg ws.Message
		if err := json.Unmarshal(message, &msg); err != nil {
			c.logger.Error("Failed to parse message", zap.Error(err))
			c.sendError("", "", ws.ErrorCodeBadRequest, "Invalid message format", nil)
			continue
		}

		// Process the message in a goroutine to avoid blocking the read pump
		// This allows concurrent message handling so long-running handlers
		// (like orchestrator.prompt) don't block other requests (like workspace.tree.get)
		go c.handleMessage(&msg)
	}
}

// handleMessage processes an incoming message.
//
// Intentionally does NOT take the connection context. Dispatched handlers run
// under the hub's lifetime context so a mid-flight client disconnect doesn't
// abort in-progress side effects (see the comment on Dispatch below).
func (c *Client) handleMessage(msg *ws.Message) {
	c.logger.Debug("Received message",
		zap.String("action", msg.Action),
		zap.String("id", msg.ID))

	if strings.HasPrefix(msg.Action, "mcp.") {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeForbidden,
			"MCP actions are not available over the raw WebSocket", nil)
		return
	}

	// Handle subscription actions specially (they need access to the client)
	switch msg.Action {
	case ws.ActionTaskSubscribe:
		c.handleSubscribe(msg)
		return
	case ws.ActionTaskUnsubscribe:
		c.handleUnsubscribe(msg)
		return
	case ws.ActionSessionSubscribe:
		c.handleSessionSubscribe(msg)
		return
	case ws.ActionSessionUnsubscribe:
		c.handleSessionUnsubscribe(msg)
		return
	case ws.ActionSessionAck:
		c.handleSessionAck(msg)
		return
	case ws.ActionSessionPoisonRequeue:
		c.handleSessionPoisonRequeue(msg)
		return
	case ws.ActionSessionFocus:
		c.handleSessionFocus(msg)
		return
	case ws.ActionSessionGitRefresh:
		c.handleSessionGitRefresh(msg)
		return
	case ws.ActionSessionDataRefresh:
		c.handleSessionDataRefresh(msg)
		return
	case ws.ActionSessionUnfocus:
		c.handleSessionUnfocus(msg)
		return
	case ws.ActionUserSubscribe:
		c.handleUserSubscribe(msg)
		return
	case ws.ActionUserUnsubscribe:
		c.handleUserUnsubscribe(msg)
		return
	case ws.ActionRunSubscribe:
		c.handleRunSubscribe(msg)
		return
	case ws.ActionRunUnsubscribe:
		c.handleRunUnsubscribe(msg)
		return
	case ws.ActionSystemMetricsSubscribe:
		c.handleSystemMetricsSubscribe(msg)
		return
	case ws.ActionSystemMetricsUnsubscribe:
		c.handleSystemMetricsUnsubscribe(msg)
		return
	}

	// Dispatch to handler using the hub's lifetime context, not the per-
	// connection one. The connection ctx is cancelled when the client
	// disconnects (page reload, nav, network drop). Using it here would
	// SIGKILL any exec.CommandContext subprocesses the handler spawned
	// (e.g. `gh pr`, `git`, agentctl HTTP requests) and abort
	// side-effecting work like session.launch mid-flight. We can't deliver
	// the response either way once the connection is gone, but the
	// handler's work should run to completion so it doesn't leave partial
	// state behind. The dispatch ctx still cancels on server shutdown.
	dispatchCtx := c.dispatchContext()

	// Per-user scoping backstop: refuse before the handler runs if the payload
	// names a task or session this client may not touch. See dispatch_scope.go
	// for why this lives here and not only in each handler.
	if err := c.authorizeAction(dispatchCtx, msg.Action, msg.Payload); err != nil {
		c.logger.Debug("denied out-of-scope action",
			zap.String("action", msg.Action),
			zap.String("user_id", c.identity.UserID),
			zap.Error(err))
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "not found", nil)
		return
	}

	response, err := c.hub.dispatcher.Dispatch(dispatchCtx, msg)
	if err != nil {
		c.logger.Error("Handler error",
			zap.String("action", msg.Action),
			zap.Error(err))
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
		return
	}

	if response != nil {
		c.sendMessage(response)
	}
}

// SubscribeRequest is the payload for task.subscribe
type SubscribeRequest struct {
	TaskID string `json:"task_id"`
}

// handleSubscribe handles task.subscribe action
func (c *Client) handleSubscribe(msg *ws.Message) {
	var req SubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return
	}

	if req.TaskID == "" {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		return
	}

	// Per-user scoping: a client may only observe tasks in its own workspaces.
	if check := c.hub.authPolicy.Subscriptions.Task; check != nil {
		if err := check(c.dispatchContext(), req.TaskID); err != nil {
			c.sendError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "cannot subscribe to this task", nil)
			return
		}
	}

	c.hub.SubscribeToTask(c, req.TaskID)

	// Send success response
	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
		"task_id": req.TaskID,
	})
	c.sendMessage(resp)
}

type UserSubscribeRequest struct {
	UserID string `json:"user_id,omitempty"`
}

type SessionSubscribeRequest struct {
	SessionID        string  `json:"session_id"`
	ConsumerKind     string  `json:"consumer_kind,omitempty"`
	PluginID         string  `json:"plugin_id,omitempty"`
	Generation       int64   `json:"generation,omitempty"`
	BindingToken     string  `json:"binding_token,omitempty"`
	LastSeenSequence *uint64 `json:"last_seen_sequence,omitempty"`
	ResumeToken      string  `json:"resume_token,omitempty"`
	ReplaceCursor    bool    `json:"replace_cursor,omitempty"`
	ConsumerID       string  `json:"consumer_id,omitempty"`
	WireID           string  `json:"wire_id,omitempty"`
}

// ownUserTopic resolves the user-topic this client may subscribe to: its own
// authenticated user, or the pre-auth default user for anonymous/synthetic
// connections (today's single-user behavior).
func (c *Client) ownUserTopic() string {
	if c.identity.UserID != "" && !c.identity.Synthetic {
		return c.identity.UserID
	}
	return store.DefaultUserID
}

func (c *Client) handleUserSubscribe(msg *ws.Message) {
	var req UserSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return
	}

	userID := req.UserID
	if userID == "" {
		userID = c.ownUserTopic()
	}
	if userID != c.ownUserTopic() {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "cannot subscribe to another user", nil)
		return
	}

	c.hub.SubscribeToUser(c, userID)
	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
		"user_id": userID,
	})
	c.sendMessage(resp)
}

func (c *Client) handleSessionSubscribe(msg *ws.Message) {
	var req SessionSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return
	}

	if req.SessionID == "" {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
		return
	}
	if req.ConsumerKind != "" {
		c.handleOrderedSessionSubscribe(msg, req)
		return
	}

	if !c.maySubscribeSession(msg, req.SessionID) {
		return
	}

	newMembership := c.hub.SubscribeToSession(c, req.SessionID)
	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success":    true,
		"session_id": req.SessionID,
	})
	c.sendMessage(resp)

	// Send initial session data only when this client newly joins. Duplicate
	// subscribe requests are acknowledgements, not snapshot replay commands.
	if newMembership {
		c.sendSessionData(req.SessionID)
	}
}

func (c *Client) handleOrderedSessionSubscribe(msg *ws.Message, req SessionSubscribeRequest) {
	service := c.hub.pluginConversationService
	if service == nil {
		c.sendSessionStreamFailure(msg, req.SessionID, "upstream_failure", "session stream unavailable", true)
		return
	}
	userID := c.ownUserTopic()
	if req.ConsumerKind == orderedConsumerPlugin {
		lastSeen := uint64(0)
		if req.LastSeenSequence != nil {
			lastSeen = *req.LastSeenSequence
		}
		candidate := plugins.SessionDeliveryCursorKey{
			SessionID: req.SessionID, ConsumerKind: req.ConsumerKind, ConsumerID: req.ConsumerID,
			WireID: req.WireID, PluginID: req.PluginID, Generation: req.Generation, UserID: userID,
		}
		if req.ResumeToken == "" || service.ValidateSessionResume(req.ResumeToken, candidate, lastSeen) != nil {
			req.ConsumerID = uuid.NewString()
		}
	}
	key := plugins.SessionDeliveryCursorKey{
		SessionID: req.SessionID, ConsumerKind: req.ConsumerKind,
		ConsumerID: req.ConsumerID, WireID: req.WireID, PluginID: req.PluginID,
		Generation: req.Generation, UserID: userID,
	}
	if !c.authorizeOrderedSessionConsumer(msg, req, service, userID) {
		return
	}
	if !c.maySubscribeOrderedSession(msg, req, key, service) {
		return
	}
	c.hub.orderedSessionMu.Lock()
	defer c.hub.orderedSessionMu.Unlock()
	committedEvents, err := service.SyncCommittedSessionEvents(context.Background(), req.SessionID)
	if err != nil {
		c.sendSessionStreamFailure(msg, req.SessionID, "upstream_failure", "session stream unavailable", true)
		return
	}
	for _, event := range committedEvents {
		c.hub.broadcastCommittedOrderedSessionEvent(service, event)
	}
	replay := resolveOrderedSessionReplay(service, req, key)
	if replay.terminal && (replay.resumeValid || service.SessionEvents().HasCursor(key)) {
		c.sendSessionStreamFailure(msg, req.SessionID, "session_removed", "session was removed", false)
		return
	}
	c.acceptOrderedSessionSubscription(msg, req, key, userID, replay)
}

type orderedSessionReplay struct {
	events         []plugins.SessionEvent
	watermark      uint64
	cursorSequence uint64
	result         string
	resumeValid    bool
	terminal       bool
}

//nolint:goconst // Result values are protocol literals.
func resolveOrderedSessionReplay(
	service *plugins.Service,
	req SessionSubscribeRequest,
	key plugins.SessionDeliveryCursorKey,
) orderedSessionReplay {
	lastSeen := uint64(0)
	lastSeenSupplied := req.LastSeenSequence != nil
	if lastSeenSupplied {
		lastSeen = *req.LastSeenSequence
	}
	events, watermark, terminal := service.SessionEvents().ReplayState(req.SessionID, lastSeen)
	// A retained replay that starts past lastSeen+1 - or has no retained rows
	// at all while the partition watermark is still ahead of the cursor -
	// means intermediate rows aged out on both sides; replaying it as if
	// contiguous would hide lost history. Rebind to the watermark instead
	// (same replacement-cursor machinery as invalid_resume) so the client
	// reconciles from the authoritative snapshot boundary.
	retainedGap := lastSeenSupplied &&
		lastSeen < watermark &&
		(len(events) == 0 || events[0].Sequence > lastSeen+1)
	if retainedGap {
		return orderedSessionReplay{
			watermark: watermark, cursorSequence: watermark,
			result: "invalid_resume", terminal: terminal,
		}
	}
	if req.ResumeToken != "" {
		if err := service.ValidateSessionResume(req.ResumeToken, key, lastSeen); err != nil {
			return orderedSessionReplay{
				watermark: watermark, cursorSequence: watermark,
				result: "invalid_resume", terminal: terminal,
			}
		}
		result := "fresh"
		if lastSeenSupplied && len(events) > 0 {
			result = "replay"
		}
		return orderedSessionReplay{
			events: events, watermark: watermark, cursorSequence: lastSeen,
			result: result, resumeValid: true, terminal: terminal,
		}
	}
	if !lastSeenSupplied {
		return orderedSessionReplay{
			watermark: watermark, cursorSequence: watermark,
			result: "fresh", terminal: terminal,
		}
	}
	result := "fresh"
	if len(events) > 0 {
		result = "replay"
	}
	return orderedSessionReplay{
		events: events, watermark: watermark, cursorSequence: lastSeen,
		result: result, terminal: terminal,
	}
}

func (c *Client) acceptOrderedSessionSubscription(
	msg *ws.Message,
	req SessionSubscribeRequest,
	key plugins.SessionDeliveryCursorKey,
	userID string,
	replay orderedSessionReplay,
) bool {
	service := c.hub.pluginConversationService
	snapshotToken, resumeToken, expiresAt, err := service.MintSessionStreamGrant(
		req.PluginID,
		userID,
		req.Generation,
		req.SessionID,
		req.ConsumerID,
		req.WireID,
		replay.watermark,
	)
	if err != nil {
		c.sendSessionStreamFailure(msg, req.SessionID, "invalid_binding", "cannot mint session stream grant", true)
		return false
	}
	replay, claims, err := prepareOrderedReplayDelivery(service, replay, time.Now().UTC())
	if err != nil {
		c.sendSessionStreamFailure(msg, req.SessionID, "upstream_failure", "cannot claim session event delivery", true)
		return false
	}
	cursorSequence := replay.cursorSequence
	if req.ReplaceCursor || replay.result == "invalid_resume" {
		cursorSequence = replay.watermark
	}
	var cursorErr error
	if req.ReplaceCursor || replay.result == "invalid_resume" {
		cursorErr = service.SessionEvents().ReplaceCursor(key, cursorSequence)
	} else {
		cursorErr = service.SessionEvents().RegisterCursor(key, cursorSequence)
	}
	if cursorErr != nil {
		if releaseErr := releaseOrderedReplayClaims(service, claims); releaseErr != nil && c.logger != nil {
			c.logger.Error("release ordered replay claims", zap.Error(releaseErr))
		}
		c.sendSessionStreamFailure(msg, req.SessionID, "upstream_failure", "cannot register session cursor", true)
		return false
	}
	payload := orderedSessionSubscribePayload(req, replay, snapshotToken, resumeToken, expiresAt)
	c.storeOrderedSessionSubscription(req, key)
	response, _ := ws.NewResponse(msg.ID, msg.Action, payload)
	c.sendMessage(response)
	for _, event := range replay.events {
		queued := c.sendOrderedSessionEvent(event)
		if claim, ok := claims[event.ID]; ok {
			if err := service.SessionDelivery().Complete(claim, queued, time.Now().UTC()); err != nil && c.logger != nil {
				c.logger.Error("complete ordered replay poison", zap.String("event_id", event.ID), zap.Error(err))
			}
		}
	}
	return true
}

func prepareOrderedReplayDelivery(
	service *plugins.Service,
	replay orderedSessionReplay,
	now time.Time,
) (orderedSessionReplay, map[string]plugins.SessionDeliveryClaim, error) {
	claims := make(map[string]plugins.SessionDeliveryClaim)
	for _, event := range replay.events {
		claim, err := service.SessionDelivery().Claim(event.SessionID, event.ID, now)
		if err != nil {
			releaseErr := releaseOrderedReplayClaims(service, claims)
			return replay, nil, errors.Join(err, releaseErr)
		}
		switch claim.Disposition {
		case plugins.SessionDeliveryUntracked:
		case plugins.SessionDeliveryClaimed:
			claims[event.ID] = claim
		default:
			if err := releaseOrderedReplayClaims(service, claims); err != nil {
				return replay, nil, err
			}
			replay.events = nil
			replay.result = "invalid_resume"
			replay.cursorSequence = replay.watermark
			return replay, nil, nil
		}
	}
	return replay, claims, nil
}

func releaseOrderedReplayClaims(
	service *plugins.Service,
	claims map[string]plugins.SessionDeliveryClaim,
) error {
	now := time.Now().UTC()
	var result error
	for _, claim := range claims {
		result = errors.Join(result, service.SessionDelivery().Complete(claim, false, now))
	}
	return result
}

func orderedSessionSubscribePayload(
	req SessionSubscribeRequest,
	replay orderedSessionReplay,
	snapshotToken string,
	resumeToken string,
	expiresAt time.Time,
) map[string]any {
	payload := map[string]any{
		"success": true, sessionIDPayloadKey: req.SessionID, "result": replay.result,
		"event_watermark": replay.watermark, "snapshot_cutoff": replay.watermark,
		"snapshot_token": snapshotToken, "resume_token": resumeToken, "expires_at": expiresAt,
	}
	if req.ConsumerKind == orderedConsumerPlugin {
		payload["consumer_id"] = req.ConsumerID
	} else {
		payload["wire_id"] = req.WireID
	}
	if replay.result == "replay" && len(replay.events) > 0 {
		payload["replay_from"] = replay.events[0].Sequence
		payload["replay_to"] = replay.events[len(replay.events)-1].Sequence
	}
	return payload
}

func (c *Client) storeOrderedSessionSubscription(
	req SessionSubscribeRequest,
	key plugins.SessionDeliveryCursorKey,
) {
	subscriptionID := req.ConsumerID
	if subscriptionID == "" {
		subscriptionID = req.WireID
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	byConsumer := c.orderedSessionSubscriptions[req.SessionID]
	if byConsumer == nil {
		byConsumer = make(map[string]plugins.SessionDeliveryCursorKey)
		c.orderedSessionSubscriptions[req.SessionID] = byConsumer
	}
	byConsumer[subscriptionID] = key
}

func (c *Client) authorizeOrderedSessionConsumer(
	msg *ws.Message,
	req SessionSubscribeRequest,
	service *plugins.Service,
	userID string,
) bool {
	switch req.ConsumerKind {
	case orderedConsumerPlugin:
		return c.authorizeOrderedPluginConsumer(msg, req, service, userID)
	case orderedConsumerCore:
		if req.WireID != "" && req.ConsumerID == "" && req.PluginID == "" &&
			req.Generation == 0 && req.BindingToken == "" {
			return true
		}
		c.sendSessionStreamFailure(msg, req.SessionID, "invalid_request", "invalid core consumer identity", false)
	default:
		c.sendSessionStreamFailure(msg, req.SessionID, "invalid_request", "unsupported consumer kind", false)
	}
	return false
}

func (c *Client) authorizeOrderedPluginConsumer(
	msg *ws.Message,
	req SessionSubscribeRequest,
	service *plugins.Service,
	userID string,
) bool {
	if req.ConsumerID == "" || req.WireID != "" || req.PluginID == "" || req.Generation == 0 {
		c.sendSessionStreamFailure(msg, req.SessionID, "invalid_request", "invalid plugin consumer identity", false)
		return false
	}
	err := service.AuthorizeConversationConsumer(
		req.PluginID,
		userID,
		req.Generation,
		req.BindingToken,
	)
	if err == nil {
		return true
	}
	code := "invalid_binding"
	retryable := false
	if errors.Is(err, plugins.ErrConversationGenerationSuperseded) {
		code = "generation_superseded"
		retryable = true
	}
	c.sendSessionStreamFailure(msg, req.SessionID, code, "plugin conversation binding rejected", retryable)
	return false
}

func (c *Client) sendSessionStreamFailure(
	msg *ws.Message,
	sessionID string,
	code string,
	message string,
	retryable bool,
) {
	payload := map[string]any{
		"success":        false,
		responseErrorKey: map[string]any{"code": code, "message": message, "retryable": retryable},
	}
	if sessionID != "" {
		payload["session_id"] = sessionID
	}
	response, _ := ws.NewResponse(msg.ID, msg.Action, payload)
	c.sendMessage(response)
}

func (c *Client) sendOrderedSessionEvent(event plugins.SessionEvent) bool {
	frame, err := sessionEventFrame(event)
	if err != nil {
		c.closeSend()
		return false
	}
	if c.sendNotification(frame, "session.event") {
		return true
	}
	// An ordered frame that is not queued cannot be acknowledged safely. End
	// this connection so the client reconnects and replays from its cursor.
	c.closeSend()
	return false
}

type SessionAckRequest struct {
	SessionID    string `json:"session_id"`
	ConsumerKind string `json:"consumer_kind"`
	PluginID     string `json:"plugin_id,omitempty"`
	Generation   int64  `json:"generation,omitempty"`
	Sequence     uint64 `json:"sequence"`
	ResumeToken  string `json:"resume_token,omitempty"`
	ConsumerID   string `json:"consumer_id,omitempty"`
	WireID       string `json:"wire_id,omitempty"`
}

func (c *Client) handleSessionAck(msg *ws.Message) {
	var req SessionAckRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendSessionStreamFailure(msg, "", "invalid_request", "invalid acknowledgement", false)
		return
	}
	service := c.hub.pluginConversationService
	identity, validIdentity := sessionAckSubscriptionID(req)
	if !validIdentity {
		c.sendSessionStreamFailure(msg, req.SessionID, "invalid_request", "invalid session consumer identity", false)
		return
	}
	c.mu.RLock()
	key, ok := c.orderedSessionSubscriptions[req.SessionID][identity]
	c.mu.RUnlock()
	if service == nil || !ok || !sessionAckMatchesKey(req, key) {
		c.sendSessionStreamFailure(msg, req.SessionID, "invalid_request", "unknown session consumer", false)
		return
	}
	if err := service.ValidateSessionResume(req.ResumeToken, key, req.Sequence); err != nil {
		c.sendSessionStreamFailure(msg, req.SessionID, "invalid_binding", "invalid resume token", false)
		return
	}
	if err := service.SessionEvents().Acknowledge(key, req.Sequence); err != nil {
		code := "forward_gap"
		retryable := true
		if errors.Is(err, plugins.ErrPoisonEvent) {
			code = "invalid_request"
			retryable = false
		}
		c.sendSessionStreamFailure(msg, req.SessionID, code, "acknowledgement rejected", retryable)
		return
	}
	_, resumeToken, _, err := service.MintSessionStreamGrant(
		key.PluginID,
		key.UserID,
		key.Generation,
		key.SessionID,
		key.ConsumerID,
		key.WireID,
		req.Sequence,
	)
	if err != nil {
		c.sendSessionStreamFailure(msg, req.SessionID, "invalid_binding", "cannot mint resume token", true)
		return
	}
	payload := map[string]any{
		"success": true, "session_id": req.SessionID,
		"acknowledged_sequence": req.Sequence, "resume_token": resumeToken,
	}
	if key.ConsumerKind == orderedConsumerPlugin {
		payload["consumer_id"] = key.ConsumerID
	} else {
		payload["wire_id"] = key.WireID
	}
	response, _ := ws.NewResponse(msg.ID, msg.Action, payload)
	c.sendMessage(response)
}

func sessionAckSubscriptionID(req SessionAckRequest) (string, bool) {
	switch req.ConsumerKind {
	case orderedConsumerPlugin:
		return req.ConsumerID, req.ConsumerID != "" && req.WireID == ""
	case orderedConsumerCore:
		return req.WireID, req.WireID != "" && req.ConsumerID == ""
	default:
		return "", false
	}
}

func sessionAckMatchesKey(req SessionAckRequest, key plugins.SessionDeliveryCursorKey) bool {
	if req.SessionID != key.SessionID || req.ConsumerKind != key.ConsumerKind {
		return false
	}
	switch req.ConsumerKind {
	case orderedConsumerPlugin:
		return req.ConsumerID == key.ConsumerID &&
			req.PluginID == key.PluginID &&
			req.Generation == key.Generation
	case orderedConsumerCore:
		return req.WireID == key.WireID && req.PluginID == "" && req.Generation == 0
	default:
		return false
	}
}

type sessionPoisonRequeueRequest struct {
	SessionID          string `json:"session_id"`
	EventID            string `json:"event_id"`
	ExpectedOwnerEpoch uint64 `json:"expected_owner_epoch"`
}

func (c *Client) handleSessionPoisonRequeue(msg *ws.Message) {
	var req sessionPoisonRequeueRequest
	if err := msg.ParsePayload(&req); err != nil ||
		req.SessionID == "" ||
		req.EventID == "" ||
		req.ExpectedOwnerEpoch == 0 {
		c.sendSessionStreamFailure(msg, req.SessionID, "invalid_request", "invalid poison requeue request", false)
		return
	}
	if !canRequeueSessionEvents(c.identity) || c.hub.pluginConversationService == nil {
		c.sendSessionStreamFailure(msg, req.SessionID, "unauthorized", "session_events:requeue required", false)
		return
	}
	err := c.hub.pluginConversationService.SessionDelivery().Requeue(
		req.SessionID,
		req.EventID,
		req.ExpectedOwnerEpoch,
		c.ownUserTopic(),
		time.Now().UTC(),
	)
	if err != nil {
		c.sendSessionStreamFailure(msg, req.SessionID, "invalid_request", "poison requeue rejected", false)
		return
	}
	response, _ := ws.NewResponse(msg.ID, msg.Action, map[string]any{
		"success": true, "session_id": req.SessionID, "event_id": req.EventID,
		"state": plugins.SessionPoisonPending,
	})
	c.sendMessage(response)
}

func canRequeueSessionEvents(identity authn.Identity) bool {
	if identity.UserID == "" {
		return false
	}
	return identity.Instance || identity.Synthetic
}

// maySubscribeSession applies the per-user session scoping check, emitting the
// forbidden error itself. Returns true when the subscription may proceed.
func (c *Client) maySubscribeSession(msg *ws.Message, sessionID string) bool {
	check := c.hub.authPolicy.Subscriptions.Session
	if check == nil {
		return true
	}
	if err := check(c.dispatchContext(), sessionID); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "cannot subscribe to this session", nil)
		return false
	}
	return true
}

func (c *Client) maySubscribeOrderedSession(
	msg *ws.Message,
	req SessionSubscribeRequest,
	key plugins.SessionDeliveryCursorKey,
	service *plugins.Service,
) bool {
	check := c.hub.authPolicy.Subscriptions.Session
	if check == nil {
		return true
	}
	if err := check(c.dispatchContext(), req.SessionID); err == nil {
		return true
	}
	_, _, terminal := service.SessionEvents().ReplayState(req.SessionID, 0)
	if terminal && service.SessionEvents().HasCursor(key) {
		return true
	}
	c.sendError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "cannot subscribe to this session", nil)
	return false
}

func (c *Client) handleUserUnsubscribe(msg *ws.Message) {
	var req UserSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return
	}
	userID := req.UserID
	if userID == "" {
		userID = c.ownUserTopic()
	}
	if userID != c.ownUserTopic() {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "cannot unsubscribe from another user", nil)
		return
	}
	c.hub.UnsubscribeFromUser(c, userID)
	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
		"user_id": userID,
	})
	c.sendMessage(resp)
}

// sendSessionData sends initial session data (e.g., git status) to the client
func (c *Client) sendSessionData(sessionID string) {
	ctx := context.Background()
	data, err := c.hub.GetSessionData(ctx, sessionID)
	if err != nil {
		c.logger.Error("Failed to get session data",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}

	if len(data) == 0 {
		return
	}

	c.logger.Debug("Sending session data",
		zap.String("session_id", sessionID),
		zap.Int("count", len(data)))

	// Send each piece of session data as a notification. Snapshots are data
	// traffic, not control traffic, so they must not consume the response queue.
	for _, msg := range data {
		payload, err := json.Marshal(msg)
		if err != nil {
			c.logger.Error("Failed to marshal session data", zap.Error(err))
			continue
		}
		c.sendBytes(payload)
	}
}

// sendSessionGitData sends the bounded git snapshot used by the diff detail
// surface. The compatibility fallback may return the full legacy provider;
// filter it at the transport boundary so this action remains git-only.
func (c *Client) sendSessionGitData(sessionID string) {
	ctx := context.Background()
	data, err := c.hub.GetSessionGitData(ctx, sessionID)
	if err != nil {
		c.logger.Error("Failed to get session git data",
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}
	for _, msg := range data {
		if msg == nil || msg.Action != ws.ActionSessionGitEvent {
			continue
		}
		payload, err := json.Marshal(msg)
		if err != nil {
			c.logger.Error("Failed to marshal session git data", zap.Error(err))
			continue
		}
		c.sendBytes(payload)
	}
}

// handleUnsubscribe handles task.unsubscribe action
func (c *Client) handleUnsubscribe(msg *ws.Message) {
	var req SubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return
	}

	if req.TaskID == "" {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeValidation, "task_id is required", nil)
		return
	}

	c.hub.UnsubscribeFromTask(c, req.TaskID)

	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
		"task_id": req.TaskID,
	})
	c.sendMessage(resp)
}

// RunSubscribeRequest is the payload for run.subscribe / run.unsubscribe.
type RunSubscribeRequest struct {
	RunID string `json:"run_id"`
}

// handleRunSubscribe handles run.subscribe action — registers this
// client on the per-run topic so it receives run.event.appended
// notifications. Clients fetch the snapshot via REST and only need
// the diff stream from this point forward; we deliberately replay no
// state on subscribe.
func (c *Client) handleRunSubscribe(msg *ws.Message) {
	var req RunSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return
	}
	if req.RunID == "" {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeValidation, "run_id is required", nil)
		return
	}

	// Per-user scoping: a client may only observe runs in its own
	// workspaces. Every hook error maps to the same fixed (code, message)
	// pair below so an unknown run id and a foreign run id are
	// indistinguishable on the wire (no existence leak).
	if check := c.hub.authPolicy.Subscriptions.Run; check != nil {
		if err := check(c.dispatchContext(), req.RunID); err != nil {
			c.sendError(msg.ID, msg.Action, ws.ErrorCodeForbidden, "cannot subscribe to this run", nil)
			return
		}
	}

	c.hub.SubscribeToRun(c, req.RunID)
	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
		"run_id":  req.RunID,
	})
	c.sendMessage(resp)
}

// handleRunUnsubscribe handles run.unsubscribe action.
func (c *Client) handleRunUnsubscribe(msg *ws.Message) {
	var req RunSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return
	}
	if req.RunID == "" {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeValidation, "run_id is required", nil)
		return
	}
	c.hub.UnsubscribeFromRun(c, req.RunID)
	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
		"run_id":  req.RunID,
	})
	c.sendMessage(resp)
}

func (c *Client) handleSystemMetricsSubscribe(msg *ws.Message) {
	c.hub.SubscribeToSystemMetrics(c)
	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
	})
	c.sendMessage(resp)
}
func (c *Client) handleSystemMetricsUnsubscribe(msg *ws.Message) {
	c.hub.UnsubscribeFromSystemMetrics(c)
	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success": true,
	})
	c.sendMessage(resp)
}

// handleSessionUnsubscribe handles session.unsubscribe action
func (c *Client) handleSessionUnsubscribe(msg *ws.Message) {
	var req SessionSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return
	}
	if req.SessionID == "" {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
		return
	}
	if req.ConsumerKind != "" {
		c.handleOrderedSessionUnsubscribe(msg, req)
		return
	}
	c.hub.UnsubscribeFromSession(c, req.SessionID)
	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success":    true,
		"session_id": req.SessionID,
	})
	c.sendMessage(resp)
}

func (c *Client) handleOrderedSessionUnsubscribe(msg *ws.Message, req SessionSubscribeRequest) {
	key, removed := c.removeOrderedSessionSubscription(req)
	if !removed {
		c.sendSessionStreamFailure(msg, req.SessionID, "invalid_request", "ordered subscription not found", false)
		return
	}
	if service := c.hub.pluginConversationService; service != nil {
		if err := service.SessionEvents().ReleaseCursor(key); err != nil {
			c.storeOrderedSessionSubscription(req, key)
			c.sendSessionStreamFailure(msg, req.SessionID, "upstream_failure", "cannot release session cursor", true)
			return
		}
	}
	payload := map[string]any{"success": true, sessionIDPayloadKey: req.SessionID}
	if req.ConsumerKind == orderedConsumerPlugin {
		payload["consumer_id"] = req.ConsumerID
	} else {
		payload["wire_id"] = req.WireID
	}
	response, _ := ws.NewResponse(msg.ID, msg.Action, payload)
	c.sendMessage(response)
}

func (c *Client) removeOrderedSessionSubscription(
	req SessionSubscribeRequest,
) (plugins.SessionDeliveryCursorKey, bool) {
	subscriptionID := req.ConsumerID
	if req.ConsumerKind == orderedConsumerCore {
		subscriptionID = req.WireID
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	byConsumer := c.orderedSessionSubscriptions[req.SessionID]
	key, subscribed := byConsumer[subscriptionID]
	if !subscribed ||
		key.ConsumerKind != req.ConsumerKind ||
		key.ConsumerID != req.ConsumerID ||
		key.WireID != req.WireID ||
		key.PluginID != req.PluginID ||
		key.Generation != req.Generation {
		return plugins.SessionDeliveryCursorKey{}, false
	}
	delete(byConsumer, subscriptionID)
	if len(byConsumer) == 0 {
		delete(c.orderedSessionSubscriptions, req.SessionID)
	}
	return key, true
}

// handleSessionFocus handles session.focus — marks the session as actively
// viewed by this client, lifting backend polling to fast mode for the workspace.
// It intentionally sends only the control acknowledgement. Snapshot/data
// refreshes belong to explicit detail-surface requests and must not be replayed
// on every focus transition.
func (c *Client) handleSessionFocus(msg *ws.Message) {
	var req SessionSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return
	}
	if req.SessionID == "" {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
		return
	}
	if !c.maySubscribeSession(msg, req.SessionID) {
		return
	}
	c.hub.FocusSession(c, req.SessionID)

	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success":    true,
		"session_id": req.SessionID,
	})
	c.sendMessage(resp)
}

// handleSessionDataRefresh explicitly requests a fresh detail snapshot without
// changing subscription or focus state. Keeping this separate from focus
// makes repeated tab activation cheap and keeps focus acknowledgements small.
func (c *Client) handleSessionDataRefresh(msg *ws.Message) {
	var req SessionSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return
	}
	if req.SessionID == "" {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
		return
	}
	if !c.maySubscribeSession(msg, req.SessionID) {
		return
	}

	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success":    true,
		"session_id": req.SessionID,
	})
	c.sendMessage(resp)
	c.sendSessionData(req.SessionID)
}

// handleSessionGitRefresh explicitly requests only a fresh git-status
// snapshot for an already-focused session. Unlike the legacy generic data
// refresh, this does not replay session state, models, commands, or other
// detail-independent data.
func (c *Client) handleSessionGitRefresh(msg *ws.Message) {
	var req SessionSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return
	}
	if req.SessionID == "" {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
		return
	}
	if !c.maySubscribeSession(msg, req.SessionID) {
		return
	}

	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success":    true,
		"session_id": req.SessionID,
	})
	c.sendMessage(resp)
	c.sendSessionGitData(req.SessionID)
}

// handleSessionUnfocus handles session.unfocus — releases the focus mark for
// this client. The session falls back to slow mode (still subscribed) or
// paused (no subscribers), with a debounce to absorb tab churn.
func (c *Client) handleSessionUnfocus(msg *ws.Message) {
	var req SessionSubscribeRequest
	if err := msg.ParsePayload(&req); err != nil {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		return
	}
	if req.SessionID == "" {
		c.sendError(msg.ID, msg.Action, ws.ErrorCodeValidation, "session_id is required", nil)
		return
	}
	c.hub.UnfocusSession(c, req.SessionID)

	resp, _ := ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"success":    true,
		"session_id": req.SessionID,
	})
	c.sendMessage(resp)
}

// sendMessage sends a message to the client
func (c *Client) sendMessage(msg *ws.Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		c.logger.Error("Failed to marshal message", zap.Error(err))
		return
	}
	c.sendControlBytes(data)
}

func (c *Client) sendBytes(data []byte) bool {
	return c.enqueueNotification(newOutboundNotification(data, ""))
}

// sendNotification queues a pre-marshalled notification with its typed action
// so the client scheduler can classify it once without reparsing the envelope.
func (c *Client) sendNotification(data []byte, action string) bool {
	return c.sendNotificationFrame(newOutboundNotification(data, action))
}

func (c *Client) sendNotificationFrame(frame outboundNotification) bool {
	return c.enqueueNotification(frame)
}

// sendControlBytes queues a response/error separately from notifications.
// Keeping this queue independent is enough to prevent a burst of stream
// frames from dropping the ACK for a user action. It remains bounded so a
// stalled peer cannot grow memory without limit.
func (c *Client) sendControlBytes(data []byte) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return false
	}
	if c.controlSend == nil {
		// Keep zero-value/in-package test clients safe while all production
		// clients use the dedicated control queue.
		select {
		case c.send <- data:
			return true
		default:
			if c.logger != nil {
				c.logger.Warn("Client control send buffer full; closing connection")
			}
			c.closeSendLocked()
			return false
		}
	}
	select {
	case c.controlSend <- data:
		return true
	default:
		if c.logger != nil {
			c.logger.Warn("Client control send buffer full; closing connection")
		}
		c.closeSendLocked()
		return false
	}
}

// sendError sends an error message to the client
func (c *Client) sendError(id, action, code, message string, details map[string]interface{}) {
	msg, err := ws.NewError(id, action, code, message, details)
	if err != nil {
		c.logger.Error("Failed to create error message", zap.Error(err))
		return
	}
	c.sendMessage(msg)
}

func (c *Client) closeSend() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closeSendLocked()
}

func (c *Client) closeSendLocked() {
	if c.closed {
		return
	}
	c.closed = true
	if c.send != nil {
		close(c.send)
	}
	if c.controlSend != nil {
		close(c.controlSend)
	}
	if c.notificationWake != nil {
		// The wake channel is deliberately left open: enqueueNotification checks
		// closed while holding c.mu, so closing it here would create a
		// send-on-closed race with a concurrent producer.
		select {
		case c.notificationWake <- struct{}{}:
		default:
		}
	}
}

// WritePump pumps messages from the hub to the WebSocket connection
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		if err := c.conn.Close(); err != nil {
			c.logger.Debug("failed to close websocket connection", zap.Error(err))
		}
	}()

	controlCh := c.controlSend
	semanticCh := c.send
	semanticSinceReplaceable := 0

	writeMessage := func(message []byte) bool {
		if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
			c.logger.Debug("failed to set write deadline", zap.Error(err))
		}
		if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
			c.logger.Debug("failed to write websocket message", zap.Error(err))
			return false
		}
		return true
	}
	writeClose := func() {
		if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
			c.logger.Debug("failed to set write deadline", zap.Error(err))
		}
		if err := c.conn.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
			c.logger.Debug("failed to write close message", zap.Error(err))
		}
	}
	writePing := func() bool {
		if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
			c.logger.Debug("failed to set write deadline", zap.Error(err))
		}
		if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
			c.logger.Debug("failed to write websocket ping", zap.Error(err))
			return false
		}
		return true
	}

	for {
		// Correlated responses/errors always run first. A bounded semantic burst
		// then gives each active session's replaceable queue a turn.
		select {
		case <-ticker.C:
			if !writePing() {
				return
			}
		default:
		}
		if controlCh != nil {
			select {
			case message, ok := <-controlCh:
				if !ok {
					controlCh = nil
					continue
				}
				if !writeMessage(message) {
					return
				}
				continue
			default:
			}
		}

		if semanticSinceReplaceable >= semanticPriorityBurst {
			if frame, ok := c.popNextReplaceable(); ok {
				semanticSinceReplaceable = 0
				if !writeMessage(frame.data) {
					return
				}
				continue
			}
		}

		if semanticCh != nil {
			select {
			case message, ok := <-semanticCh:
				if !ok {
					semanticCh = nil
					continue
				}
				semanticSinceReplaceable++
				if !writeMessage(message) {
					return
				}
				continue
			default:
			}
		}

		if frame, ok := c.popNextReplaceable(); ok {
			semanticSinceReplaceable = 0
			if !writeMessage(frame.data) {
				return
			}
			continue
		}

		if controlCh == nil && semanticCh == nil && !c.hasReplaceable() {
			writeClose()
			return
		}

		c.mu.RLock()
		wakeCh := c.notificationWake
		c.mu.RUnlock()
		select {
		case message, ok := <-controlCh:
			if !ok {
				controlCh = nil
				continue
			}
			if !writeMessage(message) {
				return
			}
		case message, ok := <-semanticCh:
			if !ok {
				semanticCh = nil
				continue
			}
			semanticSinceReplaceable++
			if !writeMessage(message) {
				return
			}
		case <-wakeCh:
		case <-ticker.C:
			if !writePing() {
				return
			}
		}
	}
}
