package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/events/bus"
)

func TestHandlePrepareCompleted_PersistsViaJsonSet(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "sess-1", "step1")

	svc := &Service{logger: testLogger(), repo: repo}

	event := &bus.Event{
		Type: "executor.prepare.completed",
		Data: &lifecycle.PrepareCompletedEventPayload{
			SessionID: "sess-1",
			Success:   true,
			Steps:     []lifecycle.PrepareStep{{Name: "step1", Status: lifecycle.PrepareStepCompleted}},
		},
		Timestamp: time.Now(),
	}
	err := svc.handlePrepareCompleted(context.Background(), event)
	require.NoError(t, err)

	session, err := repo.GetTaskSession(context.Background(), "sess-1")
	require.NoError(t, err)
	require.NotNil(t, session.Metadata["prepare_result"], "prepare_result should be persisted")
}

func TestHandlePrepareCompleted_PersistsFailure(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-1", "sess-1", "step1")

	svc := &Service{logger: testLogger(), repo: repo}

	event := &bus.Event{
		Type: "executor.prepare.completed",
		Data: &lifecycle.PrepareCompletedEventPayload{
			SessionID:    "sess-1",
			Success:      false,
			ErrorMessage: "setup script exited with code 1",
			Steps: []lifecycle.PrepareStep{
				{Name: "step1", Status: lifecycle.PrepareStepCompleted},
				{Name: "step2", Status: lifecycle.PrepareStepFailed, Output: "npm ERR! missing"},
			},
		},
		Timestamp: time.Now(),
	}
	err := svc.handlePrepareCompleted(context.Background(), event)
	require.NoError(t, err)

	session, err := repo.GetTaskSession(context.Background(), "sess-1")
	require.NoError(t, err)

	pr, ok := session.Metadata["prepare_result"].(map[string]interface{})
	require.True(t, ok, "prepare_result should be a map")
	require.Equal(t, "failed", pr["status"])
	require.Equal(t, "setup script exited with code 1", pr["error_message"])

	steps, ok := pr["steps"].([]interface{})
	require.True(t, ok)
	require.Len(t, steps, 2)
}

func TestHandlePrepareCompleted_HandlesWrongType(t *testing.T) {
	svc := &Service{logger: testLogger()}

	event := &bus.Event{
		Type:      "executor.prepare.completed",
		Data:      "wrong type",
		Timestamp: time.Now(),
	}
	err := svc.handlePrepareCompleted(context.Background(), event)
	require.NoError(t, err)
}

func TestHandlePrepareCompleted_RejectsOlderCompletionWithoutPriorAttemptEvents(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-ordered", "sess-ordered", "step1")
	svc := &Service{logger: testLogger(), repo: repo}
	newerStarted := time.Date(2026, 9, 28, 12, 0, 1, 0, time.UTC).Format(time.RFC3339Nano)
	olderStarted := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)

	require.NoError(t, svc.handlePrepareCompleted(context.Background(), prepareCompletedEvent("sess-ordered", "newer", newerStarted)))
	require.NoError(t, svc.handlePrepareCompleted(context.Background(), prepareCompletedEvent("sess-ordered", "older", olderStarted)))

	session, err := repo.GetTaskSession(context.Background(), "sess-ordered")
	require.NoError(t, err)
	result, ok := session.Metadata["prepare_result"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "newer", result["preparation_id"])
	marker, ok := session.Metadata["prepare_attempt"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "newer", marker["preparation_id"])
}

func TestHandlePrepareProgressFencesOlderCompletion(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-progress-order", "sess-progress-order", "step1")
	svc := &Service{logger: testLogger(), repo: repo}
	newerStarted := time.Date(2026, 9, 28, 12, 1, 1, 0, time.UTC).Format(time.RFC3339Nano)
	olderStarted := time.Date(2026, 9, 28, 12, 1, 0, 0, time.UTC).Format(time.RFC3339Nano)
	progress := &bus.Event{Data: &lifecycle.PrepareProgressEventPayload{
		SessionID:            "sess-progress-order",
		PreparationID:        "newer",
		PreparationStartedAt: newerStarted,
	}}
	require.NoError(t, svc.handlePrepareProgress(context.Background(), progress))
	require.NoError(t, svc.handlePrepareCompleted(context.Background(), prepareCompletedEvent("sess-progress-order", "older", olderStarted)))

	session, err := repo.GetTaskSession(context.Background(), "sess-progress-order")
	require.NoError(t, err)
	require.Nil(t, session.Metadata["prepare_result"])
	marker, ok := session.Metadata["prepare_attempt"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "newer", marker["preparation_id"])
}

func TestPreparationAttemptOrderRejectsUnknownIdentityAgainstCurrentMarker(t *testing.T) {
	current := time.Date(2026, 9, 28, 12, 2, 0, 0, time.UTC).Format(time.RFC3339Nano)
	for _, tc := range []struct {
		name      string
		startedAt string
		id        string
	}{
		{name: "missing marker"},
		{name: "malformed marker", startedAt: "not-a-time", id: "old"},
		{name: "same timestamp different attempt", startedAt: current, id: "other"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.True(t, preparationAttemptIsOlder(tc.startedAt, tc.id, current, "current"))
		})
	}
}

func prepareCompletedEvent(sessionID, preparationID, startedAt string) *bus.Event {
	return &bus.Event{Data: &lifecycle.PrepareCompletedEventPayload{
		SessionID:            sessionID,
		PreparationID:        preparationID,
		PreparationStartedAt: startedAt,
		Success:              true,
	}}
}
