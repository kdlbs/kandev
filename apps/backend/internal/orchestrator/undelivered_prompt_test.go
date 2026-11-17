package orchestrator

import (
	"context"
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-UNDELIVERED-PROMPT-001.3
func TestBlockedPromptAdmission(t *testing.T) {
	ctx := t.Context()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-blocked", "session-blocked", "step-1")
	session, err := repo.GetTaskSession(ctx, "session-blocked")
	require.NoError(t, err)
	manager := &durableDeliveryTestAgentManager{mockAgentManager: &mockAgentManager{}, advertised: true,
		capability: agentruntime.DurableDeliveryCapability{Version: journal.CurrentVersion, Durable: true}}
	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
	first, err := service.prepareAgentDeliverySubmission(ctx, session, "execution", "first", nil, promptTaskOptions{deliverySubmissionID: "first"})
	require.NoError(t, err)
	require.NotNil(t, first)
	require.NoError(t, first.markInterrupted(ctx, "prompt_dispatch_failed"))
	incarnation, generation, err := service.recoverySessionIdentity(ctx, session.ID)
	require.NoError(t, err)
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{
		ID: "blocked", SessionID: session.ID, IncarnationID: incarnation, ExpectedGeneration: generation,
		Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen,
		ConsumerReference: "agent_delivery", DeliverySubmissionID: first.id,
	}))
	second, err := service.prepareAgentDeliverySubmission(ctx, session, "execution", "second", nil, promptTaskOptions{deliverySubmissionID: "second"})
	require.ErrorIs(t, err, ErrSessionRecoveryRequired)
	require.Nil(t, second)
	_, err = repo.GetAgentDeliverySubmission(ctx, "prompt:second")
	require.Error(t, err)
	block, err := repo.GetSessionRecoveryBlock(ctx, "blocked")
	require.NoError(t, err)
	require.Equal(t, first.id, block.DeliverySubmissionID)
}

// @covers AC-PLATFORM-UNDELIVERED-PROMPT-001.1
func TestUndeliveredPromptDispatch(t *testing.T) {
	rejection := (&agentctl.Client{}).PromptWithSubmissionID(t.Context(), "original", nil, 1, "rejected")
	require.Error(t, rejection)
	for _, tc := range []struct {
		name     string
		cause    error
		accepted bool
		want     models.DeliverySubmissionState
	}{
		{"not connected", rejection, false, models.DeliverySubmissionFailed},
		{"disconnected after send", agentctl.ErrAgentStreamNotConnected, false, models.DeliverySubmissionInterruptedUnknown},
		{"lost reply", context.DeadlineExceeded, false, models.DeliverySubmissionInterruptedUnknown},
		{"accepted", rejection, true, models.DeliverySubmissionInterruptedUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, repo := newHandoffService(t)
			seedSession(t, repo, "task-rejected", "session-rejected", "", "session-rejected")
			delivery, _ := admitHandoffSubmission(t, service, repo, "session-rejected", "rejected", "original")
			outcome := &promptDispatchOutcome{}
			if tc.accepted {
				outcome.recordAccepted(nil)
			}
			_, err := service.finishPromptExecutorDispatch(t.Context(), "task-rejected", "session-rejected", "original", false, false, nil,
				promptClaimRollback{}, promptTaskOptions{}, nil, nil, nil, tc.cause, outcome, nil, delivery)
			require.Error(t, err)
			stored, err := repo.GetAgentDeliverySubmission(t.Context(), delivery.id)
			require.NoError(t, err)
			require.Equal(t, tc.want, stored.State)
			block, err := service.GetOpenSessionRecoveryBlock(t.Context(), "session-rejected")
			require.NoError(t, err)
			if tc.want == models.DeliverySubmissionFailed {
				require.Nil(t, block)
				require.NotNil(t, stored.FullySettledAt)
			} else {
				require.NotNil(t, block)
			}
		})
	}
}

func TestUndeliveredPromptDispatchPreservesChangedOwner(t *testing.T) {
	service, repo := newHandoffService(t)
	seedSession(t, repo, "task-owner", "session-owner", "", "session-owner")
	delivery, _ := admitHandoffSubmission(t, service, repo, "session-owner", "owned", "original")
	_, err := repo.DB().Exec(`UPDATE agent_delivery_submissions SET harness_generation = 2, owner_generation = 2 WHERE id = ?`, delivery.id)
	require.NoError(t, err)
	require.Error(t, delivery.failUnsent(t.Context()))
	row, err := repo.GetAgentDeliverySubmission(t.Context(), delivery.id)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionDispatching, row.State)
	require.Nil(t, row.FullySettledAt)
}
