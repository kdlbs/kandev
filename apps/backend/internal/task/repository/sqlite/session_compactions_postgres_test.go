package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
)

func seedPostgresCompactionSession(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO tasks (id, workspace_id, title, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`),
		"task-native-compactions-pg", "ws-native-compactions-pg", "Native compactions", now, now)
	require.NoError(t, err)
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-native-compactions-pg", TaskID: "task-native-compactions-pg",
		QueueIncarnationID: "incarnation-native-compactions-pg", State: models.TaskSessionStateWaitingForInput,
	}))
	_, err = repo.db.Exec(repo.db.Rebind(`
		INSERT INTO harness_session_generations
			(session_id, incarnation_id, generation, predecessor_generation, native_session_id,
			 agent_type, adapter_version, original_workspace, current_workspace,
			 native_state_reference, creation_reason, created_at, committed_at)
		VALUES (?, ?, 1, 0, ?, 'copilot-acp', '', '', '', '', 'initial', ?, ?)`),
		"session-native-compactions-pg", "incarnation-native-compactions-pg", "native-pg", now, now)
	require.NoError(t, err)
}

func postgresCompactionEvent(
	t *testing.T,
	streamID string,
	sequence int64,
	nativeSessionID, attemptID string,
) *models.AgentDeliveryEvent {
	t.Helper()
	payload, err := json.Marshal(streams.AgentEvent{
		Type:      streams.EventTypeContextCompactionObservation,
		SessionID: nativeSessionID,
		ContextCompaction: &streams.ContextCompactionObservation{
			Kind:            streams.ContextCompactionObservationKindCompletion,
			Provider:        copilotNativeCompactionProvider,
			NativeSessionID: nativeSessionID,
			AttemptID:       attemptID,
			Success:         true,
			Outcome:         "applied",
		},
	})
	require.NoError(t, err)
	return &models.AgentDeliveryEvent{
		SessionID:         "session-native-compactions-pg",
		IncarnationID:     "incarnation-native-compactions-pg",
		HarnessGeneration: 1,
		StreamID:          streamID,
		Sequence:          sequence,
		EventType:         streams.EventTypeContextCompactionObservation,
		Payload:           payload,
		ReceivedAt:        time.Now().UTC(),
	}
}

// TestPostgresSessionCompactionsAtomicProjection is the environment-gated
// PostgreSQL parity check for receipt uniqueness and atomic count projection.
// A skip is not parity evidence.
func TestPostgresSessionCompactionsAtomicProjection(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	seedPostgresCompactionSession(t, repo)
	ctx := context.Background()

	project := func(streamID string, sequence int64, nativeSessionID, attemptID string) *models.CanonicalAgentDeliveryProjectionResult {
		t.Helper()
		event := postgresCompactionEvent(t, streamID, sequence, nativeSessionID, attemptID)
		effects := combinedDeliveryBatchEffects([]*models.AgentDeliveryEvent{event})
		results, err := repo.ReceiveAndProjectCanonicalAgentDeliveryEvents(ctx, []*models.AgentDeliveryEvent{event}, effects, 0)
		require.NoError(t, err)
		return &results[0]
	}

	result := project("stream-native-pg-a", 1, "native-pg", "attempt-1")
	require.NotNil(t, result.ContextCompactionCount)
	require.EqualValues(t, 1, *result.ContextCompactionCount)

	// A duplicate attempt in another stream cannot increment again.
	project("stream-native-pg-b", 1, "native-pg", "attempt-1")
	// A distinct attempt increments.
	project("stream-native-pg-c", 1, "native-pg", "attempt-2")
	// A stale native conversation is rejected.
	project("stream-native-pg-d", 1, "native-stale", "attempt-3")

	session, err := repo.GetTaskSession(ctx, "session-native-compactions-pg")
	require.NoError(t, err)
	require.EqualValues(t, 2, metadataInt64(session.Metadata[models.SessionMetaKeyContextCompactionCount]))
	raw, err := json.Marshal(session.Metadata[models.SessionMetaKeyContextCompactionReporting])
	require.NoError(t, err)
	var reporting streams.ContextCompactionReporting
	require.NoError(t, json.Unmarshal(raw, &reporting))
	require.Equal(t, streams.ContextCompactionCoverageObserved, reporting.Coverage)
}

// TestPostgresSessionCompactionsConcurrent covers multi-connection lock order
// and preservation of concurrent gauge metadata.
func TestPostgresSessionCompactionsConcurrent(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	seedPostgresCompactionSession(t, repo)

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(2)
		go func(index int) {
			defer wg.Done()
			_, err := repo.UpdateSessionContextWindow(context.Background(), "session-native-compactions-pg",
				contextWindowSample(int64(100000-index*1000), true))
			require.NoError(t, err)
		}(i)
		go func(index int) {
			defer wg.Done()
			streamID := fmt.Sprintf("stream-native-pg-concurrent-%d", index)
			event := postgresCompactionEvent(t, streamID, 1, "native-pg", fmt.Sprintf("attempt-concurrent-%d", index))
			effects := combinedDeliveryBatchEffects([]*models.AgentDeliveryEvent{event})
			_, err := repo.ReceiveAndProjectCanonicalAgentDeliveryEvents(
				context.Background(), []*models.AgentDeliveryEvent{event}, effects, 0)
			require.NoError(t, err)
		}(i)
	}
	wg.Wait()

	session, err := repo.GetTaskSession(context.Background(), "session-native-compactions-pg")
	require.NoError(t, err)
	require.EqualValues(t, 6, metadataInt64(session.Metadata[models.SessionMetaKeyContextCompactionCount]))
	window, ok := session.Metadata[models.SessionMetaKeyContextWindow].(map[string]interface{})
	require.True(t, ok)
	require.NotZero(t, window["used"])
}
