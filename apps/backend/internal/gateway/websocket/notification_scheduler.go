package websocket

import (
	"encoding/json"
	"slices"

	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

const (
	// replaceablePerSessionCapacity bounds the amount of distinct full-state
	// message updates a single session can hold for one client.
	replaceablePerSessionCapacity = 32
	// replaceableGlobalCapacity bounds all pending replaceable updates for one
	// client. A noisy session can only evict its own oldest queued replacements.
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

type queuedReplaceableKey struct {
	replaceableNotificationKey
	sequence uint64
}

type notificationQueueItemKind uint8

const (
	replaceableQueueItem notificationQueueItemKind = iota
	semanticQueueItem
)

type sessionNotificationQueueItem struct {
	kind  notificationQueueItemKind
	key   queuedReplaceableKey
	frame outboundNotification
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
		c.replaceableByKey = make(map[queuedReplaceableKey]outboundNotification)
	}
	if c.replaceableBySession == nil {
		c.replaceableBySession = make(map[string][]sessionNotificationQueueItem)
	}
	if c.replaceableCurrentByKey == nil {
		c.replaceableCurrentByKey = make(map[replaceableNotificationKey]queuedReplaceableKey)
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
	if c.closed || c.send == nil {
		c.mu.Unlock()
		return false
	}
	c.ensureNotificationSchedulerLocked()
	if frame.sessionID != "" && len(c.replaceableBySession[frame.sessionID]) > 0 {
		accepted := c.enqueueSemanticBarrierLocked(frame)
		c.mu.Unlock()
		if accepted {
			c.signalNotificationWake()
		}
		return accepted
	}
	accepted := c.enqueueSemanticLocked(frame.data)
	c.mu.Unlock()
	return accepted
}

func (c *Client) enqueueSemanticLocked(data []byte) bool {
	if c.send == nil {
		c.droppedSemantic++
		return false
	}
	select {
	case c.send <- data:
		return true
	default:
		c.droppedSemantic++
		if c.logger != nil {
			c.logger.Warn("Client send buffer full")
		}
		return false
	}
}

func (c *Client) enqueueSemanticBarrierLocked(frame outboundNotification) bool {
	if c.scheduledSemantic >= cap(c.send) {
		c.droppedSemantic++
		if c.logger != nil {
			c.logger.Warn("Client semantic queue full; dropping notification",
				queueActionField(frame.action),
				queueSessionField(frame.sessionID))
		}
		return false
	}
	c.replaceableBySession[frame.sessionID] = append(
		c.replaceableBySession[frame.sessionID],
		sessionNotificationQueueItem{kind: semanticQueueItem, frame: frame},
	)
	c.scheduledSemantic++
	c.clearCurrentReplaceableKeysLocked(frame.sessionID)
	if !slices.Contains(c.replaceableSessionOrder, frame.sessionID) {
		c.replaceableSessionOrder = append(c.replaceableSessionOrder, frame.sessionID)
	}
	return true
}

func (c *Client) clearCurrentReplaceableKeysLocked(sessionID string) {
	for key := range c.replaceableCurrentByKey {
		if key.sessionID == sessionID {
			delete(c.replaceableCurrentByKey, key)
		}
	}
}

func (c *Client) enqueueReplaceable(frame outboundNotification, key replaceableNotificationKey) bool {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return false
	}
	c.ensureNotificationSchedulerLocked()
	if queuedKey, exists := c.replaceableCurrentByKey[key]; exists {
		// Keep the key in its current segment position: a full-state update is
		// replaceable, but it must not cross a semantic barrier.
		c.replaceableByKey[queuedKey] = frame
		c.replaceableReplacements++
		depth := len(c.replaceableByKey)
		c.mu.Unlock()
		c.logReplaceablePressure("replacement", frame, depth)
		c.signalNotificationWake()
		return true
	}

	perSessionLimit, globalLimit := c.replaceableCapacitiesLocked()
	queue := c.replaceableBySession[frame.sessionID]
	if c.replaceableSessionDepthLocked(queue) >= perSessionLimit || len(c.replaceableByKey) >= globalLimit {
		// Only this session's oldest queued replacement may be evicted. If
		// the global cap is occupied by other sessions, reject this frame.
		if c.replaceableSessionDepthLocked(queue) == 0 {
			c.replaceableRejected++
			depth := len(c.replaceableByKey)
			c.mu.Unlock()
			c.logReplaceablePressure("rejected", frame, depth)
			return false
		}
		if c.removeOldestReplaceableLocked(frame.sessionID) {
			c.replaceableEvictions++
		}
		queue = c.replaceableBySession[frame.sessionID]
	}
	c.nextReplaceableSequence++
	queuedKey := queuedReplaceableKey{
		replaceableNotificationKey: key,
		sequence:                   c.nextReplaceableSequence,
	}
	c.replaceableByKey[queuedKey] = frame
	if !slices.Contains(c.replaceableSessionOrder, frame.sessionID) {
		c.replaceableSessionOrder = append(c.replaceableSessionOrder, frame.sessionID)
	}
	c.replaceableBySession[frame.sessionID] = append(queue, sessionNotificationQueueItem{
		kind: replaceableQueueItem,
		key:  queuedKey,
	})
	c.replaceableCurrentByKey[key] = queuedKey
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

func (c *Client) replaceableSessionDepthLocked(queue []sessionNotificationQueueItem) int {
	depth := 0
	for _, item := range queue {
		if item.kind == replaceableQueueItem {
			depth++
		}
	}
	return depth
}

func (c *Client) removeOldestReplaceableLocked(sessionID string) bool {
	queue := c.replaceableBySession[sessionID]
	for index, item := range queue {
		if item.kind != replaceableQueueItem {
			continue
		}
		delete(c.replaceableByKey, item.key)
		if current, ok := c.replaceableCurrentByKey[item.key.replaceableNotificationKey]; ok && current == item.key {
			delete(c.replaceableCurrentByKey, item.key.replaceableNotificationKey)
		}
		queue = append(queue[:index], queue[index+1:]...)
		// Keep the session registered in replaceableSessionOrder. The enqueue
		// caller may immediately add another key for this session; popNextReplaceable
		// owns removal from the round-robin order after the queue is truly drained.
		c.replaceableBySession[sessionID] = queue
		return true
	}
	return false
}

func (c *Client) hasReplaceable() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.replaceableByKey) > 0 || c.scheduledSemantic > 0
}

// popNextReplaceable returns one scheduled frame using round-robin session
// scheduling. Semantic barriers live in the same per-session sequence as
// replaceable updates, so a later update cannot overtake the barrier.
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

		item := queue[0]
		c.replaceableBySession[sessionID] = queue[1:]
		if item.kind == semanticQueueItem {
			c.scheduledSemantic--
			if len(queue) == 1 {
				c.removeReplaceableSessionLocked(index, sessionID)
			} else {
				c.replaceableRoundRobin = (index + 1) % len(c.replaceableSessionOrder)
			}
			return item.frame, true
		}

		frame, ok := c.replaceableByKey[item.key]
		delete(c.replaceableByKey, item.key)
		if current, currentOK := c.replaceableCurrentByKey[item.key.replaceableNotificationKey]; currentOK && current == item.key {
			delete(c.replaceableCurrentByKey, item.key.replaceableNotificationKey)
		}
		if len(queue) == 1 {
			c.removeReplaceableSessionLocked(index, sessionID)
		} else {
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
	c.clearCurrentReplaceableKeysLocked(sessionID)
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
	Depth           int
	Replacements    uint64
	Evictions       uint64
	Rejected        uint64
	DroppedSemantic uint64
}

func (c *Client) notificationQueueStats() notificationQueueStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return notificationQueueStats{
		Depth:           len(c.replaceableByKey),
		Replacements:    c.replaceableReplacements,
		Evictions:       c.replaceableEvictions,
		Rejected:        c.replaceableRejected,
		DroppedSemantic: c.droppedSemantic,
	}
}
