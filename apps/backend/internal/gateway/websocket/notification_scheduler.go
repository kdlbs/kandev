package websocket

import (
	"encoding/json"

	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

const (
	// replaceablePerSessionCapacity bounds the amount of distinct full-state
	// message updates a single session can hold for one client.
	replaceablePerSessionCapacity = 32
	// replaceableGlobalCapacity bounds all pending replaceable updates for one
	// client. A noisy session can only evict its own obsolete replacements.
	replaceableGlobalCapacity = 256
	// semanticPriorityBurst prevents a busy semantic queue from starving the
	// replaceable queues while still giving responses/control frames priority.
	semanticPriorityBurst = 8
)

type outboundNotification struct {
	data      []byte
	action    string
	sessionID string
	messageID string
}

type replaceableNotificationKey struct {
	sessionID string
	messageID string
}

func (n outboundNotification) replaceableKey() (replaceableNotificationKey, bool) {
	if n.action != ws.ActionSessionMessageUpdated || n.sessionID == "" || n.messageID == "" {
		return replaceableNotificationKey{}, false
	}
	return replaceableNotificationKey{sessionID: n.sessionID, messageID: n.messageID}, true
}

// newOutboundNotification classifies one already-marshalled websocket frame.
// Hub broadcasts pass the action directly so the envelope is not reparsed for
// routing, while direct snapshots and tests can still use sendBytes safely.
func newOutboundNotification(data []byte, action string) outboundNotification {
	frame := outboundNotification{data: data, action: action}
	var envelope struct {
		Action  string          `json:"action"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return frame
	}
	if frame.action == "" {
		frame.action = envelope.Action
	}
	if len(envelope.Payload) == 0 {
		return frame
	}
	var identity struct {
		SessionID string `json:"session_id"`
		MessageID string `json:"message_id"`
	}
	if err := json.Unmarshal(envelope.Payload, &identity); err != nil {
		return frame
	}
	frame.sessionID = identity.SessionID
	frame.messageID = identity.MessageID
	return frame
}

func (c *Client) ensureNotificationSchedulerLocked() {
	if c.replaceableByKey == nil {
		c.replaceableByKey = make(map[replaceableNotificationKey]outboundNotification)
	}
	if c.replaceableBySession == nil {
		c.replaceableBySession = make(map[string][]replaceableNotificationKey)
	}
	if c.deferredSemantic == nil {
		c.deferredSemantic = make(map[string][]outboundNotification)
	}
	if c.notificationWake == nil {
		c.notificationWake = make(chan struct{}, 1)
	}
}

func (c *Client) signalNotificationWake() {
	c.mu.RLock()
	wake := c.notificationWake
	c.mu.RUnlock()
	if wake == nil {
		return
	}
	select {
	case wake <- struct{}{}:
	default:
	}
}

func (c *Client) enqueueNotification(frame outboundNotification) bool {
	key, replaceable := frame.replaceableKey()
	if replaceable {
		return c.enqueueReplaceable(frame, key)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed || c.send == nil {
		return false
	}
	c.ensureNotificationSchedulerLocked()
	if frame.sessionID != "" && len(c.replaceableBySession[frame.sessionID]) > 0 {
		return c.deferSemanticLocked(frame)
	}
	return c.enqueueSemanticLocked(frame.data)
}

func (c *Client) enqueueSemanticLocked(data []byte) bool {
	select {
	case c.send <- data:
		return true
	default:
		if c.logger != nil {
			c.logger.Warn("Client send buffer full")
		}
		return false
	}
}

func (c *Client) deferSemanticLocked(frame outboundNotification) bool {
	if c.deferredSemanticCountLocked() >= cap(c.send) {
		if c.logger != nil {
			c.logger.Warn("Client semantic queue full; dropping notification",
				queueActionField(frame.action),
				queueSessionField(frame.sessionID))
		}
		return false
	}
	c.deferredSemantic[frame.sessionID] = append(c.deferredSemantic[frame.sessionID], frame)
	return true
}

func (c *Client) deferredSemanticCountLocked() int {
	total := 0
	for _, frames := range c.deferredSemantic {
		total += len(frames)
	}
	return total
}

func (c *Client) enqueueReplaceable(frame outboundNotification, key replaceableNotificationKey) bool {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return false
	}
	c.ensureNotificationSchedulerLocked()
	if _, exists := c.replaceableByKey[key]; exists {
		// Keep the key in its original queue position: a full-state update is
		// replaceable, but it must not overtake a later semantic barrier.
		c.replaceableByKey[key] = frame
		c.replaceableReplacements++
		depth := len(c.replaceableByKey)
		c.mu.Unlock()
		c.logReplaceablePressure("replacement", frame, depth)
		c.signalNotificationWake()
		return true
	}

	perSessionLimit, globalLimit := c.replaceableCapacitiesLocked()
	queue := c.replaceableBySession[frame.sessionID]
	if len(queue) >= perSessionLimit || len(c.replaceableByKey) >= globalLimit {
		// Only this session's oldest obsolete replacement may be evicted. If
		// the global cap is occupied by other sessions, reject this frame.
		if len(queue) == 0 {
			c.replaceableRejected++
			depth := len(c.replaceableByKey)
			c.mu.Unlock()
			c.logReplaceablePressure("rejected", frame, depth)
			return false
		}
		c.removeOldestReplaceableLocked(frame.sessionID)
		c.replaceableEvictions++
		queue = c.replaceableBySession[frame.sessionID]
	}
	c.replaceableByKey[key] = frame
	if len(queue) == 0 {
		c.replaceableSessionOrder = append(c.replaceableSessionOrder, frame.sessionID)
	}
	c.replaceableBySession[frame.sessionID] = append(queue, key)
	depth := len(c.replaceableByKey)
	c.mu.Unlock()
	c.logReplaceablePressure("queued", frame, depth)
	c.signalNotificationWake()
	return true
}

func (c *Client) replaceableCapacitiesLocked() (int, int) {
	perSessionLimit := c.replaceablePerSessionLimit
	if perSessionLimit <= 0 {
		perSessionLimit = replaceablePerSessionCapacity
	}
	globalLimit := c.replaceableGlobalLimit
	if globalLimit <= 0 {
		globalLimit = replaceableGlobalCapacity
	}
	return perSessionLimit, globalLimit
}

func (c *Client) removeOldestReplaceableLocked(sessionID string) bool {
	queue := c.replaceableBySession[sessionID]
	if len(queue) == 0 {
		return false
	}
	key := queue[0]
	delete(c.replaceableByKey, key)
	if len(queue) == 1 {
		delete(c.replaceableBySession, sessionID)
	} else {
		c.replaceableBySession[sessionID] = queue[1:]
	}
	return true
}

func (c *Client) hasReplaceable() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.replaceableByKey) > 0
}

// popNextReplaceable returns one full-state frame using round-robin session
// scheduling. It also releases semantic barriers after the final replacement
// for that session has been removed.
func (c *Client) popNextReplaceable() (outboundNotification, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for len(c.replaceableSessionOrder) > 0 {
		if c.replaceableRoundRobin >= len(c.replaceableSessionOrder) {
			c.replaceableRoundRobin = 0
		}
		index := c.replaceableRoundRobin
		sessionID := c.replaceableSessionOrder[index]
		queue := c.replaceableBySession[sessionID]
		if len(queue) == 0 {
			c.removeReplaceableSessionLocked(index, sessionID)
			continue
		}

		key := queue[0]
		frame, ok := c.replaceableByKey[key]
		delete(c.replaceableByKey, key)
		if len(queue) == 1 {
			delete(c.replaceableBySession, sessionID)
			c.removeReplaceableSessionLocked(index, sessionID)
			c.releaseDeferredSemanticLocked(sessionID)
		} else {
			c.replaceableBySession[sessionID] = queue[1:]
			c.replaceableRoundRobin = (index + 1) % len(c.replaceableSessionOrder)
		}
		if !ok {
			continue
		}
		return frame, true
	}
	return outboundNotification{}, false
}

func (c *Client) removeReplaceableSessionLocked(index int, sessionID string) {
	delete(c.replaceableBySession, sessionID)
	c.replaceableSessionOrder = append(c.replaceableSessionOrder[:index], c.replaceableSessionOrder[index+1:]...)
	if len(c.replaceableSessionOrder) == 0 {
		c.replaceableRoundRobin = 0
		return
	}
	if index >= len(c.replaceableSessionOrder) {
		c.replaceableRoundRobin = 0
	} else {
		c.replaceableRoundRobin = index
	}
}

func (c *Client) releaseDeferredSemanticLocked(sessionID string) {
	frames := c.deferredSemantic[sessionID]
	if len(frames) == 0 {
		delete(c.deferredSemantic, sessionID)
		return
	}
	delete(c.deferredSemantic, sessionID)
	if c.closed || c.send == nil {
		return
	}
	for _, frame := range frames {
		if !c.enqueueSemanticLocked(frame.data) && c.logger != nil {
			c.logger.Warn("Client semantic barrier queue full; dropping notification",
				queueActionField(frame.action),
				queueSessionField(frame.sessionID))
		}
	}
}

func (c *Client) logReplaceablePressure(reason string, frame outboundNotification, depth int) {
	if c.logger == nil {
		return
	}
	c.logger.Debug("replaceable websocket queue update",
		queueActionField(frame.action),
		queueSessionField(frame.sessionID),
		queueMessageField(frame.messageID),
		queueReasonField(reason),
		queueDepthField(depth))
}

func queueActionField(action string) zap.Field {
	return zap.String("action", action)
}

func queueSessionField(sessionID string) zap.Field {
	return zap.String("session_id", sessionID)
}

func queueMessageField(messageID string) zap.Field {
	return zap.String("message_id", messageID)
}

func queueReasonField(reason string) zap.Field {
	return zap.String("reason", reason)
}

func queueDepthField(depth int) zap.Field {
	return zap.Int("replaceable_queue_depth", depth)
}

// notificationQueueStats intentionally exposes only queue metadata. Message
// content never enters pressure logs or metrics.
type notificationQueueStats struct {
	Depth        int
	Replacements uint64
	Evictions    uint64
	Rejected     uint64
}

func (c *Client) notificationQueueStats() notificationQueueStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return notificationQueueStats{
		Depth:        len(c.replaceableByKey),
		Replacements: c.replaceableReplacements,
		Evictions:    c.replaceableEvictions,
		Rejected:     c.replaceableRejected,
	}
}
