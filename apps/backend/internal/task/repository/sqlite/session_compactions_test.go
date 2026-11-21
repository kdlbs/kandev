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
)

const (
	compactionTaskID      = "task-session-compactions"
	compactionSessionID   = "session-session-compactions"
	compactionIncarnation = "incarnation-" + compactionSessionID
	compactionStreamID    = "stream-session-compactions"
	compactionNativeID    = "native-session-compactions"
)

func seedCompactionSession(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: compactionTaskID, Title: "Compaction ledger"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: compactionSessionID, TaskID: compactionTaskID, AgentProfileID: "profile-copilot-1",
		QueueIncarnationID: compactionIncarnation, State: models.TaskSessionStateWaitingForInput,
	}))
	_, err := repo.db.Exec(repo.db.Rebind(`
		CREATE TABLE IF NOT EXISTS agents (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL
		)`))
	require.NoError(t, err)
	_, err = repo.db.Exec(repo.db.Rebind(`
		CREATE TABLE IF NOT EXISTS agent_profiles (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL
		)`))
	require.NoError(t, err)
	_, err = repo.db.Exec(repo.db.Rebind(`
		INSERT OR REPLACE INTO agents (id, name) VALUES ('agent-copilot', 'copilot-acp')`))
	require.NoError(t, err)
	_, err = repo.db.Exec(repo.db.Rebind(`
		INSERT OR REPLACE INTO agent_profiles (id, agent_id) VALUES ('profile-copilot-1', 'agent-copilot')`))
	require.NoError(t, err)
	_, err = repo.db.Exec(repo.db.Rebind(`
		INSERT OR REPLACE INTO harness_session_generations
			(session_id, incarnation_id, generation, predecessor_generation, native_session_id,
			 agent_type, adapter_version, original_workspace, current_workspace,
			 native_state_reference, creation_reason, created_at, committed_at)
		VALUES (?, ?, 1, 0, ?, 'copilot-acp', '', '', '', '', 'initial', ?, ?)
	`), compactionSessionID, compactionIncarnation, compactionNativeID, time.Now().UTC(), time.Now().UTC())
	require.NoError(t, err)
}

func compactionObservationEvent(
	t *testing.T,
	sequence int64,
	attemptID string,
) *models.AgentDeliveryEvent {
	t.Helper()
	observation := &streams.ContextCompactionObservation{
		Kind:            streams.ContextCompactionObservationKindCompletion,
		Provider:        copilotNativeCompactionProvider,
		NativeSessionID: compactionNativeID,
		AttemptID:       attemptID,
		Success:         true,
		Outcome:         "applied",
		Trigger:         "manual",
	}
	payload, err := json.Marshal(streams.AgentEvent{
		Type:              streams.EventTypeContextCompactionObservation,
		SessionID:         compactionNativeID,
		ContextCompaction: observation,
	})
	require.NoError(t, err)
	return &models.AgentDeliveryEvent{
		SessionID:         compactionSessionID,
		IncarnationID:     compactionIncarnation,
		HarnessGeneration: 1,
		StreamID:          compactionStreamID,
		Sequence:          sequence,
		EventType:         streams.EventTypeContextCompactionObservation,
		Payload:           payload,
		ReceivedAt:        time.Now().UTC(),
	}
}

func projectCompactionEvents(t *testing.T, repo *Repository, events ...*models.AgentDeliveryEvent) []models.CanonicalAgentDeliveryProjectionResult {
	t.Helper()
	effects := combinedDeliveryBatchEffects(events)
	results, err := repo.ReceiveAndProjectCanonicalAgentDeliveryEvents(context.Background(), events, effects, 0)
	require.NoError(t, err)
	return results
}

func compactionMetadata(t *testing.T, repo *Repository) (int64, *streams.ContextCompactionReporting) {
	t.Helper()
	session, err := repo.GetTaskSession(context.Background(), compactionSessionID)
	require.NoError(t, err)
	count := metadataInt64(session.Metadata[models.SessionMetaKeyContextCompactionCount])
	raw, err := json.Marshal(session.Metadata[models.SessionMetaKeyContextCompactionReporting])
	require.NoError(t, err)
	var reporting streams.ContextCompactionReporting
	require.NoError(t, json.Unmarshal(raw, &reporting))
	return count, &reporting
}

func TestSessionCompactionsAtomicProjection(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	seedCompactionSession(t, repo)

	results := projectCompactionEvents(t, repo, compactionObservationEvent(t, 1, "attempt-1"))
	require.Len(t, results, 1)
	require.NotNil(t, results[0].ContextCompactionCount)
	require.EqualValues(t, 1, *results[0].ContextCompactionCount)

	count, reporting := compactionMetadata(t, repo)
	require.EqualValues(t, 1, count)
	require.Equal(t, streams.ContextCompactionSourceCopilotNative, reporting.Source)
	require.Equal(t, streams.ContextCompactionAvailabilityAvailable, reporting.Availability)
	require.Equal(t, streams.ContextCompactionCoverageObserved, reporting.Coverage)
	require.EqualValues(t, 1, reporting.Revision)

	// Replaying the same sequence is idempotent.
	projectCompactionEvents(t, repo, compactionObservationEvent(t, 1, "attempt-1"))
	count, _ = compactionMetadata(t, repo)
	require.EqualValues(t, 1, count)

	// A new stream with the same attempt identity cannot increment again.
	duplicate := compactionObservationEvent(t, 1, "attempt-1")
	duplicate.StreamID = "stream-session-compactions-second"
	projectCompactionEvents(t, repo, duplicate)
	count, _ = compactionMetadata(t, repo)
	require.EqualValues(t, 1, count)

	// The same attempt ID in a different native conversation is distinct.
	otherNative := compactionObservationEvent(t, 1, "attempt-1")
	otherNative.Payload = mustCompactionPayload(t, compactionNativeID+"-other", "attempt-1")
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO harness_session_generations
			(session_id, incarnation_id, generation, predecessor_generation, native_session_id,
			 agent_type, adapter_version, original_workspace, current_workspace,
			 native_state_reference, creation_reason, created_at, committed_at)
		VALUES (?, ?, 2, 1, ?, 'copilot-acp', '', '', '', '', 'native_replaced', ?, ?)
	`), compactionSessionID, compactionIncarnation, compactionNativeID+"-other", time.Now().UTC(), time.Now().UTC())
	require.NoError(t, err)
	otherNative.HarnessGeneration = 2
	otherNative.StreamID = "stream-session-compactions-generation-2"
	projectCompactionEvents(t, repo, otherNative)
	count, _ = compactionMetadata(t, repo)
	require.EqualValues(t, 2, count)

	// Legacy unrelated metadata survives every projection.
	require.NoError(t, repo.SetSessionMetadataKey(ctx, compactionSessionID, "unrelated", "kept"))
	projectCompactionEvents(t, repo, compactionObservationEvent(t, 2, "attempt-2"))
	session, err := repo.GetTaskSession(ctx, compactionSessionID)
	require.NoError(t, err)
	require.Equal(t, "kept", session.Metadata["unrelated"])
}

func mustCompactionPayload(t *testing.T, nativeSessionID, attemptID string) []byte {
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
	return payload
}

func TestSessionCompactionsRejectStaleNativeIdentity(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedCompactionSession(t, repo)

	stale := compactionObservationEvent(t, 1, "attempt-stale")
	stale.Payload = mustCompactionPayload(t, "unrelated-native-session", "attempt-stale")
	results := projectCompactionEvents(t, repo, stale)
	require.Nil(t, results[0].ContextCompactionCount)
	count, _ := compactionMetadata(t, repo)
	require.EqualValues(t, 0, count)
}

func TestSessionCompactionsResolveProfileAgentType(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedCompactionSession(t, repo)
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT OR REPLACE INTO agent_profiles (id, agent_id) VALUES ('profile-copilot-2', 'agent-copilot')`))
	require.NoError(t, err)
	_, err = repo.db.Exec(repo.db.Rebind(`
		UPDATE task_sessions SET agent_profile_id = 'profile-copilot-2' WHERE id = ?`), compactionSessionID)
	require.NoError(t, err)

	results := projectCompactionEvents(t, repo, compactionObservationEvent(t, 1, "attempt-profile"))
	require.NotNil(t, results[0].ContextCompactionCount)
	require.EqualValues(t, 1, *results[0].ContextCompactionCount)
}

func TestSessionCompactionsRejectNonCopilotSessionProfile(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedCompactionSession(t, repo)
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT OR REPLACE INTO agents (id, name) VALUES ('agent-claude', 'claude-acp')`))
	require.NoError(t, err)
	_, err = repo.db.Exec(repo.db.Rebind(`
		INSERT OR REPLACE INTO agent_profiles (id, agent_id) VALUES ('profile-claude-1', 'agent-claude')`))
	require.NoError(t, err)
	_, err = repo.db.Exec(repo.db.Rebind(`
		UPDATE task_sessions SET agent_profile_id = 'profile-claude-1' WHERE id = ?`), compactionSessionID)
	require.NoError(t, err)

	results := projectCompactionEvents(t, repo, compactionObservationEvent(t, 1, "attempt-wrong-provider"))
	require.Nil(t, results[0].ContextCompactionCount)
	count, _ := compactionMetadata(t, repo)
	require.EqualValues(t, 0, count)
}

func TestSessionCompactionsConcurrentWithContextWindowWrites(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedCompactionSession(t, repo)

	type concurrentWrite struct {
		sample  map[string]interface{}
		event   *models.AgentDeliveryEvent
		effects []*models.AgentDeliveryEffect
	}
	writes := make([]concurrentWrite, 8)
	for i := range writes {
		event := compactionObservationEvent(t, 1, fmt.Sprintf("attempt-concurrent-%d", i))
		event.StreamID = fmt.Sprintf("stream-concurrent-%d", i)
		writes[i] = concurrentWrite{
			sample:  contextWindowSample(int64(100000-i*1000), true),
			event:   event,
			effects: combinedDeliveryBatchEffects([]*models.AgentDeliveryEvent{event}),
		}
	}

	var wg sync.WaitGroup
	errs := make(chan error, len(writes)*2)
	for i := range writes {
		w := writes[i]
		wg.Add(2)
		go func() {
			defer wg.Done()
			if _, err := repo.UpdateSessionContextWindow(context.Background(), compactionSessionID, w.sample); err != nil {
				errs <- err
			}
		}()
		go func() {
			defer wg.Done()
			if _, err := repo.ReceiveAndProjectCanonicalAgentDeliveryEvents(
				context.Background(), []*models.AgentDeliveryEvent{w.event}, w.effects, 0); err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}

	session, err := repo.GetTaskSession(context.Background(), compactionSessionID)
	require.NoError(t, err)
	require.EqualValues(t, 8, metadataInt64(session.Metadata[models.SessionMetaKeyContextCompactionCount]))
	window, ok := session.Metadata[models.SessionMetaKeyContextWindow].(map[string]interface{})
	require.True(t, ok)
	require.NotZero(t, window["used"])
}

func TestSessionCompactionsSchemaSurvivesReopen(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedCompactionSession(t, repo)
	projectCompactionEvents(t, repo, compactionObservationEvent(t, 1, "attempt-reopen"))
	now := time.Now().UTC()
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT OR IGNORE INTO workspaces (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)
	`), "ws-compactions", "Workspace", now, now)
	require.NoError(t, err)
	_, err = repo.db.Exec(repo.db.Rebind(`UPDATE tasks SET workspace_id = ? WHERE id = ?`), "ws-compactions", compactionTaskID)
	require.NoError(t, err)

	// NewWithDB runs schema initialization again; the receipt and count survive.
	repo2, err := NewWithDB(repo.db, repo.ro, nil)
	require.NoError(t, err)
	count, _ := compactionMetadata(t, repo2)
	require.EqualValues(t, 1, count)
}

func TestSessionCompactionsPartialCoverageIsSticky(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedCompactionSession(t, repo)

	reportingPayload, err := json.Marshal(streams.AgentEvent{
		Type:      streams.EventTypeContextCompactionObservation,
		SessionID: compactionNativeID,
		ContextCompaction: &streams.ContextCompactionObservation{
			Kind:            streams.ContextCompactionObservationKindReporting,
			Provider:        copilotNativeCompactionProvider,
			NativeSessionID: compactionNativeID,
			Coverage:        streams.ContextCompactionCoveragePartial,
			Reason:          "completion_data_omitted",
		},
	})
	require.NoError(t, err)
	reportingEvent := &models.AgentDeliveryEvent{
		SessionID:         compactionSessionID,
		IncarnationID:     compactionIncarnation,
		HarnessGeneration: 1,
		StreamID:          compactionStreamID,
		Sequence:          1,
		EventType:         streams.EventTypeContextCompactionObservation,
		Payload:           reportingPayload,
		ReceivedAt:        time.Now().UTC(),
	}
	projectCompactionEvents(t, repo, reportingEvent)
	_, reporting := compactionMetadata(t, repo)
	require.Equal(t, streams.ContextCompactionCoveragePartial, reporting.Coverage)
	require.EqualValues(t, 1, reporting.Revision)

	// A later accepted completion does not repair the missing history.
	projectCompactionEvents(t, repo, compactionObservationEvent(t, 2, "attempt-after-partial"))
	count, reporting := compactionMetadata(t, repo)
	require.EqualValues(t, 1, count)
	require.Equal(t, streams.ContextCompactionCoveragePartial, reporting.Coverage)
	require.EqualValues(t, 2, reporting.Revision)
}

func TestSessionCompactionsRollbackAfterReceiptInsertion(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedCompactionSession(t, repo)

	// Fail the session metadata write after the receipt insert. The whole
	// canonical projection transaction must roll back, including the receipt.
	_, err := repo.db.Exec(`
		CREATE TRIGGER fail_compaction_metadata
		BEFORE UPDATE ON task_sessions
		WHEN NEW.metadata LIKE '%context_compaction_reporting%'
		BEGIN
			SELECT RAISE(ABORT, 'injected metadata failure');
		END`)
	require.NoError(t, err)

	event := compactionObservationEvent(t, 1, "attempt-rollback")
	effects := combinedDeliveryBatchEffects([]*models.AgentDeliveryEvent{event})
	_, err = repo.ReceiveAndProjectCanonicalAgentDeliveryEvents(
		context.Background(), []*models.AgentDeliveryEvent{event}, effects, 0)
	require.Error(t, err)

	var receipts int
	require.NoError(t, repo.db.QueryRowx(repo.db.Rebind(`
		SELECT COUNT(*) FROM task_session_compactions WHERE session_id = ?`), compactionSessionID).Scan(&receipts))
	require.Zero(t, receipts)

	session, err := repo.GetTaskSession(context.Background(), compactionSessionID)
	require.NoError(t, err)
	require.EqualValues(t, 0, metadataInt64(session.Metadata[models.SessionMetaKeyContextCompactionCount]))
	require.Nil(t, session.Metadata[models.SessionMetaKeyContextCompactionReporting])

	// The failed inbox event was not marked projected.
	var projected int
	require.NoError(t, repo.db.QueryRowx(repo.db.Rebind(`
		SELECT COUNT(*) FROM agent_delivery_inbox WHERE stream_id = ? AND projected_at IS NOT NULL`), compactionStreamID).Scan(&projected))
	require.Zero(t, projected)
}

func TestSessionCompactionsAcceptsJournaledPredecessorAfterRebind(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedCompactionSession(t, repo)

	// Simulate an in-place A -> B reset: the current generation now names B and
	// the rebind row retains A as the predecessor.
	_, err := repo.db.Exec(repo.db.Rebind(`
		UPDATE harness_session_generations SET native_session_id = ?
		WHERE session_id = ? AND incarnation_id = ? AND generation = 1`),
		compactionNativeID+"-replacement", compactionSessionID, compactionIncarnation)
	require.NoError(t, err)
	_, err = repo.db.Exec(repo.db.Rebind(`
		INSERT INTO harness_native_session_rebinds
			(id, session_id, incarnation_id, generation, previous_native_session_id,
			 native_session_id, execution_id, rebound_at)
		VALUES (?, ?, ?, 1, ?, ?, ?, ?)`),
		"rebind-compaction-predecessor", compactionSessionID, compactionIncarnation,
		compactionNativeID, compactionNativeID+"-replacement", "exec-rebind", time.Now().UTC())
	require.NoError(t, err)

	// The completion was journaled while A was current. It must still project
	// once after the reset, but cannot mutate the replacement gauge.
	results := projectCompactionEvents(t, repo, compactionObservationEvent(t, 1, "attempt-predecessor"))
	require.Len(t, results, 1)
	require.NotNil(t, results[0].ContextCompactionCount)
	require.EqualValues(t, 1, *results[0].ContextCompactionCount)

	count, reporting := compactionMetadata(t, repo)
	require.EqualValues(t, 1, count)
	require.Equal(t, streams.ContextCompactionAvailabilityUnavailable, reporting.Availability,
		"predecessor evidence must not mark the replacement runtime available")
	session, err := repo.GetTaskSession(context.Background(), compactionSessionID)
	require.NoError(t, err)
	require.Nil(t, session.Metadata[models.SessionMetaKeyContextWindow])
}

func TestSessionCompactionsHistoricalReplayKeepsCurrentAvailability(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedCompactionSession(t, repo)
	ctx := context.Background()

	// Make generation 2 the current replacement while generation 1 remains the
	// historical owner of the predecessor conversation.
	_, err := repo.db.Exec(repo.db.Rebind(`
		INSERT INTO harness_session_generations
			(session_id, incarnation_id, generation, predecessor_generation, native_session_id,
			 agent_type, adapter_version, original_workspace, current_workspace,
			 native_state_reference, creation_reason, created_at, committed_at)
		VALUES (?, ?, 2, 1, ?, 'copilot-acp', '', '', '', '', 'native_replaced', ?, ?)`),
		compactionSessionID, compactionIncarnation, compactionNativeID+"-current", time.Now().UTC(), time.Now().UTC())
	require.NoError(t, err)
	_, err = repo.SetSessionCompactionAvailability(
		ctx, compactionSessionID, streams.ContextCompactionAvailabilityUnavailable, streams.ContextCompactionCoveragePartial)
	require.NoError(t, err)

	// Historical evidence from generation 1 adds its retained increment but
	// cannot claim the replacement runtime is available.
	results := projectCompactionEvents(t, repo, compactionObservationEvent(t, 1, "attempt-historical"))
	require.NotNil(t, results[0].ContextCompactionCount)
	count, reporting := compactionMetadata(t, repo)
	require.EqualValues(t, 1, count)
	require.Equal(t, streams.ContextCompactionAvailabilityUnavailable, reporting.Availability)
	require.Equal(t, streams.ContextCompactionCoveragePartial, reporting.Coverage)

	// A current-generation completion proves availability for the replacement.
	current := compactionObservationEvent(t, 1, "attempt-current")
	current.HarnessGeneration = 2
	current.StreamID = "stream-session-compactions-current"
	current.Payload = mustCompactionPayload(t, compactionNativeID+"-current", "attempt-current")
	projectCompactionEvents(t, repo, current)
	count, reporting = compactionMetadata(t, repo)
	require.EqualValues(t, 2, count)
	require.Equal(t, streams.ContextCompactionAvailabilityAvailable, reporting.Availability)
	require.Equal(t, streams.ContextCompactionCoveragePartial, reporting.Coverage)
}

func TestSessionCompactionsAvailabilityDowngradePreservesCount(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedCompactionSession(t, repo)
	ctx := context.Background()
	projectCompactionEvents(t, repo, compactionObservationEvent(t, 1, "attempt-available"))
	_, reporting := compactionMetadata(t, repo)
	require.Equal(t, streams.ContextCompactionAvailabilityAvailable, reporting.Availability)
	previousRevision := reporting.Revision

	downgraded, err := repo.SetSessionCompactionAvailability(
		ctx, compactionSessionID, streams.ContextCompactionAvailabilityUnavailable, streams.ContextCompactionCoveragePartial)
	require.NoError(t, err)
	require.NotNil(t, downgraded)
	require.Equal(t, streams.ContextCompactionAvailabilityUnavailable, downgraded.Availability)
	require.Greater(t, downgraded.Revision, previousRevision)

	count, stored := compactionMetadata(t, repo)
	require.EqualValues(t, 1, count)
	require.Equal(t, streams.ContextCompactionAvailabilityUnavailable, stored.Availability)
}
