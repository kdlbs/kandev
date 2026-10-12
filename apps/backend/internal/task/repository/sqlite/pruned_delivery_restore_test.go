package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

// @covers AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.5
// @covers AC-PLATFORM-PRUNED-DELIVERY-RECOVERY-001.6
func TestPrunedDeliveryRestoreCommitsHistoricalGapWithoutFabricatingReceipts(t *testing.T) {
	testPrunedDeliveryRestore(t, newRepoForSessionTests)
}

func TestPostgresPrunedDeliveryRestoreCommitsHistoricalGapWithoutFabricatingReceipts(t *testing.T) {
	dsn := testutil.PostgresDSNFromEnv(t)
	testPrunedDeliveryRestore(t, func(t *testing.T) *Repository {
		db := openIsolatedPostgresMultiConn(t, dsn, 2)
		repo, err := NewWithDB(db, db, nil)
		require.NoError(t, err)
		return repo
	})
}

func testPrunedDeliveryRestore(t *testing.T, openRepo func(*testing.T) *Repository) {
	for _, staleCursor := range []bool{false, true} {
		t.Run(map[bool]string{false: "restore", true: "cursor changed"}[staleCursor], func(t *testing.T) {
			ctx := context.Background()
			repo := openRepo(t)
			fixture := newSilentRestoreFixture(t, repo, "pruned", models.TaskSessionStateWaitingForInput)
			_, err := repo.ReceiveAgentDeliveryEvent(ctx, &models.AgentDeliveryEvent{SessionID: fixture.session.ID,
				IncarnationID: fixture.recovery.IncarnationID, HarnessGeneration: 1, StreamID: fixture.recovery.StreamID,
				Sequence: 1, EventType: "context_window", Payload: []byte(`{}`)}, 1)
			require.NoError(t, err)
			_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE agent_delivery_cursors SET projected_sequence = 1 WHERE stream_id = ?`), fixture.recovery.StreamID)
			require.NoError(t, err)
			fixture.recovery.HistoryGap = &models.AgentDeliveryHistoryGap{FirstSequence: 2, LastSequence: 6, ReceivedSequence: 1, ProjectedSequence: 1}
			fixture.recovery.Reconstruction = &models.AgentDeliveryReconstructionProvenance{PayloadHash: "payload-hash-pruned"}
			require.NoError(t, repo.SetSessionMetadataKey(ctx, fixture.session.ID, models.SessionMetaKeyAgentDeliveryRecovery, fixture.recovery))
			_, commit := prepareSilentRestoreCandidate(t, repo, fixture, "pruned-successor")
			if staleCursor {
				_, err = repo.ReceiveAgentDeliveryEvent(ctx, &models.AgentDeliveryEvent{SessionID: fixture.session.ID,
					IncarnationID: fixture.recovery.IncarnationID, HarnessGeneration: 1, StreamID: fixture.recovery.StreamID,
					Sequence: 2, EventType: "context_window", Payload: []byte(`{}`)}, 2)
				require.NoError(t, err)
			}
			changed, err := repo.CommitSilentRestore(ctx, commit)
			require.NoError(t, err)
			if staleCursor {
				require.False(t, changed, "new receipts invalidate the pinned missing range")
				return
			}
			require.True(t, changed)
			session, err := repo.GetTaskSession(ctx, fixture.session.ID)
			require.NoError(t, err)
			require.NotNil(t, session.Metadata[models.SessionMetaKeyAgentDeliveryHistoryGaps], "historical loss evidence must survive the next recovery snapshot")
			cursor, err := repo.GetAgentDeliveryCursor(ctx, fixture.recovery.StreamID)
			require.NoError(t, err)
			require.EqualValues(t, 1, cursor.ReceivedSequence)
			require.EqualValues(t, 1, cursor.ProjectedSequence)
			current, err := repo.GetCurrentHarnessSessionGeneration(ctx, fixture.session.ID, fixture.recovery.IncarnationID)
			require.NoError(t, err)
			require.Equal(t, fixture.generation.NativeSessionID, current.NativeSessionID)
			require.EqualValues(t, 2, current.Generation)
			stored, err := repo.GetAgentDeliverySubmission(ctx, fixture.recovery.SubmissionID)
			require.NoError(t, err)
			require.Equal(t, models.DeliverySubmissionInterruptedUnknown, stored.State)
			changed, err = repo.CommitSilentRestore(ctx, commit)
			require.NoError(t, err)
			require.False(t, changed, "a repeated commit cannot allocate another successor or overwrite loss evidence")
			require.NoError(t, repo.SetSessionMetadataKey(ctx, fixture.session.ID, models.SessionMetaKeyAgentDeliveryRecovery, nil))
			session, err = repo.GetTaskSession(ctx, fixture.session.ID)
			require.NoError(t, err)
			require.NotNil(t, session.Metadata[models.SessionMetaKeyAgentDeliveryHistoryGaps])
		})
	}
}
