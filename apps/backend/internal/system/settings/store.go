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

// Entry is a setting value together with the timestamp persisted by Save.
type Entry struct {
	Key       string
	Value     []byte
	UpdatedAt time.Time
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

func (s *Store) GetConsistent(ctx context.Context, key string) ([]byte, bool, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, s.db.Rebind(`SELECT value FROM settings WHERE key = ?`), key).Scan(&raw)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, err
	}
	return []byte(raw), true, nil
}

func (s *Store) CompareAndSwap(
	ctx context.Context,
	key string,
	expected, value []byte,
) (bool, error) {
	now := time.Now().UTC()
	var (
		result sql.Result
		err    error
	)
	if expected == nil {
		result, err = s.db.ExecContext(ctx, s.db.Rebind(`
			INSERT INTO settings (key, value, updated_at)
			VALUES (?, ?, ?)
			ON CONFLICT(key) DO NOTHING
		`), key, string(value), now)
	} else {
		result, err = s.db.ExecContext(ctx, s.db.Rebind(`
			UPDATE settings SET value = ?, updated_at = ?
			WHERE key = ? AND value = ?
		`), string(value), now, key, string(expected))
	}
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

// GetEntry reads one setting row, including its persisted update timestamp.
func (s *Store) GetEntry(ctx context.Context, key string) (Entry, bool, error) {
	var row struct {
		Key       string    `db:"key"`
		Value     string    `db:"value"`
		UpdatedAt time.Time `db:"updated_at"`
	}
	err := s.ro.GetContext(ctx, &row, s.ro.Rebind(`
		SELECT key, value, updated_at FROM settings WHERE key = ?
	`), key)
	if err == sql.ErrNoRows {
		return Entry{}, false, nil
	}
	if err != nil {
		return Entry{}, false, err
	}
	return Entry{Key: row.Key, Value: []byte(row.Value), UpdatedAt: row.UpdatedAt}, true, nil
}

func (s *Store) Save(ctx context.Context, key string, value []byte) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO settings (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`), key, string(value), time.Now().UTC())
	return err
}

func (s *Store) Delete(ctx context.Context, key string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`DELETE FROM settings WHERE key = ?`), key)
	return err
}
