package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

func markUnprovenCompletion(t *testing.T, repo *sqliterepo.Repository, submissionID string) *models.AgentDeliverySubmission {
	t.Helper()
	_, err := repo.DB().ExecContext(t.Context(), `UPDATE agent_delivery_submissions
		SET state = 'completed', outcome = 'completed', terminal_stream_id = '',
		    terminal_sequence = 0, terminal_turn_id = '', fully_settled_at = NULL, payload_pruned_at = NULL
		WHERE id = ?`, submissionID)
	require.NoError(t, err)
	row, err := repo.GetAgentDeliverySubmission(t.Context(), submissionID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionCompleted, row.State)
	return row
}

func seedUnprovenCursor(
	t *testing.T,
	repo *sqliterepo.Repository,
	submission *models.AgentDeliverySubmission,
	streamID string,
) *models.AgentDeliveryCursor {
	t.Helper()
	ctx := t.Context()
	event := &models.AgentDeliveryEvent{
		SessionID: submission.SessionID, IncarnationID: submission.IncarnationID,
		HarnessGeneration: submission.HarnessGeneration, StreamID: streamID,
		Sequence: 1, EventType: "usage_update", Payload: []byte(`{}`),
	}
	_, err := repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
	require.NoError(t, err)
	_, err = repo.ProjectAgentDeliveryEvent(ctx, event, nil)
	require.NoError(t, err)
	cursor, err := repo.GetAgentDeliveryCursor(ctx, streamID)
	require.NoError(t, err)
	return cursor
}

func unprovenProof(
	submission *models.AgentDeliverySubmission,
	cursor *models.AgentDeliveryCursor,
) *models.NativeResumeDeliveryEvidence {
	return &models.NativeResumeDeliveryEvidence{
		SessionID: submission.SessionID, IncarnationID: submission.IncarnationID,
		HarnessGeneration: submission.HarnessGeneration, NativeSessionID: "native-conversation",
		ExecutionID: "old-execution", StreamID: cursor.StreamID, Cursor: cursor,
		Submissions: []models.NativeResumeSubmissionEvidence{{
			ID: submission.ID, HarnessGeneration: submission.HarnessGeneration,
			PayloadHash: submission.PayloadHash, StreamID: cursor.StreamID,
			State: models.DeliverySubmissionCompleted, Outcome: string(models.DeliverySubmissionCompleted),
			UnprovenCompletion: true, Cursor: cursor,
		}},
	}
}

func requireUnprovenRecoveryResolved(
	t *testing.T,
	service *Service,
	repo *sqliterepo.Repository,
	block *models.SessionRecoveryBlock,
	submission *models.AgentDeliverySubmission,
) {
	t.Helper()
	ctx := context.Background()
	resolved, err := repo.GetSessionRecoveryBlock(ctx, block.ID)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockResolved, resolved.State)
	require.NoError(t, service.checkSessionRecoveryBlock(ctx, submission.SessionID))
	retained, err := repo.GetAgentDeliverySubmission(ctx, submission.ID)
	require.NoError(t, err)
	require.Equal(t, submission, retained, "recovery must preserve the historical completion")
}

func TestUnprovenCompletionRecoveryEntrypoints(t *testing.T) {
	t.Run("explicit resume and later admission", func(t *testing.T) {
		ctx := context.Background()
		service, manager, block, submission, launched, _ := nativeResumeFixture(t, durableDeliveryUnresolvedReason, false, false)
		repo := service.repo.(*sqliterepo.Repository)
		submission = markUnprovenCompletion(t, repo, submission.ID)
		cursor := seedUnprovenCursor(t, repo, submission, block.DeliveryStreamID)
		service.agentManager = &nativeProofTestManager{
			nativeResumeTestManager: manager,
			proof: func(context.Context, string) (*models.NativeResumeDeliveryEvidence, error) {
				return unprovenProof(submission, cursor), nil
			},
		}
		require.NoError(t, service.checkNativeResumeDeliveryBlock(ctx, "task1", submission.SessionID))
		response, err := service.RecoverSession(ctx, "task1", submission.SessionID, "resume")
		require.NoError(t, err)
		require.True(t, response.Success)
		require.NotNil(t, launched)
		requireUnprovenRecoveryResolved(t, service, repo, block, submission)
		require.Empty(t, manager.capturedPromptCalls, "recovery must not replay the old instruction")
	})
	t.Run("loaded admission", func(t *testing.T) {
		ctx := context.Background()
		service, manager, block, submission, _, _ := nativeResumeFixture(t, durableDeliveryUnresolvedReason, false, false)
		repo := service.repo.(*sqliterepo.Repository)
		submission = markUnprovenCompletion(t, repo, submission.ID)
		cursor := seedUnprovenCursor(t, repo, submission, block.DeliveryStreamID)
		proof := unprovenProof(submission, cursor)
		service.agentManager = &loadedNativeProofTestManager{&nativeProofTestManager{
			nativeResumeTestManager: manager,
			proof: func(context.Context, string) (*models.NativeResumeDeliveryEvidence, error) {
				return proof, nil
			},
		}}
		require.NoError(t, service.checkSessionRecoveryBlock(ctx, submission.SessionID))
		requireUnprovenRecoveryResolved(t, service, repo, block, submission)
		require.Empty(t, manager.capturedPromptCalls, "automatic recovery must not replay the old instruction")
	})
	t.Run("completed label without proof keeps the fence", func(t *testing.T) {
		ctx := context.Background()
		service, manager, block, submission, _, _ := nativeResumeFixture(t, durableDeliveryUnresolvedReason, false, false)
		repo := service.repo.(*sqliterepo.Repository)
		submission = markUnprovenCompletion(t, repo, submission.ID)
		// A terminal binding is not part of the unproven class, so the
		// admission gate must not treat it as recoverable uncertainty.
		_, err := repo.DB().ExecContext(ctx, `UPDATE agent_delivery_submissions SET terminal_sequence = 1 WHERE id = ?`, submission.ID)
		require.NoError(t, err)
		require.Error(t, service.checkNativeResumeDeliveryBlock(ctx, "task1", submission.SessionID))
		resolved, err := repo.GetSessionRecoveryBlock(ctx, block.ID)
		require.NoError(t, err)
		require.Equal(t, models.RecoveryBlockOpen, resolved.State)
		require.Empty(t, manager.capturedPromptCalls)
	})
}
