package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type nativeProofTestManager struct {
	*nativeResumeTestManager
	proof func(context.Context, string) (*models.NativeResumeDeliveryEvidence, error)
}

func (m *nativeProofTestManager) GetNativeResumeDeliveryEvidence(ctx context.Context, sessionID string) (*models.NativeResumeDeliveryEvidence, error) {
	return m.proof(ctx, sessionID)
}

func TestNativeResumeRepairsAbsentNoticeAndStreamThroughAuthenticatedProof(t *testing.T) {
	ctx := context.Background()
	service, manager, block, submission, _, _ := nativeResumeFixture(t, "unknown_prompt_outcome", false, false)
	repo := service.repo.(*sqliterepo.Repository)
	require.NoError(t, repo.SetSessionMetadataKey(ctx, submission.SessionID, models.SessionMetaKeyAgentDeliveryRecovery, nil))
	_, err := repo.DB().ExecContext(ctx, `UPDATE session_recovery_blocks SET delivery_stream_id = '' WHERE id = ?`, block.ID)
	require.NoError(t, err)
	service.agentManager = &nativeProofTestManager{nativeResumeTestManager: manager, proof: func(ctx context.Context, sessionID string) (*models.NativeResumeDeliveryEvidence, error) {
		running, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
		require.NoError(t, err)
		return &models.NativeResumeDeliveryEvidence{SessionID: sessionID, IncarnationID: submission.IncarnationID,
			HarnessGeneration: submission.HarnessGeneration, NativeSessionID: "native-conversation", ExecutionID: running.AgentExecutionID,
			StreamID: block.DeliveryStreamID, Submissions: []models.NativeResumeSubmissionEvidence{{ID: submission.ID,
				HarnessGeneration: submission.HarnessGeneration, PayloadHash: submission.PayloadHash, StreamID: block.DeliveryStreamID}}}, nil
	}}
	response, err := service.RecoverSession(ctx, "task1", submission.SessionID, "resume")
	require.NoError(t, err)
	require.True(t, response.Success)
	restored, err := repo.GetSessionRecoveryBlock(ctx, block.ID)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockResolved, restored.State)
	require.Equal(t, block.DeliveryStreamID, restored.DeliveryStreamID)
	require.NoError(t, service.checkSessionRecoveryBlock(ctx, submission.SessionID))
	require.Empty(t, manager.capturedPromptCalls)
	retained, err := repo.GetAgentDeliverySubmission(ctx, submission.ID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, retained.State)
	require.Equal(t, submission.Payload, retained.Payload)
	require.Zero(t, retained.TerminalSequence)
}

type loadedNativeProofTestManager struct{ *nativeProofTestManager }

func (m *loadedNativeProofTestManager) ReconcileLoadedNativeDelivery(ctx context.Context, sessionID string) (*models.NativeResumeDeliveryEvidence, error) {
	return m.proof(ctx, sessionID)
}

func TestLoadedNativeRecoveryClearsAdmissionWithoutReplay(t *testing.T) {
	for _, entry := range []string{"admission", "restart"} {
		t.Run(entry, func(t *testing.T) {
			ctx := context.Background()
			service, manager, block, submission, _, _ := nativeResumeFixture(t, "unknown_prompt_outcome", false, false)
			repo := service.repo.(*sqliterepo.Repository)
			require.NoError(t, repo.SetSessionMetadataKey(ctx, submission.SessionID, models.SessionMetaKeyAgentDeliveryRecovery, nil))
			_, err := repo.DB().ExecContext(ctx, `UPDATE session_recovery_blocks SET delivery_stream_id = '' WHERE id = ?`, block.ID)
			require.NoError(t, err)
			running, err := repo.GetExecutorRunningBySessionID(ctx, submission.SessionID)
			require.NoError(t, err)
			service.agentManager = &loadedNativeProofTestManager{&nativeProofTestManager{nativeResumeTestManager: manager, proof: func(context.Context, string) (*models.NativeResumeDeliveryEvidence, error) {
				return &models.NativeResumeDeliveryEvidence{SessionID: submission.SessionID, IncarnationID: submission.IncarnationID, HarnessGeneration: submission.HarnessGeneration, NativeSessionID: "native-conversation", ExecutionID: running.AgentExecutionID, StreamID: block.DeliveryStreamID, Submissions: []models.NativeResumeSubmissionEvidence{{ID: submission.ID, HarnessGeneration: submission.HarnessGeneration, PayloadHash: submission.PayloadHash, StreamID: block.DeliveryStreamID}}}, nil
			}}}
			if entry == "restart" {
				session, err := repo.GetTaskSession(ctx, submission.SessionID)
				require.NoError(t, err)
				_, err = service.repairRestartRecoveryPresentation(ctx, session)
				require.NoError(t, err)
			} else {
				require.NoError(t, service.checkSessionRecoveryBlock(ctx, submission.SessionID))
			}
			restored, err := repo.GetSessionRecoveryBlock(ctx, block.ID)
			require.NoError(t, err)
			require.Equal(t, models.RecoveryBlockResolved, restored.State)
			require.NoError(t, service.checkSessionRecoveryBlock(ctx, submission.SessionID))
			require.Empty(t, manager.capturedPromptCalls)
		})
	}
}

func TestNativeResumeAllowsTerminalGenericFenceThroughProof(t *testing.T) {
	ctx := context.Background()
	service, manager, block, submission, _, _ := nativeResumeFixture(t, durableDeliveryUnresolvedReason, false, false)
	repo := service.repo.(*sqliterepo.Repository)
	require.NoError(t, repo.SetSessionMetadataKey(ctx, submission.SessionID, models.SessionMetaKeyAgentDeliveryRecovery, nil))
	_, err := repo.DB().ExecContext(ctx, `UPDATE session_recovery_blocks SET delivery_submission_id = '', delivery_stream_id = '' WHERE id = ?`, block.ID)
	require.NoError(t, err)
	_, err = repo.DB().ExecContext(ctx, `UPDATE agent_delivery_submissions SET state = 'failed', outcome = 'failed' WHERE id = ?`, submission.ID)
	require.NoError(t, err)
	service.agentManager = &nativeProofTestManager{nativeResumeTestManager: manager, proof: func(ctx context.Context, sessionID string) (*models.NativeResumeDeliveryEvidence, error) {
		running, err := repo.GetExecutorRunningBySessionID(ctx, sessionID)
		require.NoError(t, err)
		return &models.NativeResumeDeliveryEvidence{SessionID: sessionID, IncarnationID: submission.IncarnationID, HarnessGeneration: submission.HarnessGeneration, NativeSessionID: "native-conversation", ExecutionID: running.AgentExecutionID, StreamID: block.DeliveryStreamID}, nil
	}}
	response, err := service.RecoverSession(ctx, "task1", submission.SessionID, "resume")
	require.NoError(t, err)
	require.True(t, response.Success)
	restored, err := repo.GetSessionRecoveryBlock(ctx, block.ID)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockResolved, restored.State)
	require.NoError(t, service.checkSessionRecoveryBlock(ctx, submission.SessionID))
	require.Empty(t, manager.capturedPromptCalls)
	retained, err := repo.GetAgentDeliverySubmission(ctx, submission.ID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionFailed, retained.State)
	require.Equal(t, "failed", retained.Outcome)
}
