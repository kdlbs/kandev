package sqlite

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// A cold native resume after continue_from_history omits a historical
// submission the peer no longer retains. That omission must not reject the
// continued notice; a retained but contradictory peer record still must.
func TestNativeResumeProofAdmitsHistoricalNotice(t *testing.T) {
	recovery := models.AgentDeliveryRecovery{SubmissionID: "predecessor", StreamID: "stream-1", HarnessGeneration: 1}
	current := models.NativeResumeSubmissionEvidence{ID: "current", HarnessGeneration: 2, StreamID: "stream-2"}

	omitted := &models.NativeResumeDeliveryEvidence{Submissions: []models.NativeResumeSubmissionEvidence{current}}
	require.True(t, nativeResumeProofAdmitsHistoricalNotice(omitted, recovery), "a record the peer no longer retains falls through to the SQL ancestry checks")

	retained := &models.NativeResumeDeliveryEvidence{Submissions: []models.NativeResumeSubmissionEvidence{
		current, {ID: "predecessor", HarnessGeneration: 1, StreamID: "stream-1"},
	}}
	require.True(t, nativeResumeProofAdmitsHistoricalNotice(retained, recovery))

	contradictory := &models.NativeResumeDeliveryEvidence{Submissions: []models.NativeResumeSubmissionEvidence{
		current, {ID: "predecessor", HarnessGeneration: 1, StreamID: "stream-other"},
	}}
	require.False(t, nativeResumeProofAdmitsHistoricalNotice(contradictory, recovery), "retained peer evidence must still match the notice")
}

func TestCommitHarnessGenerationRetiresContextNoticeOnlyForMatchingFence(t *testing.T) {
	for _, test := range []struct {
		name               string
		expectedGeneration int64
		mismatch           string
		wantCommit         bool
		wantRetired        bool
	}{
		{name: "matching fence", expectedGeneration: 1, wantCommit: true, wantRetired: true},
		{name: "wrong block stream", expectedGeneration: 1, mismatch: "block", wantCommit: true},
		{name: "block is already resolved", expectedGeneration: 1, mismatch: "resolved_block", wantCommit: true},
		{name: "submission is not unknown", expectedGeneration: 1, mismatch: "submission", wantCommit: true},
		{name: "wrong predecessor", expectedGeneration: 1, mismatch: "generation", wantCommit: true},
		{name: "stale generation", expectedGeneration: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			ctx := context.Background()
			fixture := newSilentRestoreFixture(t, repo, "notice-"+test.name, models.TaskSessionStateWaitingForInput)
			switch test.mismatch {
			case "block":
				_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE session_recovery_blocks SET delivery_stream_id = ? WHERE id = ?`), "other-stream", fixture.block.ID)
				require.NoError(t, err)
			case "resolved_block":
				_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE session_recovery_blocks SET state = 'resolved', authorized_action = 'continue_from_history' WHERE id = ?`), fixture.block.ID)
				require.NoError(t, err)
			case "submission":
				_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE agent_delivery_submissions SET state = ? WHERE id = ?`), models.DeliverySubmissionDispatching, fixture.recovery.SubmissionID)
				require.NoError(t, err)
			}
			generation := models.HarnessSessionGeneration{
				SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID,
				Generation: 2, PredecessorGeneration: 1, NativeSessionID: "native-next-" + test.name,
				CreationReason: contextContinuationOutcome, CreatedAt: time.Now().UTC(), CommittedAt: time.Now().UTC(),
			}
			if test.mismatch == "generation" {
				generation.PredecessorGeneration = 0
			}
			committed, err := repo.CommitHarnessSessionGeneration(ctx, &generation, test.expectedGeneration)
			require.NoError(t, err)
			require.Equal(t, test.wantCommit, committed)

			session, err := repo.GetTaskSession(ctx, fixture.session.ID)
			require.NoError(t, err)
			recovery, exists := models.LoadAgentDeliveryRecovery(session.Metadata)
			require.True(t, exists)
			if test.wantRetired {
				require.Equal(t, models.AgentDeliveryRecoveryContinued, recovery.Phase)
				require.Equal(t, fixture.recovery.Revision+1, recovery.Revision)
			} else {
				require.Equal(t, fixture.recovery.Phase, recovery.Phase)
				require.Equal(t, fixture.recovery.Revision, recovery.Revision)
			}
			submission, err := repo.GetAgentDeliverySubmission(ctx, fixture.recovery.SubmissionID)
			require.NoError(t, err)
			if test.mismatch == "submission" {
				require.Equal(t, models.DeliverySubmissionDispatching, submission.State)
			} else {
				require.Equal(t, models.DeliverySubmissionInterruptedUnknown, submission.State)
			}
		})
	}
}

func TestRetireContextContinuationNoticeIsIdempotent(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	fixture := newSilentRestoreFixture(t, repo, "notice-idempotent", models.TaskSessionStateWaitingForInput)
	generation := models.HarnessSessionGeneration{
		SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID,
		Generation: 2, PredecessorGeneration: 1, NativeSessionID: "native-notice-idempotent",
		CreationReason: contextContinuationOutcome, CreatedAt: time.Now().UTC(), CommittedAt: time.Now().UTC(),
	}
	tx, err := repo.db.BeginTx(ctx, nil)
	require.NoError(t, err)
	require.NoError(t, repo.retireContextContinuationNoticeTx(ctx, tx, &generation, 1))
	require.NoError(t, repo.retireContextContinuationNoticeTx(ctx, tx, &generation, 1))
	require.NoError(t, tx.Commit())

	session, err := repo.GetTaskSession(ctx, fixture.session.ID)
	require.NoError(t, err)
	recovery, exists := models.LoadAgentDeliveryRecovery(session.Metadata)
	require.True(t, exists)
	require.Equal(t, models.AgentDeliveryRecoveryContinued, recovery.Phase)
	require.Equal(t, fixture.recovery.Revision+1, recovery.Revision)
}

func TestListContextContinuedSubmissionIDsRequiresConsumedContiguousAncestry(t *testing.T) {
	for _, test := range []struct {
		name             string
		submissionState  models.DeliverySubmissionState
		snapshotStatus   string
		creationReason   string
		firstPredecessor int64
		wantIncluded     bool
	}{
		{name: "multi-generation chain", submissionState: models.DeliverySubmissionInterruptedUnknown, snapshotStatus: models.ContinuitySnapshotConsumed, creationReason: contextContinuationOutcome, firstPredecessor: 1, wantIncluded: true},
		{name: "dispatching predecessor", submissionState: models.DeliverySubmissionDispatching, snapshotStatus: models.ContinuitySnapshotConsumed, creationReason: contextContinuationOutcome, firstPredecessor: 1, wantIncluded: true},
		{name: "generation gap", submissionState: models.DeliverySubmissionInterruptedUnknown, snapshotStatus: models.ContinuitySnapshotConsumed, creationReason: contextContinuationOutcome, firstPredecessor: 0},
		{name: "unconsumed snapshot", submissionState: models.DeliverySubmissionInterruptedUnknown, snapshotStatus: models.ContinuitySnapshotPrepared, creationReason: contextContinuationOutcome, firstPredecessor: 1},
		{name: "non-continuation generation", submissionState: models.DeliverySubmissionInterruptedUnknown, snapshotStatus: models.ContinuitySnapshotConsumed, creationReason: "native_replaced", firstPredecessor: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			ctx := context.Background()
			fixture := newSilentRestoreFixture(t, repo, "ancestry-"+test.name, models.TaskSessionStateWaitingForInput)
			if test.submissionState != models.DeliverySubmissionInterruptedUnknown {
				_, err := repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE agent_delivery_submissions SET state = ? WHERE id = ?`), test.submissionState, fixture.recovery.SubmissionID)
				require.NoError(t, err)
			}
			seedConsumedContextContinuation(t, repo, fixture, 2, test.firstPredecessor, test.snapshotStatus, test.creationReason)
			seedConsumedContextContinuation(t, repo, fixture, 3, 2, test.snapshotStatus, contextContinuationOutcome)

			ids, err := repo.ListContextContinuedSubmissionIDs(ctx, fixture.session.ID, fixture.session.QueueIncarnationID, "native-chain-3", 3)
			require.NoError(t, err)
			if test.wantIncluded {
				require.Equal(t, []string{fixture.recovery.SubmissionID}, ids)
			} else {
				require.Empty(t, ids)
			}
		})
	}
}

func seedConsumedContextContinuation(
	t *testing.T,
	repo *Repository,
	fixture *silentRestoreFixture,
	generation, predecessor int64,
	snapshotStatus, creationReason string,
) {
	t.Helper()
	ctx := context.Background()
	completedAt := time.Now().UTC()
	generationID := strconv.FormatInt(generation, 10)
	attemptID := "continuation-attempt-" + fixture.session.ID + "-" + generationID
	attempt := &models.RestoreAttempt{
		ID: attemptID, SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID,
		ExpectedGeneration: predecessor, Action: "continue_from_history", Outcome: contextContinuationOutcome,
		Authorized: true, CreatedAt: completedAt, CompletedAt: &completedAt,
	}
	require.NoError(t, repo.CreateRestoreAttempt(ctx, attempt))
	require.NoError(t, repo.CreateContinuationSnapshot(ctx, &models.ContinuationSnapshot{
		AttemptID: attemptID, SessionID: fixture.session.ID, TargetGeneration: generation,
		SubmissionID: "continuation-submission-" + attemptID, Content: "continued history",
		ContentHash: "continued-history-hash", Status: snapshotStatus, CreatedAt: completedAt,
	}))
	require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{
		SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID,
		Generation: generation, PredecessorGeneration: predecessor, NativeSessionID: "native-chain-" + generationID,
		CreationReason: creationReason, CreatedAt: completedAt, CommittedAt: completedAt,
	}))
}

func TestCompleteSupersededContextAttemptLosesStaleCheckpointCAS(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	fixture := newSilentRestoreFixture(t, repo, "attempt-cas", models.TaskSessionStateWaitingForInput)
	attempt := fixture.attempt("attempt-cas-lost")
	require.NoError(t, repo.CreateRestoreAttempt(ctx, attempt))
	previous, err := marshalSilentRestoreCheckpoint(attempt.Checkpoint)
	require.NoError(t, err)
	concurrent := *attempt.Checkpoint
	concurrent.Stage = models.SilentRestoreStageCandidateAllocated
	concurrent.CandidateExecutionID = "candidate-from-concurrent-update"
	concurrentJSON, err := marshalSilentRestoreCheckpoint(&concurrent)
	require.NoError(t, err)
	_, err = repo.db.ExecContext(ctx, repo.db.Rebind(`UPDATE session_restore_attempts SET checkpoint_json = ? WHERE id = ?`), concurrentJSON, attempt.ID)
	require.NoError(t, err)

	tx, err := repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	changed, err := completeSupersededContextAttemptTx(ctx, tx, repo.db.Rebind, attempt, previous, time.Now().UTC())
	require.NoError(t, err)
	require.False(t, changed)
	require.NoError(t, tx.Commit())

	stored, err := repo.GetRestoreAttempt(ctx, attempt.ID)
	require.NoError(t, err)
	require.Equal(t, models.SilentRestoreOutcomePending, stored.Outcome)
	require.Equal(t, models.SilentRestoreStageCandidateAllocated, stored.Checkpoint.Stage)
	require.Equal(t, "candidate-from-concurrent-update", stored.Checkpoint.CandidateExecutionID)
}
