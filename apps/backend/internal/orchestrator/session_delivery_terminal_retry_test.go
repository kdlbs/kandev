package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/stretchr/testify/require"
)

type retryTerminalFixture struct {
	service  *Service
	repo     *tasksqlite.Repository
	session  *models.TaskSession
	recovery models.AgentDeliveryRecovery
	settle   func()
}

func newRetryTerminalFixture(t *testing.T) *retryTerminalFixture {
	t.Helper()
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

	settle := func() {
		event := persistedTerminalEvent(session.ID, session.QueueIncarnationID, 1, recovery.SubmissionID, recovery.StreamID, "complete")
		projectOrchestratorDeliveryEvent(t, repo, ctx, event)
		settled, err := repo.SettleAgentDeliveryTerminal(ctx, event.StreamID, event.Sequence, models.DeliverySubmissionCompleted, time.Now().UTC())
		require.NoError(t, err)
		require.True(t, settled)
	}
	return &retryTerminalFixture{service: service, repo: repo, session: session, recovery: recovery, settle: settle}
}

func TestRetrySessionDeliverySettlesProjectedTerminalWithoutOpenBlock(t *testing.T) {
	ctx := context.Background()
	f := newRetryTerminalFixture(t)
	f.settle()
	result, err := f.service.RetrySessionDelivery(ctx, f.session.TaskID, f.session.ID)
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoverySettled, result.Outcome, result.Reason)
	result, err = f.service.RetrySessionDelivery(ctx, f.session.TaskID, f.session.ID)
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoverySettled, result.Outcome)
}

func TestRetrySessionDeliveryResolvesStaleInactiveSubmissionBlock(t *testing.T) {
	// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.16
	for _, state := range []models.DeliverySubmissionState{
		models.DeliverySubmissionCompleted,
		models.DeliverySubmissionInterruptedUnknown,
	} {
		t.Run(string(state), func(t *testing.T) {
			ctx := context.Background()
			f := newRetryTerminalFixture(t)
			block, err := f.repo.GetOpenSessionRecoveryBlock(ctx, f.session.ID,
				f.session.QueueIncarnationID, f.recovery.HarnessGeneration)
			require.NoError(t, err)
			_, err = f.repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
				ID: "replacement-submission", SessionID: f.session.ID,
				IncarnationID: f.session.QueueIncarnationID, HarnessGeneration: f.recovery.HarnessGeneration,
				OwnerGeneration: 2, PayloadHash: "replacement-hash", Payload: []byte("later prompt"), State: state,
			})
			require.NoError(t, err)
			_, err = f.repo.DB().ExecContext(ctx, `UPDATE session_recovery_blocks
				SET delivery_submission_id = ?, delivery_stream_id = '' WHERE id = ?`,
				"replacement-submission", block.ID)
			require.NoError(t, err)
			f.settle()

			result, err := f.service.RetrySessionDelivery(ctx, f.session.TaskID, f.session.ID)
			require.NoError(t, err)
			require.Equal(t, SessionDeliveryRecoverySettled, result.Outcome, result.Reason)
			resolved, err := f.repo.GetSessionRecoveryBlock(ctx, block.ID)
			require.NoError(t, err)
			require.Equal(t, models.RecoveryBlockResolved, resolved.State)
			require.Equal(t, "retry_connection", resolved.AuthorizedAction)
			require.Equal(t, "replacement-submission", resolved.DeliverySubmissionID)
		})
	}
}

func TestRetrySessionDeliveryKeepsStaleInFlightSubmissionBlock(t *testing.T) {
	// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.16
	ctx := context.Background()
	f := newRetryTerminalFixture(t)
	block, err := f.repo.GetOpenSessionRecoveryBlock(ctx, f.session.ID,
		f.session.QueueIncarnationID, f.recovery.HarnessGeneration)
	require.NoError(t, err)
	_, err = f.repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: "in-flight-submission", SessionID: f.session.ID,
		IncarnationID: f.session.QueueIncarnationID, HarnessGeneration: f.recovery.HarnessGeneration,
		OwnerGeneration: 2, PayloadHash: "in-flight-hash", Payload: []byte("later prompt"),
		State: models.DeliverySubmissionDispatching,
	})
	require.NoError(t, err)
	_, err = f.repo.DB().ExecContext(ctx, `UPDATE session_recovery_blocks
		SET delivery_submission_id = ?, delivery_stream_id = '' WHERE id = ?`, "in-flight-submission", block.ID)
	require.NoError(t, err)
	f.settle()

	result, err := f.service.RetrySessionDelivery(ctx, f.session.TaskID, f.session.ID)
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoveryBlocked, result.Outcome)
	require.Equal(t, "independent_recovery_block", result.Reason)
	stillOpen, err := f.repo.GetSessionRecoveryBlock(ctx, block.ID)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockOpen, stillOpen.State)
	require.Equal(t, "in-flight-submission", stillOpen.DeliverySubmissionID)
}

func TestRetrySessionDeliveryKeepsOtherOpenBlockAfterResolvingStaleSubmission(t *testing.T) {
	// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.16
	ctx := context.Background()
	f := newRetryTerminalFixture(t)
	stale, err := f.repo.GetOpenSessionRecoveryBlock(ctx, f.session.ID,
		f.session.QueueIncarnationID, f.recovery.HarnessGeneration)
	require.NoError(t, err)
	_, err = f.repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{
		ID: "inactive-submission", SessionID: f.session.ID,
		IncarnationID: f.session.QueueIncarnationID, HarnessGeneration: f.recovery.HarnessGeneration,
		OwnerGeneration: 2, PayloadHash: "inactive-hash", Payload: []byte("later prompt"),
		State: models.DeliverySubmissionCompleted,
	})
	require.NoError(t, err)
	_, err = f.repo.DB().ExecContext(ctx, `UPDATE session_recovery_blocks
		SET delivery_submission_id = ?, delivery_stream_id = '' WHERE id = ?`, "inactive-submission", stale.ID)
	require.NoError(t, err)
	other := &models.SessionRecoveryBlock{
		ID: "independent-block", SessionID: f.session.ID, IncarnationID: f.session.QueueIncarnationID,
		ExpectedGeneration: f.recovery.HarnessGeneration, Reason: "manual_recovery_required",
		State: models.RecoveryBlockOpen, ConsumerReference: "workspace", CreatedAt: time.Now().UTC().Add(time.Hour),
	}
	require.NoError(t, f.repo.UpsertSessionRecoveryBlock(ctx, other))
	f.settle()

	result, err := f.service.RetrySessionDelivery(ctx, f.session.TaskID, f.session.ID)
	require.NoError(t, err)
	require.Equal(t, SessionDeliveryRecoveryBlocked, result.Outcome)
	require.Equal(t, "independent_recovery_block", result.Reason)
	resolved, err := f.repo.GetSessionRecoveryBlock(ctx, stale.ID)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockResolved, resolved.State)
	stillOpen, err := f.repo.GetSessionRecoveryBlock(ctx, other.ID)
	require.NoError(t, err)
	require.Equal(t, models.RecoveryBlockOpen, stillOpen.State)
}
