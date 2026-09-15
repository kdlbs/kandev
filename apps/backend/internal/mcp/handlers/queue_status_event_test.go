package handlers

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestPublishQueueStatusEventIncludesQueuePolicy(t *testing.T) {
	ctx := context.Background()
	queue := messagequeue.NewServiceMemory(testLogger(t))
	require.NoError(t, queue.SetAutoRun(ctx, "session-policy", false))
	queue.SetMergeEnabled(false)
	eventBus := &mcpRecordingEventBus{}
	handlers := &Handlers{eventBus: eventBus}

	identity, err := queue.ResolveSessionIdentity(ctx, "task-policy", "session-policy")
	require.NoError(t, err)
	handlers.publishQueueStatusEvent(ctx, identity, queue)

	require.Len(t, eventBus.events, 1)
	data, ok := eventBus.events[0].Data.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, false, data["auto_run"])
	require.Equal(t, false, data["merge_enabled"])
	require.Equal(t, "task-policy", data["task_id"])
	require.Equal(t, identity.SessionIncarnationID, data["session_incarnation_id"])
	require.NotEmpty(t, data["status_epoch"])
	require.NotEqual(t, identity.SessionIncarnationID, data["status_epoch"])
	require.Equal(t, true, data["auto_merge_available"])
}

func TestPublishQueueStatusEventUsesOwningSessionTaskForMixedQueue(t *testing.T) {
	ctx := context.Background()
	taskSvc, repo := newTestTaskService(t)
	seedMCPHandlerSession(t, repo, "task-owner", "session-mixed", models.TaskSessionStateWaitingForInput)
	queue := messagequeue.NewServiceMemory(testLogger(t))
	eventBus := &mcpRecordingEventBus{}
	handlers := &Handlers{taskSvc: taskSvc, eventBus: eventBus}
	identity, err := queue.ResolveSessionIdentity(ctx, "task-owner", "session-mixed")
	require.NoError(t, err)

	_, err = queue.QueueMessage(ctx, "session-mixed", "task-other", "from another task", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)
	_, err = queue.QueueMessage(ctx, "session-mixed", "task-owner", "from owner", "", messagequeue.QueuedByUser, false, nil)
	require.NoError(t, err)

	handlers.publishQueueStatusEvent(ctx, identity, queue)

	require.Len(t, eventBus.events, 1)
	data, ok := eventBus.events[0].Data.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "task-owner", data["task_id"])
}

func TestQueueMoveTaskPromptPublishesQueueStatus(t *testing.T) {
	ctx := context.Background()
	queue := messagequeue.NewServiceMemory(testLogger(t))
	eventBus := &mcpRecordingEventBus{}
	handlers := &Handlers{eventBus: eventBus, messageQueue: queue}

	identity, err := queue.ResolveSessionIdentity(ctx, "task-move", "session-move")
	require.NoError(t, err)
	_, err = handlers.queueMoveTaskPrompt(ctx, identity, "continue")
	require.NoError(t, err)

	require.Len(t, eventBus.events, 1)
	data, ok := eventBus.events[0].Data.(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "session-move", data["session_id"])
	require.Equal(t, "task-move", data["task_id"])
	require.Equal(t, 1, data["count"])
}
