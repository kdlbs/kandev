package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12
func TestExplicitResumeResolvesIdentitylessIncidentBlock(t *testing.T) {
	ctx := context.Background()
	service, manager, block, submission, launched, started := nativeResumeFixture(t, durableDeliveryUnresolvedReason, false, false)
	repo := service.repo.(*sqliterepo.Repository)

	identityless := *block
	identityless.DeliverySubmissionID = ""
	identityless.DeliveryStreamID = ""
	require.Zero(t, identityless.DeliverySequence)
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &identityless))

	response, err := service.RecoverSession(ctx, "task1", submission.SessionID, "resume")
	require.NoError(t, err)
	require.True(t, response.Success)
	require.True(t, *started)
	require.NotNil(t, *launched)
	require.Equal(t, "native-conversation", (*launched).ACPSessionID)
	require.Empty(t, manager.capturedPromptCalls, "Resume must not redispatch the interrupted prompt")

	storedSubmission, err := repo.GetAgentDeliverySubmission(ctx, submission.ID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, storedSubmission.State)
	require.Equal(t, submission.Payload, storedSubmission.Payload)
	storedBlock, err := repo.GetSessionRecoveryBlock(ctx, block.ID)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockResolved, storedBlock.State)
	require.Equal(t, "resume", storedBlock.AuthorizedAction)
	currentSession, err := service.repo.GetTaskSession(ctx, submission.SessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, currentSession.State)
}
