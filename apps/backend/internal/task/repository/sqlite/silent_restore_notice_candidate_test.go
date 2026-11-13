package sqlite

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestSilentRestoreCandidatesFindPendingNoticeAfterMetadataUpdate(t *testing.T) {
	testSilentRestoreCandidatesFindPendingNoticeAfterMetadataUpdate(t, newRepoForSessionTests(t), false)
}

func TestPostgresSilentRestoreCandidatesFindPendingNoticeAfterMetadataUpdate(t *testing.T) {
	database := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(database, database, nil)
	require.NoError(t, err)
	testSilentRestoreCandidatesFindPendingNoticeAfterMetadataUpdate(t, repo, true)
}

func testSilentRestoreCandidatesFindPendingNoticeAfterMetadataUpdate(
	t *testing.T,
	repo *Repository,
	postgres bool,
) {
	t.Helper()
	ctx := context.Background()
	fixture := newSilentRestoreFixture(t, repo, "pending-notice", models.TaskSessionStateWaitingForInput)
	_, err := repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE session_recovery_blocks
		SET state = ?, authorized_action = ?, resolved_at = ? WHERE id = ?`),
		models.RecoveryBlockResolved, nativeResumeRecoveryAction, time.Now().UTC(), fixture.block.ID)
	require.NoError(t, err)
	_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE agent_delivery_submissions
		SET state = ?, outcome = ? WHERE id = ?`),
		models.DeliverySubmissionCompleted, models.DeliverySubmissionCompleted, fixture.recovery.SubmissionID)
	require.NoError(t, err)
	require.NoError(t, repo.SetSessionMetadataKey(ctx, fixture.session.ID,
		models.SessionMetaKeyRecoveryNoticePending, models.RecoveryNoticeSourceIdentity(fixture.recovery)))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, fixture.session.ID, "ordinary_metadata_update", "updated"))

	if postgres {
		var metadata string
		require.NoError(t, repo.ro.GetContext(ctx, &metadata, repo.ro.Rebind(
			`SELECT metadata FROM task_sessions WHERE id = ?`), fixture.session.ID))
		require.True(t, strings.Contains(metadata, `"session_recovery_notice_pending": `),
			"PostgreSQL JSONB text formatting includes whitespace after the key separator")
	}

	candidates, err := repo.ListSilentRestoreCandidates(ctx, "", 10)
	require.NoError(t, err)
	require.Len(t, candidates, 1, "the pending notice is the only remaining startup candidate")
	require.Equal(t, fixture.session.ID, candidates[0].SessionID)
}
