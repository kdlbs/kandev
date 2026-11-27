package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestRestartRecoveryRefreshesOnlyUnallocatedNoticeRevision(t *testing.T) {
	testPreparedRestoreNoticeRefresh(t, newRepoForSessionTests(t))
}

func TestRestartRecoveryRefreshesOnlyUnallocatedNoticeRevisionPostgres(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 4)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	testPreparedRestoreNoticeRefresh(t, repo)
}

func testPreparedRestoreNoticeRefresh(t *testing.T, repo *Repository) {
	ctx := context.Background()
	fixture := newSilentRestoreFixture(t, repo, "refresh", models.TaskSessionStateRunning)
	attempt := fixture.attempt("refresh-attempt")
	attempt.Checkpoint.SourceRecovery.Phase = models.AgentDeliveryRecoveryReconnecting
	_, created, err := repo.EnsureRestoreAttempt(ctx, attempt)
	require.NoError(t, err)
	require.True(t, created)
	attempt.Checkpoint.SourceRecovery.Phase = models.AgentDeliveryRecoveryUncertain
	attempt.Checkpoint.SourceRecovery.Revision++
	attempt.Checkpoint.SourceRecovery.UpdatedAt = time.Now().UTC()
	stored, created, err := repo.EnsureRestoreAttempt(ctx, attempt)
	require.NoError(t, err, "a pending reconnecting notice must not permanently fence its later uncertain revision")
	require.False(t, created)
	require.Equal(t, attempt.Checkpoint.SourceRecovery.Revision, stored.Checkpoint.SourceRecovery.Revision)
	for _, condition := range []string{"foreign owner", "foreign block", "stale revision", "same revision notice change", "block timestamp rollback"} {
		t.Run(condition, func(t *testing.T) {
			copyAttempt := *attempt
			copyCheckpoint := *attempt.Checkpoint
			copyAttempt.Checkpoint = &copyCheckpoint
			copyCheckpoint.SourceRecovery.Revision++
			switch condition {
			case "foreign owner":
				copyCheckpoint.SourceRecovery.AgentExecutionID = "other-owner"
			case "foreign block":
				copyCheckpoint.SourceBlock.DeliveryTurnID = "other-turn"
			case "stale revision":
				copyCheckpoint.SourceRecovery.Revision = stored.Checkpoint.SourceRecovery.Revision - 1
			case "same revision notice change":
				copyCheckpoint.SourceRecovery.Revision = stored.Checkpoint.SourceRecovery.Revision
				copyCheckpoint.SourceRecovery.Message = "unversioned change"
				copyCheckpoint.SourceBlock.UpdatedAt = stored.Checkpoint.SourceBlock.UpdatedAt.Add(time.Second)
			case "block timestamp rollback":
				copyCheckpoint.SourceBlock.UpdatedAt = stored.Checkpoint.SourceBlock.UpdatedAt.Add(-time.Second)
			}
			_, _, err := repo.EnsureRestoreAttempt(ctx, &copyAttempt)
			require.ErrorIs(t, err, repoerrors.ErrSilentRestoreAttemptConflict)
		})
	}
	attempt.Checkpoint.SourceRecovery.Revision++
	attempt.Checkpoint.SourceBlock.UpdatedAt = stored.Checkpoint.SourceBlock.UpdatedAt.Add(time.Second)
	stored, _, err = repo.EnsureRestoreAttempt(ctx, attempt)
	require.NoError(t, err, "an identical block rewritten by the later notice is still the same unallocated source")
	attempt.Checkpoint.SourceBlock.UpdatedAt = stored.Checkpoint.SourceBlock.UpdatedAt.Add(time.Second)
	stored, _, err = repo.EnsureRestoreAttempt(ctx, attempt)
	require.NoError(t, err, "a same-revision block timestamp rewrite must not permanently fence an unallocated recovery")
	allocated := *stored.Checkpoint
	allocated.Stage, allocated.CandidateExecutionID = models.SilentRestoreStageCandidateAllocated, "candidate"
	changed, err := repo.CompareAndSwapSilentRestoreCheckpoint(ctx, attempt.ID, *stored.Checkpoint, allocated)
	require.NoError(t, err)
	require.True(t, changed)
	attempt.Checkpoint.SourceRecovery.Revision++
	_, _, err = repo.EnsureRestoreAttempt(ctx, attempt)
	require.ErrorIs(t, err, repoerrors.ErrSilentRestoreAttemptConflict, "a claimed restore keeps its pinned identity")
}
