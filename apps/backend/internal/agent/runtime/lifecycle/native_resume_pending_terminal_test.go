package lifecycle

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type pendingTerminalFixture struct {
	manager     *Manager
	execution   *AgentExecution
	repo        *tasksqlite.Repository
	store       *journal.Journal
	path        string
	journalPath string
	effectKey   string
}

func newPendingTerminalFixture(t *testing.T, outcome models.DeliverySubmissionState) *pendingTerminalFixture {
	t.Helper()
	ctx := t.Context()
	journalPath := filepath.Join(t.TempDir(), "delivery.bbolt")
	store := openRetentionLifecycleJournal(t, journalPath)
	for range 2 {
		_, err := store.Append(ctx, journal.Event{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, StreamID: "stream", Type: "usage_update", Payload: []byte(`{}`)})
		require.NoError(t, err)
	}
	manager := newTestManager(t)
	execution := &AgentExecution{ID: "execution", SessionID: "session", ACPSessionID: "native-conversation", DeliveryIncarnationID: "incarnation", DeliveryHarnessGeneration: 2, DeliveryStreamID: "stream", Status: v1.AgentStatusReady, agentctl: nativeResumeJournalPeer(t, store, false)}
	execution.setSessionInitialized(true)
	require.NoError(t, manager.executionStore.Add(execution))
	path := filepath.Join(t.TempDir(), "delivery.db")
	repo, _ := openRetentionLifecycleRepository(t, path)
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
	_, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{ID: peer.ID, SessionID: peer.SessionID, IncarnationID: peer.IncarnationID, HarnessGeneration: 2, OwnerGeneration: 2, PayloadHash: peer.Hash, Payload: peer.Payload, State: models.DeliverySubmissionInterruptedUnknown})
	require.NoError(t, err)
	eventType := "complete"
	if outcome == models.DeliverySubmissionFailed {
		eventType = "error"
	}
	payload := []byte(fmt.Sprintf(`{"turn_id":"turn","data":{"stop_reason":%q}}`, outcome))
	terminal, err := store.Append(ctx, journal.Event{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, StreamID: "stream", SubmissionID: peer.ID, Type: eventType, Terminal: true, Payload: payload})
	require.NoError(t, err)
	event := &models.AgentDeliveryEvent{SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, TaskID: "task", ExecutionID: "execution", OwnerGeneration: 2, ExpectedStreamID: "stream", StreamID: "stream", Sequence: int64(terminal.Sequence), SubmissionID: peer.ID, EventType: eventType, Terminal: true, Payload: payload}
	_, err = repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, event, nil)
	require.NoError(t, err)
	settled, err := repo.SettleAgentDeliveryTerminal(ctx, "stream", event.Sequence, outcome, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, settled)
	require.NoError(t, store.Acknowledge(ctx, "stream", terminal.Sequence))
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{ID: "block", SessionID: "session", IncarnationID: "incarnation", ExpectedGeneration: 2, State: models.RecoveryBlockOpen, Reason: "unknown_prompt_outcome", ConsumerReference: "agent_delivery", DeliverySubmissionID: peer.ID, DeliveryStreamID: "stream"}))
	// Retain a historical fence independently of the normal terminal-aware upsert.
	_, err = repo.DB().Exec(`UPDATE session_recovery_blocks SET state = 'open', resolved_at = NULL, delivery_sequence = 0, delivery_turn_id = '', delivery_outcome = '', authorized_action = '' WHERE id = 'block'`)
	require.NoError(t, err)
	manager.SetAgentDeliveryRepository(repo)
	return &pendingTerminalFixture{manager: manager, execution: execution, repo: repo, store: store, path: path, journalPath: journalPath, effectKey: fmt.Sprintf("agent_delivery.terminal:stream:%d", terminal.Sequence)}
}

func TestNativeResumePendingTerminalReopensAndReconcilesLoadedOwner(t *testing.T) {
	f := newPendingTerminalFixture(t, models.DeliverySubmissionFailed)
	require.NoError(t, f.manager.AcknowledgeNativeResumeDelivery(t.Context(), "session"))
	require.NoError(t, f.store.Close())
	require.NoError(t, f.repo.DB().Close())
	f.store = openRetentionLifecycleJournal(t, f.journalPath)
	f.repo, _ = openRetentionLifecycleRepository(t, f.path)
	f.manager.SetAgentDeliveryRepository(f.repo)
	f.execution.agentctl = nativeResumeJournalPeer(t, f.store, false)
	proof, err := f.manager.ReconcileLoadedNativeDelivery(t.Context(), "session")
	require.NoError(t, err)
	require.NotNil(t, proof)
	resolved, err := f.repo.ResolveSessionRecoveryBlockWithNativeEvidence(t.Context(), "block", proof, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, resolved)
}

func TestNativeResumePendingTerminalMismatchPreservesFence(t *testing.T) {
	for _, scenario := range []string{"hash", "owner", "missing effect", "projection", "live SQL", "live peer"} {
		t.Run(scenario, func(t *testing.T) {
			f := newPendingTerminalFixture(t, models.DeliverySubmissionFailed)
			var err error
			switch scenario {
			case "hash":
				_, err = f.repo.DB().Exec(`UPDATE agent_delivery_submissions SET payload_hash = 'foreign' WHERE id = 'interrupted'`)
			case "owner":
				_, err = f.repo.DB().Exec(`UPDATE agent_delivery_submissions SET incarnation_id = 'foreign' WHERE id = 'interrupted'`)
			case "missing effect":
				_, err = f.repo.DB().Exec(`DELETE FROM agent_delivery_effects WHERE effect_key = ?`, f.effectKey)
			case "projection":
				_, err = f.repo.DB().Exec(`UPDATE agent_delivery_cursors SET projected_sequence = 2 WHERE stream_id = 'stream'`)
			case "live SQL":
				_, err = f.repo.PrepareAgentDeliverySubmission(t.Context(), &models.AgentDeliverySubmission{ID: "live", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, OwnerGeneration: 2, PayloadHash: "live", Payload: []byte("live"), State: models.DeliverySubmissionAccepted})
			case "live peer":
				_, err = f.store.PutSubmission(t.Context(), journal.Submission{ID: "live", SessionID: "session", IncarnationID: "incarnation", HarnessGeneration: 2, StreamID: "stream", Hash: "live", Payload: []byte("live"), State: journal.SubmissionAccepted})
			}
			require.NoError(t, err)
			err = f.manager.AcknowledgeNativeResumeDelivery(t.Context(), "session")
			require.Error(t, err)
			block, err := f.repo.GetSessionRecoveryBlock(t.Context(), "block")
			require.NoError(t, err)
			require.Equal(t, models.RecoveryBlockOpen, block.State)
			peer, err := f.store.GetSubmission(t.Context(), "interrupted")
			require.NoError(t, err)
			require.False(t, peer.Retired)
			row, err := f.repo.GetAgentDeliverySubmission(t.Context(), "interrupted")
			require.NoError(t, err)
			require.Equal(t, models.DeliverySubmissionFailed, row.State)
			require.Zero(t, row.TerminalSequence)
			require.Nil(t, row.FullySettledAt)
		})
	}
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.24
// @covers AC-PLATFORM-DELIVERY-RETENTION-003.1
func TestNativeResumePendingTerminalSettlement(t *testing.T) {
	for _, outcome := range []models.DeliverySubmissionState{models.DeliverySubmissionCompleted, models.DeliverySubmissionFailed, models.DeliverySubmissionCancelled} {
		t.Run(string(outcome), func(t *testing.T) {
			f := newPendingTerminalFixture(t, outcome)
			before, err := f.repo.GetAgentDeliverySubmission(t.Context(), "interrupted")
			require.NoError(t, err)
			require.Empty(t, before.TerminalStreamID)
			require.Zero(t, before.TerminalSequence)
			require.Nil(t, before.FullySettledAt)
			effect, err := f.repo.GetAgentDeliveryEffect(t.Context(), f.effectKey)
			require.NoError(t, err)
			require.Equal(t, models.DeliveryEffectBlockResolved, effect.State)
			for range 2 {
				require.NoError(t, f.manager.AcknowledgeNativeResumeDelivery(t.Context(), "session"))
				proof, err := f.manager.GetNativeResumeDeliveryEvidence(t.Context(), "session")
				require.NoError(t, err)
				require.Len(t, proof.Submissions, 1)
				require.Equal(t, int64(3), proof.Submissions[0].TerminalSequence)
				require.Equal(t, "native-conversation", proof.NativeSessionID)
			}
			block, err := f.repo.GetSessionRecoveryBlock(t.Context(), "block")
			require.NoError(t, err)
			require.Equal(t, models.RecoveryBlockOpen, block.State, "journal acknowledgement cannot release a SQL fence")
			proof, err := f.manager.GetNativeResumeDeliveryEvidence(t.Context(), "session")
			require.NoError(t, err)
			resolved, err := f.repo.ResolveSessionRecoveryBlockWithNativeEvidence(t.Context(), "block", proof, time.Now().UTC())
			require.NoError(t, err)
			require.True(t, resolved)
			after, err := f.repo.GetAgentDeliverySubmission(t.Context(), "interrupted")
			require.NoError(t, err)
			require.Equal(t, before, after)
			afterEffect, err := f.repo.GetAgentDeliveryEffect(t.Context(), f.effectKey)
			require.NoError(t, err)
			require.Equal(t, effect, afterEffect, "workflow settlement remains pending")
			retained, err := f.store.GetSubmission(t.Context(), "interrupted")
			require.NoError(t, err)
			require.True(t, retained.Retired)
			require.Equal(t, journal.SubmissionInterruptedUnknown, retained.State)
			require.Equal(t, before.Payload, retained.Payload)
			require.Empty(t, f.execution.deliverySubmissionIDSnapshot())
		})
	}
}
