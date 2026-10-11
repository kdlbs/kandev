package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.1
func TestRestartRecoveryRepairsMissingCanonicalProjection(t *testing.T) {
	testMissingProjectionReconstruction(t, "unknown")
}

func TestRestartRecoveryPreservesCanonicalLiveDispatch(t *testing.T) {
	for _, condition := range []string{"live", "no death proof", "dead", "lost CAS", "retired verified", "retired changed", "retired live"} {
		t.Run(condition, func(t *testing.T) { testMissingProjectionReconstruction(t, condition) })
	}
}

func testMissingProjectionReconstruction(t *testing.T, condition string) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", Title: "Recovery"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task",
		QueueIncarnationID: "incarnation", State: models.TaskSessionStateWaitingForInput}))
	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: "session", IncarnationID: "incarnation", Generation: 1, NativeSessionID: "native", CreationReason: "initial"}))
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{ID: "turn", TaskID: "task", TaskSessionID: "session"}))
	require.NoError(t, repo.CreateMessage(ctx, &models.Message{ID: "message", TaskID: "task", TaskSessionID: "session",
		TurnID: "turn", AuthorType: models.MessageAuthorUser, Content: "interrupted prompt"}))
	block := &models.SessionRecoveryBlock{ID: "block", SessionID: "session", IncarnationID: "incarnation",
		ExpectedGeneration: 1, ConsumerReference: "agent_delivery", Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen}
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, block))
	payload := []byte(`{"text":"interrupted prompt"}`)
	submission := models.AgentDeliverySubmission{ID: "prompt:message", SessionID: "session", IncarnationID: "incarnation",
		HarnessGeneration: 1, OwnerGeneration: 1, DispatchAttemptID: "message", Payload: payload,
		PayloadHash: journal.SubmissionHash(payload), State: models.DeliverySubmissionInterruptedUnknown, Outcome: "unknown"}
	if condition != "unknown" {
		submission.State = models.DeliverySubmissionDispatching
	}
	_, err := repo.PrepareAgentDeliverySubmission(ctx, &submission)
	require.NoError(t, err)
	request := &models.AgentDeliveryReconstructionRequest{TaskID: "task", SessionID: "session", IncarnationID: "incarnation",
		HarnessGeneration: 1, ExpectedSessionState: models.TaskSessionStateWaitingForInput, NativeSessionID: "native",
		MessageID: "message", ExpectedBlock: *block, Submission: submission, SourceStreamFirstRetained: 1,
		Recovery: models.AgentDeliveryRecovery{Phase: models.AgentDeliveryRecoveryUncertain, SessionID: "session",
			SubmissionID: submission.ID, StreamID: "stream", IncarnationID: "incarnation", HarnessGeneration: 1,
			Reconstruction: &models.AgentDeliveryReconstructionProvenance{SourceSessionID: "session", SourceIncarnationID: "incarnation",
				SourceHarnessGeneration: 1, SourceStreamID: "stream", PayloadHash: submission.PayloadHash,
				ObservedState: "dispatching", SourceStreamFirstRetained: 1}}}
	request.Submission.State = models.DeliverySubmissionInterruptedUnknown
	request.Submission.Outcome = "reconstructed_unknown"
	if condition != "unknown" && condition != "no death proof" {
		terminated := condition != "live"
		request.Recovery.Reconstruction.ProcessTerminated = &terminated
	}
	if condition == "lost CAS" {
		_, err = repo.DB().ExecContext(ctx, `CREATE TRIGGER skip_dispatch_repair BEFORE UPDATE ON agent_delivery_submissions
			WHEN OLD.state = 'dispatching' BEGIN SELECT RAISE(IGNORE); END`)
		require.NoError(t, err)
	}
	if condition == "retired verified" || condition == "retired changed" || condition == "retired live" {
		_, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{ID: "historical", SessionID: "session",
			IncarnationID: "incarnation", HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "historical-hash",
			Payload: []byte("history"), State: models.DeliverySubmissionInterruptedUnknown})
		require.NoError(t, err)
		historical, readErr := repo.GetAgentDeliverySubmission(ctx, "historical")
		require.NoError(t, readErr)
		request.VerifiedRetiredSubmissions = []models.AgentDeliverySubmission{*historical}
		if condition == "retired changed" {
			_, err = repo.DB().ExecContext(ctx, `UPDATE agent_delivery_submissions SET payload_hash = 'changed' WHERE id = 'historical'`)
			require.NoError(t, err)
		}
		if condition == "retired live" {
			_, err = repo.DB().ExecContext(ctx, `UPDATE agent_delivery_submissions SET state = 'dispatching' WHERE id = 'historical'`)
			require.NoError(t, err)
		}
	}
	result, err := repo.ReconstructAgentDeliverySubmission(ctx, request)
	if condition == "retired changed" || condition == "retired live" {
		require.ErrorIs(t, err, repoerrors.ErrAgentDeliveryReconstructionConflict)
		current, readErr := repo.GetTaskSession(ctx, "session")
		require.NoError(t, readErr)
		_, exists := models.LoadAgentDeliveryRecovery(current.Metadata)
		require.False(t, exists)
		return
	}
	if condition == "lost CAS" {
		require.ErrorIs(t, err, repoerrors.ErrAgentDeliveryReconstructionStale)
		current, readErr := repo.GetTaskSession(ctx, "session")
		require.NoError(t, readErr)
		_, exists := models.LoadAgentDeliveryRecovery(current.Metadata)
		require.False(t, exists)
		unchanged, readErr := repo.GetSessionRecoveryBlock(ctx, "block")
		require.NoError(t, readErr)
		require.Empty(t, unchanged.DeliverySubmissionID)
		return
	}
	if condition == "live" || condition == "no death proof" {
		require.ErrorIs(t, err, repoerrors.ErrAgentDeliveryReconstructionConflict)
		retained, readErr := repo.GetAgentDeliverySubmission(ctx, submission.ID)
		require.NoError(t, readErr)
		require.Equal(t, models.DeliverySubmissionDispatching, retained.State)
		session, readErr := repo.GetTaskSession(ctx, "session")
		require.NoError(t, readErr)
		_, exists := models.LoadAgentDeliveryRecovery(session.Metadata)
		require.False(t, exists)
		return
	}
	require.NoError(t, err, "matching canonical bytes must permit repair of the missing session projection")
	require.Equal(t, submission.ID, result.Recovery.SubmissionID)
	retained, err := repo.GetAgentDeliverySubmission(ctx, submission.ID)
	require.NoError(t, err)
	require.Equal(t, "unknown", retained.Outcome, "projection repair preserves the canonical historical outcome")
	repeated, err := repo.ReconstructAgentDeliverySubmission(ctx, request)
	require.NoError(t, err)
	require.Equal(t, result.Recovery.Revision, repeated.Recovery.Revision)
}
