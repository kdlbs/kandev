package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestTurnRepositoryUnavailableEndCannotBeRecaptured(t *testing.T) {
	testTurnRepositoryUnavailableEnd(t, newRepoForSessionTests(t))
}

func TestPostgresTurnRepositoryUnavailableEndCannotBeRecaptured(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	testTurnRepositoryUnavailableEnd(t, repo)
}

func testTurnRepositoryUnavailableEnd(t *testing.T, repo *Repository) {
	t.Helper()
	ctx := context.Background()
	setID, repositoryID := seedTurnChangeContentOwner(t, repo, "end-failure", time.Now().Add(time.Hour))
	_, err := repo.db.Exec(repo.db.Rebind(`UPDATE turn_change_sets SET terminal_at = NULL, availability = 'pending' WHERE id = ?`), setID)
	require.NoError(t, err)
	_, err = repo.db.Exec(repo.db.Rebind(`UPDATE turn_repository_changes SET availability = 'pending', end_commit_oid = '', end_tree_oid = '', end_ref = '', end_captured_at = NULL WHERE id = ?`), repositoryID)
	require.NoError(t, err)
	for range 2 {
		accepted, err := repo.SetTurnRepositoryEndUnavailable(ctx, setID, repositoryID, "abc123", "tree123", models.TurnChangeReasonCaptureFailed)
		require.NoError(t, err)
		require.True(t, accepted)
	}
	row, err := repo.GetTurnRepositoryChange(ctx, setID, repositoryID)
	require.NoError(t, err)
	require.Equal(t, models.TurnChangeAvailabilityUnavailable, row.Availability)
	require.Equal(t, models.TurnChangeReasonCaptureFailed, row.Reason)
	require.Empty(t, row.EndCommitOID)
	now := time.Now().UTC()
	laterEnd := models.TurnRepositoryChangeSet{EndCommitOID: "later-commit", EndTreeOID: "later-tree", HashAlgorithm: "sha1", EndReachabilityRef: "refs/kandev/turn-changes/later", EndCapturedAt: &now}
	accepted, err := repo.AcceptTurnRepositoryEnd(ctx, setID, repositoryID, "abc123", "tree123", laterEnd)
	require.NoError(t, err)
	require.False(t, accepted, "a settled failure cannot accept a later snapshot")
	accepted, err = repo.SetTurnRepositoryEndUnavailable(ctx, setID, repositoryID, "wrong-start", "tree123", models.TurnChangeReasonCaptureFailed)
	require.Error(t, err)
	require.False(t, accepted)
}
