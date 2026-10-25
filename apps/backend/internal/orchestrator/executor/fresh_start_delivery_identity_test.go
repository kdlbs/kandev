package executor

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func freshStartSession() *models.TaskSession {
	return &models.TaskSession{ID: "session", QueueIncarnationID: "incarnation"}
}

func TestFreshStartResumeRequestReplacesNativeConversation(t *testing.T) {
	req, _ := newResumeLaunchRequest(&v1.Task{ID: "task"}, freshStartSession(), true, ResumeOptions{RecoveryAction: RecoveryActionFreshStart})
	require.True(t, req.ReplaceNativeConversation)

	req, _ = newResumeLaunchRequest(&v1.Task{ID: "task"}, freshStartSession(), true, ResumeOptions{RecoveryAction: "resume"})
	require.False(t, req.ReplaceNativeConversation)
}

func TestApplyDeliveryIdentityFreshStartAdvancesCommittedGeneration(t *testing.T) {
	repo := &interruptedGenerationRepository{
		mockRepository: newMockRepository(),
		generation:     &models.HarnessSessionGeneration{Generation: 1, NativeSessionID: "previous-native"},
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{ReplaceNativeConversation: true}

	require.NoError(t, exec.applyDeliveryIdentity(context.Background(), req, freshStartSession()))

	require.EqualValues(t, 2, req.DeliveryHarnessGeneration)
	require.Equal(t, "incarnation:g2", req.DeliveryStreamID)
	require.True(t, req.ReplaceNativeConversation)
}

func TestApplyDeliveryIdentityFreshStartWithoutCommittedGenerationStaysFirst(t *testing.T) {
	repo := &interruptedGenerationRepository{mockRepository: newMockRepository()}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{ReplaceNativeConversation: true}

	require.NoError(t, exec.applyDeliveryIdentity(context.Background(), req, freshStartSession()))

	require.EqualValues(t, 1, req.DeliveryHarnessGeneration)
	require.Equal(t, "incarnation:g1", req.DeliveryStreamID)
	require.False(t, req.ReplaceNativeConversation, "no committed predecessor exists to replace")
}

func TestApplyDeliveryIdentityProcessReplacementKeepsGeneration(t *testing.T) {
	repo := &interruptedGenerationRepository{
		mockRepository: newMockRepository(),
		generation:     &models.HarnessSessionGeneration{Generation: 1, NativeSessionID: "previous-native"},
	}
	exec := newTestExecutor(t, &mockAgentManager{}, repo)
	req := &LaunchAgentRequest{ACPSessionID: "previous-native"}

	require.NoError(t, exec.applyDeliveryIdentity(context.Background(), req, freshStartSession()))

	require.EqualValues(t, 1, req.DeliveryHarnessGeneration)
	require.Equal(t, "incarnation:g1", req.DeliveryStreamID)
}
