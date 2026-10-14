package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/notifications/models"
	notificationstore "github.com/kandev/kandev/internal/notifications/store"
)

func atomicityService(t *testing.T) (*Service, notificationstore.Repository, *sqlx.DB) {
	t.Helper()
	raw, err := db.OpenSQLite(filepath.Join(t.TempDir(), "notifications.db"))
	require.NoError(t, err)
	database := sqlx.NewDb(raw, "sqlite3")
	t.Cleanup(func() { require.NoError(t, database.Close()) })
	repo, _, err := notificationstore.Provide(context.Background(), database, database)
	require.NoError(t, err)
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	return NewService(repo, nil, nil, log, nil), repo, database
}

func rejectSubscriptionInsert(t *testing.T, database *sqlx.DB) {
	t.Helper()
	_, err := database.Exec(`CREATE TRIGGER reject_notification_subscription
 BEFORE INSERT ON notification_subscriptions
 WHEN NEW.event_type = 'session.turn_finished'
 BEGIN SELECT RAISE(ABORT, 'injected subscription failure'); END`)
	require.NoError(t, err)
}

// @covers AC-PLATFORM-NOTIFICATIONS-001.9
func TestNotificationRejectedUpdatePreservesConfiguration(t *testing.T) {
	for _, failure := range []string{"invalid_events", "invalid_config", "subscription_write"} {
		t.Run(failure, func(t *testing.T) {
			svc, repo, database := atomicityService(t)
			ctx := context.Background()
			original, err := svc.CreateProvider(ctx, "owner", "Original", models.ProviderTypeApprise,
				map[string]interface{}{"urls": "json://original"}, true, []string{EventTaskSessionClarificationAsked})
			require.NoError(t, err)
			before, err := repo.GetProvider(ctx, "owner", original.ID)
			require.NoError(t, err)
			beforeSubs, err := repo.ListSubscriptionsByProvider(ctx, original.ID)
			require.NoError(t, err)
			name, enabled, providerType := "Changed", false, models.ProviderTypeApprise
			events := []string{EventSystemUpdateAvailable, EventTaskSessionTurnFinished}
			config := map[string]interface{}{"urls": "json://changed"}
			switch failure {
			case "invalid_events":
				events = []string{"unsupported.event"}
			case "invalid_config":
				config = map[string]interface{}{}
			case "subscription_write":
				rejectSubscriptionInsert(t, database)
			}
			_, err = svc.UpdateProvider(ctx, "owner", original.ID, ProviderUpdate{
				Name: &name, Enabled: &enabled, Type: &providerType, Config: config, Events: &events,
			})
			require.Error(t, err)
			after, err := repo.GetProvider(ctx, "owner", original.ID)
			require.NoError(t, err)
			require.Equal(t, before, after, "rejected save must preserve every provider field and timestamp")
			afterSubs, err := repo.ListSubscriptionsByProvider(ctx, original.ID)
			require.NoError(t, err)
			require.Equal(t, beforeSubs, afterSubs, "rejected save must preserve subscription rows")
		})
	}
}

// @covers AC-PLATFORM-NOTIFICATIONS-001.9
func TestNotificationFailedCreateLeavesNoProvider(t *testing.T) {
	for _, failure := range []string{"invalid_events", "subscription_write"} {
		t.Run(failure, func(t *testing.T) {
			svc, repo, database := atomicityService(t)
			events := []string{EventSystemUpdateAvailable, EventTaskSessionTurnFinished}
			if failure == "invalid_events" {
				events = []string{"unsupported.event"}
			} else {
				rejectSubscriptionInsert(t, database)
			}
			provider, err := svc.CreateProvider(context.Background(), "owner", "New", models.ProviderTypeLocal, nil, true, events)
			require.Error(t, err)
			require.Nil(t, provider)
			rows, err := repo.ListProvidersByUser(context.Background(), "owner")
			require.NoError(t, err)
			require.Empty(t, rows, "failed creation must not leave a provider row")
			var count int
			require.NoError(t, database.Get(&count, `SELECT COUNT(*) FROM notification_subscriptions`))
			require.Zero(t, count)
		})
	}
}

// @covers AC-PLATFORM-NOTIFICATIONS-001.9, AC-PLATFORM-NOTIFICATIONS-001.10
func TestNotificationSuccessfulSaveAndEventSelection(t *testing.T) {
	svc, repo, _ := atomicityService(t)
	ctx := context.Background()
	original, err := svc.CreateProvider(ctx, "owner", "Original", models.ProviderTypeLocal,
		map[string]interface{}{"marker": "original"}, true, []string{EventTaskSessionClarificationAsked})
	require.NoError(t, err)
	beforeSubs, err := repo.ListSubscriptionsByProvider(ctx, original.ID)
	require.NoError(t, err)
	name, enabled := "Changed", false
	updated, err := svc.UpdateProvider(ctx, "owner", original.ID, ProviderUpdate{
		Name: &name, Enabled: &enabled, Config: map[string]interface{}{"marker": "changed"},
	})
	require.NoError(t, err)
	persisted, err := repo.GetProvider(ctx, "owner", original.ID)
	require.NoError(t, err)
	require.Equal(t, updated, persisted)
	require.Equal(t, "Changed", persisted.Name)
	require.False(t, persisted.Enabled)
	require.Equal(t, "changed", persisted.Config["marker"])
	afterSubs, err := repo.ListSubscriptionsByProvider(ctx, original.ID)
	require.NoError(t, err)
	require.Equal(t, beforeSubs, afterSubs, "omitted selection preserves subscription rows")
	events := []string{EventTaskSessionTurnFinished, EventSystemUpdateAvailable}
	_, err = svc.UpdateProvider(ctx, "owner", original.ID, ProviderUpdate{Events: &events})
	require.NoError(t, err)
	afterSubs, err = repo.ListSubscriptionsByProvider(ctx, original.ID)
	require.NoError(t, err)
	require.Len(t, afterSubs, 2)
	require.ElementsMatch(t, events, []string{afterSubs[0].EventType, afterSubs[1].EventType})
	events = []string{}
	_, err = svc.UpdateProvider(ctx, "owner", original.ID, ProviderUpdate{Events: &events})
	require.NoError(t, err)
	afterSubs, err = repo.ListSubscriptionsByProvider(ctx, original.ID)
	require.NoError(t, err)
	require.Empty(t, afterSubs)
}

// @covers AC-PLATFORM-NOTIFICATIONS-001.9
func TestNotificationDefaultProviderCreationRetriesAfterFailure(t *testing.T) {
	for _, providerType := range []models.ProviderType{models.ProviderTypeLocal, models.ProviderTypeSystem} {
		t.Run(string(providerType), func(t *testing.T) {
			svc, repo, database := atomicityService(t)
			svc.providers[models.ProviderTypeSystem] = &captureProvider{}
			_, err := database.Exec(`CREATE TRIGGER reject_default_subscription
    BEFORE INSERT ON notification_subscriptions
    WHEN NEW.event_type = 'system.update_available' AND
     (SELECT type FROM notification_providers WHERE id = NEW.provider_id) = '` + string(providerType) + `'
    BEGIN SELECT RAISE(ABORT, 'injected default subscription failure'); END`)
			require.NoError(t, err)
			_, _, err = svc.ListProviders(context.Background(), "owner")
			require.Error(t, err)
			saved, err := repo.ListProvidersByUser(context.Background(), "owner")
			require.NoError(t, err)
			for _, provider := range saved {
				require.NotEqual(t, providerType, provider.Type, "failed bootstrap must not persist incomplete provider")
			}
			_, err = database.Exec(`DROP TRIGGER reject_default_subscription`)
			require.NoError(t, err)
			saved, subscriptions, err := svc.ListProviders(context.Background(), "owner")
			require.NoError(t, err)
			require.Len(t, saved, 2)
			for _, provider := range saved {
				require.ElementsMatch(t, []string{EventTaskSessionClarificationAsked, EventOfficeInboxItem, EventSystemUpdateAvailable}, subscriptions[provider.ID])
			}
		})
	}
}

// @covers AC-PLATFORM-NOTIFICATIONS-001.10
func TestNotificationAtomicSaveHonorsOwnership(t *testing.T) {
	svc, repo, _ := atomicityService(t)
	ctx := context.Background()
	provider, err := svc.CreateProvider(ctx, "owner", "Original", models.ProviderTypeLocal, nil, true,
		[]string{EventTaskSessionClarificationAsked})
	require.NoError(t, err)
	before, err := repo.GetProvider(ctx, "owner", provider.ID)
	require.NoError(t, err)
	beforeSubs, err := repo.ListSubscriptionsByProvider(ctx, provider.ID)
	require.NoError(t, err)
	for _, id := range []string{provider.ID, "missing-provider"} {
		name := "Forged"
		events := []string{EventTaskSessionTurnFinished}
		_, err := svc.UpdateProvider(ctx, "intruder", id, ProviderUpdate{Name: &name, Events: &events})
		require.ErrorIs(t, err, ErrProviderNotFound)
		persisted, err := repo.GetProvider(ctx, "owner", provider.ID)
		require.NoError(t, err)
		require.Equal(t, before, persisted)
		subs, err := repo.ListSubscriptionsByProvider(ctx, provider.ID)
		require.NoError(t, err)
		require.Equal(t, beforeSubs, subs)
	}
}
