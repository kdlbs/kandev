package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/notifications/models"
)

type SQLiteRepository struct {
	db *sql.DB
}

var _ Repository = (*SQLiteRepository)(nil)

func NewSQLiteRepository(dbPath string) (*SQLiteRepository, error) {
	normalizedPath := normalizeSQLitePath(dbPath)
	if err := ensureSQLiteDir(normalizedPath); err != nil {
		return nil, fmt.Errorf("failed to prepare database path: %w", err)
	}
	if err := ensureSQLiteFile(normalizedPath); err != nil {
		return nil, fmt.Errorf("failed to create database file: %w", err)
	}
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on&_mode=rwc", normalizedPath)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	repo := &SQLiteRepository{db: db}
	if err := repo.initSchema(); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			return nil, fmt.Errorf("failed to close database after schema error: %w", closeErr)
		}
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}
	return repo, nil
}

func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}

func ensureSQLiteDir(dbPath string) error {
	dir := filepath.Dir(dbPath)
	if dir == "." || dir == "" {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func ensureSQLiteFile(dbPath string) error {
	file, err := os.OpenFile(dbPath, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	return file.Close()
}

func normalizeSQLitePath(dbPath string) string {
	if dbPath == "" {
		return dbPath
	}
	abs, err := filepath.Abs(dbPath)
	if err != nil {
		return dbPath
	}
	return abs
}

func (r *SQLiteRepository) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS notification_providers (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		config TEXT DEFAULT '{}',
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE TABLE IF NOT EXISTS notification_subscriptions (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		provider_id TEXT NOT NULL,
		event_type TEXT NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL,
		UNIQUE(provider_id, event_type),
		FOREIGN KEY (provider_id) REFERENCES notification_providers(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS notification_deliveries (
		id TEXT PRIMARY KEY,
		user_id TEXT NOT NULL,
		provider_id TEXT NOT NULL,
		event_type TEXT NOT NULL,
		task_session_id TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		UNIQUE(provider_id, event_type, task_session_id)
	);

	CREATE INDEX IF NOT EXISTS idx_notification_providers_user_id ON notification_providers(user_id);
	CREATE INDEX IF NOT EXISTS idx_notification_subscriptions_provider_id ON notification_subscriptions(provider_id);
	CREATE INDEX IF NOT EXISTS idx_notification_subscriptions_user_id ON notification_subscriptions(user_id);
	CREATE INDEX IF NOT EXISTS idx_notification_deliveries_session_id ON notification_deliveries(task_session_id);
	`

	_, err := r.db.Exec(schema)
	return err
}

func (r *SQLiteRepository) CreateProvider(ctx context.Context, provider *models.Provider) error {
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
	_, err = r.db.ExecContext(ctx, `
		INSERT INTO notification_providers (id, user_id, name, type, config, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, provider.ID, provider.UserID, provider.Name, provider.Type, string(configJSON), boolToInt(provider.Enabled), provider.CreatedAt, provider.UpdatedAt)
	return err
}

func (r *SQLiteRepository) UpdateProvider(ctx context.Context, provider *models.Provider) error {
	provider.UpdatedAt = time.Now().UTC()
	if provider.Config == nil {
		provider.Config = map[string]interface{}{}
	}
	configJSON, err := json.Marshal(provider.Config)
	if err != nil {
		return fmt.Errorf("failed to serialize provider config: %w", err)
	}
	_, err = r.db.ExecContext(ctx, `
		UPDATE notification_providers
		SET name = ?, type = ?, config = ?, enabled = ?, updated_at = ?
		WHERE id = ?
	`, provider.Name, provider.Type, string(configJSON), boolToInt(provider.Enabled), provider.UpdatedAt, provider.ID)
	return err
}

func (r *SQLiteRepository) GetProvider(ctx context.Context, id string) (*models.Provider, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, user_id, name, type, config, enabled, created_at, updated_at
		FROM notification_providers
		WHERE id = ?
	`, id)
	return scanProvider(row)
}

func (r *SQLiteRepository) ListProvidersByUser(ctx context.Context, userID string) ([]*models.Provider, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, name, type, config, enabled, created_at, updated_at
		FROM notification_providers
		WHERE user_id = ?
		ORDER BY created_at ASC
	`, userID)
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

func (r *SQLiteRepository) DeleteProvider(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM notification_providers WHERE id = ?`, id)
	return err
}

func (r *SQLiteRepository) ListSubscriptionsByProvider(ctx context.Context, providerID string) ([]*models.Subscription, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, user_id, provider_id, event_type, enabled, created_at, updated_at
		FROM notification_subscriptions
		WHERE provider_id = ?
		ORDER BY created_at ASC
	`, providerID)
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

func (r *SQLiteRepository) ReplaceSubscriptions(ctx context.Context, providerID, userID string, events []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM notification_subscriptions WHERE provider_id = ?`, providerID); err != nil {
		_ = tx.Rollback()
		return err
	}
	now := time.Now().UTC()
	for _, eventType := range events {
		subID := uuid.New().String()
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO notification_subscriptions (id, user_id, provider_id, event_type, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`, subID, userID, providerID, eventType, boolToInt(true), now, now); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (r *SQLiteRepository) InsertDelivery(ctx context.Context, delivery *models.Delivery) (bool, error) {
	delivery.ID = uuid.New().String()
	delivery.CreatedAt = time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
		INSERT OR IGNORE INTO notification_deliveries (id, user_id, provider_id, event_type, task_session_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, delivery.ID, delivery.UserID, delivery.ProviderID, delivery.EventType, delivery.TaskSessionID, delivery.CreatedAt)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (r *SQLiteRepository) DeleteDelivery(ctx context.Context, providerID, eventType, taskSessionID string) error {
	_, err := r.db.ExecContext(ctx, `
		DELETE FROM notification_deliveries
		WHERE provider_id = ? AND event_type = ? AND task_session_id = ?
	`, providerID, eventType, taskSessionID)
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

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
