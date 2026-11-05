package sqlite

import (
	"encoding/json"
	"testing"
	"time"

	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestExecutionObservationProjectionRejectsMissingOrFutureOwner(t *testing.T) {
	now := time.Date(2026, 10, 24, 0, 0, 0, 0, time.UTC)
	until := now.Add(30 * time.Second)
	snapshot := &models.ExecutionSnapshot{AttemptID: "attempt", SessionID: "session", TaskID: "task", Sequence: 1, Sandbox: &models.ExecutionSandboxSnapshot{Generation: 7}}
	observation := models.ExecutionObservation{SchemaVersion: 1, ObserverGeneration: 2, Revision: 1, SessionID: "session", TaskID: "task", SnapshotSequence: 1,
		Observation: contract.Observation{AttemptID: "attempt", Generation: 7, AttemptVersion: 3, Status: "active", ProviderHealth: "available", CheckedAt: now, EvidenceAt: &now, ValidUntil: &until, Mechanism: "fixture-kernel"}}
	raw, err := json.Marshal(observation)
	require.NoError(t, err)
	require.Equal(t, "active", decodeExecutionObservation(string(raw), snapshot, 2, now).Status)
	for _, generation := range []uint64{0, 1} {
		require.Nil(t, decodeExecutionObservation(string(raw), snapshot, generation, now))
	}
	expired := decodeExecutionObservation(string(raw), snapshot, 2, until)
	require.Equal(t, "unknown", expired.Status)
	require.Nil(t, expired.ValidUntil)
	require.Equal(t, &now, expired.EvidenceAt)
	observation.Status = "stopped"
	observation.ValidUntil = nil
	raw, err = json.Marshal(observation)
	require.NoError(t, err)
	history := decodeExecutionObservation(string(raw), snapshot, 3, now)
	require.Equal(t, "stopped", history.Status)
	require.Equal(t, "unknown", history.ProviderHealth, "old availability is not current provider health")
}
