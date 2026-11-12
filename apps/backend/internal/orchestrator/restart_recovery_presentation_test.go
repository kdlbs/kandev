package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
func TestRestartRecoveryRecreateDoesNotAddCompanionFence(t *testing.T) {
	ctx := context.Background()
	service, repo, _, session, _ := newSilentRestoreEligibilityFixture(t, "native", "companion-fence")
	initial, err := repo.GetOpenSessionRecoveryBlock(ctx, session.ID, session.QueueIncarnationID, 1)
	require.NoError(t, err)
	changed, err := repo.ResolveSessionRecoveryBlock(ctx, initial.ID, "resume", time.Now().UTC())
	require.NoError(t, err)
	require.True(t, changed)
	currentGeneration := &models.HarnessSessionGeneration{
		SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		Generation: 2, PredecessorGeneration: 1, NativeSessionID: "native",
		CreationReason: models.SilentRestoreAction,
	}
	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, currentGeneration))
	_, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: "current-submission", SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: 2, OwnerGeneration: 2, PayloadHash: "current-hash",
		Payload: []byte("current prompt"), State: models.DeliverySubmissionInterruptedUnknown,
	})
	require.NoError(t, err)
	currentBlock := &models.SessionRecoveryBlock{
		ID: "current-bound-block", SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		ExpectedGeneration: 2, Reason: unknownPromptOutcome, State: models.RecoveryBlockOpen,
		ConsumerReference: agentDeliveryConsumer, DeliverySubmissionID: "current-submission",
		DeliveryStreamID: "current-stream",
	}
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, currentBlock))
	storedSession, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	historical, exists := models.LoadAgentDeliveryRecovery(storedSession.Metadata)
	require.True(t, exists)
	current, err := repo.GetCurrentHarnessSessionGeneration(ctx, session.ID, session.QueueIncarnationID)
	require.NoError(t, err)
	got, created, err := service.recreateRestartRecoveryBlock(ctx, storedSession, current, historical, true)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, currentBlock.ID, got.ID)
	require.False(t, created)
	blocks, err := repo.ListOpenSessionRecoveryBlocks(ctx, session.ID, session.QueueIncarnationID, 2)
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	require.Equal(t, currentBlock.ID, blocks[0].ID)
}

func TestRestartRecoveryRecreateReusesCurrentFenceControl(t *testing.T) {
	ctx := context.Background()
	service, repo, _, session, _ := newSilentRestoreEligibilityFixture(t, "native", "existing-fence-control")
	block, err := repo.GetOpenSessionRecoveryBlock(ctx, session.ID, session.QueueIncarnationID, 1)
	require.NoError(t, err)
	storedSession, err := repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	recovery, exists := models.LoadAgentDeliveryRecovery(storedSession.Metadata)
	require.True(t, exists)
	generation, err := repo.GetCurrentHarnessSessionGeneration(ctx, session.ID, session.QueueIncarnationID)
	require.NoError(t, err)
	got, created, err := service.recreateRestartRecoveryBlock(ctx, storedSession, generation, recovery, true)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.Equal(t, block.ID, got.ID)
	require.False(t, created)
	blocks, err := repo.ListOpenSessionRecoveryBlocks(ctx, session.ID, session.QueueIncarnationID, 1)
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	require.Equal(t, block.ID, blocks[0].ID)
}

func TestRestartRecoveryRecreatesMissingPresentationFence(t *testing.T) {
	ctx := context.Background()
	service, repo, _, session, _ := newSilentRestoreEligibilityFixture(t, "native", "missing-fence")
	block, err := repo.GetOpenSessionRecoveryBlock(ctx, session.ID, session.QueueIncarnationID, 1)
	require.NoError(t, err)
	_, err = repo.DB().ExecContext(ctx, `DELETE FROM session_recovery_blocks WHERE id = ?`, block.ID)
	require.NoError(t, err)
	inputs, err := service.loadSilentRestoreInputs(ctx, models.SilentRestoreCandidate{
		TaskID: session.TaskID, SessionID: session.ID, WorkspaceID: "ws1"})
	require.NoError(t, err)
	require.NotNil(t, inputs, "an exact complete snapshot must repair its missing fence without browser interaction")
	require.Equal(t, inputs.recovery.SubmissionID, inputs.block.DeliverySubmissionID)
	require.Equal(t, inputs.recovery.StreamID, inputs.block.DeliveryStreamID)
	require.Empty(t, service.agentManager.(*mockAgentManager).capturedPromptCalls)
}

func TestRestartInventoryPreservesLiveAndUnknownPromptOwners(t *testing.T) {
	for _, condition := range []string{"live", "unknown"} {
		t.Run(condition, func(t *testing.T) {
			ctx := context.Background()
			service, repo, base, session, _ := newSilentRestoreEligibilityFixture(t, "native", "live-inventory")
			_, err := repo.DB().ExecContext(ctx, `UPDATE task_sessions SET metadata = '{}', state = 'RUNNING' WHERE id = ?`, session.ID)
			require.NoError(t, err)
			_, err = repo.DB().ExecContext(ctx, `DELETE FROM session_recovery_blocks WHERE session_id = ?`, session.ID)
			require.NoError(t, err)
			_, err = repo.DB().ExecContext(ctx, `UPDATE agent_delivery_submissions SET state = 'dispatching' WHERE session_id = ?`, session.ID)
			require.NoError(t, err)
			root := t.TempDir()
			store, err := journal.Open(journal.Config{Path: journal.CheckStorage(root, session.ID).Path})
			require.NoError(t, err)
			_, err = store.PutSubmission(ctx, journal.Submission{ID: "live-inventory-submission", SessionID: session.ID,
				IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1, StreamID: "live-inventory-stream", Hash: "hash", State: journal.SubmissionDispatching})
			require.NoError(t, err)
			require.NoError(t, store.Close())
			manager := &retainedJournalRecoveryManager{mockAgentManager: base, root: root}
			service.agentManager = manager
			if condition == "live" {
				service.agentManager = &terminatedRestartJournalManager{manager}
			}
			inputs, err := service.loadSilentRestoreInputs(ctx, models.SilentRestoreCandidate{TaskID: session.TaskID, SessionID: session.ID, WorkspaceID: "ws1"})
			require.NoError(t, err)
			require.Nil(t, inputs)
			blocks, err := repo.ListOpenSessionRecoveryBlocks(ctx, session.ID, session.QueueIncarnationID, 1)
			require.NoError(t, err)
			require.Empty(t, blocks, "scanning a normal live prompt must not create a recovery admission fence")
			sql, err := repo.GetAgentDeliverySubmission(ctx, "live-inventory-submission")
			require.NoError(t, err)
			require.Equal(t, models.DeliverySubmissionDispatching, sql.State)
		})
	}
}
