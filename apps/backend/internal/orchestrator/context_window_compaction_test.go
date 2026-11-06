package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/events"
	eventbus "github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

// TestContextWindowCopilotUsageDoesNotInferCompaction replays the observations
// from issue #5089 through the real publisher, watcher conversion, handler,
// SQLite repository, and metadata publication. Every parent/subagent switch
// must keep the inferred count at zero while each usage value is still stored.
func TestContextWindowCopilotUsageDoesNotInferCompaction(t *testing.T) {
	const (
		taskID    = "task-copilot-usage"
		sessionID = "session-copilot-usage"
	)
	usages := []int64{13267, 2877, 13390, 2877, 13485}

	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, taskID, sessionID, "step1")
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, "unrelated", "kept"))

	log := testLogger()
	eventBus := eventbus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)

	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eventBus

	published := make(chan map[string]interface{}, len(usages))
	sub, err := eventBus.Subscribe(events.TaskSessionStateChanged, func(_ context.Context, event *eventbus.Event) error {
		if payload, ok := event.Data.(map[string]interface{}); ok {
			published <- payload
		}
		return nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	eventWatcher := watcher.NewWatcher(eventBus, watcher.EventHandlers{
		OnContextWindowUpdated: svc.handleContextWindowUpdated,
	}, "copilot-compaction-regression", log)
	require.NoError(t, eventWatcher.Start(ctx))
	t.Cleanup(func() { require.NoError(t, eventWatcher.Stop()) })

	publisher := lifecycle.NewEventPublisher(eventBus, log)
	execution := &lifecycle.AgentExecution{
		ID:        "exec-copilot",
		TaskID:    taskID,
		SessionID: sessionID,
		AgentID:   agents.CopilotACPAgentID,
	}

	for _, used := range usages {
		publisher.PublishContextWindow(execution, 200000, used, 200000-used, float64(used)/2000)

		session, err := repo.GetTaskSession(ctx, sessionID)
		require.NoError(t, err)
		require.EqualValues(t, 0, session.Metadata[models.SessionMetaKeyContextCompactionCount],
			"excluded Copilot sample %d must not infer a compaction", used)
		window, ok := session.Metadata[models.SessionMetaKeyContextWindow].(map[string]interface{})
		require.True(t, ok)
		require.EqualValues(t, used, window["used"], "usage must survive for %d", used)
		require.Equal(t, true, window["compaction_inference_disabled"])

		select {
		case payload := <-published:
			metadata, ok := payload["metadata"].(map[string]interface{})
			require.True(t, ok)
			require.EqualValues(t, 0, metadata[models.SessionMetaKeyContextCompactionCount])
			publishedWindow, ok := metadata[models.SessionMetaKeyContextWindow].(map[string]interface{})
			require.True(t, ok)
			require.EqualValues(t, used, publishedWindow["used"])
		default:
			t.Fatalf("expected a published session metadata patch for usage %d", used)
		}
	}

	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, "kept", session.Metadata["unrelated"])
	require.EqualValues(t, 0, session.Metadata[models.SessionMetaKeyContextCompactionCount])
}
