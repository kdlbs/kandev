package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestRetrySessionDeliverySettlesProjectedTerminalWithoutOpenBlock(t *testing.T) {
	ctx := context.Background()
	service, repo := newServiceWithRealRepo(t)
	seedSession(t, repo, "terminal-task", "terminal-session", "step-1")
	seedExecutorRunning(t, repo, "terminal-session", "terminal-task", "old-execution")
	session, err := repo.GetTaskSession(ctx, "terminal-session")
	require.NoError(t, err)
	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{SessionID: session.ID, IncarnationID: session.QueueIncarnationID, Generation: 1, NativeSessionID: "native", CreationReason: "initial"}))
	_, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: "terminal-submission", SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "hash", Payload: []byte("original instruction"), State: models.DeliverySubmissionDispatching,
	})
	require.NoError(t, err)
	recovery := models.AgentDeliveryRecovery{
		SessionID: session.ID, AgentExecutionID: "old-execution", IncarnationID: session.QueueIncarnationID,
		HarnessGeneration: 1, SubmissionID: "terminal-submission", StreamID: "terminal-stream", PromptGeneration: 1, Phase: models.AgentDeliveryRecoveryUncertain,
	}
	changed, err := repo.UpsertAgentDeliveryRecovery(ctx, &recovery, &models.SessionRecoveryBlock{
		SessionID: session.ID, IncarnationID: session.QueueIncarnationID, ExpectedGeneration: 1,
		Reason: "unknown_prompt_outcome", ConsumerReference: "agent_delivery", DeliverySubmissionID: recovery.SubmissionID, DeliveryStreamID: recovery.StreamID, State: models.RecoveryBlockOpen,
	})
	require.NoError(t, err)
	require.True(t, changed)
	event := persistedTerminalEvent(session.ID, session.QueueIncarnationID, 1, recovery.SubmissionID, recovery.StreamID, "complete")
	projectOrchestratorDeliveryEvent(t, repo, ctx, event)
	settled, err := repo.SettleAgentDeliveryTerminal(ctx, event.StreamID, event.Sequence, models.DeliverySubmissionCompleted, time.Now().UTC())
	require.NoError(t, err)
	require.True(t, settled)
	result, err := service.RetrySessionDelivery(ctx, session.TaskID, session.ID)
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoverySettled, result.Outcome, result.Reason)
	result, err = service.RetrySessionDelivery(ctx, session.TaskID, session.ID)
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoverySettled, result.Outcome)
}
