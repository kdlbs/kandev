package orchestrator

import (
	"context"
	"errors"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type readinessProofManager struct {
	*loadedNativeProofTestManager
	unresolved     bool
	keepUnresolved bool
}

func (m *readinessProofManager) DurableDeliveryCapabilityForExecution(context.Context, string) (agentruntime.DurableDeliveryCapability, bool) {
	return agentruntime.DurableDeliveryCapability{Version: journal.CurrentVersion, Durable: true, Unresolved: m.unresolved}, true
}

func nativeReadinessFixture(t *testing.T, refuse bool) (*Service, *readinessProofManager, *models.AgentDeliverySubmission) {
	t.Helper()
	ctx := context.Background()
	svc, manager, block, submission, _, _ := nativeResumeFixture(t, durableDeliveryUnresolvedReason, false, false)
	repo := svc.repo.(*sqliterepo.Repository)
	_, err := repo.DB().ExecContext(ctx, `DELETE FROM session_recovery_blocks WHERE id = ?`, block.ID)
	require.NoError(t, err)
	require.NoError(t, repo.SetSessionMetadataKey(ctx, submission.SessionID, models.SessionMetaKeyAgentDeliveryRecovery, nil))
	setNativeResumeSessionState(t, svc, submission.SessionID, models.TaskSessionStateStarting)
	proofManager := &readinessProofManager{unresolved: true}
	proofManager.loadedNativeProofTestManager = &loadedNativeProofTestManager{&nativeProofTestManager{
		nativeResumeTestManager: manager,
		proof: func(context.Context, string) (*models.NativeResumeDeliveryEvidence, error) {
			if refuse {
				return nil, errors.New("original owner death is unproven")
			}
			proofManager.unresolved = proofManager.keepUnresolved
			return &models.NativeResumeDeliveryEvidence{SessionID: submission.SessionID, IncarnationID: submission.IncarnationID,
				HarnessGeneration: submission.HarnessGeneration, NativeSessionID: "native-conversation", ExecutionID: "old-execution",
				StreamID: block.DeliveryStreamID, Submissions: []models.NativeResumeSubmissionEvidence{{ID: submission.ID,
					HarnessGeneration: submission.HarnessGeneration, PayloadHash: submission.PayloadHash, StreamID: block.DeliveryStreamID}}}, nil
		},
	}}
	svc.agentManager = proofManager
	return svc, proofManager, submission
}

func TestNativeReadinessReconcilesFirstDeliveryWithoutExistingFence(t *testing.T) {
	for _, protocol := range []string{messagequeue.DeliveryProtocolPending, messagequeue.DeliveryProtocolV1} {
		t.Run(protocol, func(t *testing.T) {
			svc, manager, submission := nativeReadinessFixture(t, false)
			got, err := svc.resolveDeliveryProtocol(context.Background(), submission.SessionID, "old-execution", protocol)
			require.NoError(t, err)
			require.Equal(t, messagequeue.DeliveryProtocolV1, got)
			block, err := svc.GetOpenSessionRecoveryBlock(context.Background(), submission.SessionID)
			require.NoError(t, err)
			require.Nil(t, block)
			require.Empty(t, manager.capturedPromptCalls)
		})
	}
}

func TestNativeReadinessBootSettlesOrExposesRecoveryBeforeFirstSend(t *testing.T) {
	for _, refuse := range []bool{false, true} {
		t.Run(map[bool]string{false: "reconciled", true: "explicit resume required"}[refuse], func(t *testing.T) {
			svc, manager, submission := nativeReadinessFixture(t, refuse)
			ctx := context.Background()
			svc.handleAgentBootReady(ctx, watcher.AgentEventData{TaskID: "task1", SessionID: submission.SessionID, AgentExecutionID: "old-execution"})
			block, err := svc.GetOpenSessionRecoveryBlock(ctx, submission.SessionID)
			require.NoError(t, err)
			if refuse {
				require.NotNil(t, block, "boot must surface the recovery requirement before any user send")
				require.Equal(t, durableDeliveryUnresolvedReason, block.Reason)
			} else {
				require.False(t, manager.unresolved, "boot must reconcile before publishing waiting for input")
				require.Nil(t, block)
				_, err = svc.resolveDeliveryProtocol(ctx, submission.SessionID, "old-execution", messagequeue.DeliveryProtocolPending)
				require.NoError(t, err)
			}
			stored, err := svc.repo.(*sqliterepo.Repository).GetAgentDeliverySubmission(ctx, submission.ID)
			require.NoError(t, err)
			require.Equal(t, submission.State, stored.State)
			require.Equal(t, submission.Payload, stored.Payload)
			require.Empty(t, manager.capturedPromptCalls)
		})
	}
}

func TestNativeReadinessKeepsFenceWhenProofOrFreshCapabilityRefuses(t *testing.T) {
	for _, refuseProof := range []bool{true, false} {
		t.Run(map[bool]string{true: "missing owner proof", false: "inventory changed"}[refuseProof], func(t *testing.T) {
			svc, manager, submission := nativeReadinessFixture(t, refuseProof)
			manager.keepUnresolved = true
			ctx := context.Background()
			_, err := svc.resolveDeliveryProtocol(ctx, submission.SessionID, "old-execution", messagequeue.DeliveryProtocolPending)
			require.ErrorIs(t, err, ErrSessionRecoveryRequired)
			block, err := svc.GetOpenSessionRecoveryBlock(ctx, submission.SessionID)
			require.NoError(t, err)
			require.NotNil(t, block)
			require.Empty(t, manager.capturedPromptCalls)
		})
	}
}
