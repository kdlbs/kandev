package handlers

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockEventBus is a no-op event bus for handler tests.
type mockEventBus struct{}

func (m *mockEventBus) Publish(_ context.Context, _ string, _ *bus.Event) error {
	return nil
}
func (m *mockEventBus) Subscribe(_ string, _ bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}
func (m *mockEventBus) QueueSubscribe(_, _ string, _ bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}
func (m *mockEventBus) Request(_ context.Context, _ string, _ *bus.Event, _ time.Duration) (*bus.Event, error) {
	return nil, nil
}
func (m *mockEventBus) Close()            {}
func (m *mockEventBus) IsConnected() bool { return true }

func setupQueueHandlers(t *testing.T) (*QueueHandlers, *messagequeue.Service) {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:      "error",
		Format:     "console",
		OutputPath: "stderr",
	})
	require.NoError(t, err)
	svc := messagequeue.NewServiceMemory(log)
	return NewQueueHandlers(svc, &mockEventBus{}, log), svc
}

func createTestMessage(t *testing.T, action string, payload interface{}) *ws.Message {
	t.Helper()
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	return &ws.Message{
		ID:      "msg-1",
		Type:    ws.MessageTypeRequest,
		Action:  action,
		Payload: data,
	}
}

func parseError(t *testing.T, response *ws.Message) ws.ErrorPayload {
	t.Helper()
	var errorPayload ws.ErrorPayload
	require.NoError(t, json.Unmarshal(response.Payload, &errorPayload))
	return errorPayload
}

func TestWsQueueMessage(t *testing.T) {
	t.Run("queues a message", func(t *testing.T) {
		handlers, _ := setupQueueHandlers(t)
		ctx := context.Background()

		msg := createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{
			"session_id": "session-1",
			"task_id":    "task-1",
			"content":    "test message",
			"user_id":    "user-1",
		})

		response, err := handlers.wsQueueMessage(ctx, msg)
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeResponse, response.Type)
	})

	t.Run("rejects missing session_id", func(t *testing.T) {
		handlers, _ := setupQueueHandlers(t)
		response, err := handlers.wsQueueMessage(context.Background(),
			createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{"task_id": "t", "content": "x"}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
		assert.Contains(t, parseError(t, response).Message, "session_id is required")
	})

	t.Run("rejects missing task_id", func(t *testing.T) {
		handlers, _ := setupQueueHandlers(t)
		response, err := handlers.wsQueueMessage(context.Background(),
			createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{"session_id": "s", "content": "x"}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
		assert.Contains(t, parseError(t, response).Message, "task_id is required")
	})

	t.Run("rejects missing content and attachments", func(t *testing.T) {
		handlers, _ := setupQueueHandlers(t)
		response, err := handlers.wsQueueMessage(context.Background(),
			createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{"session_id": "s", "task_id": "t"}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
		assert.Contains(t, parseError(t, response).Message, "content or attachments are required")
	})

	t.Run("returns queue_full when over cap", func(t *testing.T) {
		handlers, svc := setupQueueHandlers(t)
		ctx := context.Background()

		// Saturate the queue.
		for i := 0; i < messagequeue.DefaultMaxPerSession; i++ {
			_, err := svc.QueueMessage(ctx, "s", "t", "x", "", "u", false, nil)
			require.NoError(t, err)
		}

		response, err := handlers.wsQueueMessage(ctx, createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{
			"session_id": "s",
			"task_id":    "t",
			"content":    "overflow",
			"user_id":    "u",
		}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
		errPayload := parseError(t, response)
		assert.Equal(t, "queue_full", errPayload.Code)
		assert.EqualValues(t, messagequeue.DefaultMaxPerSession, errPayload.Details["queue_size"])
		assert.EqualValues(t, messagequeue.DefaultMaxPerSession, errPayload.Details["max"])
	})
}

func TestWsCancelAll(t *testing.T) {
	t.Run("clears the queue", func(t *testing.T) {
		handlers, svc := setupQueueHandlers(t)
		ctx := context.Background()

		for i := 0; i < 3; i++ {
			_, err := svc.QueueMessage(ctx, "s", "t", "x", "", "u", false, nil)
			require.NoError(t, err)
		}

		response, err := handlers.wsCancelAll(ctx,
			createTestMessage(t, ws.ActionMessageQueueCancel, map[string]interface{}{"session_id": "s"}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeResponse, response.Type)

		var payload map[string]interface{}
		require.NoError(t, json.Unmarshal(response.Payload, &payload))
		assert.EqualValues(t, 3, payload["removed"])

		assert.Equal(t, 0, svc.GetStatus(ctx, "s").Count)
	})

	t.Run("rejects missing session_id", func(t *testing.T) {
		handlers, _ := setupQueueHandlers(t)
		response, err := handlers.wsCancelAll(context.Background(),
			createTestMessage(t, ws.ActionMessageQueueCancel, map[string]interface{}{}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
		assert.Contains(t, parseError(t, response).Message, "session_id is required")
	})
}

func TestWsGetQueueStatus(t *testing.T) {
	t.Run("returns ordered list", func(t *testing.T) {
		handlers, svc := setupQueueHandlers(t)
		ctx := context.Background()

		for _, body := range []string{"a", "b", "c"} {
			_, err := svc.QueueMessage(ctx, "s", "t", body, "", "u", false, nil)
			require.NoError(t, err)
		}

		response, err := handlers.wsGetQueueStatus(ctx,
			createTestMessage(t, ws.ActionMessageQueueGet, map[string]interface{}{"session_id": "s"}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeResponse, response.Type)

		var status messagequeue.QueueStatus
		require.NoError(t, json.Unmarshal(response.Payload, &status))
		assert.Equal(t, 3, status.Count)
		assert.Equal(t, "a", status.Entries[0].Content)
		assert.Equal(t, "c", status.Entries[2].Content)
	})

	t.Run("rejects missing session_id", func(t *testing.T) {
		handlers, _ := setupQueueHandlers(t)
		response, err := handlers.wsGetQueueStatus(context.Background(),
			createTestMessage(t, ws.ActionMessageQueueGet, map[string]interface{}{}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
		assert.Contains(t, parseError(t, response).Message, "session_id is required")
	})
}

func TestWsUpdateMessage(t *testing.T) {
	t.Run("updates an entry", func(t *testing.T) {
		handlers, svc := setupQueueHandlers(t)
		ctx := context.Background()

		queued, err := svc.QueueMessage(ctx, "s", "t", "original", "", "u", false, nil)
		require.NoError(t, err)

		response, err := handlers.wsUpdateMessage(ctx,
			createTestMessage(t, ws.ActionMessageQueueUpdate, map[string]interface{}{
				"session_id": "s",
				"entry_id":   queued.ID,
				"content":    "edited",
				"user_id":    "u",
			}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeResponse, response.Type)
		assert.Equal(t, "edited", svc.GetStatus(ctx, "s").Entries[0].Content)
	})

	t.Run("returns entry_not_found when drained", func(t *testing.T) {
		handlers, svc := setupQueueHandlers(t)
		ctx := context.Background()

		queued, _ := svc.QueueMessage(ctx, "s", "t", "x", "", "u", false, nil)
		_, _ = svc.TakeQueued(ctx, "s")

		response, err := handlers.wsUpdateMessage(ctx,
			createTestMessage(t, ws.ActionMessageQueueUpdate, map[string]interface{}{
				"session_id": "s",
				"entry_id":   queued.ID,
				"content":    "edit",
				"user_id":    "u",
			}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
		assert.Equal(t, "entry_not_found", parseError(t, response).Code)
	})

	t.Run("rejects missing session_id", func(t *testing.T) {
		handlers, _ := setupQueueHandlers(t)
		response, err := handlers.wsUpdateMessage(context.Background(),
			createTestMessage(t, ws.ActionMessageQueueUpdate, map[string]interface{}{"entry_id": "e", "content": "x"}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
		assert.Contains(t, parseError(t, response).Message, "session_id is required")
	})

	t.Run("rejects missing entry_id", func(t *testing.T) {
		handlers, _ := setupQueueHandlers(t)
		response, err := handlers.wsUpdateMessage(context.Background(),
			createTestMessage(t, ws.ActionMessageQueueUpdate, map[string]interface{}{"session_id": "s", "content": "x"}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
		assert.Contains(t, parseError(t, response).Message, "entry_id is required")
	})

	t.Run("rejects when content and attachments missing", func(t *testing.T) {
		handlers, _ := setupQueueHandlers(t)
		response, err := handlers.wsUpdateMessage(context.Background(),
			createTestMessage(t, ws.ActionMessageQueueUpdate, map[string]interface{}{"session_id": "s", "entry_id": "e"}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
		assert.Contains(t, parseError(t, response).Message, "content or attachments are required")
	})

	t.Run("rejects user_id impersonating the agent identity", func(t *testing.T) {
		handlers, svc := setupQueueHandlers(t)
		ctx := context.Background()
		queued, _ := svc.QueueMessageWithMetadata(ctx, "s", "t", "agent prompt", "", messagequeue.QueuedByAgent, false, nil, nil)

		response, err := handlers.wsUpdateMessage(ctx,
			createTestMessage(t, ws.ActionMessageQueueUpdate, map[string]interface{}{
				"session_id": "s",
				"entry_id":   queued.ID,
				"content":    "hijack",
				"user_id":    messagequeue.QueuedByAgent,
			}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
		assert.Contains(t, parseError(t, response).Message, "may not impersonate the agent identity")

		// And confirm the agent entry was not overwritten.
		entries := svc.GetStatus(ctx, "s").Entries
		require.Len(t, entries, 1)
		assert.Equal(t, "agent prompt", entries[0].Content)
	})
}

func TestWsRemoveEntry(t *testing.T) {
	t.Run("removes a single entry", func(t *testing.T) {
		handlers, svc := setupQueueHandlers(t)
		ctx := context.Background()

		a, _ := svc.QueueMessage(ctx, "s", "t", "a", "", "u", false, nil)
		_, _ = svc.QueueMessage(ctx, "s", "t", "b", "", "u", false, nil)

		response, err := handlers.wsRemoveEntry(ctx,
			createTestMessage(t, ws.ActionMessageQueueRemove, map[string]interface{}{
				"session_id": "s",
				"entry_id":   a.ID,
			}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeResponse, response.Type)
		status := svc.GetStatus(ctx, "s")
		assert.Equal(t, 1, status.Count)
		assert.Equal(t, "b", status.Entries[0].Content)
	})

	t.Run("returns entry_not_found when missing", func(t *testing.T) {
		handlers, _ := setupQueueHandlers(t)
		response, err := handlers.wsRemoveEntry(context.Background(),
			createTestMessage(t, ws.ActionMessageQueueRemove, map[string]interface{}{
				"session_id": "s",
				"entry_id":   "ghost",
			}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
		assert.Equal(t, "entry_not_found", parseError(t, response).Code)
	})

	t.Run("returns entry_not_found for agent-authored entries", func(t *testing.T) {
		handlers, svc := setupQueueHandlers(t)
		ctx := context.Background()

		queued, err := svc.QueueMessageWithMetadata(ctx, "s", "t", "agent prompt", "", messagequeue.QueuedByAgent, false, nil, nil)
		require.NoError(t, err)

		response, err := handlers.wsRemoveEntry(ctx,
			createTestMessage(t, ws.ActionMessageQueueRemove, map[string]interface{}{
				"session_id": "s",
				"entry_id":   queued.ID,
			}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
		assert.Equal(t, "entry_not_found", parseError(t, response).Code)

		status := svc.GetStatus(ctx, "s")
		assert.Equal(t, 1, status.Count)
		assert.Equal(t, "agent prompt", status.Entries[0].Content)
	})

	t.Run("rejects missing session_id", func(t *testing.T) {
		handlers, _ := setupQueueHandlers(t)
		response, err := handlers.wsRemoveEntry(context.Background(),
			createTestMessage(t, ws.ActionMessageQueueRemove, map[string]interface{}{"entry_id": "e"}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
		assert.Contains(t, parseError(t, response).Message, "session_id is required")
	})
}

func TestWsAppendToQueue(t *testing.T) {
	t.Run("appends to tail when same sender", func(t *testing.T) {
		handlers, svc := setupQueueHandlers(t)
		ctx := context.Background()

		_, err := svc.QueueMessage(ctx, "s", "t", "first", "", "user-1", false, nil)
		require.NoError(t, err)

		response, err := handlers.wsAppendToQueue(ctx,
			createTestMessage(t, ws.ActionMessageQueueAppend, map[string]interface{}{
				"session_id": "s",
				"task_id":    "t",
				"content":    "second",
				"user_id":    "user-1",
			}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeResponse, response.Type)
		assert.Equal(t, 1, svc.GetStatus(ctx, "s").Count)
		assert.Contains(t, svc.GetStatus(ctx, "s").Entries[0].Content, "first\n\n---\n\nsecond")
	})

	t.Run("inserts when queue empty", func(t *testing.T) {
		handlers, svc := setupQueueHandlers(t)
		ctx := context.Background()

		response, err := handlers.wsAppendToQueue(ctx,
			createTestMessage(t, ws.ActionMessageQueueAppend, map[string]interface{}{
				"session_id": "s",
				"task_id":    "t",
				"content":    "fresh",
				"user_id":    "u",
			}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeResponse, response.Type)
		assert.Equal(t, 1, svc.GetStatus(ctx, "s").Count)
	})

	t.Run("rejects missing fields", func(t *testing.T) {
		handlers, _ := setupQueueHandlers(t)
		response, err := handlers.wsAppendToQueue(context.Background(),
			createTestMessage(t, ws.ActionMessageQueueAppend, map[string]interface{}{
				"session_id": "s",
				"task_id":    "t",
				"content":    "",
			}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
		assert.Contains(t, parseError(t, response).Message, "content is required")
	})
}
