package runtimeflags

import (
	"context"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
)

type SQLiteStore struct {
	writer *sqlx.DB
	reader *sqlx.DB
}

// Override is one persisted runtime flag override, including its database
// timestamps. It is useful to callers that need to distinguish an upserted
// row from a value reconstructed from the effective flag registry.
type Override struct {
	Key       string
	Value     bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

func NewSQLiteStore(writer, reader *sqlx.DB) (*SQLiteStore, error) {
	store := &SQLiteStore{writer: writer, reader: reader}
	if err := store.initSchema(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) initSchema() error {
	timestampType := "DATETIME"
	if dialect.IsPostgres(s.writer.DriverName()) {
		timestampType = "TIMESTAMPTZ"
	}
	_, err := s.writer.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS runtime_flag_overrides (
		key TEXT PRIMARY KEY,
		value INTEGER NOT NULL CHECK (value IN (0, 1)),
		created_at %s NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at %s NOT NULL DEFAULT CURRENT_TIMESTAMP
	)`, timestampType, timestampType))
	if err != nil {
		return fmt.Errorf("create runtime_flag_overrides: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListOverrides(ctx context.Context) (map[string]bool, error) {
	rows, err := s.reader.QueryxContext(ctx, `SELECT key, value FROM runtime_flag_overrides`)
	if err != nil {
		return nil, fmt.Errorf("list runtime flag overrides: %w", err)
	}
	defer func() { _ = rows.Close() }()

	out := map[string]bool{}
	for rows.Next() {
		var key string
		var value int
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("scan runtime flag override: %w", err)
		}
		out[key] = value != 0
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate runtime flag overrides: %w", err)
	}
	return out, nil
}

// GetOverride reads one override and its persisted timestamps.
func (s *SQLiteStore) GetOverride(ctx context.Context, key string) (Override, error) {
	var row struct {
		Key       string    `db:"key"`
		Value     int       `db:"value"`
		CreatedAt time.Time `db:"created_at"`
		UpdatedAt time.Time `db:"updated_at"`
	}
	if err := s.reader.GetContext(ctx, &row, s.reader.Rebind(`
		SELECT key, value, created_at, updated_at FROM runtime_flag_overrides WHERE key = ?
	`), key); err != nil {
		return Override{}, err
	}
	return Override{Key: row.Key, Value: row.Value != 0, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt}, nil
}

func (s *SQLiteStore) SetOverride(ctx context.Context, key string, value bool) error {
	now := time.Now().UTC()
	intValue := 0
	if value {
		intValue = 1
	}
	_, err := s.writer.ExecContext(ctx, s.writer.Rebind(`INSERT INTO runtime_flag_overrides (key, value, created_at, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`),
		key, intValue, now, now)
	if err != nil {
		return fmt.Errorf("set runtime flag override %q: %w", key, err)
	}
	return nil
}

func (s *SQLiteStore) DeleteOverride(ctx context.Context, key string) error {
	if _, err := s.writer.ExecContext(ctx, s.writer.Rebind(`DELETE FROM runtime_flag_overrides WHERE key = ?`), key); err != nil {
		return fmt.Errorf("delete runtime flag override %q: %w", key, err)
	}
	return nil
}
