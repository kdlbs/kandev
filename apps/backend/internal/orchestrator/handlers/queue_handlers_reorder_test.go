package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWsReorder covers the ActionMessageQueueReorder websocket path: happy
// path (publish + reversed order), validation, access denial, and drift
// mapping to queue_changed.
func TestWsReorder(t *testing.T) {
	t.Run("reorders the visible queue and publishes status", func(t *testing.T) {
		handlers, svc := setupQueueHandlers(t)
		ctx := context.Background()
		first, err := svc.QueueMessage(ctx, "s", "t", "first", "", messagequeue.QueuedByUser, false, nil)
		require.NoError(t, err)
		second, err := svc.QueueMessage(ctx, "s", "t", "second", "", messagequeue.QueuedByUser, false, nil)
		require.NoError(t, err)

		events := &mockEventBus{}
		handlers.eventBus = events

		response, err := handlers.wsReorder(ctx, createTestMessage(t, ws.ActionMessageQueueReorder, map[string]interface{}{
			"session_id":  "s",
			"ordered_ids": []string{second.ID, first.ID},
		}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeResponse, response.Type)

		var payload map[string]interface{}
		require.NoError(t, json.Unmarshal(response.Payload, &payload))
		assert.Equal(t, "s", payload["session_id"])
		assert.Equal(t, float64(2), payload["reordered"])

		status := svc.GetStatus(ctx, "s")
		require.Len(t, status.Entries, 2)
		assert.Equal(t, second.ID, status.Entries[0].ID)
		assert.Equal(t, first.ID, status.Entries[1].ID)
		assert.Equal(t, 1, events.published)
	})

	t.Run("rejects empty and duplicate ordered_ids", func(t *testing.T) {
		handlers, svc := setupQueueHandlers(t)
		ctx := context.Background()
		first, err := svc.QueueMessage(ctx, "s", "t", "first", "", messagequeue.QueuedByUser, false, nil)
		require.NoError(t, err)

		events := &mockEventBus{}
		handlers.eventBus = events

		empty := createTestMessage(t, ws.ActionMessageQueueReorder, map[string]interface{}{
			"session_id":  "s",
			"ordered_ids": []string{},
		})
		response, err := handlers.wsReorder(ctx, empty)
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
		assert.Equal(t, ws.ErrorCodeValidation, parseError(t, response).Code)
		assert.Contains(t, parseError(t, response).Message, "ordered_ids is required")

		duplicate := createTestMessage(t, ws.ActionMessageQueueReorder, map[string]interface{}{
			"session_id":  "s",
			"ordered_ids": []string{first.ID, first.ID},
		})
		response, err = handlers.wsReorder(ctx, duplicate)
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
		assert.Equal(t, ws.ErrorCodeValidation, parseError(t, response).Code)
		assert.Contains(t, parseError(t, response).Message, "duplicates")

		assert.Equal(t, 0, events.published)
		assert.Equal(t, 1, svc.GetStatus(ctx, "s").Count)
	})

	t.Run("rejects drift with queue_changed and no mutation", func(t *testing.T) {
		handlers, svc := setupQueueHandlers(t)
		ctx := context.Background()
		first, err := svc.QueueMessage(ctx, "s", "t", "first", "", messagequeue.QueuedByUser, false, nil)
		require.NoError(t, err)
		second, err := svc.QueueMessage(ctx, "s", "t", "second", "", messagequeue.QueuedByUser, false, nil)
		require.NoError(t, err)
		// A drain wins between the client's snapshot and the reorder.
		_, ok := svc.TakeQueued(ctx, "s")
		require.True(t, ok)

		events := &mockEventBus{}
		handlers.eventBus = events

		response, err := handlers.wsReorder(ctx, createTestMessage(t, ws.ActionMessageQueueReorder, map[string]interface{}{
			"session_id":  "s",
			"ordered_ids": []string{first.ID, second.ID},
		}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
		assert.Equal(t, queueErrorCodeQueueChanged, parseError(t, response).Code)
		assert.Equal(t, 0, events.published)
		status := svc.GetStatus(ctx, "s")
		require.Len(t, status.Entries, 1)
		assert.Equal(t, second.ID, status.Entries[0].ID)
	})

	t.Run("denies when the session is not authorized", func(t *testing.T) {
		log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console", OutputPath: "stderr"})
		require.NoError(t, err)
		svc := messagequeue.NewServiceMemory(log)
		first, err := svc.QueueMessage(context.Background(), "s", "t", "first", "", messagequeue.QueuedByUser, false, nil)
		require.NoError(t, err)
		access := &fakeQueueAccessAuthorizer{sessionErr: errors.New("secret denial")}
		events := &mockEventBus{}
		handlers := NewQueueHandlers(svc, events, log, nil, access, nil)

		response, err := handlers.wsReorder(context.Background(), createTestMessage(t, ws.ActionMessageQueueReorder, map[string]interface{}{
			"session_id":  "s",
			"ordered_ids": []string{first.ID},
		}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
		assert.Equal(t, ws.ErrorCodeNotFound, parseError(t, response).Code)
		assert.NotContains(t, string(response.Payload), "secret denial")
		assert.Equal(t, 1, access.sessionCalls)
		assert.Equal(t, 0, events.published)
	})
}
