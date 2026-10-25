package sqlite

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestInterruptedContinuationCommitPreservesUncertainty(t *testing.T) {
	testInterruptedContinuationCommit(t, newRepoForSessionTests(t))
}
func TestPostgresInterruptedContinuationCommitPreservesUncertainty(t *testing.T) {
	database := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 4)
	repo, err := NewWithDB(database, database, nil)
	require.NoError(t, err)
	testInterruptedContinuationCommit(t, repo)
}
func testInterruptedContinuationCommit(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: "task", Title: "Recovery"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: "session", TaskID: "task", State: models.TaskSessionStateRunning}))
	session, err := repo.GetTaskSession(ctx, "session")
	require.NoError(t, err)
	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{SessionID: session.ID, IncarnationID: session.QueueIncarnationID, Generation: 1, NativeSessionID: "native", CreationReason: "initial"}))
	_, err = repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{ID: "old", SessionID: session.ID, IncarnationID: session.QueueIncarnationID, HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "hash", Payload: []byte("old instruction"), State: models.DeliverySubmissionInterruptedUnknown})
	require.NoError(t, err)
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "running", SessionID: session.ID, TaskID: "task", ExecutorID: "executor", AgentExecutionID: "old-exec", Status: "running"}))
	recovery := models.AgentDeliveryRecovery{SessionID: session.ID, IncarnationID: session.QueueIncarnationID, AgentExecutionID: "old-exec", HarnessGeneration: 1, StreamID: "stream", SubmissionID: "old", PromptGeneration: 1, Phase: models.AgentDeliveryRecoveryUncertain}
	block := deliveryRecoveryBlock(recovery)
	stored, err := repo.UpsertAgentDeliveryRecovery(ctx, &recovery, block)
	require.NoError(t, err)
	require.True(t, stored)
	require.NoError(t, repo.CreateRestoreAttempt(ctx, &models.RestoreAttempt{ID: "attempt", SessionID: session.ID, IncarnationID: session.QueueIncarnationID, ExpectedGeneration: 1, Action: "resume_interrupted", Authorized: true}))
	require.NoError(t, repo.CreateContinuationSnapshot(ctx, &models.ContinuationSnapshot{ID: "snapshot", AttemptID: "attempt", SessionID: session.ID, TargetGeneration: 2, SubmissionID: "new", Content: "new instruction", ContentHash: "request-hash", Status: models.ContinuitySnapshotPrepared}))
	require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "running", SessionID: session.ID, TaskID: "task", ExecutorID: "executor", AgentExecutionID: "new-exec", Status: "running"}))
	commit := &models.InterruptedContinuationCommit{Recovery: recovery, Generation: models.HarnessSessionGeneration{SessionID: session.ID, IncarnationID: session.QueueIncarnationID, Generation: 2, PredecessorGeneration: 1, NativeSessionID: "native", CreationReason: "interrupted_continued"}, CandidateExecutionID: "new-exec", BlockID: block.ID, SnapshotID: "snapshot", ContentHash: "request-hash"}
	wrong := *commit
	wrong.Recovery.Revision++
	changed, err := repo.CommitInterruptedContinuation(ctx, &wrong)
	require.NoError(t, err)
	require.False(t, changed, "stale recovery must not resolve a block")
	wrong = *commit
	wrong.Generation.NativeSessionID = "replacement-conversation"
	changed, err = repo.CommitInterruptedContinuation(ctx, &wrong)
	require.NoError(t, err)
	require.False(t, changed, "native conversation must be preserved")
	other := &models.SessionRecoveryBlock{ID: "other", SessionID: session.ID, IncarnationID: session.QueueIncarnationID, ExpectedGeneration: 1, Reason: "independent", State: models.RecoveryBlockOpen}
	require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, other))
	changed, err = repo.CommitInterruptedContinuation(ctx, commit)
	require.NoError(t, err)
	require.False(t, changed, "independent block must fence continuation")
	_, err = repo.ResolveSessionRecoveryBlock(ctx, other.ID, "test", time.Now())
	require.NoError(t, err)
	changed, err = repo.CommitInterruptedContinuation(ctx, commit)
	require.NoError(t, err)
	require.True(t, changed, "verified restore must commit the successor generation")
	old, err := repo.GetAgentDeliverySubmission(ctx, "old")
	require.NoError(t, err)
	require.Equal(t, models.DeliverySubmissionInterruptedUnknown, old.State)
	require.Equal(t, "old instruction", string(old.Payload))
	snapshot, err := repo.GetContinuationSnapshot(ctx, "snapshot")
	require.NoError(t, err)
	require.Equal(t, "restored", snapshot.Status)
	gen, err := repo.GetCurrentHarnessSessionGeneration(ctx, session.ID, session.QueueIncarnationID)
	require.NoError(t, err)
	require.EqualValues(t, 2, gen.Generation)
	require.Equal(t, "native", gen.NativeSessionID)
	remaining, err := repo.GetOpenSessionRecoveryBlock(ctx, session.ID, session.QueueIncarnationID, 2)
	require.ErrorIs(t, err, sql.ErrNoRows)
	require.Nil(t, remaining)
	changed, err = repo.CommitInterruptedContinuation(ctx, commit)
	require.NoError(t, err)
	require.False(t, changed, "repeated commit must not advance twice")
}
