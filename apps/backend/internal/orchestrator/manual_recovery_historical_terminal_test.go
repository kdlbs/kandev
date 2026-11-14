package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestManualRecoveryAfterHistoricalTerminalRecovery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	service, repo, manager, session, current, proofManager := seedAutomaticHistoricalTerminalRecovery(t, ctx)
	service.messageCreator = newServiceBackedMessageCreator(repo)
	service.agentManager = proofManager

	response, err := service.RetrySessionDelivery(ctx, session.TaskID, session.ID)
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoveryBlocked, response.Outcome,
		"Retry connection must report the current incident fence while startup restoration is pending")
	require.Zero(t, proofManager.retirements.Load(), "Retry connection must not retire journal work")
	require.Empty(t, manager.capturedPromptCalls, "Retry connection must not relaunch the interrupted prompt")
	block, err := repo.GetSessionRecoveryBlock(ctx, current.BlockID)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockOpen, block.State)

	manager.startAgentProcessFunc = func(startCtx context.Context, _ string) error {
		return repo.UpdateTaskSessionState(startCtx, session.ID, models.TaskSessionStateWaitingForInput, "")
	}
	manager.isAgentRunning = true
	resumed, err := service.RecoverSession(ctx, session.TaskID, session.ID, "resume")
	require.NoError(t, err)
	require.NotNil(t, resumed)
	require.True(t, resumed.Success)
	require.Empty(t, manager.capturedPromptCalls, "Resume must restore the saved conversation without resending the prompt")
	stored, err := repo.GetAgentDeliverySubmission(ctx, current.ID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, stored.State)
	require.Equal(t, current.Payload, stored.Payload)
	block, err = repo.GetSessionRecoveryBlock(ctx, current.BlockID)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockResolved, block.State)

	messages, err := repo.ListMessages(ctx, session.ID)
	require.NoError(t, err)
	require.Len(t, messages, 2)
	require.Equal(t, sessionRecoveryNoticeContent, messages[1].Content)
	require.Equal(t, models.MessageTypeStatus, messages[1].Type)
	require.Equal(t, "session_restored_after_interruption", messages[1].Metadata["kind"])

	retry, err := service.RetrySessionDelivery(ctx, session.TaskID, session.ID)
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoveryAttached, retry.Outcome,
		"Retry connection must read the already committed restoration disposition")
	require.EqualValues(t, 0, proofManager.retirements.Load(), "state-only Retry must not perform native retirement")
}
