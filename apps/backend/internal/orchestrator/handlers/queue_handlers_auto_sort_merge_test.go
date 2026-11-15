package handlers

import (
	"context"
	"encoding/json"
	"testing"

	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

// @covers AC-UI-MESSAGE-QUEUE-AUTO-SORT-MERGE-001.3
func TestAutoSortMergeSetPersistsExplicitPreference(t *testing.T) {
	handlers, _ := setupQueueHandlers(t)
	dispatcher := ws.NewDispatcher()
	handlers.RegisterHandlers(dispatcher)
	response, err := dispatcher.Dispatch(context.Background(), createTestMessage(t, "message.queue.auto_sort_merge.set", map[string]any{
		"task_id": "task", "session_id": "session", "session_incarnation_id": "memory:session", "enabled": true,
	}))
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(response.Payload, &payload))
	require.Equal(t, true, payload["auto_sort_merge_available"])
	require.Equal(t, true, payload["auto_sort_merge_enabled"])
	require.Equal(t, "session", payload["auto_sort_merge_source"])
	require.Equal(t, float64(1), payload["auto_sort_merge_revision"])
	require.Equal(t, float64(0), payload["compacted_count"])
}
