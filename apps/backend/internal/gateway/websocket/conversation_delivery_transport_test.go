package websocket

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// The NATS subscriber decodes Event.Data into map[string]any, including nested receipts.
func TestConversationDeliveryPreservesSerializedReceipt(t *testing.T) {
	for _, serialized := range []bool{false, true} {
		name := "memory"
		if serialized {
			name = "json-transport"
		}
		t.Run(name, func(t *testing.T) {
			hub := NewHub(ws.NewDispatcher(), testLogger())
			client := NewClient("transport-client", authn.Identity{UserID: "user-1"}, nil, hub, testLogger())
			hub.clients[client] = true
			client.conversationSubscriptions["scope-1"] = conversationSubscription{
				ScopeID: "scope-1", SessionID: "session-1", ConsumerKind: conversationConsumerCore,
				UserID: "user-1", Epoch: "epoch-1",
			}
			broadcaster := &TaskEventBroadcaster{hub: hub, logger: testLogger()}
			for revision := int64(1); revision <= 100; revision++ {
				message := &models.Message{
					ID: "message-1", TaskID: "task-1", TaskSessionID: "session-1",
					AuthorType: models.MessageAuthorAgent, Type: models.MessageTypeMessage, Content: "stream update",
				}
				event := &bus.Event{Type: events.MessageUpdated, Data: map[string]any{
					"session_id": "session-1", "task_id": "task-1", "message": message,
					"conversation_receipt": &models.ConversationMutationReceipt{
						SessionID: "session-1", BaseRevision: revision - 1, Revision: revision, Complete: true,
						Operations: []models.ConversationMutationOperation{{
							Kind: models.ConversationMutationUpsert, Entity: models.ConversationEntityMessage,
							ID: message.ID, TaskID: message.TaskID, SessionID: message.TaskSessionID,
							AuthorType: string(message.AuthorType), Message: message,
						}},
					},
				}}
				if serialized {
					encoded, err := json.Marshal(event)
					require.NoError(t, err)
					event = &bus.Event{}
					require.NoError(t, json.Unmarshal(encoded, event))
				}
				require.NoError(t, broadcaster.broadcastEvent(context.Background(), event, ws.ActionSessionMessageUpdated))
				var frame ws.Message
				require.NoError(t, json.Unmarshal(<-client.send, &frame))
				require.Equal(t, ws.ActionSessionConversationChanged, frame.Action)
				var payload conversationChangedPayload
				require.NoError(t, json.Unmarshal(frame.Payload, &payload))
				require.False(t, payload.Reset, "contiguous revision %d must update by ID", revision)
				require.Equal(t, formatRevision(revision-1), payload.BaseRevision)
				require.Equal(t, formatRevision(revision), payload.Revision)
				require.Len(t, payload.Operations, 1)
				require.Equal(t, message.ID, payload.Operations[0].ID)
			}
		})
	}
}
