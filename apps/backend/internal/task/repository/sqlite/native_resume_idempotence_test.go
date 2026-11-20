package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestNativeResumeAuthenticatedResolutionIsIdempotent(t *testing.T) {
	testNativeResumeResolutionIsIdempotent(t, newRepoForSessionTests)
}

func TestPostgresNativeResumeAuthenticatedResolutionIsIdempotent(t *testing.T) {
	testNativeResumeResolutionIsIdempotent(t, func(t *testing.T) *Repository {
		database := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
		repo, err := NewWithDB(database, database, nil)
		require.NoError(t, err)
		return repo
	})
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12
func testNativeResumeResolutionIsIdempotent(t *testing.T, open func(*testing.T) *Repository) {
	for _, scenario := range []string{"same proof", "stale native identity", "changed payload", "different action", "new incident"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			repo := open(t)
			fixture := newSilentRestoreFixture(t, repo, "idempotent-native", models.TaskSessionStateWaitingForInput)
			proof := &models.NativeResumeDeliveryEvidence{
				SessionID: fixture.session.ID, IncarnationID: fixture.recovery.IncarnationID,
				HarnessGeneration: 1, NativeSessionID: fixture.generation.NativeSessionID,
				ExecutionID: fixture.recovery.AgentExecutionID, StreamID: fixture.recovery.StreamID,
				Submissions: []models.NativeResumeSubmissionEvidence{{ID: fixture.recovery.SubmissionID,
					HarnessGeneration: 1, PayloadHash: "payload-hash-idempotent-native", StreamID: fixture.recovery.StreamID}},
			}
			changed, err := repo.ResolveSessionRecoveryBlockWithNativeEvidence(ctx, fixture.block.ID, proof, time.Now().UTC())
			require.NoError(t, err)
			require.True(t, changed)
			switch scenario {
			case "stale native identity":
				proof.NativeSessionID = "foreign-native"
			case "changed payload":
				proof.Submissions[0].PayloadHash = "foreign-payload"
			case "different action":
				_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE session_recovery_blocks SET authorized_action = 'fresh_start' WHERE id = ?`), fixture.block.ID)
				require.NoError(t, err)
			case "new incident":
				block := fixture.block
				block.ID, block.ConsumerReference = "independent-incident", "office"
				require.NoError(t, repo.UpsertSessionRecoveryBlock(ctx, &block))
			}
			before, err := repo.GetTaskSession(ctx, fixture.session.ID)
			require.NoError(t, err)
			changed, err = repo.ResolveSessionRecoveryBlockWithNativeEvidence(ctx, fixture.block.ID, proof, time.Now().UTC())
			require.NoError(t, err)
			require.False(t, changed, "repeated resolution must not release work twice")
			verified, err := repo.VerifyResolvedSessionRecoveryBlockWithNativeEvidence(ctx, fixture.block.ID, proof)
			require.NoError(t, err)
			require.Equal(t, scenario == "same proof", verified)
			after, err := repo.GetTaskSession(ctx, fixture.session.ID)
			require.NoError(t, err)
			require.Equal(t, before.Metadata, after.Metadata)
			require.Equal(t, before.UpdatedAt, after.UpdatedAt)
		})
	}
}
