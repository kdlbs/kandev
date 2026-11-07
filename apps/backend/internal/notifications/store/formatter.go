package store

import (
	"context"
	"database/sql"
	"errors"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/notifications/models"
)

var ErrFormatterConflict = errors.New("notification formatter changed; reload before retrying")
var ErrFormatterInvalid = errors.New("invalid notification formatter binding")

// FormatterRepository is the notification-owned, transactional binding seam.
type FormatterRepository interface {
	GetFormatterBinding(context.Context, string, string) (*models.FormatterBinding, error)
	SetFormatterBinding(context.Context, string, string, string, *models.FormatterBinding) (*models.FormatterBinding, error)
}

const formatterSchema = `
CREATE TABLE IF NOT EXISTS notification_formatter_bindings (
 provider_id TEXT PRIMARY KEY REFERENCES notification_providers(id) ON DELETE CASCADE,
 provider_type TEXT NOT NULL, plugin_id TEXT NOT NULL, installation_id TEXT NOT NULL,
 formatter_key TEXT NOT NULL, configuration_revision TEXT NOT NULL, digest TEXT NOT NULL,
 revision TEXT NOT NULL
);`

func (r *sqliteRepository) GetFormatterBinding(ctx context.Context, owner, providerID string) (*models.FormatterBinding, error) {
	if _, err := r.GetProvider(ctx, owner, providerID); err != nil {
		return nil, err
	}
	var result models.FormatterBinding
	err := r.ro.GetContext(ctx, &result, r.ro.Rebind(`SELECT b.* FROM notification_formatter_bindings b
JOIN notification_providers p ON p.id = b.provider_id AND p.type = b.provider_type
WHERE p.id = ? AND p.user_id = ?`), providerID, owner)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &result, err
}

func (r *sqliteRepository) SetFormatterBinding(ctx context.Context, owner, providerID, expected string, next *models.FormatterBinding) (*models.FormatterBinding, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	query := `SELECT type FROM notification_providers WHERE id = ? AND user_id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += " FOR UPDATE"
	}
	var kind models.ProviderType
	if err := tx.GetContext(ctx, &kind, r.db.Rebind(query), providerID, owner); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrProviderNotFound
		}
		return nil, err
	}
	var current string
	err = tx.GetContext(ctx, &current, r.db.Rebind(`SELECT revision FROM notification_formatter_bindings WHERE provider_id = ?`), providerID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if current != expected {
		return nil, ErrFormatterConflict
	}
	result := models.FormatterBinding{ProviderID: providerID, ProviderType: kind}
	if next != nil {
		if !validFormatterBinding(next, kind) {
			return nil, ErrFormatterInvalid
		}
		result = *next
		result.ProviderID = providerID
	}
	result.Revision = uuid.NewString()
	_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO notification_formatter_bindings
(provider_id, provider_type, plugin_id, installation_id, formatter_key, configuration_revision, digest, revision)
VALUES (?, ?, ?, ?, ?, ?, ?, ?) ON CONFLICT(provider_id) DO UPDATE SET
provider_type = excluded.provider_type, plugin_id = excluded.plugin_id, installation_id = excluded.installation_id,
formatter_key = excluded.formatter_key, configuration_revision = excluded.configuration_revision,
digest = excluded.digest, revision = excluded.revision`), result.ProviderID, result.ProviderType, result.PluginID,
		result.InstallationID, result.FormatterKey, result.ConfigurationRevision, result.Digest, result.Revision)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &result, nil
}

func validFormatterBinding(binding *models.FormatterBinding, kind models.ProviderType) bool {
	return binding.ProviderType == kind && binding.PluginID != "" && binding.InstallationID != "" && binding.FormatterKey != "" && binding.ConfigurationRevision != "" && binding.Digest != ""
}
