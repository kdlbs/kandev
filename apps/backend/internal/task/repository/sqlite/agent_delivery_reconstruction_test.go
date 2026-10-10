package sqlite

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
)

type agentDeliveryReconstructionStore interface {
	ReconstructAgentDeliverySubmission(
		context.Context,
		*models.AgentDeliveryReconstructionRequest,
	) (*models.AgentDeliveryReconstructionResult, error)
}

func TestReconstructAgentDeliverySubmission(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForSessionTests(t)
	const taskID = "task-reconstruct-delivery"
	const sessionID = "session-reconstruct-delivery"
	const incarnationID = "incarnation-reconstruct-delivery"
	const messageID = "message-reconstruct-delivery"
	const streamID = "stream-reconstruct-delivery"
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)

	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: taskID}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, QueueIncarnationID: incarnationID,
		State:    models.TaskSessionStateWaitingForInput,
		Metadata: map[string]interface{}{"unrelated": "preserved"},
	}))
	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: sessionID, IncarnationID: incarnationID, Generation: 4,
		NativeSessionID: "native-conversation", CreationReason: "test",
	}))
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-reconstruct-delivery", TaskID: taskID, TaskSessionID: sessionID,
		StartedAt: now,
	}))
	require.NoError(t, repo.CreateMessage(ctx, &models.Message{
		ID: messageID, TaskID: taskID, TaskSessionID: sessionID, TurnID: "turn-reconstruct-delivery",
		AuthorType: models.MessageAuthorUser, Content: "continue the review",
	}))
	block := &models.SessionRecoveryBlock{
		ID: "block-reconstruct-delivery", SessionID: sessionID, IncarnationID: incarnationID,
		ExpectedGeneration: 4, Reason: "unknown_prompt_outcome", State: models.RecoveryBlockOpen,
		ConsumerReference: "agent_delivery",
	}
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, block))
	event := &models.AgentDeliveryEvent{
		SessionID: sessionID, IncarnationID: incarnationID, HarnessGeneration: 4,
		StreamID: streamID, Sequence: 1, EventType: "assistant_message", Payload: []byte(`{"text":"done"}`),
	}
	inserted, err := repo.ReceiveAgentDeliveryEvent(ctx, event, 1)
	require.NoError(t, err)
	require.True(t, inserted)
	cursor, err := repo.GetAgentDeliveryCursor(ctx, streamID)
	require.NoError(t, err)

	payload, err := json.Marshal(struct {
		Text string `json:"text"`
	}{Text: "continue the review"})
	require.NoError(t, err)
	hash := sha256.Sum256(payload)
	submissionID := "prompt:" + messageID
	request := &models.AgentDeliveryReconstructionRequest{
		TaskID: taskID, SessionID: sessionID, IncarnationID: incarnationID,
		HarnessGeneration: 4, ExpectedSessionState: models.TaskSessionStateWaitingForInput,
		NativeSessionID: "native-conversation", MessageID: messageID,
		ExpectedBlock:         *block,
		SourceStreamHighWater: 1, SourceStreamAcknowledged: 1, SourceStreamFirstRetained: 1,
		ExpectedCursor: cursor,
		Submission: models.AgentDeliverySubmission{
			ID: submissionID, SessionID: sessionID, IncarnationID: incarnationID,
			HarnessGeneration: 4, OwnerGeneration: 4, DispatchAttemptID: messageID,
			PayloadHash: hex.EncodeToString(hash[:]), Payload: payload,
			State: models.DeliverySubmissionInterruptedUnknown, Outcome: "reconstructed_unknown",
			CreatedAt: now, UpdatedAt: now,
		},
		Recovery: models.AgentDeliveryRecovery{
			Phase: models.AgentDeliveryRecoveryUncertain, SessionID: sessionID,
			SubmissionID: submissionID, StreamID: streamID, IncarnationID: incarnationID,
			HarnessGeneration: 4,
			Reconstruction: &models.AgentDeliveryReconstructionProvenance{
				SourceSessionID: sessionID, SourceIncarnationID: incarnationID,
				SourceHarnessGeneration: 4, SourceStreamID: streamID,
				PayloadHash: hex.EncodeToString(hash[:]), ObservedState: "dispatching",
				SourceStreamHighWater: 1, SourceStreamAcknowledged: 1, SourceStreamFirstRetained: 1,
			},
		},
	}

	store, ok := any(repo).(agentDeliveryReconstructionStore)
	require.True(t, ok, "SQLite repository must expose atomic delivery reconstruction")
	_, err = repo.db.ExecContext(ctx, `CREATE TRIGGER fail_reconstructed_delivery_block
		BEFORE UPDATE OF delivery_submission_id ON session_recovery_blocks
		BEGIN SELECT RAISE(ABORT, 'injected reconstruction failure'); END`)
	require.NoError(t, err)
	_, err = store.ReconstructAgentDeliverySubmission(ctx, request)
	require.Error(t, err)
	var rolledBackCount int
	require.NoError(t, repo.db.GetContext(ctx, &rolledBackCount,
		`SELECT COUNT(*) FROM agent_delivery_submissions WHERE id = ?`, submissionID))
	require.Zero(t, rolledBackCount, "a failed block bind must roll back the inserted submission")
	unchangedSession, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	_, hasRecovery := models.LoadAgentDeliveryRecovery(unchangedSession.Metadata)
	require.False(t, hasRecovery, "a failed block bind must roll back the recovery projection")
	_, err = repo.db.ExecContext(ctx, `DROP TRIGGER fail_reconstructed_delivery_block`)
	require.NoError(t, err)
	_, err = repo.db.ExecContext(ctx, `INSERT INTO agent_delivery_submissions
		(id, session_id, incarnation_id, harness_generation, owner_generation, dispatch_attempt_id,
		 payload_hash, payload, state, outcome, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		submissionID, "foreign-session", incarnationID, int64(4), int64(4), messageID,
		"different-payload", []byte("different-payload"), models.DeliverySubmissionDispatching,
		"dispatching", now, now)
	require.NoError(t, err)
	_, err = store.ReconstructAgentDeliverySubmission(ctx, request)
	require.ErrorIs(t, err, repoerrors.ErrAgentDeliveryReconstructionConflict,
		"a globally reused submission ID owned by another session is a conflict")
	_, err = repo.db.ExecContext(ctx, `DELETE FROM agent_delivery_submissions WHERE id = ?`, submissionID)
	require.NoError(t, err)
	result, err := store.ReconstructAgentDeliverySubmission(ctx, request)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.True(t, result.Inserted)
	require.EqualValues(t, 1, result.Recovery.Revision)
	require.Empty(t, result.Recovery.AgentExecutionID, "a resumed execution is not historical process proof")
	require.Zero(t, result.Recovery.PromptGeneration, "the old prompt generation is not reconstructible")
	require.False(t, result.Recovery.Reconstruction.ProcessIdentityKnown)
	require.Equal(t, submissionID, result.Block.DeliverySubmissionID)
	require.Equal(t, streamID, result.Block.DeliveryStreamID)

	storedSubmission, err := repo.GetAgentDeliverySubmission(ctx, submissionID)
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, storedSubmission.State)
	require.Equal(t, payload, storedSubmission.Payload)
	require.Equal(t, hex.EncodeToString(hash[:]), storedSubmission.PayloadHash)
	var submissionCount int
	require.NoError(t, repo.db.GetContext(ctx, &submissionCount,
		`SELECT COUNT(*) FROM agent_delivery_submissions WHERE session_id = ?`, sessionID))
	require.Equal(t, 1, submissionCount)

	session, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	require.Equal(t, "preserved", session.Metadata["unrelated"])
	restored, ok := models.LoadAgentDeliveryRecovery(session.Metadata)
	require.True(t, ok)
	require.Equal(t, result.Recovery, restored)

	blockAfterRetry, err := repo.GetOpenSessionRecoveryBlock(ctx, sessionID, incarnationID, 4)
	require.NoError(t, err)
	require.Equal(t, block.ID, blockAfterRetry.ID)
	require.Equal(t, submissionID, blockAfterRetry.DeliverySubmissionID)
	require.Equal(t, streamID, blockAfterRetry.DeliveryStreamID)

	duplicate, err := store.ReconstructAgentDeliverySubmission(ctx, request)
	require.NoError(t, err)
	require.NotNil(t, duplicate)
	require.False(t, duplicate.Inserted)
	require.Equal(t, result.Recovery.Revision, duplicate.Recovery.Revision)
	require.Equal(t, result.Recovery.UpdatedAt, duplicate.Recovery.UpdatedAt)

	// A later authenticated owner observation can complete the same recovery.
	request.Recovery.Revision = duplicate.Recovery.Revision
	request.Recovery.AgentExecutionID = "historical-execution"
	request.Recovery.PromptGeneration = 9
	request.Recovery.OriginalRuntime = processidentity.Identity{PID: 42, BirthToken: "verified-birth"}
	request.Recovery.Reconstruction.ProcessIdentityKnown = true
	enriched, err := store.ReconstructAgentDeliverySubmission(ctx, request)
	require.NoError(t, err)
	require.False(t, enriched.Inserted)
	require.EqualValues(t, 2, enriched.Recovery.Revision)
	require.Equal(t, "historical-execution", enriched.Recovery.AgentExecutionID)
	require.EqualValues(t, 9, enriched.Recovery.PromptGeneration)
	require.True(t, enriched.Recovery.Reconstruction.ProcessIdentityKnown)
	stale := *request
	stale.Recovery.AgentExecutionID = "conflicting-execution"
	_, err = store.ReconstructAgentDeliverySubmission(ctx, &stale)
	require.Error(t, err, "stale evidence cannot replace an established identity")
	repeated, err := store.ReconstructAgentDeliverySubmission(ctx, request)
	require.NoError(t, err)
	require.Equal(t, enriched.Recovery.Revision, repeated.Recovery.Revision)
}
