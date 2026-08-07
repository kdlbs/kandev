package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWsMergeIntoAbove covers the ActionMessageQueueMerge websocket path:
// happy-path merge, ownership rejection, and reserved-identity handling.
func TestWsMergeIntoAbove(t *testing.T) {
	t.Run("merges a user entry into the entry above", func(t *testing.T) {
		handlers, svc := setupQueueHandlers(t)
		ctx := context.Background()

		a, _ := svc.QueueMessage(ctx, "s", "t", "first", "", "user-1", false, nil)
		b, _ := svc.QueueMessage(ctx, "s", "t", "second", "", "user-1", false, nil)

		response, err := handlers.wsMergeIntoAbove(ctx,
			createTestMessage(t, ws.ActionMessageQueueMerge, map[string]interface{}{
				"session_id": "s",
				"entry_id":   b.ID,
				"user_id":    "user-1",
			}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeResponse, response.Type)
		var payload map[string]interface{}
		require.NoError(t, json.Unmarshal(response.Payload, &payload))
		assert.Equal(t, a.ID, payload["entry_id"])

		status := svc.GetStatus(ctx, "s")
		require.Equal(t, 1, status.Count)
		assert.Equal(t, "first\n\nsecond", status.Entries[0].Content)
	})

	t.Run("returns validation when the target is the head", func(t *testing.T) {
		handlers, svc := setupQueueHandlers(t)
		ctx := context.Background()

		head, _ := svc.QueueMessage(ctx, "s", "t", "only", "", "u", false, nil)

		response, err := handlers.wsMergeIntoAbove(ctx,
			createTestMessage(t, ws.ActionMessageQueueMerge, map[string]interface{}{
				"session_id": "s",
				"entry_id":   head.ID,
				"user_id":    "u",
			}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
		assert.Equal(t, ws.ErrorCodeValidation, parseError(t, response).Code)
		assert.Equal(t, 1, svc.GetStatus(ctx, "s").Count)
	})

	t.Run("returns validation for mismatched sender kinds", func(t *testing.T) {
		handlers, svc := setupQueueHandlers(t)
		ctx := context.Background()

		_, err := svc.QueueMessageWithMetadata(ctx, "s", "t", "agent prompt", "", messagequeue.QueuedByAgent, false, nil,
			map[string]interface{}{messagequeue.MetadataSenderTaskID: "task-1"})
		require.NoError(t, err)
		user, err := svc.QueueMessage(ctx, "s", "t", "user prompt", "", "u", false, nil)
		require.NoError(t, err)

		response, err := handlers.wsMergeIntoAbove(ctx,
			createTestMessage(t, ws.ActionMessageQueueMerge, map[string]interface{}{
				"session_id": "s",
				"entry_id":   user.ID,
				"user_id":    "u",
			}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
		assert.Equal(t, ws.ErrorCodeValidation, parseError(t, response).Code)
		assert.Equal(t, 2, svc.GetStatus(ctx, "s").Count)
	})

	t.Run("returns entry_not_found for a drained source", func(t *testing.T) {
		handlers, svc := setupQueueHandlers(t)
		ctx := context.Background()

		queued, _ := svc.QueueMessage(ctx, "s", "t", "x", "", "u", false, nil)
		_, _ = svc.TakeQueued(ctx, "s")

		response, err := handlers.wsMergeIntoAbove(ctx,
			createTestMessage(t, ws.ActionMessageQueueMerge, map[string]interface{}{
				"session_id": "s",
				"entry_id":   queued.ID,
				"user_id":    "u",
			}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
		assert.Equal(t, "entry_not_found", parseError(t, response).Code)
	})

	t.Run("returns merge_disabled when merging is turned off", func(t *testing.T) {
		handlers, svc := setupQueueHandlers(t)
		ctx := context.Background()

		a, _ := svc.QueueMessage(ctx, "s", "t", "first", "", "user-1", false, nil)
		b, _ := svc.QueueMessage(ctx, "s", "t", "second", "", "user-1", false, nil)
		svc.SetMergeEnabled(false)

		response, err := handlers.wsMergeIntoAbove(ctx,
			createTestMessage(t, ws.ActionMessageQueueMerge, map[string]interface{}{
				"session_id": "s",
				"entry_id":   b.ID,
				"user_id":    "user-1",
			}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
		assert.Equal(t, "merge_disabled", parseError(t, response).Code)

		status := svc.GetStatus(ctx, "s")
		require.Equal(t, 2, status.Count)
		assert.Equal(t, a.ID, status.Entries[0].ID)
	})

	t.Run("rejects reserved user_id", func(t *testing.T) {
		handlers, _ := setupQueueHandlers(t)
		response, err := handlers.wsMergeIntoAbove(context.Background(),
			createTestMessage(t, ws.ActionMessageQueueMerge, map[string]interface{}{
				"session_id": "s",
				"entry_id":   "e",
				"user_id":    messagequeue.QueuedByAgent,
			}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
		assert.Contains(t, parseError(t, response).Message, "may not impersonate the agent identity")
	})

	t.Run("rejects missing session_id", func(t *testing.T) {
		handlers, _ := setupQueueHandlers(t)
		response, err := handlers.wsMergeIntoAbove(context.Background(),
			createTestMessage(t, ws.ActionMessageQueueMerge, map[string]interface{}{"entry_id": "e"}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
		assert.Contains(t, parseError(t, response).Message, "session_id is required")
	})

	t.Run("rejects missing entry_id", func(t *testing.T) {
		handlers, _ := setupQueueHandlers(t)
		response, err := handlers.wsMergeIntoAbove(context.Background(),
			createTestMessage(t, ws.ActionMessageQueueMerge, map[string]interface{}{"session_id": "s"}))
		require.NoError(t, err)
		assert.Equal(t, ws.MessageTypeError, response.Type)
		assert.Contains(t, parseError(t, response).Message, "entry_id is required")
	})
}
