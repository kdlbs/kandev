package store

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/notifications/models"
	"github.com/stretchr/testify/require"
)

func TestFormatterBindingRevisionAndOwnership(t *testing.T) {
	testFormatterBindingRevisionAndOwnership(t, openNotificationTestDB(t))
}

func testFormatterBindingRevisionAndOwnership(t *testing.T, database *sqlx.DB) {
	t.Helper()
	ctx := context.Background()
	repo, err := newSQLiteRepositoryWithDB(ctx, database, database)
	require.NoError(t, err)
	provider := &models.Provider{UserID: "alice", Name: "Mail", Type: models.ProviderTypeApprise, Enabled: true}
	require.NoError(t, repo.CreateProvider(ctx, provider))
	next := &models.FormatterBinding{PluginID: "formatter", InstallationID: "installation", FormatterKey: "text", ConfigurationRevision: "snapshot-a", Digest: "digest-a", ProviderType: models.ProviderTypeApprise}
	_, err = repo.SetFormatterBinding(ctx, "bob", provider.ID, "", next)
	require.ErrorIs(t, err, ErrProviderNotFound)
	first, err := repo.SetFormatterBinding(ctx, "alice", provider.ID, "", next)
	require.NoError(t, err)
	require.NotEmpty(t, first.Revision)
	_, err = repo.SetFormatterBinding(ctx, "alice", provider.ID, "", next)
	require.ErrorIs(t, err, ErrFormatterConflict)
	saved, err := repo.GetFormatterBinding(ctx, "alice", provider.ID)
	require.NoError(t, err)
	require.Equal(t, "snapshot-a", saved.ConfigurationRevision)
	require.Equal(t, first.Revision, saved.Revision)
	cleared, err := repo.SetFormatterBinding(ctx, "alice", provider.ID, first.Revision, nil)
	require.NoError(t, err)
	require.Empty(t, cleared.PluginID)
	require.NotEqual(t, first.Revision, cleared.Revision)
	_, err = repo.SetFormatterBinding(ctx, "alice", provider.ID, "", next)
	require.ErrorIs(t, err, ErrFormatterConflict, "clearing must not reopen the absent-row revision")
	_, err = newSQLiteRepositoryWithDB(ctx, database, database)
	require.NoError(t, err)
	saved, err = repo.GetFormatterBinding(ctx, "alice", provider.ID)
	require.NoError(t, err)
	require.Equal(t, cleared.Revision, saved.Revision)
}

func TestFormatterBindingDoesNotReviveAfterProviderTypeChange(t *testing.T) {
	testFormatterBindingDoesNotReviveAfterProviderTypeChange(t, openNotificationTestDB(t))
}

func testFormatterBindingDoesNotReviveAfterProviderTypeChange(t *testing.T, database *sqlx.DB) {
	t.Helper()
	ctx := context.Background()
	repo, err := newSQLiteRepositoryWithDB(ctx, database, database)
	require.NoError(t, err)
	provider := &models.Provider{UserID: "alice", Name: "Mail", Type: models.ProviderTypeApprise}
	require.NoError(t, repo.CreateProvider(ctx, provider))
	first, err := repo.SetFormatterBinding(ctx, "alice", provider.ID, "", &models.FormatterBinding{
		ProviderType: models.ProviderTypeApprise, PluginID: "formatter", InstallationID: "install",
		FormatterKey: "text", ConfigurationRevision: "snapshot", Digest: "digest",
	})
	require.NoError(t, err)
	provider.Type = models.ProviderTypeWebhook
	require.NoError(t, repo.UpdateProvider(ctx, provider))
	provider.Type = models.ProviderTypeApprise
	require.NoError(t, repo.UpdateProvider(ctx, provider))
	binding, err := repo.GetFormatterBinding(ctx, "alice", provider.ID)
	require.NoError(t, err)
	require.Empty(t, binding.PluginID)
	require.NotEqual(t, first.Revision, binding.Revision)
	require.NoError(t, repo.DeleteProvider(ctx, "alice", provider.ID))
	_, err = repo.GetFormatterBinding(ctx, "alice", provider.ID)
	require.ErrorIs(t, err, ErrProviderNotFound)
}
