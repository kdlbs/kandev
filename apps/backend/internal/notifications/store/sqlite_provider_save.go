package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/notifications/models"
)

type providerWriter interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (r *sqliteRepository) CreateProviderWithSubscriptions(ctx context.Context, provider *models.Provider, events []string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.createProvider(ctx, tx, provider); err != nil {
		return err
	}
	if err := r.replaceSubscriptions(ctx, tx, provider.ID, provider.UserID, events); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *sqliteRepository) UpdateProviderWithSubscriptions(ctx context.Context, provider *models.Provider, events *[]string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.updateProvider(ctx, tx, provider); err != nil {
		return err
	}
	if events != nil {
		if err := r.replaceSubscriptions(ctx, tx, provider.ID, provider.UserID, *events); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *sqliteRepository) createProvider(ctx context.Context, writer providerWriter, provider *models.Provider) error {
	if provider.ID == "" {
		provider.ID = uuid.New().String()
	}
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
	_, err = writer.ExecContext(ctx, r.db.Rebind(`
  INSERT INTO notification_providers (id, user_id, name, type, config, enabled, created_at, updated_at)
  VALUES (?, ?, ?, ?, ?, ?, ?, ?)
 `), provider.ID, provider.UserID, provider.Name, provider.Type, string(configJSON), dialect.BoolToInt(provider.Enabled), provider.CreatedAt, provider.UpdatedAt)
	return err
}

func (r *sqliteRepository) updateProvider(ctx context.Context, writer providerWriter, provider *models.Provider) error {
	provider.UpdatedAt = time.Now().UTC()
	if provider.Config == nil {
		provider.Config = map[string]interface{}{}
	}
	configJSON, err := json.Marshal(provider.Config)
	if err != nil {
		return fmt.Errorf("failed to serialize provider config: %w", err)
	}
	result, err := writer.ExecContext(ctx, r.db.Rebind(`
  UPDATE notification_providers
  SET name = ?, type = ?, config = ?, enabled = ?, updated_at = ?
  WHERE id = ? AND user_id = ?
 `), provider.Name, provider.Type, string(configJSON), dialect.BoolToInt(provider.Enabled), provider.UpdatedAt, provider.ID, provider.UserID)
	if err != nil {
		return err
	}
	return errIfNoRows(result)
}

func (r *sqliteRepository) replaceSubscriptions(ctx context.Context, writer providerWriter, providerID, userID string, events []string) error {
	if _, err := writer.ExecContext(ctx, r.db.Rebind(`DELETE FROM notification_subscriptions WHERE provider_id = ?`), providerID); err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, eventType := range events {
		if _, err := writer.ExecContext(ctx, r.db.Rebind(`
   INSERT INTO notification_subscriptions (id, user_id, provider_id, event_type, enabled, created_at, updated_at)
   VALUES (?, ?, ?, ?, ?, ?, ?)
  `), uuid.New().String(), userID, providerID, eventType, dialect.BoolToInt(true), now, now); err != nil {
			return err
		}
	}
	return nil
}
