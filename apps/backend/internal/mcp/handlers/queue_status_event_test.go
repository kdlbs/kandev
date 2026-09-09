package handlers

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/stretchr/testify/require"
)

func TestPublishQueueStatusEventIncludesQueuePolicy(t *testing.T) {
	ctx := context.Background()
	queue := messagequeue.NewServiceMemory(testLogger(t))
	require.NoError(t, queue.SetAutoRun(ctx, "session-policy", false))
	queue.SetMergeEnabled(false)
	eventBus := &mcpRecordingEventBus{}
	handlers := &Handlers{eventBus: eventBus}

	handlers.publishQueueStatusEvent(ctx, "session-policy", queue)

	require.Len(t, eventBus.events, 1)
	data, ok := eventBus.events[0].Data.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, false, data["auto_run"])
	require.Equal(t, false, data["merge_enabled"])
}
