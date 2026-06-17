package settings

import (
	"context"
	"database/sql"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
)

// Store persists install-wide Kandev settings that are not scoped to a user,
// workspace, agent profile, or integration.
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
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at TIMESTAMP NOT NULL
		);
	`); err != nil {
		return err
	}
	return s.migrateLegacySystemSettings()
}

func (s *Store) migrateLegacySystemSettings() error {
	if dialect.IsPostgres(s.db.DriverName()) {
		return nil
	}

	var exists int
	if err := s.db.QueryRow(`
		SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'system_settings'
	`).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return nil
	}
	_, err := s.db.Exec(`
		INSERT OR IGNORE INTO settings (key, value, updated_at)
		SELECT key, value, updated_at FROM system_settings
	`)
	return err
}

func (s *Store) Get(ctx context.Context, key string) ([]byte, bool, error) {
	var raw string
	err := s.ro.QueryRowContext(ctx, s.ro.Rebind(`SELECT value FROM settings WHERE key = ?`), key).Scan(&raw)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}
	return []byte(raw), true, nil
}

func (s *Store) Save(ctx context.Context, key string, value []byte) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO settings (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`), key, string(value), time.Now().UTC())
	return err
}
