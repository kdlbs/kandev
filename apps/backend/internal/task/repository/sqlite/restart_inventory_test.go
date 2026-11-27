package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestSilentRestoreCandidatesFindMissingBlocks(t *testing.T) {
	testRestartMissingBlocks(t, newRepoForSessionTests(t))
}

func TestSilentRestoreCandidatesFindMissingBlocksPostgres(t *testing.T) {
	database := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 4)
	repo, err := NewWithDB(database, database, nil)
	require.NoError(t, err)
	testRestartMissingBlocks(t, repo)
}

func testRestartMissingBlocks(t *testing.T, repo *Repository) {
	ctx := context.Background()
	fixture := newSilentRestoreFixture(t, repo, "missing-block", models.TaskSessionStateWaitingForInput)
	_, err := repo.DB().ExecContext(ctx, repo.db.Rebind(`DELETE FROM session_recovery_blocks WHERE id = ?`), fixture.block.ID)
	require.NoError(t, err)
	candidates, err := repo.ListSilentRestoreCandidates(ctx, "", 100)
	require.NoError(t, err)
	require.Len(t, candidates, 1, "unresolved SQL work must be discoverable even if its presentation fence was lost")
	require.Equal(t, fixture.session.ID, candidates[0].SessionID)
	changed, err := repo.ResolveSessionRecoveryBlock(ctx, fixture.block.ID, "resume", fixture.recovery.UpdatedAt)
	require.NoError(t, err)
	require.False(t, changed, "a deleted block cannot masquerade as a successful resume")
}
