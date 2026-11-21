package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestClearAgentDeliveryRecoveryResolvesMatchingBlock(t *testing.T) {
	testClearAgentDeliveryRecoveryResolvesMatchingBlock(t, newRepoForSessionTests(t))
}

func TestPostgresClearAgentDeliveryRecoveryResolvesMatchingBlock(t *testing.T) {
	testClearAgentDeliveryRecoveryResolvesMatchingBlock(t, newPostgresRetentionTestRepo(t))
}

func testClearAgentDeliveryRecoveryResolvesMatchingBlock(t *testing.T, repo *Repository) {
	// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
	// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.9
	ctx := context.Background()
	const suffix = "clear-matching"
	const submissionID = "submission-clear-matching"
	seedDeliverySettlement(t, repo, ctx, suffix, submissionID)
	event := terminalDeliveryEvent(suffix, submissionID, "stream-clear-matching", 1, "complete")
	recovery := models.AgentDeliveryRecovery{
		Phase: models.AgentDeliveryRecoveryUncertain, Revision: 1,
		SessionID: event.SessionID, AgentExecutionID: "execution-clear-matching",
		SubmissionID: submissionID, StreamID: "stream-clear-notice",
		IncarnationID: event.IncarnationID, HarnessGeneration: event.HarnessGeneration,
		PromptGeneration: 1, UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, repo.SetSessionMetadataKey(ctx, event.SessionID,
		models.SessionMetaKeyAgentDeliveryRecovery, recovery))
	_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE session_recovery_blocks
		SET delivery_stream_id = ? WHERE id = ?`), recovery.StreamID, "block-delivery-"+suffix)
	require.NoError(t, err)

	otherSubmission := &models.AgentDeliverySubmission{
		ID: "submission-clear-other", SessionID: event.SessionID, IncarnationID: event.IncarnationID,
		HarnessGeneration: event.HarnessGeneration, OwnerGeneration: 2,
		DispatchAttemptID: "dispatch-clear-other", PayloadHash: "hash-clear-other",
		Payload: []byte("other prompt"), State: models.DeliverySubmissionDispatching,
	}
	_, err = repo.PrepareAgentDeliverySubmission(ctx, otherSubmission)
	require.NoError(t, err)
	otherBlock := &models.SessionRecoveryBlock{
		ID: "block-clear-other", SessionID: event.SessionID, IncarnationID: event.IncarnationID,
		ExpectedGeneration: event.HarnessGeneration, Reason: deliveryUnresolvedWorkReason,
		State: models.RecoveryBlockOpen, ConsumerReference: reconstructionDeliveryConsumer,
		DeliverySubmissionID: otherSubmission.ID, DeliveryStreamID: "stream-clear-other",
	}
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, otherBlock))

	projectDeliveryEvent(t, repo, ctx, event)
	settled, err := repo.SettleAgentDeliveryTerminal(ctx, event.StreamID, event.Sequence,
		models.DeliverySubmissionCompleted, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, settled)
	block, err := repo.GetSessionRecoveryBlock(ctx, "block-delivery-"+suffix)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockOpen, block.State, "settlement on a reattached stream leaves the notice's old-stream block for Clear")
	require.Equal(t, recovery.StreamID, block.DeliveryStreamID)

	changed, err := repo.ClearAgentDeliveryRecovery(ctx, event.SessionID, event.IncarnationID,
		submissionID, event.HarnessGeneration)
	require.NoError(t, err)
	require.True(t, changed)

	block, err = repo.GetSessionRecoveryBlock(ctx, "block-delivery-"+suffix)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockResolved, block.State)
	require.Equal(t, event.StreamID, block.DeliveryStreamID)
	require.Equal(t, event.Sequence, block.DeliverySequence)
	require.Equal(t, "turn-"+suffix, block.DeliveryTurnID)
	require.Equal(t, string(models.DeliverySubmissionCompleted), block.DeliveryOutcome)
	require.Equal(t, "durable_delivery_completed", block.AuthorizedAction)
	require.NotNil(t, block.ResolvedAt)

	other, err := repo.GetSessionRecoveryBlock(ctx, otherBlock.ID)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockOpen, other.State)
	require.Equal(t, otherSubmission.ID, other.DeliverySubmissionID)
	require.Equal(t, "stream-clear-other", other.DeliveryStreamID)

	session, err := repo.GetTaskSession(ctx, event.SessionID)
	require.NoError(t, err)
	settledRecovery, ok := models.LoadAgentDeliveryRecovery(session.Metadata)
	require.True(t, ok)
	require.Equal(t, models.AgentDeliveryRecoverySettled, settledRecovery.Phase)
	require.Equal(t, event.StreamID, settledRecovery.StreamID)
	require.Equal(t, int64(2), settledRecovery.Revision)
	putRetentionWorkflowOutcome(t, repo, event, "not_required", "turn-"+suffix)
	completed, err := repo.CompleteAgentDeliveryTerminalSettlement(ctx,
		terminalRetentionEffectKey(event), time.Now().UTC())
	require.NoError(t, err)
	require.True(t, completed)

	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: event.SessionID, IncarnationID: event.IncarnationID,
		Generation: event.HarnessGeneration, NativeSessionID: "native-clear-matching",
	}))
	tx, err := repo.ro.BeginTxx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	historical, valid, err := repo.loadHistoricalTerminalSettlementTx(ctx, tx, settledRecovery,
		"native-clear-matching", block.ID, block.Reason)
	require.NoError(t, err)
	require.True(t, valid)
	require.Equal(t, event.StreamID, historical.streamID)
	require.Equal(t, event.Sequence, historical.sequence)
}

func TestClearAgentDeliveryRecoveryWithoutTerminalEffectSettlesNoticeOnly(t *testing.T) {
	testClearAgentDeliveryRecoveryWithoutTerminalEffectSettlesNoticeOnly(t, newRepoForSessionTests(t))
}

func TestPostgresClearAgentDeliveryRecoveryWithoutTerminalEffectSettlesNoticeOnly(t *testing.T) {
	testClearAgentDeliveryRecoveryWithoutTerminalEffectSettlesNoticeOnly(t, newPostgresRetentionTestRepo(t))
}

func testClearAgentDeliveryRecoveryWithoutTerminalEffectSettlesNoticeOnly(t *testing.T, repo *Repository) {
	// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
	ctx := context.Background()
	const suffix = "clear-unsettled"
	const submissionID = "submission-clear-unsettled"
	seedDeliverySettlement(t, repo, ctx, suffix, submissionID)
	event := terminalDeliveryEvent(suffix, submissionID, "stream-clear-unsettled", 1, "complete")
	recovery := models.AgentDeliveryRecovery{
		Phase: models.AgentDeliveryRecoveryUncertain, Revision: 1,
		SessionID: event.SessionID, AgentExecutionID: "execution-clear-unsettled",
		SubmissionID: submissionID, StreamID: event.StreamID,
		IncarnationID: event.IncarnationID, HarnessGeneration: event.HarnessGeneration,
		PromptGeneration: 1, UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, repo.SetSessionMetadataKey(ctx, event.SessionID,
		models.SessionMetaKeyAgentDeliveryRecovery, recovery))

	changed, err := repo.ClearAgentDeliveryRecovery(ctx, event.SessionID, event.IncarnationID,
		submissionID, event.HarnessGeneration)
	require.NoError(t, err)
	require.True(t, changed)

	session, err := repo.GetTaskSession(ctx, event.SessionID)
	require.NoError(t, err)
	current, ok := models.LoadAgentDeliveryRecovery(session.Metadata)
	require.True(t, ok)
	require.Equal(t, models.AgentDeliveryRecoverySettled, current.Phase)
	require.Equal(t, int64(2), current.Revision)
	block, err := repo.GetSessionRecoveryBlock(ctx, "block-delivery-"+suffix)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockOpen, block.State)
	require.Equal(t, submissionID, block.DeliverySubmissionID)
}

func TestClearAgentDeliveryRecoveryRepairsSettledNoticeAfterLateReattachedTerminal(t *testing.T) {
	// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	const suffix = "clear-late-terminal"
	const submissionID = "submission-clear-late-terminal"
	seedDeliverySettlement(t, repo, ctx, suffix, submissionID)
	event := terminalDeliveryEvent(suffix, submissionID, "stream-clear-late-terminal", 1, "complete")
	recovery := models.AgentDeliveryRecovery{
		Phase: models.AgentDeliveryRecoveryUncertain, Revision: 1,
		SessionID: event.SessionID, AgentExecutionID: "execution-clear-late-terminal",
		SubmissionID: submissionID, StreamID: "stream-clear-before-reattach",
		IncarnationID: event.IncarnationID, HarnessGeneration: event.HarnessGeneration,
		PromptGeneration: 1, UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, repo.SetSessionMetadataKey(ctx, event.SessionID,
		models.SessionMetaKeyAgentDeliveryRecovery, recovery))
	_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE session_recovery_blocks
		SET delivery_stream_id = ? WHERE id = ?`), recovery.StreamID, "block-delivery-"+suffix)
	require.NoError(t, err)

	changed, err := repo.ClearAgentDeliveryRecovery(ctx, event.SessionID, event.IncarnationID,
		submissionID, event.HarnessGeneration)
	require.NoError(t, err)
	require.True(t, changed)
	block, err := repo.GetSessionRecoveryBlock(ctx, "block-delivery-"+suffix)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockOpen, block.State)

	projectDeliveryEvent(t, repo, ctx, event)
	settled, err := repo.SettleAgentDeliveryTerminal(ctx, event.StreamID, event.Sequence,
		models.DeliverySubmissionCompleted, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, settled)

	changed, err = repo.ClearAgentDeliveryRecovery(ctx, event.SessionID, event.IncarnationID,
		submissionID, event.HarnessGeneration)
	require.NoError(t, err)
	require.True(t, changed, "a settled notice can bind terminal evidence that arrived later")

	block, err = repo.GetSessionRecoveryBlock(ctx, "block-delivery-"+suffix)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockResolved, block.State)
	require.Equal(t, event.StreamID, block.DeliveryStreamID)
	require.Equal(t, event.Sequence, block.DeliverySequence)
	session, err := repo.GetTaskSession(ctx, event.SessionID)
	require.NoError(t, err)
	settledRecovery, ok := models.LoadAgentDeliveryRecovery(session.Metadata)
	require.True(t, ok)
	require.Equal(t, models.AgentDeliveryRecoverySettled, settledRecovery.Phase)
	require.Equal(t, event.StreamID, settledRecovery.StreamID)

	putRetentionWorkflowOutcome(t, repo, event, "not_required", "turn-"+suffix)
	completed, err := repo.CompleteAgentDeliveryTerminalSettlement(ctx,
		terminalRetentionEffectKey(event), time.Now().UTC())
	require.NoError(t, err)
	require.True(t, completed)
	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: event.SessionID, IncarnationID: event.IncarnationID,
		Generation: event.HarnessGeneration, NativeSessionID: "native-clear-late-terminal",
	}))
	tx, err := repo.ro.BeginTxx(ctx, nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback() })
	historical, valid, err := repo.loadHistoricalTerminalSettlementTx(ctx, tx, settledRecovery,
		"native-clear-late-terminal", block.ID, block.Reason)
	require.NoError(t, err)
	require.True(t, valid)
	require.Equal(t, event.StreamID, historical.streamID)
}
