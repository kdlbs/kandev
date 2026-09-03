package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/notifications/models"
)

type sqliteRepository struct {
	db     *sqlx.DB // writer
	ro     *sqlx.DB // reader
	ownsDB bool
}

var _ Repository = (*sqliteRepository)(nil)

func newSQLiteRepositoryWithDB(ctx context.Context, writer, reader *sqlx.DB) (*sqliteRepository, error) {
	return newSQLiteRepository(ctx, writer, reader, false)
}

func newSQLiteRepository(ctx context.Context, writer, reader *sqlx.DB, ownsDB bool) (*sqliteRepository, error) {
	repo := &sqliteRepository{db: writer, ro: reader, ownsDB: ownsDB}
	if err := repo.initSchema(ctx); err != nil {
		if ownsDB {
			if closeErr := writer.Close(); closeErr != nil {
				return nil, fmt.Errorf("failed to close database after schema error: %w", closeErr)
			}
		}
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}
	return repo, nil
}

func (r *sqliteRepository) Close() error {
	if !r.ownsDB {
		return nil
	}
	return r.db.Close()
}

func (r *sqliteRepository) initSchema(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS notification_providers (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		config TEXT DEFAULT '{}',
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);

	CREATE TABLE IF NOT EXISTS notification_subscriptions (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		provider_id TEXT NOT NULL,
		event_type TEXT NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		UNIQUE(provider_id, event_type),
		FOREIGN KEY (provider_id) REFERENCES notification_providers(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS notification_deliveries (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		provider_id TEXT NOT NULL,
		event_type TEXT NOT NULL,
		task_session_id TEXT NOT NULL,
		occurrence_id TEXT NOT NULL,
		created_at TIMESTAMP NOT NULL,
		UNIQUE(provider_id, event_type, occurrence_id)
	);

	CREATE TABLE IF NOT EXISTS notification_migrations (
		name TEXT PRIMARY KEY,
		completed_at TIMESTAMP NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_notification_providers_user_id ON notification_providers(user_id);
	CREATE INDEX IF NOT EXISTS idx_notification_subscriptions_provider_id ON notification_subscriptions(provider_id);
	CREATE INDEX IF NOT EXISTS idx_notification_subscriptions_user_id ON notification_subscriptions(user_id);
	CREATE INDEX IF NOT EXISTS idx_notification_deliveries_session_id ON notification_deliveries(task_session_id);
	`

	if _, err := r.db.Exec(schema); err != nil {
		return err
	}
	if err := r.migrateDeliveries(); err != nil {
		return err
	}
	if err := r.migrateLegacySubscriptions(); err != nil {
		return err
	}
	return r.migrateUpdateAvailableSubscriptions(ctx)
}

const updateAvailableSubscriptionMigration = "add_system_update_available_to_local_and_system_v1"

func (r *sqliteRepository) migrateUpdateAvailableSubscriptions(ctx context.Context) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var completed bool
	err = tx.GetContext(ctx, &completed, r.db.Rebind(`SELECT TRUE FROM notification_migrations WHERE name = ?`), updateAvailableSubscriptionMigration)
	if err == nil {
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(`
		SELECT p.id, p.user_id FROM notification_providers p
		WHERE p.type IN (?, ?)
		AND NOT EXISTS (
			SELECT 1 FROM notification_subscriptions s
			WHERE s.provider_id = p.id AND s.event_type = ?
		)
	`), "local", "system", "system.update_available")
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	type providerRow struct{ id, userID string }
	var providers []providerRow
	for rows.Next() {
		var provider providerRow
		if err := rows.Scan(&provider.id, &provider.userID); err != nil {
			return err
		}
		providers = append(providers, provider)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, provider := range providers {
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`
			INSERT INTO notification_subscriptions (id, user_id, provider_id, event_type, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`), uuid.New().String(), provider.userID, provider.id, "system.update_available", dialect.BoolToInt(true), now, now); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO notification_migrations (name, completed_at) VALUES (?, ?)`), updateAvailableSubscriptionMigration, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *sqliteRepository) migrateDeliveries() error {
	if dialect.IsPostgres(r.db.DriverName()) {
		return r.migratePostgresDeliveries()
	}
	var schema string
	err := r.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'notification_deliveries'`).Scan(&schema)
	if errors.Is(err, sql.ErrNoRows) || strings.Contains(schema, "occurrence_id") {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect notification deliveries schema: %w", err)
	}
	tx, err := r.db.Beginx()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, statement := range []string{
		`CREATE TABLE notification_deliveries_new (
			id TEXT PRIMARY KEY, user_id TEXT NOT NULL, provider_id TEXT NOT NULL,
			event_type TEXT NOT NULL, task_session_id TEXT NOT NULL, occurrence_id TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL, UNIQUE(provider_id, event_type, occurrence_id)
		)`,
		`INSERT INTO notification_deliveries_new (id, user_id, provider_id, event_type, task_session_id, occurrence_id, created_at)
			SELECT id, user_id, provider_id, event_type, task_session_id, CAST(task_session_id AS TEXT), created_at
			FROM notification_deliveries`,
		`DROP TABLE notification_deliveries`,
		`ALTER TABLE notification_deliveries_new RENAME TO notification_deliveries`,
		`CREATE INDEX IF NOT EXISTS idx_notification_deliveries_session_id ON notification_deliveries(task_session_id)`,
	} {
		if _, err := tx.Exec(statement); err != nil {
			return fmt.Errorf("migrate notification deliveries: %w", err)
		}
	}
	return tx.Commit()
}

func (r *sqliteRepository) migratePostgresDeliveries() error {
	_, err := r.db.Exec(`
		ALTER TABLE notification_deliveries
		ADD COLUMN IF NOT EXISTS occurrence_id TEXT NOT NULL DEFAULT '';
		UPDATE notification_deliveries SET occurrence_id = task_session_id WHERE occurrence_id = '';
		DO $$
		DECLARE old_constraint text;
		BEGIN
			SELECT con.conname INTO old_constraint
			FROM pg_constraint con
			JOIN pg_class rel ON rel.oid = con.conrelid
			JOIN pg_namespace nsp ON nsp.oid = rel.relnamespace
			WHERE rel.relname = 'notification_deliveries' AND nsp.nspname = current_schema()
				AND con.contype = 'u'
				AND (SELECT array_agg(attr.attname::text ORDER BY cols.ordinality)
					FROM unnest(con.conkey) WITH ORDINALITY AS cols(attnum, ordinality)
					JOIN pg_attribute attr ON attr.attrelid = con.conrelid AND attr.attnum = cols.attnum)
				= ARRAY['provider_id', 'event_type', 'task_session_id'];
			IF old_constraint IS NOT NULL THEN
				EXECUTE format('ALTER TABLE notification_deliveries DROP CONSTRAINT %I', old_constraint);
			END IF;
			IF NOT EXISTS (
				SELECT 1 FROM pg_constraint con JOIN pg_class rel ON rel.oid = con.conrelid
				JOIN pg_namespace nsp ON nsp.oid = rel.relnamespace
				WHERE rel.relname = 'notification_deliveries' AND nsp.nspname = current_schema()
					AND con.contype = 'u'
					AND (SELECT array_agg(attr.attname::text ORDER BY cols.ordinality)
						FROM unnest(con.conkey) WITH ORDINALITY AS cols(attnum, ordinality)
						JOIN pg_attribute attr ON attr.attrelid = con.conrelid AND attr.attnum = cols.attnum)
					= ARRAY['provider_id', 'event_type', 'occurrence_id']
			) THEN
				ALTER TABLE notification_deliveries ADD CONSTRAINT notification_deliveries_provider_event_occurrence_key
					UNIQUE (provider_id, event_type, occurrence_id);
			END IF;
		END $$;
	`)
	return err
}

func (r *sqliteRepository) migrateLegacySubscriptions() error {
	tx, err := r.db.Beginx()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.Queryx(r.db.Rebind(`
		SELECT id, provider_id, enabled FROM notification_subscriptions
		WHERE event_type = ?
	`), "session.waiting_for_input")
	if err != nil {
		return err
	}
	type legacySubscription struct {
		id, providerID string
		enabled        int
	}
	legacy := make([]legacySubscription, 0)
	for rows.Next() {
		var subscription legacySubscription
		if err := rows.Scan(&subscription.id, &subscription.providerID, &subscription.enabled); err != nil {
			return err
		}
		legacy = append(legacy, subscription)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, subscription := range legacy {
		var semanticEnabled int
		err := tx.Get(&semanticEnabled, r.db.Rebind(`SELECT enabled FROM notification_subscriptions WHERE provider_id = ? AND event_type = ?`), subscription.providerID, "session.clarification_requested")
		if errors.Is(err, sql.ErrNoRows) {
			if _, err := tx.Exec(r.db.Rebind(`UPDATE notification_subscriptions SET event_type = ? WHERE id = ?`), "session.clarification_requested", subscription.id); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if _, err := tx.Exec(r.db.Rebind(`UPDATE notification_subscriptions SET enabled = ? WHERE provider_id = ? AND event_type = ?`), dialect.BoolToInt(subscription.enabled == 1 || semanticEnabled == 1), subscription.providerID, "session.clarification_requested"); err != nil {
			return err
		}
		if _, err := tx.Exec(r.db.Rebind(`DELETE FROM notification_subscriptions WHERE id = ?`), subscription.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *sqliteRepository) CreateProvider(ctx context.Context, provider *models.Provider) error {
	provider.ID = uuid.New().String()
	now := time.Now().UTC()
	provider.CreatedAt = now
	provider.UpdatedAt = now
	if provider.Config == nil {
		provider.Config = map[string]interface{}{}
	}
	configJSON, err := json.Marshal(provider.Config)
	if err != nil {
		return fmt.Errorf("failed to serialize provider config: %w", err)
	}
	_, err = r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO notification_providers (id, user_id, name, type, config, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`), provider.ID, provider.UserID, provider.Name, provider.Type, string(configJSON), dialect.BoolToInt(provider.Enabled), provider.CreatedAt, provider.UpdatedAt)
	return err
}

// UpdateProvider writes the provider back, scoped to provider.UserID. A row
// owned by someone else is reported as ErrProviderNotFound and left untouched.
func (r *sqliteRepository) UpdateProvider(ctx context.Context, provider *models.Provider) error {
	provider.UpdatedAt = time.Now().UTC()
	if provider.Config == nil {
		provider.Config = map[string]interface{}{}
	}
	configJSON, err := json.Marshal(provider.Config)
	if err != nil {
		return fmt.Errorf("failed to serialize provider config: %w", err)
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE notification_providers
		SET name = ?, type = ?, config = ?, enabled = ?, updated_at = ?
		WHERE id = ? AND user_id = ?
	`), provider.Name, provider.Type, string(configJSON), dialect.BoolToInt(provider.Enabled), provider.UpdatedAt, provider.ID, provider.UserID)
	if err != nil {
		return err
	}
	return errIfNoRows(result)
}

// GetProvider reads one provider owned by userID. A provider ID belonging to
// another user is reported exactly like an ID that does not exist, so probing
// IDs reveals nothing about other users' configuration.
func (r *sqliteRepository) GetProvider(ctx context.Context, userID, id string) (*models.Provider, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT id, user_id, name, type, config, enabled, created_at, updated_at
		FROM notification_providers
		WHERE id = ? AND user_id = ?
	`), id, userID)
	provider, err := scanProvider(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrProviderNotFound
	}
	return provider, err
}

func (r *sqliteRepository) ListProvidersByUser(ctx context.Context, userID string) ([]*models.Provider, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT id, user_id, name, type, config, enabled, created_at, updated_at
		FROM notification_providers
		WHERE user_id = ?
		ORDER BY created_at ASC
	`), userID)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var providers []*models.Provider
	for rows.Next() {
		provider, err := scanProvider(rows)
		if err != nil {
			return nil, err
		}
		providers = append(providers, provider)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return providers, nil
}

// DeleteProvider removes one provider owned by userID, reporting a foreign or
// nonexistent ID identically as ErrProviderNotFound.
// ListProviderUserIDs returns every distinct user that owns at least one
// provider. Instance-wide events (a new release) have no owning workspace to
// resolve a recipient from, so they fan out across these owners instead of
// landing on one hardcoded user.
func (r *sqliteRepository) ListProviderUserIDs(ctx context.Context) ([]string, error) {
	rows, err := r.ro.QueryContext(ctx, `
		SELECT DISTINCT user_id FROM notification_providers ORDER BY user_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var userIDs []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		userIDs = append(userIDs, userID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return userIDs, nil
}

func (r *sqliteRepository) DeleteProvider(ctx context.Context, userID, id string) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM notification_providers WHERE id = ? AND user_id = ?
	`), id, userID)
	if err != nil {
		return err
	}
	return errIfNoRows(result)
}

// errIfNoRows turns a write that matched nothing into ErrProviderNotFound, so
// an unauthorized target and a missing one are indistinguishable to the caller.
func errIfNoRows(result sql.Result) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrProviderNotFound
	}
	return nil
}

func (r *sqliteRepository) ListSubscriptionsByProvider(ctx context.Context, providerID string) ([]*models.Subscription, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT id, user_id, provider_id, event_type, enabled, created_at, updated_at
		FROM notification_subscriptions
		WHERE provider_id = ?
		ORDER BY created_at ASC
	`), providerID)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = rows.Close()
	}()

	var subs []*models.Subscription
	for rows.Next() {
		sub, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		subs = append(subs, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return subs, nil
}

func (r *sqliteRepository) ReplaceSubscriptions(ctx context.Context, providerID, userID string, events []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM notification_subscriptions WHERE provider_id = ?`), providerID); err != nil {
		_ = tx.Rollback()
		return err
	}
	now := time.Now().UTC()
	for _, eventType := range events {
		subID := uuid.New().String()
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`
			INSERT INTO notification_subscriptions (id, user_id, provider_id, event_type, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`), subID, userID, providerID, eventType, dialect.BoolToInt(true), now, now); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (r *sqliteRepository) InsertDelivery(ctx context.Context, delivery *models.Delivery) (bool, error) {
	if delivery.OccurrenceID == "" {
		return false, errors.New("notification delivery occurrence ID is required")
	}
	delivery.ID = uuid.New().String()
	delivery.CreatedAt = time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO notification_deliveries (id, user_id, provider_id, event_type, task_session_id, occurrence_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT DO NOTHING
	`), delivery.ID, delivery.UserID, delivery.ProviderID, delivery.EventType, delivery.TaskSessionID, delivery.OccurrenceID, delivery.CreatedAt)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (r *sqliteRepository) DeleteDelivery(ctx context.Context, providerID, eventType, occurrenceID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM notification_deliveries
		WHERE provider_id = ? AND event_type = ? AND occurrence_id = ?
	`), providerID, eventType, occurrenceID)
	return err
}

func scanProvider(scanner interface{ Scan(dest ...any) error }) (*models.Provider, error) {
	provider := &models.Provider{}
	var configJSON string
	var enabledInt int
	if err := scanner.Scan(
		&provider.ID,
		&provider.UserID,
		&provider.Name,
		&provider.Type,
		&configJSON,
		&enabledInt,
		&provider.CreatedAt,
		&provider.UpdatedAt,
	); err != nil {
		return nil, err
	}
	provider.Enabled = enabledInt == 1
	if configJSON != "" && configJSON != "{}" {
		if err := json.Unmarshal([]byte(configJSON), &provider.Config); err != nil {
			return nil, fmt.Errorf("failed to deserialize provider config: %w", err)
		}
	} else {
		provider.Config = map[string]interface{}{}
	}
	return provider, nil
}

func scanSubscription(scanner interface{ Scan(dest ...any) error }) (*models.Subscription, error) {
	sub := &models.Subscription{}
	var enabledInt int
	if err := scanner.Scan(
		&sub.ID,
		&sub.UserID,
		&sub.ProviderID,
		&sub.EventType,
		&enabledInt,
		&sub.CreatedAt,
		&sub.UpdatedAt,
	); err != nil {
		return nil, err
	}
	sub.Enabled = enabledInt == 1
	return sub, nil
}
