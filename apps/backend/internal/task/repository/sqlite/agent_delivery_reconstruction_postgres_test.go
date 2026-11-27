package sqlite

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/processidentity"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestPostgresReconstructAgentDeliverySubmissionSerializesConcurrentRetries(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 4)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	ctx := context.Background()
	const taskID = "task-reconstruct-delivery-pg"
	const sessionID = "session-reconstruct-delivery-pg"
	const incarnationID = "incarnation-reconstruct-delivery-pg"
	const messageID = "message-reconstruct-delivery-pg"
	const streamID = "stream-reconstruct-delivery-pg"
	const blockID = "block-reconstruct-delivery-pg"
	now := time.Now().UTC().Truncate(time.Microsecond)

	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: taskID}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: sessionID, TaskID: taskID, QueueIncarnationID: incarnationID,
		State: models.TaskSessionStateWaitingForInput,
	}))
	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: sessionID, IncarnationID: incarnationID, Generation: 4,
		NativeSessionID: "native-conversation-pg", CreationReason: "test",
	}))
	require.NoError(t, repo.CreateTurn(ctx, &models.Turn{
		ID: "turn-reconstruct-delivery-pg", TaskID: taskID, TaskSessionID: sessionID, StartedAt: now,
	}))
	require.NoError(t, repo.CreateMessage(ctx, &models.Message{
		ID: messageID, TaskID: taskID, TaskSessionID: sessionID, TurnID: "turn-reconstruct-delivery-pg",
		AuthorType: models.MessageAuthorUser, Content: "continue the review",
	}))
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{
		ID: blockID, SessionID: sessionID, IncarnationID: incarnationID, ExpectedGeneration: 4,
		Reason: "unresolved_durable_work", State: models.RecoveryBlockOpen, ConsumerReference: "agent_delivery",
	}))
	block, err := repo.GetOpenSessionRecoveryBlock(ctx, sessionID, incarnationID, 4)
	require.NoError(t, err)

	payload, err := json.Marshal(struct {
		Text string `json:"text"`
	}{Text: "continue the review"})
	require.NoError(t, err)
	hash := sha256.Sum256(payload)
	payloadHash := hex.EncodeToString(hash[:])
	submissionID := "prompt:" + messageID
	request := &models.AgentDeliveryReconstructionRequest{
		TaskID: taskID, SessionID: sessionID, IncarnationID: incarnationID,
		HarnessGeneration: 4, ExpectedSessionState: models.TaskSessionStateWaitingForInput,
		NativeSessionID: "native-conversation-pg", MessageID: messageID,
		ExpectedBlock: *block, SourceStreamFirstRetained: 1,
		Submission: models.AgentDeliverySubmission{
			ID: submissionID, SessionID: sessionID, IncarnationID: incarnationID,
			HarnessGeneration: 4, OwnerGeneration: 4, DispatchAttemptID: messageID,
			PayloadHash: payloadHash, Payload: payload, State: models.DeliverySubmissionInterruptedUnknown,
			Outcome: "reconstructed_unknown", CreatedAt: now, UpdatedAt: now,
		},
		Recovery: models.AgentDeliveryRecovery{
			Phase: models.AgentDeliveryRecoveryUncertain, SessionID: sessionID,
			SubmissionID: submissionID, StreamID: streamID, IncarnationID: incarnationID,
			HarnessGeneration: 4,
			Reconstruction: &models.AgentDeliveryReconstructionProvenance{
				SourceSessionID: sessionID, SourceIncarnationID: incarnationID,
				SourceHarnessGeneration: 4, SourceStreamID: streamID,
				PayloadHash: payloadHash, ObservedState: "dispatching",
				SourceStreamFirstRetained: 1,
			},
		},
	}

	blocker, err := db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = blocker.Rollback() }()
	lockKey := agentDeliveryReconstructionLockNamespace + sessionID
	_, err = blocker.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, lockKey)
	require.NoError(t, err)

	type result struct {
		value *models.AgentDeliveryReconstructionResult
		err   error
	}
	started := make(chan struct{}, 2)
	results := make(chan result, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			started <- struct{}{}
			value, callErr := repo.ReconstructAgentDeliverySubmission(ctx, request)
			results <- result{value: value, err: callErr}
		}()
	}
	<-started
	<-started
	deadline := time.Now().Add(5 * time.Second)
	for db.Stats().InUse < 3 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	require.GreaterOrEqual(t, db.Stats().InUse, 3, "both retries must use separate PostgreSQL connections while the lock is held")
	require.NoError(t, blocker.Commit())
	workers.Wait()
	close(results)

	inserted := 0
	duplicates := 0
	var committed *models.AgentDeliveryReconstructionResult
	for got := range results {
		require.NoError(t, got.err)
		require.NotNil(t, got.value)
		if got.value.Inserted {
			inserted++
		} else {
			duplicates++
		}
		committed = got.value
	}
	require.Equal(t, 1, inserted)
	require.Equal(t, 1, duplicates)
	require.EqualValues(t, 1, committed.Recovery.Revision)
	require.Equal(t, submissionID, committed.Block.DeliverySubmissionID)

	rows, err := repo.ListAgentDeliverySubmissions(ctx, sessionID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, rows[0].State)
	require.Equal(t, streamID, committed.Recovery.StreamID)
	request.ExpectedBlock = committed.Block
	request.Recovery.Revision = committed.Recovery.Revision
	request.Recovery.AgentExecutionID = "original-execution"
	request.Recovery.PromptGeneration = 9
	request.Recovery.OriginalRuntime = processidentity.Identity{PID: 42, BirthToken: "verified-birth"}
	request.Recovery.Reconstruction.ProcessIdentityKnown = true
	enriched := make(chan result, 2)
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			value, callErr := repo.ReconstructAgentDeliverySubmission(ctx, request)
			enriched <- result{value: value, err: callErr}
		}()
	}
	workers.Wait()
	close(enriched)
	for got := range enriched {
		require.NoError(t, got.err)
		require.False(t, got.value.Inserted)
		require.EqualValues(t, 2, got.value.Recovery.Revision)
		require.Equal(t, "original-execution", got.value.Recovery.AgentExecutionID)
	}

}
