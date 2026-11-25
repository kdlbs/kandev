package lifecycle

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type unprovenCompletionFixture struct {
	manager     *Manager
	execution   *AgentExecution
	repo        *tasksqlite.Repository
	store       *journal.Journal
	journalPath string
	database    string
	peer        journal.Submission
}

// newUnprovenCompletionFixture reproduces the foreground-handoff mismatch with
// real SQLite and bbolt stores: SQL completed without terminal proof against an
// interrupted-unknown journal entry.
func newUnprovenCompletionFixture(t *testing.T) *unprovenCompletionFixture {
	t.Helper()
	ctx := t.Context()
	journalPath := filepath.Join(t.TempDir(), "delivery.bbolt")
	store := openRetentionLifecycleJournal(t, journalPath)
	for range 2 {
		_, err := store.Append(ctx, journal.Event{
			SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2,
			StreamID: "stream", Type: "usage_update", Payload: []byte(`{}`),
		})
		require.NoError(t, err)
	}
	require.NoError(t, store.Acknowledge(ctx, "stream", 2))
	manager := newTestManager(t)
	execution := &AgentExecution{
		ID: "execution", SessionID: "session", ACPSessionID: "native-conversation",
		DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 2,
		DeliveryStreamID: "stream", Status: v1.AgentStatusReady,
		agentctl: nativeResumeJournalPeer(t, store, false),
	}
	execution.setSessionInitialized(true)
	require.NoError(t, manager.executionStore.Add(execution))
	databasePath := filepath.Join(t.TempDir(), "delivery.db")
	repo, _ := openRetentionLifecycleRepository(t, databasePath)
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace", Name: "Recovery"}))
	seedProjectionResumeOwner(t, repo)
	_, err := repo.DB().Exec(`UPDATE tasks SET workspace_id = 'workspace' WHERE id = 'task'`)
	require.NoError(t, err)
	for sequence := int64(1); sequence <= 2; sequence++ {
		_, err := repo.ProjectAgentDeliveryEvent(ctx, projectionSQLReceipt(t, repo, sequence), nil)
		require.NoError(t, err)
	}
	peer := journal.Submission{ID: "interrupted", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, StreamID: "stream", Hash: "hash", Payload: []byte("original prompt"), State: journal.SubmissionInterruptedUnknown}
	_, err = store.PutSubmission(ctx, peer)
	require.NoError(t, err)
	_, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{ID: peer.ID, SessionID: peer.SessionID, IncarnationID: peer.IncarnationID, HarnessGeneration: 2, OwnerGeneration: 2, PayloadHash: peer.Hash, Payload: peer.Payload, State: models.DeliverySubmissionCompleted, Outcome: string(models.DeliverySubmissionCompleted)})
	require.NoError(t, err)
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{ID: "block", SessionID: "session", IncarnationID: "incarnation", ExpectedGeneration: 2, State: models.RecoveryBlockOpen, Reason: "unknown_prompt_outcome", ConsumerReference: "agent_delivery", DeliverySubmissionID: peer.ID, DeliveryStreamID: "stream"}))
	manager.SetAgentDeliveryRepository(repo)
	return &unprovenCompletionFixture{
		manager: manager, execution: execution, repo: repo, store: store,
		journalPath: journalPath, database: databasePath, peer: peer,
	}
}

func (f *unprovenCompletionFixture) submission(t *testing.T) *models.AgentDeliverySubmission {
	t.Helper()
	row, err := f.repo.GetAgentDeliverySubmission(t.Context(), f.peer.ID)
	require.NoError(t, err)
	return row
}

func (f *unprovenCompletionFixture) submissionMetadata(t *testing.T) *models.AgentDeliverySubmission {
	t.Helper()
	row, err := f.repo.GetAgentDeliverySubmissionMetadata(t.Context(), f.peer.ID)
	require.NoError(t, err)
	return row
}

func (f *unprovenCompletionFixture) requireRecoveryBlocked(t *testing.T, before *models.AgentDeliverySubmission) {
	t.Helper()
	block, err := f.repo.GetSessionRecoveryBlock(t.Context(), "block")
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockOpen, block.State)
	retained, err := f.store.GetSubmission(t.Context(), f.peer.ID)
	require.NoError(t, err)
	require.False(t, retained.Retired)
	require.Equal(t, f.peer.State, retained.State)
	require.Equal(t, f.peer.Payload, retained.Payload)
	after, err := f.repo.GetAgentDeliverySubmissionMetadata(t.Context(), f.peer.ID)
	require.NoError(t, err)
	require.Equal(t, before, after)
}

func TestNativeResumeUnprovenSQLCompletionRecovery(t *testing.T) {
	f := newUnprovenCompletionFixture(t)
	before := f.submission(t)
	require.NoError(t, f.manager.AcknowledgeNativeResumeDelivery(t.Context(), "session"))
	retained, err := f.store.GetSubmission(t.Context(), f.peer.ID)
	require.NoError(t, err)
	require.True(t, retained.Retired)
	require.Equal(t, journal.SubmissionInterruptedUnknown, retained.State)
	require.Equal(t, f.peer.Payload, retained.Payload)
	require.Equal(t, before, f.submission(t), "historical SQL completion must remain immutable")

	proof, err := f.manager.GetNativeResumeDeliveryEvidence(t.Context(), "session")
	require.NoError(t, err)
	require.Equal(t, "native-conversation", proof.NativeSessionID)
	require.Len(t, proof.Submissions, 1)
	require.True(t, proof.Submissions[0].UnprovenCompletion)
	require.Equal(t, models.DeliverySubmissionCompleted, proof.Submissions[0].State)
	require.Zero(t, proof.Submissions[0].TerminalSequence)
	require.Equal(t, int64(2), proof.Cursor.ProjectedSequence)

	resolved, err := f.repo.ResolveSessionRecoveryBlockWithNativeEvidence(t.Context(), "block", proof, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, resolved)
	block, err := f.repo.GetSessionRecoveryBlock(t.Context(), "block")
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockResolved, block.State)
	require.Equal(t, before, f.submission(t))
	require.NoError(t, f.manager.AcknowledgeNativeResumeDelivery(t.Context(), "session"), "repeat recovery uses retained evidence")
}

// TestNativeResumeUnprovenSQLCompletionReload proves a restart reuses the
// retained journal retirement instead of retiring or settling again.
func TestNativeResumeUnprovenSQLCompletionReload(t *testing.T) {
	f := newUnprovenCompletionFixture(t)
	before := f.submission(t)
	require.NoError(t, f.manager.AcknowledgeNativeResumeDelivery(t.Context(), "session"))
	require.NoError(t, f.store.Close())
	require.NoError(t, f.repo.DB().Close())
	f.store = openRetentionLifecycleJournal(t, f.journalPath)
	f.repo, _ = openRetentionLifecycleRepository(t, f.database)
	f.manager.SetAgentDeliveryRepository(f.repo)
	f.execution.agentctl = nativeResumeJournalPeer(t, f.store, false)

	proof, err := f.manager.ReconcileLoadedNativeDelivery(t.Context(), "session")
	require.NoError(t, err)
	require.NotNil(t, proof)
	require.Len(t, proof.Submissions, 1)
	require.True(t, proof.Submissions[0].UnprovenCompletion)
	resolved, err := f.repo.ResolveSessionRecoveryBlockWithNativeEvidence(t.Context(), "block", proof, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, resolved)
	require.Equal(t, before, f.submission(t))
	retained, err := f.store.GetSubmission(t.Context(), f.peer.ID)
	require.NoError(t, err)
	require.True(t, retained.Retired)
}

func TestNativeResumeUnprovenSQLCompletionDiscovery(t *testing.T) {
	f := newUnprovenCompletionFixture(t)
	status, err := f.execution.agentctl.GetDeliveryStatus(t.Context(), "stream")
	require.NoError(t, err)
	peer, err := f.execution.agentctl.ListDeliverySubmissions(t.Context(), "session")
	require.NoError(t, err)
	active, err := activeBackendSubmissions(t.Context(), f.repo, "session")
	require.NoError(t, err)
	rows, err := nativeResumeUnprovenBackendRows(t.Context(), f.execution, status, peer, active, f.repo)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, f.peer.ID, rows[0].ID)
	require.Equal(t, models.DeliverySubmissionCompleted, rows[0].State)

	_, err = f.repo.DB().Exec(`UPDATE agent_delivery_submissions SET terminal_sequence = 1 WHERE id = 'interrupted'`)
	require.NoError(t, err)
	rows, err = nativeResumeUnprovenBackendRows(t.Context(), f.execution, status, peer, active, f.repo)
	require.NoError(t, err)
	require.Empty(t, rows, "a terminal binding must not enter unproven discovery")
}

func TestNativeResumeUnprovenSQLCompletionHistoricalStream(t *testing.T) {
	ctx := t.Context()
	journalPath := filepath.Join(t.TempDir(), "delivery.bbolt")
	store := openRetentionLifecycleJournal(t, journalPath)
	for range 2 {
		_, err := store.Append(ctx, journal.Event{
			SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2,
			StreamID: "stream", Type: "usage_update", Payload: []byte(`{}`),
		})
		require.NoError(t, err)
	}
	require.NoError(t, store.Acknowledge(ctx, "stream", 2))
	manager := newTestManager(t)
	execution := &AgentExecution{
		ID: "execution", SessionID: "session", ACPSessionID: "native-conversation",
		DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 2,
		DeliveryStreamID: "stream", Status: v1.AgentStatusReady,
		agentctl: nativeResumeJournalPeer(t, store, false),
	}
	execution.setSessionInitialized(true)
	require.NoError(t, manager.executionStore.Add(execution))
	repo, _ := openRetentionLifecycleRepository(t, filepath.Join(t.TempDir(), "delivery.db"))
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "workspace", Name: "Recovery"}))
	seedProjectionResumeOwner(t, repo)
	_, err := repo.DB().Exec(`UPDATE tasks SET workspace_id = 'workspace' WHERE id = 'task'`)
	require.NoError(t, err)
	for sequence := int64(1); sequence <= 2; sequence++ {
		_, err := repo.ProjectAgentDeliveryEvent(ctx, projectionSQLReceipt(t, repo, sequence), nil)
		require.NoError(t, err)
	}
	oldEvent := &models.AgentDeliveryEvent{
		SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1,
		StreamID: "earlier", Sequence: 1, EventType: "usage_update", Payload: []byte(`{}`),
	}
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, oldEvent, 1)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, oldEvent, nil)
	require.NoError(t, err)
	peer := journal.Submission{ID: "interrupted", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 1, StreamID: "earlier", Hash: "hash", Payload: []byte("original prompt"), State: journal.SubmissionInterruptedUnknown}
	_, err = store.PutSubmission(ctx, peer)
	require.NoError(t, err)
	_, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{ID: peer.ID, SessionID: peer.SessionID, IncarnationID: peer.IncarnationID, HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: peer.Hash, Payload: peer.Payload, State: models.DeliverySubmissionCompleted, Outcome: string(models.DeliverySubmissionCompleted)})
	require.NoError(t, err)
	manager.SetAgentDeliveryRepository(repo)
	require.NoError(t, manager.AcknowledgeNativeResumeDelivery(ctx, "session"))
	proof, err := manager.GetNativeResumeDeliveryEvidence(ctx, "session")
	require.NoError(t, err)
	require.Len(t, proof.Submissions, 1)
	require.True(t, proof.Submissions[0].UnprovenCompletion)
	require.Equal(t, "earlier", proof.Submissions[0].StreamID)
	require.NotNil(t, proof.Submissions[0].Cursor)
	require.Equal(t, "earlier", proof.Submissions[0].Cursor.StreamID)
	require.Equal(t, int64(1), proof.Submissions[0].Cursor.HarnessGeneration)

	_, err = repo.DB().Exec(`UPDATE agent_delivery_cursors SET received_sequence = 2 WHERE stream_id = 'earlier'`)
	require.NoError(t, err)
	_, err = manager.GetNativeResumeDeliveryEvidence(ctx, "session")
	require.Error(t, err, "an unprojected historical receipt must invalidate the proof")
	_, err = repo.DB().Exec(`UPDATE agent_delivery_cursors SET projected_sequence = 2 WHERE stream_id = 'earlier'`)
	require.NoError(t, err)
	refreshed, err := manager.GetNativeResumeDeliveryEvidence(ctx, "session")
	require.NoError(t, err)
	require.Equal(t, int64(2), refreshed.Submissions[0].Cursor.ProjectedSequence)
	_, err = repo.DB().Exec(`UPDATE agent_delivery_cursors SET remote_high_water = 3 WHERE stream_id = 'earlier'`)
	require.NoError(t, err)
	_, err = manager.GetNativeResumeDeliveryEvidence(ctx, "session")
	require.Error(t, err, "an unreceived historical high-water mark must invalidate the proof")
	_, err = repo.DB().Exec(`UPDATE agent_delivery_cursors SET received_sequence = 3, projected_sequence = 3 WHERE stream_id = 'earlier'`)
	require.NoError(t, err)
	refreshed, err = manager.GetNativeResumeDeliveryEvidence(ctx, "session")
	require.NoError(t, err)
	require.Equal(t, int64(3), refreshed.Submissions[0].Cursor.ProjectedSequence)
	require.Equal(t, int64(3), refreshed.Submissions[0].Cursor.RemoteHighWater)
}

func TestNativeResumeUnprovenSQLCompletionNegativeEvidence(t *testing.T) {
	mutations := map[string]string{
		"terminal stream":        `UPDATE agent_delivery_submissions SET terminal_stream_id = 'stream' WHERE id = 'interrupted'`,
		"terminal sequence":      `UPDATE agent_delivery_submissions SET terminal_sequence = 1 WHERE id = 'interrupted'`,
		"terminal turn":          `UPDATE agent_delivery_submissions SET terminal_turn_id = 'turn' WHERE id = 'interrupted'`,
		"settled marker":         `UPDATE agent_delivery_submissions SET fully_settled_at = CURRENT_TIMESTAMP WHERE id = 'interrupted'`,
		"pruned marker":          `UPDATE agent_delivery_submissions SET payload_pruned_at = CURRENT_TIMESTAMP WHERE id = 'interrupted'`,
		"failed label":           `UPDATE agent_delivery_submissions SET state = 'failed', outcome = 'failed' WHERE id = 'interrupted'`,
		"foreign hash":           `UPDATE agent_delivery_submissions SET payload_hash = 'foreign' WHERE id = 'interrupted'`,
		"foreign incarnation":    `UPDATE agent_delivery_submissions SET incarnation_id = 'foreign' WHERE id = 'interrupted'`,
		"foreign generation":     `UPDATE agent_delivery_submissions SET harness_generation = 3 WHERE id = 'interrupted'`,
		"projection gap":         `UPDATE agent_delivery_cursors SET projected_sequence = 1 WHERE stream_id = 'stream'`,
		"invalid retired cursor": `UPDATE agent_delivery_cursors SET retired_sequence = 3 WHERE stream_id = 'stream'`,
		"terminal inbox": `INSERT INTO agent_delivery_inbox (stream_id, sequence, session_id, incarnation_id, harness_generation, submission_id, event_type, payload, terminal, received_at)
			VALUES ('stream', 99, 'session', 'incarnation', 2, 'interrupted', 'complete', '{}', 1, CURRENT_TIMESTAMP)`,
		"terminal effect": `INSERT INTO agent_delivery_effects (effect_key, stream_id, sequence, effect_type, state, created_at, session_id, incarnation_id, harness_generation, submission_id, outcome)
			VALUES ('foreign-terminal', 'stream', 99, 'agent_delivery.terminal_settlement', 'pending', CURRENT_TIMESTAMP, 'session', 'incarnation', 2, 'interrupted', 'completed')`,
	}
	for scenario, mutation := range mutations {
		t.Run(scenario, func(t *testing.T) {
			f := newUnprovenCompletionFixture(t)
			_, err := f.repo.DB().Exec(mutation)
			require.NoError(t, err)
			before := f.submissionMetadata(t)
			require.Error(t, f.manager.AcknowledgeNativeResumeDelivery(t.Context(), "session"))
			f.requireRecoveryBlocked(t, before)
		})
	}
}

func TestNativeResumeUnprovenSQLCompletionMixedInventory(t *testing.T) {
	t.Run("independent live work", func(t *testing.T) {
		f := newUnprovenCompletionFixture(t)
		_, err := f.repo.PrepareAgentDeliverySubmission(t.Context(), &models.AgentDeliverySubmission{ID: "live", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, OwnerGeneration: 2, PayloadHash: "live", Payload: []byte("live"), State: models.DeliverySubmissionAccepted})
		require.NoError(t, err)
		_, err = f.store.PutSubmission(t.Context(), journal.Submission{ID: "live", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, StreamID: "stream", Hash: "live", Payload: []byte("live"), State: journal.SubmissionAccepted})
		require.NoError(t, err)
		before := f.submissionMetadata(t)
		require.Error(t, f.manager.AcknowledgeNativeResumeDelivery(t.Context(), "session"))
		f.requireRecoveryBlocked(t, before)
		live, err := f.repo.GetAgentDeliverySubmission(t.Context(), "live")
		require.NoError(t, err)
		require.Equal(t, models.DeliverySubmissionAccepted, live.State)
	})
	t.Run("conflicting candidate after eligible", func(t *testing.T) {
		f := newUnprovenCompletionFixture(t)
		conflict := journal.Submission{ID: "conflict", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, StreamID: "stream", Hash: "conflict-hash", Payload: []byte("conflict prompt"), State: journal.SubmissionInterruptedUnknown}
		_, err := f.store.PutSubmission(t.Context(), conflict)
		require.NoError(t, err)
		_, err = f.repo.PrepareAgentDeliverySubmission(t.Context(), &models.AgentDeliverySubmission{ID: conflict.ID, SessionID: conflict.SessionID, IncarnationID: conflict.IncarnationID, HarnessGeneration: 2, OwnerGeneration: 2, PayloadHash: "foreign-hash", Payload: conflict.Payload, State: models.DeliverySubmissionCompleted, Outcome: string(models.DeliverySubmissionCompleted)})
		require.NoError(t, err)
		before := f.submissionMetadata(t)
		require.Error(t, f.manager.AcknowledgeNativeResumeDelivery(t.Context(), "session"))
		f.requireRecoveryBlocked(t, before)
		retained, err := f.store.GetSubmission(t.Context(), conflict.ID)
		require.NoError(t, err)
		require.False(t, retained.Retired)
	})
}

func TestNativeResumeUnprovenSQLCompletionStaleProof(t *testing.T) {
	f := newUnprovenCompletionFixture(t)
	require.NoError(t, f.manager.AcknowledgeNativeResumeDelivery(t.Context(), "session"))
	proof, err := f.manager.GetNativeResumeDeliveryEvidence(t.Context(), "session")
	require.NoError(t, err)
	_, err = f.repo.DB().Exec(`UPDATE agent_delivery_submissions SET terminal_stream_id = 'stream', terminal_sequence = 1 WHERE id = 'interrupted'`)
	require.NoError(t, err)
	resolved, err := f.repo.ResolveSessionRecoveryBlockWithNativeEvidence(t.Context(), "block", proof, time.Now().UTC())
	require.NoError(t, err)
	require.False(t, resolved, "a terminal that arrives after assessment invalidates the proof")
	block, err := f.repo.GetSessionRecoveryBlock(t.Context(), "block")
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockOpen, block.State)
	row, err := f.repo.GetAgentDeliverySubmission(t.Context(), "interrupted")
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionCompleted, row.State)
	require.Equal(t, int64(1), row.TerminalSequence)
}

// TestNativeResumeUnprovenSQLCompletionRequiresExactOwner keeps the ordinary
// terminal and legacy dispositions unchanged when identity is foreign.
func TestNativeResumeUnprovenSQLCompletionRequiresExactOwner(t *testing.T) {
	for scenario, mutate := range map[string]func(*unprovenCompletionFixture){
		"foreign journal incarnation": func(f *unprovenCompletionFixture) {
			f.execution.DeliveryIncarnationID = "foreign"
		},
		"foreign stream owner": func(f *unprovenCompletionFixture) {
			f.execution.DeliveryStreamID = "foreign"
		},
	} {
		t.Run(scenario, func(t *testing.T) {
			f := newUnprovenCompletionFixture(t)
			mutate(f)
			before := f.submissionMetadata(t)
			require.Error(t, f.manager.AcknowledgeNativeResumeDelivery(t.Context(), "session"))
			f.requireRecoveryBlocked(t, before)
		})
	}
}
