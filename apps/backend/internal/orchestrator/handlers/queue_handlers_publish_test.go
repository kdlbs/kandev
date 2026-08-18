package handlers

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// capturingQueueEventBus records the last message.queue.status_changed payload
// so tests can assert the task_id enrichment on the published event.
type capturingQueueEventBus struct {
	lastData map[string]interface{}
}

func (m *capturingQueueEventBus) Publish(_ context.Context, _ string, event *bus.Event) error {
	if data, ok := event.Data.(map[string]interface{}); ok {
		m.lastData = data
	}
	return nil
}
func (m *capturingQueueEventBus) Subscribe(_ string, _ bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}
func (m *capturingQueueEventBus) QueueSubscribe(_, _ string, _ bus.EventHandler) (bus.Subscription, error) {
	return nil, nil
}
func (m *capturingQueueEventBus) Request(_ context.Context, _ string, _ *bus.Event, _ time.Duration) (*bus.Event, error) {
	return nil, nil
}
func (m *capturingQueueEventBus) Close()            {}
func (m *capturingQueueEventBus) IsConnected() bool { return true }

func setupQueueHandlersWithResolver(
	t *testing.T,
	resolver SessionTaskResolver,
) (*QueueHandlers, *messagequeue.Service, *capturingQueueEventBus) {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:      "error",
		Format:     "console",
		OutputPath: "stderr",
	})
	require.NoError(t, err)
	events := &capturingQueueEventBus{}
	svc := messagequeue.NewServiceMemory(log)
	handlers := NewQueueHandlers(svc, events, log, nil, allowQueueAccess{}, resolver)
	return handlers, svc, events
}

func TestPublishStatusIncludesTaskIDWhenResolvable(t *testing.T) {
	handlers, svc, events := setupQueueHandlersWithResolver(t, func(_ context.Context, sessionID string) (string, error) {
		if sessionID == "s1" {
			return "task-1", nil
		}
		return "", nil
	})
	ctx := context.Background()

	msg := &messagequeue.QueuedMessage{SessionID: "s1", TaskID: "task-1", Content: "x", QueuedBy: messagequeue.QueuedByUser}
	_, err := svc.QueueMessageWithMetadata(ctx, msg.SessionID, msg.TaskID, msg.Content, "", msg.QueuedBy, false, nil, nil)
	require.NoError(t, err)
	require.NoError(t, svc.SetAutoRun(ctx, "s1", false))

	handlers.publishStatus(ctx, "s1")

	require.NotNil(t, events.lastData, "expected a published queue status event")
	require.Equal(t, "task-1", events.lastData["task_id"])
	require.Equal(t, "s1", events.lastData["session_id"])
	require.Equal(t, false, events.lastData["auto_run"])
	require.Equal(t, true, events.lastData["merge_enabled"])
}

func TestWsQueueMessagePublishesUserPromptActivity(t *testing.T) {
	handlers, _, events := setupQueueHandlersWithResolver(t, func(_ context.Context, sessionID string) (string, error) {
		if sessionID == "s1" {
			return "task-1", nil
		}
		return "", nil
	})

	response, err := handlers.wsQueueMessage(context.Background(), createTestMessage(t, ws.ActionMessageQueueAdd, map[string]interface{}{
		"session_id": "s1",
		"task_id":    "task-1",
		"content":    "queued prompt",
		"user_id":    "user-1",
	}))
	require.NoError(t, err)
	require.NotNil(t, response)
	require.NotNil(t, events.lastData)
	require.Equal(t, "user-1", events.lastData["queued_by"])
	require.IsType(t, time.Time{}, events.lastData["queued_at"])
	require.False(t, events.lastData["queued_at"].(time.Time).IsZero())
}

func TestPublishStatusOmitsTaskIDWhenResolutionFails(t *testing.T) {
	handlers, svc, events := setupQueueHandlersWithResolver(t, func(context.Context, string) (string, error) {
		return "", errors.New("session gone")
	})
	ctx := context.Background()

	msg := &messagequeue.QueuedMessage{SessionID: "s1", TaskID: "task-1", Content: "x", QueuedBy: messagequeue.QueuedByUser}
	_, err := svc.QueueMessageWithMetadata(ctx, msg.SessionID, msg.TaskID, msg.Content, "", msg.QueuedBy, false, nil, nil)
	require.NoError(t, err)

	handlers.publishStatus(ctx, "s1")

	require.NotNil(t, events.lastData, "expected a published queue status event")
	_, hasTaskID := events.lastData["task_id"]
	require.False(t, hasTaskID, "failed resolution must not fabricate a task_id")
	require.Equal(t, 1, events.lastData["count"])
}

func TestPublishStatusWithoutResolverPublishesWithoutTaskID(t *testing.T) {
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:      "error",
		Format:     "console",
		OutputPath: "stderr",
	})
	require.NoError(t, err)
	events := &capturingQueueEventBus{}
	svc := messagequeue.NewServiceMemory(log)
	handlers := NewQueueHandlers(svc, events, log, nil, allowQueueAccess{}, nil)
	ctx := context.Background()

	handlers.publishStatus(ctx, "s1")

	require.NotNil(t, events.lastData, "expected a published queue status event")
	_, hasTaskID := events.lastData["task_id"]
	require.False(t, hasTaskID, "nil resolver must not fabricate a task_id")
}
