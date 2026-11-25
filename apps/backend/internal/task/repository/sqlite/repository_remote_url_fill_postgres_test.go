package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestPostgresFillRepositoryRemoteURLOnlyFillsAnEmptyURL(t *testing.T) {
	db := openIsolatedPostgresMultiConn(t, testutil.PostgresDSNFromEnv(t), 2)
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	ctx := context.Background()
	now := time.Now().UTC()
	_, err = db.Exec(`INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('ws-1', 'ws-1', $1, $2)`, now, now)
	require.NoError(t, err)
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{ID: "empty", WorkspaceID: "ws-1", Name: "empty", SourceType: "provider"}))
	require.NoError(t, repo.CreateRepository(ctx, &models.Repository{ID: "set", WorkspaceID: "ws-1", Name: "set", SourceType: "provider", RemoteURL: "https://example.test/own.git"}))

	filled, err := repo.FillRepositoryRemoteURL(ctx, "empty", "https://example.test/source.git")
	require.NoError(t, err)
	require.True(t, filled)
	filled, err = repo.FillRepositoryRemoteURL(ctx, "set", "https://example.test/source.git")
	require.NoError(t, err)
	require.False(t, filled)

	empty, err := repo.GetRepository(ctx, "empty")
	require.NoError(t, err)
	require.Equal(t, "https://example.test/source.git", empty.RemoteURL)
	set, err := repo.GetRepository(ctx, "set")
	require.NoError(t, err)
	require.Equal(t, "https://example.test/own.git", set.RemoteURL)
}
