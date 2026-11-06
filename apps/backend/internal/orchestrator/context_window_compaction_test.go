package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
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

// TestContextWindowCopilotWithoutNativeCollectionReportsUnavailable proves a
// Copilot gauge from a runtime without the native event stream records honest
// unavailable/partial reporting instead of implying zero compactions.
func TestContextWindowCopilotWithoutNativeCollectionReportsUnavailable(t *testing.T) {
	const (
		taskID    = "task-copilot-unavailable"
		sessionID = "session-copilot-unavailable"
	)
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, taskID, sessionID, "step1")

	log := testLogger()
	eventBus := eventbus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eventBus

	eventWatcher := watcher.NewWatcher(eventBus, watcher.EventHandlers{
		OnContextWindowUpdated: svc.handleContextWindowUpdated,
	}, "copilot-native-unavailable", log)
	require.NoError(t, eventWatcher.Start(ctx))
	t.Cleanup(func() { require.NoError(t, eventWatcher.Stop()) })

	publisher := lifecycle.NewEventPublisher(eventBus, log)
	publisher.PublishContextWindow(&lifecycle.AgentExecution{
		ID: "exec-copilot-unavailable", TaskID: taskID, SessionID: sessionID,
		AgentID: agents.CopilotACPAgentID, CopilotNativeSessionEvents: false,
	}, 272000, 12171, 259829, 4.4)

	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	reporting, ok := session.Metadata[models.SessionMetaKeyContextCompactionReporting].(map[string]interface{})
	require.True(t, ok, "metadata = %#v", session.Metadata)
	require.Equal(t, "copilot_native", reporting["source"])
	require.Equal(t, "unavailable", reporting["availability"])
	require.Equal(t, "partial", reporting["coverage"])
}

// TestContextWindowCopilotWithNativeCollectionLeavesReportingToProjection
// proves a negotiated native launch does not claim availability before the
// adapter's own reporting observation arrives.
func TestContextWindowCopilotWithNativeCollectionLeavesReportingToProjection(t *testing.T) {
	const (
		taskID    = "task-copilot-native-available"
		sessionID = "session-copilot-native-available"
	)
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, taskID, sessionID, "step1")

	log := testLogger()
	eventBus := eventbus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eventBus

	eventWatcher := watcher.NewWatcher(eventBus, watcher.EventHandlers{
		OnContextWindowUpdated: svc.handleContextWindowUpdated,
	}, "copilot-native-available", log)
	require.NoError(t, eventWatcher.Start(ctx))
	t.Cleanup(func() { require.NoError(t, eventWatcher.Stop()) })

	publisher := lifecycle.NewEventPublisher(eventBus, log)
	publisher.PublishContextWindow(&lifecycle.AgentExecution{
		ID: "exec-copilot-native", TaskID: taskID, SessionID: sessionID,
		AgentID: agents.CopilotACPAgentID, CopilotNativeSessionEvents: true,
	}, 272000, 12171, 259829, 4.4)

	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.Nil(t, session.Metadata[models.SessionMetaKeyContextCompactionReporting])
}

// TestPublishCommittedContextCompactionMetadata publishes the count/reporting
// already committed by canonical projection, and refuses unprojected events.
func TestPublishCommittedContextCompactionMetadata(t *testing.T) {
	ctx := context.Background()
	log := testLogger()
	eventBus := eventbus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-committed", "session-committed", "step1")
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eventBus

	published := make(chan map[string]interface{}, 2)
	sub, err := eventBus.Subscribe(events.TaskSessionStateChanged, func(_ context.Context, event *eventbus.Event) error {
		if payload, ok := event.Data.(map[string]interface{}); ok {
			published <- payload
		}
		return nil
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Unsubscribe() })

	payload := &lifecycle.AgentStreamEventPayload{
		TaskID: "task-committed", SessionID: "session-committed",
		Data: &lifecycle.AgentStreamEventData{
			Type:                streams.EventTypeContextCompactionObservation,
			CanonicalProjection: true,
			Data: map[string]interface{}{
				models.SessionMetaKeyContextCompactionCount: int64(3),
				models.SessionMetaKeyContextCompactionReporting: map[string]interface{}{
					"source": "copilot_native", "availability": "available",
					"coverage": "observed", "revision": int64(4),
				},
			},
		},
	}
	svc.publishCommittedContextCompaction(
		ctx, payload.TaskID, payload.SessionID, payload.Data.CanonicalProjection, payload.Data.Data,
	)

	select {
	case event := <-published:
		metadata, ok := event["metadata"].(map[string]interface{})
		require.True(t, ok)
		require.EqualValues(t, 3, metadata[models.SessionMetaKeyContextCompactionCount])
		reporting, ok := metadata[models.SessionMetaKeyContextCompactionReporting].(map[string]interface{})
		require.True(t, ok)
		require.Equal(t, "copilot_native", reporting["source"])
		require.NotEmpty(t, event["updated_at"], "authoritative session timestamp must accompany the patch")
	case <-time.After(time.Second):
		t.Fatal("expected committed compaction metadata publication")
	}

	payload.Data.CanonicalProjection = false
	svc.publishCommittedContextCompaction(
		ctx, payload.TaskID, payload.SessionID, payload.Data.CanonicalProjection, payload.Data.Data,
	)
	select {
	case <-published:
		t.Fatal("unprojected compaction observation must not publish a count")
	case <-time.After(20 * time.Millisecond):
	}
}

// TestContextWindowCopilotUnavailableDowngradesAvailableReporting proves a
// current runtime that cannot collect native events leaves the retained total
// intact while changing availability and preserving sticky partial coverage.
func TestContextWindowCopilotUnavailableDowngradesAvailableReporting(t *testing.T) {
	const (
		taskID    = "task-copilot-downgrade"
		sessionID = "session-copilot-downgrade"
	)
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, taskID, sessionID, "step1")
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyContextCompactionCount, 2))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyContextCompactionReporting, map[string]interface{}{
		"source": "copilot_native", "availability": "available", "coverage": "observed", "revision": 3,
	}))

	log := testLogger()
	eventBus := eventbus.NewMemoryEventBus(log)
	t.Cleanup(eventBus.Close)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	svc.eventBus = eventBus

	eventWatcher := watcher.NewWatcher(eventBus, watcher.EventHandlers{
		OnContextWindowUpdated: svc.handleContextWindowUpdated,
	}, "copilot-native-downgrade", log)
	require.NoError(t, eventWatcher.Start(ctx))
	t.Cleanup(func() { require.NoError(t, eventWatcher.Stop()) })

	publisher := lifecycle.NewEventPublisher(eventBus, log)
	publisher.PublishContextWindow(&lifecycle.AgentExecution{
		ID: "exec-copilot-downgrade", TaskID: taskID, SessionID: sessionID,
		AgentID: agents.CopilotACPAgentID, CopilotNativeSessionEvents: false,
	}, 272000, 9695, 262305, 3.5)

	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.EqualValues(t, 2, session.Metadata[models.SessionMetaKeyContextCompactionCount])
	reporting, ok := session.Metadata[models.SessionMetaKeyContextCompactionReporting].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "unavailable", reporting["availability"])
	require.Equal(t, "partial", reporting["coverage"])
	require.Greater(t, reporting["revision"], float64(3))

	// A later legacy gauge must not rewrite the same sticky state or advance
	// its revision on every sample.
	stableRevision := reporting["revision"]
	publisher.PublishContextWindow(&lifecycle.AgentExecution{
		ID: "exec-copilot-downgrade", TaskID: taskID, SessionID: sessionID,
		AgentID: agents.CopilotACPAgentID, CopilotNativeSessionEvents: false,
	}, 272000, 10000, 262000, 3.7)
	session, err = repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	reporting, ok = session.Metadata[models.SessionMetaKeyContextCompactionReporting].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, stableRevision, reporting["revision"])
}
