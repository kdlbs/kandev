package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/notifications/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestPostgresFormatterBindingRevisionAndOwnership(t *testing.T) {
	testFormatterBindingRevisionAndOwnership(t, testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t)))
}

func TestPostgresFormatterBindingDoesNotReviveAfterProviderTypeChange(t *testing.T) {
	testFormatterBindingDoesNotReviveAfterProviderTypeChange(t, testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t)))
}

func TestPostgresFormatterBindingConcurrentActivationHasOneWinner(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	database.SetMaxOpenConns(2)
	database.SetMaxIdleConns(2)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	repo, err := newSQLiteRepositoryWithDB(ctx, database, database)
	require.NoError(t, err)
	provider := &models.Provider{UserID: "owner", Name: "Concurrent formatter", Type: models.ProviderTypeApprise}
	require.NoError(t, repo.CreateProvider(ctx, provider))
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, revision := range []string{"snapshot-a", "snapshot-b"} {
		go func() {
			<-start
			_, err := repo.SetFormatterBinding(ctx, "owner", provider.ID, "", &models.FormatterBinding{
				ProviderType: models.ProviderTypeApprise, PluginID: "formatter", InstallationID: "install",
				FormatterKey: "text", ConfigurationRevision: revision, Digest: "digest-" + revision,
			})
			results <- err
		}()
	}
	close(start)
	winners, conflicts := 0, 0
	for range 2 {
		select {
		case err := <-results:
			switch {
			case err == nil:
				winners++
			case errors.Is(err, ErrFormatterConflict):
				conflicts++
			default:
				t.Fatalf("activate formatter: %v", err)
			}
		case <-ctx.Done():
			t.Fatalf("concurrent activation did not settle: %v", ctx.Err())
		}
	}
	require.Equal(t, 1, winners)
	require.Equal(t, 1, conflicts)
	binding, err := repo.GetFormatterBinding(ctx, "owner", provider.ID)
	require.NoError(t, err)
	require.Contains(t, []string{"snapshot-a", "snapshot-b"}, binding.ConfigurationRevision)
	require.NotEmpty(t, binding.Revision)
}
