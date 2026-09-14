package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ws "github.com/kandev/kandev/pkg/websocket"
)

// @covers AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.3
// @covers AC-UI-MESSAGE-QUEUE-AUTO-MERGE-OVERRIDE-001.7
func TestQueueHandlersAutoMergeSetReturnsEffectiveSessionPolicy(t *testing.T) {
	const action = "message.queue.auto_merge.set"
	handlers, _ := setupQueueHandlers(t)
	dispatcher := ws.NewDispatcher()
	handlers.RegisterHandlers(dispatcher)
	require.True(t, dispatcher.HasHandler(action))

	response, err := dispatcher.Dispatch(context.Background(), createTestMessage(t, action, map[string]interface{}{
		"task_id":                "task",
		"session_id":             "session",
		"session_incarnation_id": "memory:session",
		"enabled":                false,
	}))
	require.NoError(t, err)
	assert.Equal(t, ws.MessageTypeResponse, response.Type)
	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(response.Payload, &payload))
	assert.Equal(t, "task", payload["task_id"])
	assert.Equal(t, "session", payload["session_id"])
	assert.Equal(t, "memory:session", payload["session_incarnation_id"])
	assert.Equal(t, false, payload["auto_merge_enabled"])
	assert.Equal(t, "session", payload["auto_merge_source"])
	assert.Equal(t, float64(1), payload["auto_merge_revision"])
}
