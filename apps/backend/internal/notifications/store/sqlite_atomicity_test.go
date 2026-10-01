package store

import (
	"context"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/notifications/models"
	"github.com/kandev/kandev/internal/testutil"
)

// @covers AC-PLATFORM-NOTIFICATIONS-001.9, AC-PLATFORM-NOTIFICATIONS-001.10
func TestSQLiteRepositoryAtomicProviderSave(t *testing.T) {
	testAtomicProviderSave(t, openNotificationTestDB(t))
}

// @covers AC-PLATFORM-NOTIFICATIONS-001.9, AC-PLATFORM-NOTIFICATIONS-001.10
func TestPostgresRepositoryAtomicProviderSave(t *testing.T) {
	testAtomicProviderSave(t, testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t)))
}

func testAtomicProviderSave(t *testing.T, database *sqlx.DB) {
	t.Helper()
	ctx := context.Background()
	repo, err := newSQLiteRepositoryWithDB(ctx, database, database)
	require.NoError(t, err)
	original := &models.Provider{UserID: "owner", Name: "Original", Type: models.ProviderTypeLocal, Enabled: true}
	require.NoError(t, repo.CreateProviderWithSubscriptions(ctx, original, []string{"session.clarification_requested"}))
	before, err := repo.GetProvider(ctx, "owner", original.ID)
	require.NoError(t, err)
	beforeSubs, err := repo.ListSubscriptionsByProvider(ctx, original.ID)
	require.NoError(t, err)
	require.Len(t, beforeSubs, 1)
	require.Equal(t, "owner", beforeSubs[0].UserID)

	t.Run("create_rollback", func(t *testing.T) { testAtomicCreateRollback(t, repo) })
	t.Run("update_rollback", func(t *testing.T) { testAtomicUpdateRollback(t, repo, before, beforeSubs) })
	t.Run("foreign_and_missing", func(t *testing.T) { testAtomicForeignAndMissing(t, repo, before, beforeSubs) })
	t.Run("omitted_then_empty", func(t *testing.T) { testAtomicOmittedThenEmpty(t, repo, before, beforeSubs) })
	t.Run("complete_update", func(t *testing.T) { testAtomicCompleteUpdate(t, repo, before, beforeSubs) })
}

func testAtomicCreateRollback(t *testing.T, repo Repository) {
	t.Helper()
	ctx := context.Background()
	failed := &models.Provider{ID: "failed-create", UserID: "owner", Name: "Failed", Type: models.ProviderTypeLocal}
	require.Error(t, repo.CreateProviderWithSubscriptions(ctx, failed, []string{"session.turn_finished", "session.turn_finished"}))
	_, err := repo.GetProvider(ctx, "owner", failed.ID)
	require.ErrorIs(t, err, ErrProviderNotFound)
	subs, err := repo.ListSubscriptionsByProvider(ctx, failed.ID)
	require.NoError(t, err)
	require.Empty(t, subs)
}

func testAtomicUpdateRollback(t *testing.T, repo Repository, before *models.Provider, beforeSubs []*models.Subscription) {
	t.Helper()
	ctx := context.Background()
	changed := *before
	changed.Name, changed.Enabled = "Changed", false
	changed.Config = map[string]interface{}{"marker": "changed"}
	events := []string{"session.turn_finished", "session.turn_finished"}
	require.Error(t, repo.UpdateProviderWithSubscriptions(ctx, &changed, &events))
	assertProviderConfiguration(t, repo, before, beforeSubs)
}

func testAtomicForeignAndMissing(t *testing.T, repo Repository, before *models.Provider, beforeSubs []*models.Subscription) {
	t.Helper()
	ctx := context.Background()
	for _, id := range []string{before.ID, "missing-provider"} {
		for _, selection := range []*[]string{nil, {}, {"session.turn_finished"}} {
			forged := &models.Provider{ID: id, UserID: "intruder", Name: "Forged", Type: models.ProviderTypeLocal}
			require.ErrorIs(t, repo.UpdateProviderWithSubscriptions(ctx, forged, selection), ErrProviderNotFound)
			assertProviderConfiguration(t, repo, before, beforeSubs)
		}
	}
}

func testAtomicOmittedThenEmpty(t *testing.T, repo Repository, before *models.Provider, beforeSubs []*models.Subscription) {
	t.Helper()
	ctx := context.Background()
	changed := *before
	changed.Name = "Changed"
	require.NoError(t, repo.UpdateProviderWithSubscriptions(ctx, &changed, nil))
	persisted, err := repo.GetProvider(ctx, "owner", before.ID)
	require.NoError(t, err)
	require.Equal(t, changed.Name, persisted.Name)
	require.Equal(t, changed.Type, persisted.Type)
	require.Equal(t, changed.Config, persisted.Config)
	require.Equal(t, changed.Enabled, persisted.Enabled)
	require.Equal(t, before.CreatedAt, persisted.CreatedAt)
	require.False(t, persisted.UpdatedAt.IsZero())
	subs, err := repo.ListSubscriptionsByProvider(ctx, before.ID)
	require.NoError(t, err)
	require.Equal(t, beforeSubs, subs)
	empty := []string{}
	require.NoError(t, repo.UpdateProviderWithSubscriptions(ctx, &changed, &empty))
	subs, err = repo.ListSubscriptionsByProvider(ctx, before.ID)
	require.NoError(t, err)
	require.Empty(t, subs)
}

func testAtomicCompleteUpdate(t *testing.T, repo Repository, before *models.Provider, beforeSubs []*models.Subscription) {
	t.Helper()
	ctx := context.Background()
	changed := *before
	changed.Name, changed.Enabled, changed.Type = "Changed", false, models.ProviderTypeApprise
	changed.Config = map[string]interface{}{"urls": "json://changed"}
	events := []string{"session.turn_finished", "system.update_available"}
	require.NoError(t, repo.UpdateProviderWithSubscriptions(ctx, &changed, &events))
	persisted, err := repo.GetProvider(ctx, "owner", before.ID)
	require.NoError(t, err)
	require.Equal(t, changed.Name, persisted.Name)
	require.Equal(t, changed.Type, persisted.Type)
	require.Equal(t, changed.Config, persisted.Config)
	require.Equal(t, changed.Enabled, persisted.Enabled)
	require.Equal(t, before.CreatedAt, persisted.CreatedAt)
	require.False(t, persisted.UpdatedAt.IsZero())
	subs, err := repo.ListSubscriptionsByProvider(ctx, before.ID)
	require.NoError(t, err)
	require.Len(t, subs, len(events))
	got := make([]string, 0, len(subs))
	for _, sub := range subs {
		require.Equal(t, "owner", sub.UserID)
		require.Equal(t, before.ID, sub.ProviderID)
		require.True(t, sub.Enabled)
		got = append(got, sub.EventType)
	}
	require.ElementsMatch(t, events, got)
}

func assertProviderConfiguration(t *testing.T, repo Repository, before *models.Provider, beforeSubs []*models.Subscription) {
	t.Helper()
	persisted, err := repo.GetProvider(context.Background(), before.UserID, before.ID)
	require.NoError(t, err)
	require.Equal(t, before, persisted)
	subs, err := repo.ListSubscriptionsByProvider(context.Background(), before.ID)
	require.NoError(t, err)
	require.Equal(t, beforeSubs, subs)
}
