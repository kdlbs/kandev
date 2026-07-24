package plugins

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

// settingsRowID is the fixed primary key of the single instance-wide plugin
// settings row, so upserts never duplicate it (the table is CHECK-constrained
// to this one id).
const settingsRowID = 1

// Settings holds instance-wide plugin preferences (currently just the
// auto-update default). It is a value type so callers can read it once and
// pass it around without touching the store again.
type Settings struct {
	// AutoUpdateDefault is the instance-wide default applied to every installed
	// plugin that has no per-plugin override (store.Record.AutoUpdate == nil).
	// Defaults to false — auto-update is strictly opt-in.
	AutoUpdateDefault bool `json:"auto_update_default"`
}

// settingsStore persists instance-wide plugin settings in the single-row
// plugin_settings table. It mirrors marketplace.SourceStore's self-initializing
// schema convention (the plugin subsystem's SQLite tables init in-package
// rather than through the central base_schema migrations).
type settingsStore struct {
	db *sqlx.DB
	ro *sqlx.DB
}

// newSettingsStore creates a settingsStore and initializes its schema.
func newSettingsStore(pool *db.Pool) (*settingsStore, error) {
	s := &settingsStore{db: pool.Writer(), ro: pool.Reader()}
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("plugin settings schema init: %w", err)
	}
	return s, nil
}

func (s *settingsStore) initSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS plugin_settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			auto_update_default INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL
		);
	`)
	return err
}

// Get returns the instance-wide settings. When no row has been written yet it
// returns the zero-value defaults (auto-update off) rather than an error, so
// callers never have to special-case a fresh install.
func (s *settingsStore) Get() (Settings, error) {
	var autoUpdate int
	err := s.ro.QueryRow(s.ro.Rebind(`
		SELECT auto_update_default FROM plugin_settings WHERE id = ?
	`), settingsRowID).Scan(&autoUpdate)
	if errors.Is(err, sql.ErrNoRows) {
		return Settings{}, nil
	}
	if err != nil {
		return Settings{}, err
	}
	return Settings{AutoUpdateDefault: autoUpdate != 0}, nil
}

// SetAutoUpdateDefault upserts the instance-wide auto-update default.
func (s *settingsStore) SetAutoUpdateDefault(enabled bool) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.Exec(s.db.Rebind(`
		INSERT INTO plugin_settings (id, auto_update_default, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET auto_update_default = excluded.auto_update_default, updated_at = excluded.updated_at
	`), settingsRowID, boolToInt(enabled), now)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
