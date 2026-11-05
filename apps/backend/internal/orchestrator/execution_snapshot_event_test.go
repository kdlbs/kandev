package orchestrator

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestExecutionSnapshotStateEvent(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task", "session", "step")
	session, err := repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	events := &recordingEventBus{}
	service := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	service.eventBus = events
	session.QueueIncarnationID = "session-owner"
	session.ExecutionSnapshot = &models.ExecutionSnapshot{SchemaVersion: 1, AttemptID: "attempt", SessionID: session.ID, TaskID: session.TaskID, ExecutionID: "execution", Sequence: 2, Generation: 1}
	session.ExecutionObservation = &models.ExecutionObservation{SchemaVersion: 1, ObserverGeneration: 2, Revision: 3}
	service.publishTaskSessionStateChanged(t.Context(), session.TaskID, session.ID, models.TaskSessionStateStarting, models.TaskSessionStateRunning, "", nil, session)
	require.Len(t, events.events, 1)
	payload := events.events[0].event.Data.(map[string]any)
	require.Equal(t, session.ExecutionSnapshot, payload["execution_snapshot"])
	require.Equal(t, session.ExecutionObservation, payload["execution_observation"])
	require.Equal(t, session.QueueIncarnationID, payload["queue_incarnation_id"])
}
