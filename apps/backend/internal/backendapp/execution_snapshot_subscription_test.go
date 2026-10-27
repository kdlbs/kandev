package backendapp

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestExecutionSnapshotSubscribeResponse(t *testing.T) {
	session := &models.TaskSession{ID: "session", TaskID: "task", State: models.TaskSessionStateRunning, QueueIncarnationID: "session-owner"}
	messages := appendSessionStateMessage(session.ID, session, nil)
	require.Len(t, messages, 1)
	payload := decodePayload(t, messages[0].Payload)
	require.Contains(t, payload, "execution_snapshot")
	require.Nil(t, payload["execution_snapshot"])
	require.Contains(t, payload, "execution_observation")
	require.Nil(t, payload["execution_observation"])
	require.Equal(t, session.QueueIncarnationID, payload["queue_incarnation_id"])
	session.ExecutionSnapshot = &models.ExecutionSnapshot{SchemaVersion: 1, AttemptID: "attempt", SessionID: session.ID, TaskID: session.TaskID, ExecutionID: "execution", Sequence: 2, Generation: 1}
	session.ExecutionObservation = &models.ExecutionObservation{SchemaVersion: 1, ObserverGeneration: 4, Revision: 3}
	messages = appendSessionStateMessage(session.ID, session, nil)
	require.Len(t, messages, 1)
	payload = decodePayload(t, messages[0].Payload)
	snapshot, ok := payload["execution_snapshot"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "attempt", snapshot["attempt_id"])
	require.Equal(t, float64(2), snapshot["sequence"])
	observation, ok := payload["execution_observation"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, float64(4), observation["observer_generation"])
	require.Equal(t, float64(3), observation["revision"])
}
