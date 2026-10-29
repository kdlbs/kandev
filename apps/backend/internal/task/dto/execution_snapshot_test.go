package dto

import (
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/common/launchwrapper"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestExecutionSnapshotSessionResponse(t *testing.T) {
	session := &models.TaskSession{ID: "session", TaskID: "task"}
	raw, err := json.Marshal(FromTaskSession(session))
	require.NoError(t, err)
	require.Contains(t, string(raw), `"execution_snapshot":null`)
	require.Contains(t, string(raw), `"execution_observation":null`)
	wrapper, err := launchwrapper.Profile([]string{"/private/wrapper", "credential-value"}).Summary()
	require.NoError(t, err)
	session.ExecutionSnapshot = &models.ExecutionSnapshot{SchemaVersion: 1, AttemptID: "attempt", SessionID: session.ID, TaskID: session.TaskID, ExecutionID: "execution", Generation: 1, Sequence: 1, Wrapper: wrapper, Agent: launchwrapper.SummarizeCommand([]string{"/private/agent", "secret-value"})}
	session.ExecutionObservation = &models.ExecutionObservation{SchemaVersion: 1, ObserverGeneration: 2, Revision: 3, SessionID: session.ID, TaskID: session.TaskID, SnapshotSequence: 1}
	raw, err = json.Marshal(FromTaskSession(session))
	require.NoError(t, err)
	var decoded struct {
		Snapshot    *models.ExecutionSnapshot    `json:"execution_snapshot"`
		Observation *models.ExecutionObservation `json:"execution_observation"`
	}
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, session.ExecutionSnapshot, decoded.Snapshot)
	require.Equal(t, session.ExecutionObservation, decoded.Observation)
	for _, private := range []string{"/private", "secret-value", "credential-value"} {
		require.NotContains(t, string(raw), private)
	}
}
