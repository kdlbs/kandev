package store

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/notifications/models"
	"github.com/kandev/kandev/internal/testutil"
)

func openNotificationTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "notifications.db"))
	if err != nil {
		t.Fatalf("open SQLite database: %v", err)
	}
	database := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	return database
}

func TestSQLiteRepositoryDeliverySchemaUsesOccurrenceID(t *testing.T) {
	database := openNotificationTestDB(t)
	if _, err := newSQLiteRepositoryWithDB(context.Background(), database, database); err != nil {
		t.Fatalf("create repository: %v", err)
	}

	rows, err := database.Queryx(`PRAGMA table_info(notification_deliveries)`)
	if err != nil {
		t.Fatalf("inspect delivery schema: %v", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan delivery schema: %v", err)
		}
		if name == "occurrence_id" {
			return
		}
	}
	t.Fatal("notification_deliveries must retain a semantic occurrence_id")
}

func TestSQLiteRepositoryInitializationHonorsCanceledContext(t *testing.T) {
	database := openNotificationTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := newSQLiteRepositoryWithDB(ctx, database, database)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("initialize repository error = %v, want context canceled", err)
	}
}

func TestSQLiteRepositoryMigratesLegacyWaitingSubscriptionToClarificationOnly(t *testing.T) {
	database := openNotificationTestDB(t)
	ctx := context.Background()
	legacySchema := `
		CREATE TABLE notification_providers (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, name TEXT NOT NULL, type TEXT NOT NULL,
			config TEXT DEFAULT '{}', enabled INTEGER NOT NULL DEFAULT 1,
			created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL
		);
		CREATE TABLE notification_subscriptions (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, provider_id TEXT NOT NULL,
			event_type TEXT NOT NULL, enabled INTEGER NOT NULL DEFAULT 1,
			created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL,
			UNIQUE(provider_id, event_type)
		);
	`
	if _, err := database.Exec(legacySchema); err != nil {
		t.Fatalf("create legacy schema: %v", err)
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO notification_providers (id, user_id, name, type, created_at, updated_at)
		VALUES ('provider-1', 'user-1', 'Legacy', 'local', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
		INSERT INTO notification_subscriptions (id, user_id, provider_id, event_type, enabled, created_at, updated_at)
		VALUES ('subscription-1', 'user-1', 'provider-1', 'session.waiting_for_input', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
	`); err != nil {
		t.Fatalf("seed legacy subscription: %v", err)
	}

	repo, err := newSQLiteRepositoryWithDB(ctx, database, database)
	if err != nil {
		t.Fatalf("migrate legacy schema: %v", err)
	}
	subscriptions, err := repo.ListSubscriptionsByProvider(ctx, "provider-1")
	if err != nil {
		t.Fatalf("list migrated subscriptions: %v", err)
	}
	if !hasEnabledSubscription(subscriptions, "session.clarification_requested") || !hasEnabledSubscription(subscriptions, "system.update_available") {
		t.Fatalf("legacy subscriptions = %#v, want enabled clarification and update subscriptions", subscriptions)
	}
}

func TestSQLiteRepositoryMigratesExistingLocalAndSystemUpdateSubscriptionsOnlyOnce(t *testing.T) {
	database := openNotificationTestDB(t)
	ctx := context.Background()
	repo, err := newSQLiteRepositoryWithDB(ctx, database, database)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}
	providerIDs := make(map[string]string)
	for _, provider := range []*models.Provider{
		{ID: "local", UserID: "user-1", Name: "Local", Type: models.ProviderTypeLocal, Enabled: true},
		{ID: "system", UserID: "user-1", Name: "System", Type: models.ProviderTypeSystem, Enabled: true},
		{ID: "apprise", UserID: "user-1", Name: "Apprise", Type: models.ProviderTypeApprise, Enabled: true},
	} {
		if err := repo.CreateProvider(ctx, provider); err != nil {
			t.Fatalf("create %s provider: %v", provider.Type, err)
		}
		providerIDs[string(provider.Type)] = provider.ID
	}
	if _, err := database.Exec(`DELETE FROM notification_migrations`); err != nil {
		t.Fatalf("clear fresh migration marker: %v", err)
	}

	if err := repo.migrateUpdateAvailableSubscriptions(ctx); err != nil {
		t.Fatalf("migrate update subscriptions: %v", err)
	}
	if err := repo.ReplaceSubscriptions(ctx, providerIDs[string(models.ProviderTypeLocal)], "user-1", []string{}); err != nil {
		t.Fatalf("user opt-out: %v", err)
	}
	if err := repo.migrateUpdateAvailableSubscriptions(ctx); err != nil {
		t.Fatalf("replay update migration: %v", err)
	}
	for providerType, want := range map[string]bool{"local": false, "system": true, "apprise": false} {
		providerID := providerIDs[providerType]
		subscriptions, err := repo.ListSubscriptionsByProvider(ctx, providerID)
		if err != nil {
			t.Fatalf("list %s subscriptions: %v", providerType, err)
		}
		got := false
		for _, subscription := range subscriptions {
			got = got || (subscription.EventType == "system.update_available" && subscription.Enabled)
		}
		if got != want {
			t.Fatalf("%s update subscription = %v, want %v", providerType, got, want)
		}
	}
}

func TestSQLiteRepositoryMergesLegacySubscriptionIdempotently(t *testing.T) {
	database := openNotificationTestDB(t)
	ctx := context.Background()
	if _, err := database.Exec(`
		CREATE TABLE notification_providers (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, name TEXT NOT NULL, type TEXT NOT NULL,
			config TEXT DEFAULT '{}', enabled INTEGER NOT NULL DEFAULT 1,
			created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL
		);
		CREATE TABLE notification_subscriptions (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, provider_id TEXT NOT NULL,
			event_type TEXT NOT NULL, enabled INTEGER NOT NULL DEFAULT 1,
			created_at TIMESTAMP NOT NULL, updated_at TIMESTAMP NOT NULL,
			UNIQUE(provider_id, event_type)
		);
		INSERT INTO notification_providers (id, user_id, name, type, created_at, updated_at)
		VALUES ('provider-1', 'user-1', 'Legacy', 'local', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
		INSERT INTO notification_subscriptions (id, user_id, provider_id, event_type, enabled, created_at, updated_at) VALUES
			('legacy', 'user-1', 'provider-1', 'session.waiting_for_input', 1, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP),
			('semantic', 'user-1', 'provider-1', 'session.clarification_requested', 0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP);
	`); err != nil {
		t.Fatalf("seed legacy and semantic subscriptions: %v", err)
	}
	if _, err := newSQLiteRepositoryWithDB(ctx, database, database); err != nil {
		t.Fatalf("first migration: %v", err)
	}
	repo, err := newSQLiteRepositoryWithDB(ctx, database, database)
	if err != nil {
		t.Fatalf("replay migration: %v", err)
	}
	subscriptions, err := repo.ListSubscriptionsByProvider(ctx, "provider-1")
	if err != nil {
		t.Fatalf("list migrated subscriptions: %v", err)
	}
	if !hasEnabledSubscription(subscriptions, "session.clarification_requested") || !hasEnabledSubscription(subscriptions, "system.update_available") {
		t.Fatalf("merged subscriptions = %#v, want enabled clarification and update subscriptions", subscriptions)
	}
}

func hasEnabledSubscription(subscriptions []*models.Subscription, eventType string) bool {
	for _, subscription := range subscriptions {
		if subscription.EventType == eventType && subscription.Enabled {
			return true
		}
	}
	return false
}

func TestSQLiteRepositoryDeliveryIdempotencyIsScopedToOccurrence(t *testing.T) {
	database := openNotificationTestDB(t)
	repo, err := newSQLiteRepositoryWithDB(context.Background(), database, database)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}
	ctx := context.Background()
	first, err := repo.InsertDelivery(ctx, &models.Delivery{UserID: "user-1", ProviderID: "provider-1", EventType: "session.turn_finished", TaskSessionID: "session-1", OccurrenceID: "turn-1"})
	if err != nil || !first {
		t.Fatalf("insert first occurrence: inserted=%v err=%v", first, err)
	}
	replayed, err := repo.InsertDelivery(ctx, &models.Delivery{UserID: "user-1", ProviderID: "provider-1", EventType: "session.turn_finished", TaskSessionID: "session-1", OccurrenceID: "turn-1"})
	if err != nil || replayed {
		t.Fatalf("replay occurrence: inserted=%v err=%v", replayed, err)
	}
	later, err := repo.InsertDelivery(ctx, &models.Delivery{UserID: "user-1", ProviderID: "provider-1", EventType: "session.turn_finished", TaskSessionID: "session-1", OccurrenceID: "turn-2"})
	if err != nil || !later {
		t.Fatalf("insert later occurrence: inserted=%v err=%v", later, err)
	}
	if _, err := repo.InsertDelivery(ctx, &models.Delivery{UserID: "user-1", ProviderID: "provider-1", EventType: "session.turn_finished", TaskSessionID: "session-1"}); err == nil {
		t.Fatal("empty occurrence ID must be rejected")
	}
}

func TestSQLiteRepositoryMigratesLegacyDeliveriesPreservingHistoryAndReplayability(t *testing.T) {
	database := openNotificationTestDB(t)
	seedLegacyDeliverySchema(t, database)

	repo, err := newSQLiteRepositoryWithDB(context.Background(), database, database)
	if err != nil {
		t.Fatalf("first migration: %v", err)
	}
	if _, err := newSQLiteRepositoryWithDB(context.Background(), database, database); err != nil {
		t.Fatalf("replay migration: %v", err)
	}

	assertMigratedLegacyDeliveries(t, database)
	assertDeliverySchema(t, database)
	assertOccurrenceUniqueness(t, repo)
}

func TestSQLiteRepositoryDeliveryMigrationRollsBackOnMalformedLegacyTable(t *testing.T) {
	database := openNotificationTestDB(t)
	if _, err := database.Exec(`
		CREATE TABLE notification_deliveries (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, provider_id TEXT NOT NULL,
			event_type TEXT NOT NULL, task_session_id TEXT NOT NULL,
			UNIQUE(provider_id, event_type, task_session_id)
		);
		INSERT INTO notification_deliveries (id, user_id, provider_id, event_type, task_session_id)
		VALUES ('delivery-1', 'user-1', 'provider-1', 'session.waiting_for_input', 'session-1');
	`); err != nil {
		t.Fatalf("seed malformed legacy deliveries: %v", err)
	}

	if _, err := newSQLiteRepositoryWithDB(context.Background(), database, database); err == nil {
		t.Fatal("migration must fail after beginning when legacy history is malformed")
	}
	var count int
	if err := database.Get(&count, `SELECT COUNT(*) FROM notification_deliveries WHERE id = 'delivery-1'`); err != nil || count != 1 {
		t.Fatalf("legacy delivery after rollback = count %d, err %v", count, err)
	}
	var occurrenceColumns int
	if err := database.Get(&occurrenceColumns, `SELECT COUNT(*) FROM pragma_table_info('notification_deliveries') WHERE name = 'occurrence_id'`); err != nil || occurrenceColumns != 0 {
		t.Fatalf("legacy schema after rollback = occurrence columns %d, err %v", occurrenceColumns, err)
	}
}

func TestPostgresRepositoryMigratesLegacyDeliveriesReplayably(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	seedLegacyDeliverySchema(t, database)

	repo, err := newSQLiteRepositoryWithDB(context.Background(), database, database)
	if err != nil {
		t.Fatalf("first migration: %v", err)
	}
	if _, err := newSQLiteRepositoryWithDB(context.Background(), database, database); err != nil {
		t.Fatalf("replay migration: %v", err)
	}

	assertMigratedLegacyDeliveries(t, database)
	assertDeliverySchema(t, database)
	assertOccurrenceUniqueness(t, repo)
}

func TestPostgresRepositoryFreshSchemaReinitializes(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	if _, err := newSQLiteRepositoryWithDB(context.Background(), database, database); err != nil {
		t.Fatalf("first schema initialization: %v", err)
	}
	if _, err := newSQLiteRepositoryWithDB(context.Background(), database, database); err != nil {
		t.Fatalf("replayed schema initialization: %v", err)
	}
}

func seedLegacyDeliverySchema(t *testing.T, database *sqlx.DB) {
	t.Helper()
	if _, err := database.Exec(`
		CREATE TABLE notification_deliveries (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, provider_id TEXT NOT NULL,
			event_type TEXT NOT NULL, task_session_id TEXT NOT NULL, created_at TIMESTAMP NOT NULL,
			UNIQUE(provider_id, event_type, task_session_id)
		);
		INSERT INTO notification_deliveries (id, user_id, provider_id, event_type, task_session_id, created_at) VALUES
			('delivery-1', 'user-1', 'provider-1', 'session.waiting_for_input', 'session-1', CURRENT_TIMESTAMP),
			('delivery-2', 'user-1', 'provider-1', 'session.waiting_for_input', 'session-2', CURRENT_TIMESTAMP);
	`); err != nil {
		t.Fatalf("seed legacy deliveries: %v", err)
	}
}

func assertMigratedLegacyDeliveries(t *testing.T, database *sqlx.DB) {
	t.Helper()
	var rows []struct {
		ID           string `db:"id"`
		OccurrenceID string `db:"occurrence_id"`
	}
	if err := database.Select(&rows, `SELECT id, occurrence_id FROM notification_deliveries ORDER BY id`); err != nil {
		t.Fatalf("list migrated deliveries: %v", err)
	}
	if got := fmt.Sprint(rows); got != "[{delivery-1 session-1} {delivery-2 session-2}]" {
		t.Fatalf("migrated delivery history = %s", got)
	}
	var occurrenceColumns int
	if err := database.Get(&occurrenceColumns, database.Rebind(`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = ? AND column_name = ?`), "notification_deliveries", "occurrence_id"); err != nil && database.DriverName() != "sqlite3" {
		t.Fatalf("inspect postgres occurrence column: %v", err)
	}
	if database.DriverName() == "sqlite3" {
		if err := database.Get(&occurrenceColumns, `SELECT COUNT(*) FROM pragma_table_info('notification_deliveries') WHERE name = 'occurrence_id'`); err != nil {
			t.Fatalf("inspect sqlite occurrence column: %v", err)
		}
	}
	if occurrenceColumns != 1 {
		t.Fatalf("occurrence_id column count = %d, want 1", occurrenceColumns)
	}
}

func assertOccurrenceUniqueness(t *testing.T, repo *sqliteRepository) {
	t.Helper()
	ctx := context.Background()
	first := &models.Delivery{UserID: "user-1", ProviderID: "provider-1", EventType: "session.turn_finished", TaskSessionID: "session-3", OccurrenceID: "turn-1"}
	if inserted, err := repo.InsertDelivery(ctx, first); err != nil || !inserted {
		t.Fatalf("insert first occurrence: inserted=%v err=%v", inserted, err)
	}
	later := &models.Delivery{UserID: "user-1", ProviderID: "provider-1", EventType: "session.turn_finished", TaskSessionID: "session-3", OccurrenceID: "turn-2"}
	if inserted, err := repo.InsertDelivery(ctx, later); err != nil || !inserted {
		t.Fatalf("insert later same-session occurrence: inserted=%v err=%v", inserted, err)
	}
	if inserted, err := repo.InsertDelivery(ctx, first); err != nil || inserted {
		t.Fatalf("replay same occurrence: inserted=%v err=%v", inserted, err)
	}
}

func assertDeliverySchema(t *testing.T, database *sqlx.DB) {
	t.Helper()
	if database.DriverName() == "sqlite3" {
		assertSQLiteDeliverySchema(t, database)
		return
	}
	assertPostgresDeliverySchema(t, database)
}

func assertSQLiteDeliverySchema(t *testing.T, database *sqlx.DB) {
	t.Helper()
	var indexNames []string
	if err := database.Select(&indexNames, `SELECT name FROM pragma_index_list('notification_deliveries') WHERE "unique" = 1`); err != nil {
		t.Fatalf("list sqlite delivery indexes: %v", err)
	}
	for _, indexName := range indexNames {
		var columns []string
		if err := database.Select(&columns, `SELECT name FROM pragma_index_info(?) ORDER BY seqno`, indexName); err != nil {
			t.Fatalf("inspect sqlite index %s: %v", indexName, err)
		}
		if fmt.Sprint(columns) == "[provider_id event_type occurrence_id]" {
			return
		}
	}
	t.Fatalf("sqlite unique indexes = %#v, want provider/event/occurrence uniqueness", indexNames)
}

func assertPostgresDeliverySchema(t *testing.T, database *sqlx.DB) {
	t.Helper()
	var definitions []string
	if err := database.Select(&definitions, `
		SELECT pg_get_constraintdef(con.oid)
		FROM pg_constraint con
		JOIN pg_class rel ON rel.oid = con.conrelid
		JOIN pg_namespace nsp ON nsp.oid = rel.relnamespace
		WHERE rel.relname = 'notification_deliveries'
			AND nsp.nspname = current_schema()
			AND con.contype = 'u'
	`); err != nil {
		t.Fatalf("list postgres delivery constraints: %v", err)
	}
	for _, definition := range definitions {
		if definition == "UNIQUE (provider_id, event_type, occurrence_id)" {
			return
		}
	}
	t.Fatalf("postgres unique constraints = %#v, want provider/event/occurrence uniqueness", definitions)
}
