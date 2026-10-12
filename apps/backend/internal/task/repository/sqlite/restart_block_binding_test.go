package sqlite

import (
	"context"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRestartBlockBindingRepair(t *testing.T) {
	testRestartBlockBindingRepair(t, newRepoForSessionTests)
}

func TestRestartBlockBindingRepairPostgres(t *testing.T) {
	testRestartBlockBindingRepair(t, func(t *testing.T) *Repository {
		db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 4)
		repo, err := NewWithDB(db, db, nil)
		require.NoError(t, err)
		return repo
	})
}

func testRestartBlockBindingRepair(t *testing.T, create func(*testing.T) *Repository) {
	for _, condition := range []string{"matching", "generic", "stale notice", "foreign submission", "foreign stream", "independent block", "cancelled", "new generation", "stale block"} {
		t.Run(condition, func(t *testing.T) {
			repo := create(t)
			ctx := context.Background()
			fixture := newSilentRestoreFixture(t, repo, "binding", models.TaskSessionStateWaitingForInput)
			_, err := repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE session_recovery_blocks SET delivery_stream_id = '' WHERE id = ?`), fixture.block.ID)
			require.NoError(t, err)
			if condition == "generic" {
				_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE session_recovery_blocks SET delivery_submission_id = '' WHERE id = ?`), fixture.block.ID)
				require.NoError(t, err)
			}
			block, err := repo.GetSessionRecoveryBlock(ctx, fixture.block.ID)
			require.NoError(t, err)
			recovery := fixture.recovery
			switch condition {
			case "stale notice":
				recovery.Revision++
			case "foreign submission":
				block.DeliverySubmissionID = "foreign"
			case "foreign stream":
				block.DeliveryStreamID = "foreign"
			case "independent block":
				require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &models.SessionRecoveryBlock{ID: "independent", SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID, ExpectedGeneration: 1, State: models.RecoveryBlockOpen, Reason: "independent"}))
			case "cancelled":
				require.NoError(t, repo.UpdateTaskSessionState(ctx, fixture.session.ID, models.TaskSessionStateCancelled, ""))
			case "new generation":
				require.NoError(t, repo.CreateHarnessSessionGeneration(ctx, &models.HarnessSessionGeneration{SessionID: fixture.session.ID, IncarnationID: fixture.session.QueueIncarnationID, Generation: 2, NativeSessionID: "foreign", CreationReason: "reset"}))
			case "stale block":
				block.UpdatedAt = block.UpdatedAt.Add(time.Second)
			}
			changed, err := repo.RepairRestartRecoveryBlock(ctx, fixture.session.TaskID, recovery, *block)
			require.NoError(t, err)
			require.Equal(t, condition == "matching" || condition == "generic", changed)
			stored, err := repo.GetSessionRecoveryBlock(ctx, fixture.block.ID)
			require.NoError(t, err)
			if changed {
				require.Equal(t, fixture.recovery.SubmissionID, stored.DeliverySubmissionID)
				require.Equal(t, fixture.recovery.StreamID, stored.DeliveryStreamID)
			} else {
				require.Empty(t, stored.DeliveryStreamID)
			}
			require.Equal(t, models.RecoveryBlockOpen, stored.State)
		})
	}
}
