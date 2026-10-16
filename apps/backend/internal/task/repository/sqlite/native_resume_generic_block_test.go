package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestNativeResumeEmptyHistoricalBlock(t *testing.T) {
	testNativeResumeEmptyHistoricalBlock(t, newRepoForSessionTests)
}

func TestPostgresNativeResumeEmptyHistoricalBlock(t *testing.T) {
	testNativeResumeEmptyHistoricalBlock(t, func(t *testing.T) *Repository {
		database := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
		repo, err := NewWithDB(database, database, nil)
		require.NoError(t, err)
		return repo
	})
}

func testNativeResumeEmptyHistoricalBlock(t *testing.T, open func(*testing.T) *Repository) {
	for _, scenario := range []string{"absent", "resolved history", "legacy queue history", "unresolved history", "active work", "partial block", "independent block"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			repo := open(t)
			fixture := newSilentRestoreFixture(t, repo, "empty-native", models.TaskSessionStateWaitingForInput)
			if scenario != "absent" {
				changed, err := repo.ResolveSessionRecoveryBlock(ctx, fixture.block.ID, "resume", time.Now().UTC())
				require.NoError(t, err)
				require.True(t, changed)
			} else {
				require.NoError(t, repo.SetSessionMetadataKey(ctx, fixture.session.ID, models.SessionMetaKeyAgentDeliveryRecovery, nil))
				_, err := repo.DB().ExecContext(ctx, repo.db.Rebind(`DELETE FROM session_recovery_blocks WHERE id = ?`), fixture.block.ID)
				require.NoError(t, err)
			}
			if scenario == "legacy queue history" {
				_, err := repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE session_recovery_blocks SET delivery_stream_id = '', resolved_at = ? WHERE id = ?`), time.Now().UTC().Add(-time.Hour), fixture.block.ID)
				require.NoError(t, err)
			}
			generic := fixture.block
			generic.ID = "empty-current-block"
			generic.State = models.RecoveryBlockOpen
			generic.DeliverySubmissionID, generic.DeliveryStreamID = "", ""
			generic.ResolvedAt, generic.AuthorizedAction = nil, ""
			generic.Reason = "unresolved_durable_work"
			require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &generic))
			if scenario == "unresolved history" {
				_, err := repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE session_recovery_blocks SET authorized_action = 'stop' WHERE id = ?`), fixture.block.ID)
				require.NoError(t, err)
			}
			if scenario == "active work" {
				_, err := repo.PrepareAgentDeliverySubmission(ctx, &models.AgentDeliverySubmission{ID: "independent-active", SessionID: fixture.session.ID, IncarnationID: fixture.recovery.IncarnationID, HarnessGeneration: 1, OwnerGeneration: 1, PayloadHash: "independent", Payload: []byte("independent"), State: models.DeliverySubmissionAccepted})
				require.NoError(t, err)
			}
			if scenario == "partial block" {
				_, err := repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE session_recovery_blocks SET delivery_submission_id = 'foreign' WHERE id = ?`), generic.ID)
				require.NoError(t, err)
			}
			if scenario == "independent block" {
				independent := generic
				independent.ID, independent.ConsumerReference, independent.Reason = "independent-block", "office", "independent_reason"
				require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &independent))
			}
			changed, err := repo.ResolveSessionRecoveryBlock(ctx, generic.ID, "resume", time.Now().UTC())
			require.NoError(t, err)
			require.Equal(t, scenario == "absent" || scenario == "resolved history" || scenario == "legacy queue history", changed)
			submission, err := repo.GetAgentDeliverySubmission(ctx, fixture.recovery.SubmissionID)
			require.NoError(t, err)
			require.Equal(t, models.DeliverySubmissionInterruptedUnknown, submission.State)
			require.Zero(t, submission.TerminalSequence)
			require.Equal(t, []byte("old immutable prompt"), submission.Payload)
			current, err := repo.GetCurrentHarnessSessionGeneration(ctx, fixture.session.ID, fixture.recovery.IncarnationID)
			require.NoError(t, err)
			require.Equal(t, fixture.generation.NativeSessionID, current.NativeSessionID)
			require.EqualValues(t, 1, current.Generation)
		})
	}
}
