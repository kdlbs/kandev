package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type recordingPromptStreamRecoveryManager struct {
	*mockAgentManager
	calls []string
	err   error
}

type identityAwarePromptStreamRecoveryManager struct {
	*recordingPromptStreamRecoveryManager
	identityCalls int
}

func (m *recordingPromptStreamRecoveryManager) RecoverAgentPromptStream(_ context.Context, sessionID string) error {
	m.calls = append(m.calls, sessionID)
	return m.err
}

func (m *identityAwarePromptStreamRecoveryManager) RecoverAgentPromptStreamWithIdentity(
	_ context.Context,
	identity lifecycle.AgentDeliveryRecoveryIdentity,
) lifecycle.DeliveryReconciliationResult {
	m.identityCalls++
	return lifecycle.DeliveryReconciliationResult{
		Outcome: lifecycle.DeliveryReconciliationBlocked,
		Reason:  "recovery_identity_incomplete",
	}
}

var _ executor.AgentManagerClient = (*recordingPromptStreamRecoveryManager)(nil)

func TestRetrySessionDeliveryReconnectsWithoutPromptDispatch(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-delivery-retry", "session-delivery-retry", "step-1")
	manager := &recordingPromptStreamRecoveryManager{mockAgentManager: &mockAgentManager{}}
	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)

	response, err := service.RetrySessionDelivery(
		context.Background(),
		"task-delivery-retry",
		"session-delivery-retry",
	)

	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoveryAttached, response.Outcome)
	require.Empty(t, response.Reason)
	require.Equal(t, []string{"session-delivery-retry"}, manager.calls)
	require.Empty(t, manager.capturedPromptCalls)
}

func TestRetrySessionDeliveryPropagatesReconnectFailure(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-delivery-retry-error", "session-delivery-retry-error", "step-1")
	manager := &recordingPromptStreamRecoveryManager{
		mockAgentManager: &mockAgentManager{},
		err:              context.DeadlineExceeded,
	}
	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)

	response, err := service.RetrySessionDelivery(
		context.Background(),
		"task-delivery-retry-error",
		"session-delivery-retry-error",
	)

	require.NoError(t, err)
	require.NotNil(t, response)
	require.Equal(t, SessionDeliveryRecoveryUnavailable, response.Outcome)
	require.Equal(t, "recovery_unavailable", response.Reason)
}

func TestRetryLegacySessionDeliveryUsesLiveRecoveryInterface(t *testing.T) {
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-delivery-legacy", "session-delivery-legacy", "step-1")
	legacy := &recordingPromptStreamRecoveryManager{mockAgentManager: &mockAgentManager{}}
	manager := &identityAwarePromptStreamRecoveryManager{recordingPromptStreamRecoveryManager: legacy}
	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)

	response, err := service.RetrySessionDelivery(
		context.Background(),
		"task-delivery-legacy",
		"session-delivery-legacy",
	)

	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoveryAttached, response.Outcome)
	require.Empty(t, response.Reason)
	require.Equal(t, []string{"session-delivery-legacy"}, manager.calls)
	require.Zero(t, manager.identityCalls,
		"a legacy retry has no durable identity and must use the legacy live execution path")
	require.Empty(t, manager.capturedPromptCalls)
}

func TestRetrySessionDeliveryMissingCanonicalSubmissionStaysVisible(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "task-delivery-missing", "session-delivery-missing", "step-1")
	session, err := repo.GetTaskSession(ctx, "session-delivery-missing")
	require.NoError(t, err)
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{
		ID: "block-delivery-missing", SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		ExpectedGeneration: 1, Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen,
		ConsumerReference: "agent_delivery", DeliverySubmissionID: "submission-missing",
		DeliveryStreamID: "stream-missing",
	}))
	manager := &recordingPromptStreamRecoveryManager{mockAgentManager: &mockAgentManager{}}
	service := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)

	response, err := service.RetrySessionDelivery(ctx, session.TaskID, session.ID)
	require.NoError(t, err)
	require.Equal(t, "blocked", string(response.Outcome))
	require.Equal(t, "missing_canonical_submission", response.Reason)
	require.EqualValues(t, 0, response.RecoveryRevision)
	require.Empty(t, manager.calls, "a missing submission must not be retried through the live-only path")
	block, err := repo.GetOpenSessionRecoveryBlock(ctx, session.ID, session.QueueIncarnationID, 1)
	require.NoError(t, err)
	require.NotNil(t, block, "retry must retain the original admission block")
	require.Equal(t, "submission-missing", block.DeliverySubmissionID)
}
