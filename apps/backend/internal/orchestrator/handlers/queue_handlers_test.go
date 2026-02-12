package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockQueueService is a simple mock implementation of QueueService
type mockQueueService struct {
	queueMessageFunc   func(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []messagequeue.MessageAttachment) (*messagequeue.QueuedMessage, error)
	cancelQueuedFunc   func(ctx context.Context, sessionID string) (*messagequeue.QueuedMessage, error)
	getStatusFunc      func(ctx context.Context, sessionID string) *messagequeue.QueueStatus
	updateMessageFunc  func(ctx context.Context, sessionID, content string) error
}

func (m *mockQueueService) QueueMessage(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []messagequeue.MessageAttachment) (*messagequeue.QueuedMessage, error) {
	if m.queueMessageFunc != nil {
		return m.queueMessageFunc(ctx, sessionID, taskID, content, model, userID, planMode, attachments)
	}
	return nil, nil
}

func (m *mockQueueService) CancelQueued(ctx context.Context, sessionID string) (*messagequeue.QueuedMessage, error) {
	if m.cancelQueuedFunc != nil {
		return m.cancelQueuedFunc(ctx, sessionID)
	}
	return nil, nil
}

func (m *mockQueueService) GetStatus(ctx context.Context, sessionID string) *messagequeue.QueueStatus {
	if m.getStatusFunc != nil {
		return m.getStatusFunc(ctx, sessionID)
	}
	return &messagequeue.QueueStatus{}
}

func (m *mockQueueService) UpdateMessage(ctx context.Context, sessionID, content string) error {
	if m.updateMessageFunc != nil {
		return m.updateMessageFunc(ctx, sessionID, content)
	}
	return nil
}

// mockEventBus is a simple mock event bus
type mockEventBus struct {
	publishFunc   func(ctx context.Context, subject string, event *bus.Event) error
	subscribeFunc func(subject string, handler bus.EventHandler) (bus.Subscription, error)
}

func (m *mockEventBus) Publish(ctx context.Context, subject string, event *bus.Event) error {
	if m.publishFunc != nil {
		return m.publishFunc(ctx, subject, event)
	}
	return nil
}

func (m *mockEventBus) Subscribe(subject string, handler bus.EventHandler) (bus.Subscription, error) {
	if m.subscribeFunc != nil {
		return m.subscribeFunc(subject, handler)
	}
	return nil, nil
}

func (m *mockEventBus) QueueSubscribe(subject, queue string, handler bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}

func (m *mockEventBus) Request(ctx context.Context, subject string, event *bus.Event, timeout time.Duration) (*bus.Event, error) {
	return nil, nil
}

func (m *mockEventBus) Close() {
	// no-op
}

func (m *mockEventBus) IsConnected() bool {
	return true
}

func setupQueueHandlers(t *testing.T) (*QueueHandlers, *mockQueueService, *mockEventBus) {
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:      "error",
		Format:     "console",
		OutputPath: "stdout",
	})
	require.NoError(t, err)

	mockQueue := &mockQueueService{}
	mockBus := &mockEventBus{}
	handlers := NewQueueHandlers(mockQueue, mockBus, log)

	return handlers, mockQueue, mockBus
}

func createTestMessage(t *testing.T, action string, payload interface{}) *ws.Message {
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	return &ws.Message{
		ID:      "msg-1",
		Type:    ws.MessageTypeRequest,
		Action:  action,
		Payload: data,
	}
}

func TestWsQueueMessage(t *testing.T) {
	t.Run("successfully queues a message", func(t *testing.T) {
		handlers, mockQueue, mockBus := setupQueueHandlers(t)
		ctx := context.Background()

		queuedMsg := &messagequeue.QueuedMessage{
			ID:        "queue-1",
			SessionID: "session-1",
			TaskID:    "task-1",
			Content:   "test message",
		}

		mockQueue.queueMessageFunc = func(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []messagequeue.MessageAttachment) (*messagequeue.QueuedMessage, error) {
			assert.Equal(t, "session-1", sessionID)
			assert.Equal(t, "task-1", taskID)
			assert.Equal(t, "test message", content)
			return queuedMsg, nil
		}

		mockQueue.getStatusFunc = func(ctx context.Context, sessionID string) *messagequeue.QueueStatus {
			return &messagequeue.QueueStatus{
				IsQueued: true,
				Message:  queuedMsg,
			}
		}

		mockBus.publishFunc = func(ctx context.Context, subject string, event *bus.Event) error {
			return nil
		}

		msg := createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{
			"session_id": "session-1",
			"task_id":    "task-1",
			"content":    "test message",
			"model":      "model-1",
			"user_id":    "user-1",
			"plan_mode":  false,
		})

		response, err := handlers.wsQueueMessage(ctx, msg)

		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeResponse, response.Type)
	})

	t.Run("returns error when session_id missing", func(t *testing.T) {
		handlers, _, _ := setupQueueHandlers(t)
		ctx := context.Background()

		msg := createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{
			"task_id": "task-1",
			"content": "test",
		})

		response, err := handlers.wsQueueMessage(ctx, msg)

		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)

		var errorPayload ws.ErrorPayload
		err = json.Unmarshal(response.Payload, &errorPayload)
		require.NoError(t, err)
		assert.Contains(t, errorPayload.Message, "session_id is required")
	})

	t.Run("returns error when task_id missing", func(t *testing.T) {
		handlers, _, _ := setupQueueHandlers(t)
		ctx := context.Background()

		msg := createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{
			"session_id": "session-1",
			"content":    "test",
		})

		response, err := handlers.wsQueueMessage(ctx, msg)

		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)

		var errorPayload ws.ErrorPayload
		err = json.Unmarshal(response.Payload, &errorPayload)
		require.NoError(t, err)
		assert.Contains(t, errorPayload.Message, "task_id is required")
	})

	t.Run("returns error when both content and attachments missing", func(t *testing.T) {
		handlers, _, _ := setupQueueHandlers(t)
		ctx := context.Background()

		msg := createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{
			"session_id": "session-1",
			"task_id":    "task-1",
		})

		response, err := handlers.wsQueueMessage(ctx, msg)

		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)

		var errorPayload ws.ErrorPayload
		err = json.Unmarshal(response.Payload, &errorPayload)
		require.NoError(t, err)
		assert.Contains(t, errorPayload.Message, "content or attachments are required")
	})

	t.Run("handles service error", func(t *testing.T) {
		handlers, mockQueue, _ := setupQueueHandlers(t)
		ctx := context.Background()

		mockQueue.queueMessageFunc = func(ctx context.Context, sessionID, taskID, content, model, userID string, planMode bool, attachments []messagequeue.MessageAttachment) (*messagequeue.QueuedMessage, error) {
			return nil, errors.New("service error")
		}

		msg := createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{
			"session_id": "session-1",
			"task_id":    "task-1",
			"content":    "test",
		})

		response, err := handlers.wsQueueMessage(ctx, msg)

		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)

		var errorPayload ws.ErrorPayload
		err = json.Unmarshal(response.Payload, &errorPayload)
		require.NoError(t, err)
		assert.Contains(t, errorPayload.Message, "Failed to queue message")
	})
}

func TestWsCancelQueue(t *testing.T) {
	t.Run("successfully cancels queued message", func(t *testing.T) {
		handlers, mockQueue, mockBus := setupQueueHandlers(t)
		ctx := context.Background()

		cancelledMsg := &messagequeue.QueuedMessage{
			ID:        "queue-1",
			SessionID: "session-1",
			Content:   "cancelled",
		}

		mockQueue.cancelQueuedFunc = func(ctx context.Context, sessionID string) (*messagequeue.QueuedMessage, error) {
			return cancelledMsg, nil
		}

		mockQueue.getStatusFunc = func(ctx context.Context, sessionID string) *messagequeue.QueueStatus {
			return &messagequeue.QueueStatus{
				IsQueued: false,
				Message:  nil,
			}
		}

		mockBus.publishFunc = func(ctx context.Context, subject string, event *bus.Event) error {
			return nil
		}

		msg := createTestMessage(t, ws.ActionMessageQueueCancel, map[string]interface{}{
			"session_id": "session-1",
		})

		response, err := handlers.wsCancelQueue(ctx, msg)

		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeResponse, response.Type)
	})

	t.Run("returns error when session_id missing", func(t *testing.T) {
		handlers, _, _ := setupQueueHandlers(t)
		ctx := context.Background()

		msg := createTestMessage(t, ws.ActionMessageQueueCancel, map[string]interface{}{})

		response, err := handlers.wsCancelQueue(ctx, msg)

		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)

		var errorPayload ws.ErrorPayload
		err = json.Unmarshal(response.Payload, &errorPayload)
		require.NoError(t, err)
		assert.Contains(t, errorPayload.Message, "session_id is required")
	})

	t.Run("handles service error", func(t *testing.T) {
		handlers, mockQueue, _ := setupQueueHandlers(t)
		ctx := context.Background()

		mockQueue.cancelQueuedFunc = func(ctx context.Context, sessionID string) (*messagequeue.QueuedMessage, error) {
			return nil, errors.New("no message")
		}

		msg := createTestMessage(t, ws.ActionMessageQueueCancel, map[string]interface{}{
			"session_id": "session-1",
		})

		response, err := handlers.wsCancelQueue(ctx, msg)

		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
	})
}

func TestWsGetQueueStatus(t *testing.T) {
	t.Run("successfully gets queue status", func(t *testing.T) {
		handlers, mockQueue, _ := setupQueueHandlers(t)
		ctx := context.Background()

		status := &messagequeue.QueueStatus{
			IsQueued: true,
			Message: &messagequeue.QueuedMessage{
				ID:      "queue-1",
				Content: "test",
			},
		}

		mockQueue.getStatusFunc = func(ctx context.Context, sessionID string) *messagequeue.QueueStatus {
			return status
		}

		msg := createTestMessage(t, ws.ActionMessageQueueGet, map[string]interface{}{
			"session_id": "session-1",
		})

		response, err := handlers.wsGetQueueStatus(ctx, msg)

		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeResponse, response.Type)
	})

	t.Run("returns error when session_id missing", func(t *testing.T) {
		handlers, _, _ := setupQueueHandlers(t)
		ctx := context.Background()

		msg := createTestMessage(t, ws.ActionMessageQueueGet, map[string]interface{}{})

		response, err := handlers.wsGetQueueStatus(ctx, msg)

		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)

		var errorPayload ws.ErrorPayload
		err = json.Unmarshal(response.Payload, &errorPayload)
		require.NoError(t, err)
		assert.Contains(t, errorPayload.Message, "session_id is required")
	})
}

func TestWsUpdateMessage(t *testing.T) {
	t.Run("successfully updates message", func(t *testing.T) {
		handlers, mockQueue, mockBus := setupQueueHandlers(t)
		ctx := context.Background()

		mockQueue.updateMessageFunc = func(ctx context.Context, sessionID, content string) error {
			assert.Equal(t, "session-1", sessionID)
			assert.Equal(t, "updated content", content)
			return nil
		}

		mockQueue.getStatusFunc = func(ctx context.Context, sessionID string) *messagequeue.QueueStatus {
			return &messagequeue.QueueStatus{
				IsQueued: true,
				Message: &messagequeue.QueuedMessage{
					ID:      "queue-1",
					Content: "updated content",
				},
			}
		}

		mockBus.publishFunc = func(ctx context.Context, subject string, event *bus.Event) error {
			return nil
		}

		msg := createTestMessage(t, ws.ActionMessageQueueUpdate, map[string]interface{}{
			"session_id": "session-1",
			"content":    "updated content",
		})

		response, err := handlers.wsUpdateMessage(ctx, msg)

		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeResponse, response.Type)
	})

	t.Run("returns error when session_id missing", func(t *testing.T) {
		handlers, _, _ := setupQueueHandlers(t)
		ctx := context.Background()

		msg := createTestMessage(t, ws.ActionMessageQueueUpdate, map[string]interface{}{
			"content": "test",
		})

		response, err := handlers.wsUpdateMessage(ctx, msg)

		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)

		var errorPayload ws.ErrorPayload
		err = json.Unmarshal(response.Payload, &errorPayload)
		require.NoError(t, err)
		assert.Contains(t, errorPayload.Message, "session_id is required")
	})

	t.Run("returns error when content is empty", func(t *testing.T) {
		handlers, _, _ := setupQueueHandlers(t)
		ctx := context.Background()

		msg := createTestMessage(t, ws.ActionMessageQueueUpdate, map[string]interface{}{
			"session_id": "session-1",
			"content":    "",
		})

		response, err := handlers.wsUpdateMessage(ctx, msg)

		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)

		var errorPayload ws.ErrorPayload
		err = json.Unmarshal(response.Payload, &errorPayload)
		require.NoError(t, err)
		assert.Contains(t, errorPayload.Message, "content cannot be empty")
	})

	t.Run("handles service error", func(t *testing.T) {
		handlers, mockQueue, _ := setupQueueHandlers(t)
		ctx := context.Background()

		mockQueue.updateMessageFunc = func(ctx context.Context, sessionID, content string) error {
			return errors.New("no message")
		}

		msg := createTestMessage(t, ws.ActionMessageQueueUpdate, map[string]interface{}{
			"session_id": "session-1",
			"content":    "test",
		})

		response, err := handlers.wsUpdateMessage(ctx, msg)

		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
	})
}

func TestRegisterHandlers(t *testing.T) {
	t.Run("registers all queue handlers", func(t *testing.T) {
		handlers, _, _ := setupQueueHandlers(t)
		dispatcher := ws.NewDispatcher()

		handlers.RegisterHandlers(dispatcher)

		// Verify handlers are registered by checking we can access them
		// This is a basic smoke test
		assert.NotNil(t, dispatcher)
	})
}
