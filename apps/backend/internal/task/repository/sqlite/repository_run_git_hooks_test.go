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

	on := true
	got.RunGitHooks = false
	require.NoError(t, repo.UpdateRepositoryWithCheckoutIntent(ctx, got, models.RepositoryCheckoutIntent{RunGitHooks: &on}))
	require.True(t, got.RunGitHooks, "the write reports the stored value")
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

func seedHooksRepository(t *testing.T, repo *Repository, workspaceID string, hooks bool) *models.Repository {
	t.Helper()
	seedWorkspace(t, repo, workspaceID)
	row := &models.Repository{ID: "repo-" + workspaceID, WorkspaceID: workspaceID, Name: "r", SourceType: "local", RunGitHooks: hooks}
	require.NoError(t, repo.CreateRepository(context.Background(), row))
	return row
}

func storedHooks(t *testing.T, repo *Repository, id string) bool {
	t.Helper()
	got, err := repo.GetRepository(context.Background(), id)
	require.NoError(t, err)
	return got.RunGitHooks
}

func TestRepositoryUpdateOmittingHooksLeavesTheStoredValue(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	row := seedHooksRepository(t, repo, "ws-hooks-omit", true)

	stale, err := repo.GetRepository(ctx, row.ID)
	require.NoError(t, err)
	stale.Name = "renamed"
	require.NoError(t, repo.UpdateRepositoryWithCheckoutIntent(ctx, stale, models.RepositoryCheckoutIntent{}))
	require.True(t, storedHooks(t, repo, row.ID), "a save that omits the setting must not rewrite it")
	require.True(t, stale.RunGitHooks)
}

func TestFullModelWritesNeverWriteRunGitHooks(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	row := seedHooksRepository(t, repo, "ws-hooks-full", false)

	stale, err := repo.GetRepository(ctx, row.ID) // read while hooks are off
	require.NoError(t, err)
	on := true
	toggle, err := repo.GetRepository(ctx, row.ID)
	require.NoError(t, err)
	require.NoError(t, repo.UpdateRepositoryWithCheckoutIntent(ctx, toggle, models.RepositoryCheckoutIntent{RunGitHooks: &on}))

	stale.Name = "stale-writer"
	require.NoError(t, repo.UpdateRepository(ctx, stale))
	require.True(t, storedHooks(t, repo, row.ID), "a stale full-model write must not turn hooks back off")
	require.True(t, stale.RunGitHooks, "the response carries the stored value")

	off := false
	reset, err := repo.GetRepository(ctx, row.ID)
	require.NoError(t, err)
	require.NoError(t, repo.UpdateRepositoryWithCheckoutIntent(ctx, reset, models.RepositoryCheckoutIntent{RunGitHooks: &off}))
	hot, err := repo.GetRepository(ctx, row.ID)
	require.NoError(t, err)
	hot.RunGitHooks = true // an in-memory value must never be persisted
	hot.Name = "exact-writer"
	require.NoError(t, repo.UpdateRepositoryIfUnchanged(ctx, hot, hot.UpdatedAt))
	require.False(t, storedHooks(t, repo, row.ID), "the exact-match write must not enable hooks")
	require.False(t, hot.RunGitHooks)
}

func TestConcurrentOmittingSaveAndHooksSaveKeepTheSetValue(t *testing.T) {
	repo := newRepoForEntityTests(t)
	ctx := context.Background()
	row := seedHooksRepository(t, repo, "ws-hooks-race", false)
	on := true

	omit, err := repo.GetRepository(ctx, row.ID)
	require.NoError(t, err)
	setter, err := repo.GetRepository(ctx, row.ID)
	require.NoError(t, err)

	omit.Name = "omitting-save"
	require.NoError(t, repo.UpdateRepositoryWithCheckoutIntent(ctx, setter, models.RepositoryCheckoutIntent{RunGitHooks: &on}))
	require.NoError(t, repo.UpdateRepositoryWithCheckoutIntent(ctx, omit, models.RepositoryCheckoutIntent{}))
	require.True(t, storedHooks(t, repo, row.ID), "the omitting save, applied last from a stale read, keeps the set value")
}
