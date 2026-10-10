package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/stretchr/testify/require"
)

type recordingPromptStreamRecoveryManager struct {
	*mockAgentManager
	calls []string
	err   error
}

func (m *recordingPromptStreamRecoveryManager) RecoverAgentPromptStream(_ context.Context, sessionID string) error {
	m.calls = append(m.calls, sessionID)
	return m.err
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
	require.Equal(t, &LaunchSessionResponse{
		Success:   true,
		TaskID:    "task-delivery-retry",
		SessionID: "session-delivery-retry",
	}, response)
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

	require.Error(t, err)
	require.Nil(t, response)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}
