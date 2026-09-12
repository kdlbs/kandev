package websocket

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/sysprompt"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

const (
	orderedMessageAddedEvent   = "message.added"
	orderedMessageUpdatedEvent = "message.updated"
	orderedMessageDeletedEvent = "message.deleted"
	orderedTurnStartedEvent    = "session.turn.started"
	orderedTurnCompletedEvent  = "session.turn.completed"
	orderedTurnRemovedEvent    = "session.turn.removed"
	orderedSessionRemovedEvent = "session.removed"
)

var orderedEventTypeByAction = map[string]string{
	ws.ActionSessionMessageAdded:   orderedMessageAddedEvent,
	ws.ActionSessionMessageUpdated: orderedMessageUpdatedEvent,
	ws.ActionSessionMessageDeleted: orderedMessageDeletedEvent,
	ws.ActionSessionTurnStarted:    orderedTurnStartedEvent,
	ws.ActionSessionTurnCompleted:  orderedTurnCompletedEvent,
	ws.ActionSessionTurnRemoved:    orderedTurnRemovedEvent,
	ws.ActionSessionRemoved:        orderedSessionRemovedEvent,
}

const orderedContentPayloadKey = "content"

//nolint:cyclop // This boundary coordinates journal sync, poison recording, and live fan-out.
func (h *Hub) appendAndBroadcastOrderedSessionEvent(sessionID string, message *ws.Message) {
	h.orderedSessionMu.Lock()
	defer h.orderedSessionMu.Unlock()
	service := h.pluginConversationService
	eventType := orderedEventTypeByAction[message.Action]
	if service == nil || eventType == "" {
		return
	}
	if service.HasConversationJournal() {
		events, err := service.SyncCommittedSessionEvents(context.Background(), sessionID)
		if err != nil {
			if h.logger != nil {
				h.logger.Error("mirror committed ordered session events", zap.Error(err))
			}
			return
		}
		for _, event := range events {
			h.broadcastCommittedOrderedSessionEvent(service, event)
		}
		return
	}
	var source map[string]any
	if err := json.Unmarshal(message.Payload, &source); err != nil {
		source = map[string]any{"raw": message.Payload}
	}
	payload := sanitizedOrderedSessionPayload(eventType, source)
	encoded, err := json.Marshal(payload)
	if err != nil {
		return
	}
	var taskID *string
	if value, ok := payload["task_id"].(string); ok && value != "" {
		taskID = &value
	}
	event, err := service.SessionEvents().Append(sessionID, taskID, eventType, encoded)
	if err != nil {
		if h.logger != nil {
			h.logger.Error("append ordered session event", zap.Error(err))
		}
		return
	}
	h.deliverOrderedSessionEvent(service, event, h.orderedSessionRecipients(sessionID))
}

// broadcastCommittedOrderedSessionEvent fans a mirrored primary-journal event
// out through the same atomic delivery claim as locally appended events.
func (h *Hub) broadcastCommittedOrderedSessionEvent(service *plugins.Service, event plugins.SessionEvent) {
	h.deliverOrderedSessionEvent(service, event, h.orderedSessionRecipients(event.SessionID))
}

func (h *Hub) deliverOrderedSessionEvent(
	service *plugins.Service,
	event plugins.SessionEvent,
	recipients []*Client,
) {
	if len(recipients) == 0 {
		return
	}
	claim, err := service.SessionDelivery().Claim(event.SessionID, event.ID, time.Now().UTC())
	if err != nil {
		if h.logger != nil {
			h.logger.Error("claim ordered session poison", zap.String("event_id", event.ID), zap.Error(err))
		}
		return
	}
	if claim.Disposition != plugins.SessionDeliveryUntracked &&
		claim.Disposition != plugins.SessionDeliveryClaimed {
		return
	}
	queued := false
	for _, client := range recipients {
		queued = client.sendOrderedSessionEvent(event) || queued
	}
	if claim.Disposition == plugins.SessionDeliveryClaimed {
		if err := service.SessionDelivery().Complete(claim, queued, time.Now().UTC()); err != nil && h.logger != nil {
			h.logger.Error("complete ordered session poison", zap.String("event_id", event.ID), zap.Error(err))
		}
	}
}

func sanitizedOrderedSessionPayload(eventType string, source map[string]any) map[string]any {
	payload := map[string]any{eventTypePayloadKey: eventType}
	keys := []string{sessionIDPayloadKey, taskIDPayloadKey}
	switch eventType {
	case orderedMessageAddedEvent, orderedMessageUpdatedEvent:
		keys = append(keys,
			"message_id", "turn_id", "author_type", orderedContentPayloadKey,
			"created_at", "updated_at", "prompt_index", "requests_input",
		)
		if messageType, exists := source[eventTypePayloadKey]; exists {
			payload["message_type"] = messageType
		}
		if metadata, ok := source["metadata"].(map[string]any); ok {
			if sanitized := plugins.SanitizeConversationMessageMetadata(metadata); len(sanitized) > 0 {
				payload["metadata"] = sanitized
			}
		}
		if senderTaskID, ok := source["sender_task_id"].(string); ok && senderTaskID != "" {
			payload["sender_task_id"] = senderTaskID
			if metadata, ok := payload["metadata"].(map[string]any); !ok {
				payload["metadata"] = map[string]any{"sender_task_id": senderTaskID}
			} else {
				metadata["sender_task_id"] = senderTaskID
			}
		}
	case orderedMessageDeletedEvent:
		keys = append(keys, "message_id")
	case orderedTurnStartedEvent, orderedTurnCompletedEvent, orderedTurnRemovedEvent:
		keys = append(keys, "id", "started_at", "completed_at", "updated_at", "had_output")
	}
	for _, key := range keys {
		if value, exists := source[key]; exists {
			if key == orderedContentPayloadKey {
				if content, ok := value.(string); ok {
					value = sysprompt.StripSystemContent(content)
				}
			}
			payload[key] = value
		}
	}
	return payload
}

func (h *Hub) orderedSessionRecipients(sessionID string) []*Client {
	h.mu.RLock()
	clients := make([]*Client, 0)
	for client := range h.clients {
		if client.hasOrderedSessionSubscription(sessionID) {
			clients = append(clients, client)
		}
	}
	h.mu.RUnlock()
	// Re-authorize at fanout time, mirroring the legacy session recipients: a
	// client whose workspace/session membership was revoked after subscribing
	// must not keep receiving ordered frames. Denied clients lose the ordered
	// subscription itself AND its durable cursors (same revocation semantics
	// as the legacy path), so the partition stops being pinned by them and
	// retention can reclaim it.
	allowed, denied := h.partitionAuthorized(clients, sessionID, h.authPolicy.Subscriptions.Session)
	service := h.pluginConversationService
	for _, client := range denied {
		client.mu.Lock()
		byConsumer := client.orderedSessionSubscriptions[sessionID]
		if service != nil {
			for _, key := range byConsumer {
				if err := service.SessionEvents().ReleaseCursor(key); err != nil && h.logger != nil {
					h.logger.Warn(
						"release revoked ordered session cursor",
						zap.String("session_id", sessionID),
						zap.Error(err),
					)
				}
			}
		}
		delete(client.orderedSessionSubscriptions, sessionID)
		client.mu.Unlock()
	}
	return allowed
}

func (c *Client) hasOrderedSessionSubscription(sessionID string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.orderedSessionSubscriptions[sessionID]) > 0
}

func sessionEventFrame(event plugins.SessionEvent) ([]byte, error) {
	return json.Marshal(map[string]any{
		eventTypePayloadKey: "session.event", "protocol_version": event.ProtocolVersion,
		"event_type": event.EventType, sessionIDPayloadKey: event.SessionID,
		taskIDPayloadKey: event.TaskID, eventSequencePayloadKey: event.Sequence,
		"event_id": event.ID, "payload": event.Payload,
	})
}
