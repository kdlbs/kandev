package metrics

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

const settingsKey = "system_metrics"

type Store struct {
	db *sqlx.DB
	ro *sqlx.DB
}

func NewStore(pool *db.Pool) (*Store, error) {
	store := &Store{db: pool.Writer(), ro: pool.Reader()}
	if err := store.initSchema(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) initSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS system_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
	`)
	return err
}

func (s *Store) GetSettings(ctx context.Context) (GlobalSettings, error) {
	var raw string
	err := s.ro.QueryRowContext(ctx, s.ro.Rebind(`SELECT value FROM system_settings WHERE key = ?`), settingsKey).Scan(&raw)
	if err != nil {
		if err == sql.ErrNoRows {
			return DefaultSettings(), nil
		}
		return GlobalSettings{}, err
	}
	var settings GlobalSettings
	if err := json.Unmarshal([]byte(raw), &settings); err != nil {
		return GlobalSettings{}, err
	}
	return NormalizeSettings(settings)
}

func (s *Store) SaveSettings(ctx context.Context, settings GlobalSettings) (GlobalSettings, error) {
	normalized, err := NormalizeSettings(settings)
	if err != nil {
		return GlobalSettings{}, err
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return GlobalSettings{}, err
	}
	if _, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO system_settings (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`), settingsKey, string(data), time.Now().UTC()); err != nil {
		return GlobalSettings{}, fmt.Errorf("save metrics settings: %w", err)
	}
	return normalized, nil
}
