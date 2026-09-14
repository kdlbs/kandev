package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// queueServiceWithoutBatch preserves the legacy handler surface so the
// fallback cancellation path is exercised without CancelAllWithEntries.
type queueServiceWithoutBatch struct {
	QueueService
}

func TestWsCancelAllFallbackReportsActualRemovedCount(t *testing.T) {
	handlers, queue := setupQueueHandlers(t)
	handlers.queueService = queueServiceWithoutBatch{QueueService: queue}
	ctx := context.Background()

	_, err := queue.QueueMessage(ctx, "session-cancel-fallback", "task", "queued", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)

	response, err := handlers.wsCancelAll(ctx, createTestMessage(t, ws.ActionMessageQueueCancel, map[string]string{
		"session_id": "session-cancel-fallback",
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)

	var payload struct {
		Removed int `json:"removed"`
	}
	require.NoError(t, json.Unmarshal(response.Payload, &payload))
	require.Equal(t, 1, payload.Removed)
}
