package sqlite

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

func TestRepositoryRunGitHooksDefaultsOffAndPersists(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-run-git-hooks")

	plain := &models.Repository{ID: "repo-hooks-default", WorkspaceID: "ws-run-git-hooks", Name: "plain", SourceType: "local"}
	require.NoError(t, repo.CreateRepository(ctx, plain))
	got, err := repo.GetRepository(ctx, plain.ID)
	require.NoError(t, err)
	require.False(t, got.RunGitHooks, "a repository defaults to hooks off")

	got.RunGitHooks = true
	require.NoError(t, repo.UpdateRepository(ctx, got))
	got, err = repo.GetRepository(ctx, plain.ID)
	require.NoError(t, err)
	require.True(t, got.RunGitHooks)

	require.NoError(t, repo.runMigrations(ctx))
	got, err = repo.GetRepository(ctx, plain.ID)
	require.NoError(t, err)
	require.True(t, got.RunGitHooks, "replaying migrations keeps the value")
}

func TestRepositoryRunGitHooksIsAbsentFromExportLookup(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	seedWorkspace(t, repo, "ws-run-git-hooks-export")
	opted := &models.Repository{ID: "repo-hooks-export", WorkspaceID: "ws-run-git-hooks-export", Name: "opted", SourceType: "local", RunGitHooks: true}
	require.NoError(t, repo.CreateRepository(ctx, opted))

	tx, err := repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	exported, found, err := repo.GetRepositoryTx(ctx, tx, opted.ID)
	require.NoError(t, err)
	require.True(t, found)
	require.False(t, exported.RunGitHooks, "export reads no hooks setting")
}
