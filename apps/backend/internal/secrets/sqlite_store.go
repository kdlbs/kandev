package secrets

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type sqliteStore struct {
	db     *sqlx.DB // writer
	ro     *sqlx.DB // reader
	crypto *MasterKeyProvider
	ownsDB bool
}

var _ SecretStore = (*sqliteStore)(nil)

// Provide creates the SQLite secret store using separate writer and reader pools.
func Provide(writer, reader *sqlx.DB, crypto *MasterKeyProvider) (*sqliteStore, func() error, error) {
	store := &sqliteStore{db: writer, ro: reader, crypto: crypto}
	if err := store.initSchema(); err != nil {
		return nil, nil, fmt.Errorf("secrets schema init: %w", err)
	}
	return store, store.Close, nil
}

func (s *sqliteStore) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS secrets (
		id              TEXT PRIMARY KEY,
		name            TEXT NOT NULL,
		env_key         TEXT NOT NULL UNIQUE,
		encrypted_value BLOB NOT NULL,
		nonce           BLOB NOT NULL,
		category        TEXT NOT NULL DEFAULT 'custom',
		metadata        TEXT DEFAULT '{}',
		created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_secrets_env_key ON secrets(env_key);
	CREATE INDEX IF NOT EXISTS idx_secrets_category ON secrets(category);
	`
	_, err := s.db.Exec(schema)
	return err
}

func (s *sqliteStore) Close() error {
	if !s.ownsDB {
		return nil
	}
	return s.db.Close()
}

func (s *sqliteStore) Create(ctx context.Context, secret *SecretWithValue) error {
	if secret.ID == "" {
		secret.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	secret.CreatedAt = now
	secret.UpdatedAt = now

	if secret.Category == "" {
		secret.Category = CategoryCustom
	}

	ciphertext, nonce, err := Encrypt([]byte(secret.Value), s.crypto.Key())
	if err != nil {
		return fmt.Errorf("encrypt secret: %w", err)
	}

	metadataJSON, err := json.Marshal(secret.Metadata)
	if err != nil {
		metadataJSON = []byte("{}")
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO secrets (id, name, env_key, encrypted_value, nonce, category, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		secret.ID, secret.Name, secret.EnvKey, ciphertext, nonce,
		string(secret.Category), string(metadataJSON), now, now,
	)
	if err != nil {
		return fmt.Errorf("insert secret: %w", err)
	}
	return nil
}

func (s *sqliteStore) Get(ctx context.Context, id string) (*Secret, error) {
	var row secretRow
	err := s.ro.GetContext(ctx, &row, `
		SELECT id, name, env_key, category, metadata, created_at, updated_at
		FROM secrets WHERE id = ?`, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("secret not found: %s", id)
		}
		return nil, fmt.Errorf("get secret: %w", err)
	}
	return row.toSecret(), nil
}

func (s *sqliteStore) GetByEnvKey(ctx context.Context, envKey string) (*Secret, error) {
	var row secretRow
	err := s.ro.GetContext(ctx, &row, `
		SELECT id, name, env_key, category, metadata, created_at, updated_at
		FROM secrets WHERE env_key = ?`, envKey)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("secret not found for env key: %s", envKey)
		}
		return nil, fmt.Errorf("get secret by env_key: %w", err)
	}
	return row.toSecret(), nil
}

func (s *sqliteStore) Reveal(ctx context.Context, id string) (string, error) {
	var ciphertext, nonce []byte
	err := s.ro.QueryRowContext(ctx, `
		SELECT encrypted_value, nonce FROM secrets WHERE id = ?`, id).
		Scan(&ciphertext, &nonce)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("secret not found: %s", id)
		}
		return "", fmt.Errorf("reveal secret: %w", err)
	}

	plaintext, err := Decrypt(ciphertext, nonce, s.crypto.Key())
	if err != nil {
		return "", fmt.Errorf("decrypt secret: %w", err)
	}
	return string(plaintext), nil
}

func (s *sqliteStore) RevealByEnvKey(ctx context.Context, envKey string) (string, error) {
	var ciphertext, nonce []byte
	err := s.ro.QueryRowContext(ctx, `
		SELECT encrypted_value, nonce FROM secrets WHERE env_key = ?`, envKey).
		Scan(&ciphertext, &nonce)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("secret not found for env key: %s", envKey)
		}
		return "", fmt.Errorf("reveal secret by env_key: %w", err)
	}

	plaintext, err := Decrypt(ciphertext, nonce, s.crypto.Key())
	if err != nil {
		return "", fmt.Errorf("decrypt secret: %w", err)
	}
	return string(plaintext), nil
}

func (s *sqliteStore) Update(ctx context.Context, id string, req *UpdateSecretRequest) error {
	existing, err := s.Get(ctx, id)
	if err != nil {
		return err
	}

	now := time.Now().UTC()

	if req.Name != nil {
		existing.Name = *req.Name
	}
	if req.Category != nil {
		existing.Category = *req.Category
	}
	if req.Metadata != nil {
		existing.Metadata = req.Metadata
	}

	metadataJSON, err := json.Marshal(existing.Metadata)
	if err != nil {
		metadataJSON = []byte("{}")
	}

	if req.Value != nil {
		ciphertext, nonce, err := Encrypt([]byte(*req.Value), s.crypto.Key())
		if err != nil {
			return fmt.Errorf("encrypt secret: %w", err)
		}
		_, err = s.db.ExecContext(ctx, `
			UPDATE secrets SET name = ?, category = ?, metadata = ?, encrypted_value = ?, nonce = ?, updated_at = ?
			WHERE id = ?`,
			existing.Name, string(existing.Category), string(metadataJSON), ciphertext, nonce, now, id,
		)
		if err != nil {
			return fmt.Errorf("update secret: %w", err)
		}
	} else {
		_, err = s.db.ExecContext(ctx, `
			UPDATE secrets SET name = ?, category = ?, metadata = ?, updated_at = ?
			WHERE id = ?`,
			existing.Name, string(existing.Category), string(metadataJSON), now, id,
		)
		if err != nil {
			return fmt.Errorf("update secret: %w", err)
		}
	}
	return nil
}

func (s *sqliteStore) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM secrets WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete secret: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("secret not found: %s", id)
	}
	return nil
}

func (s *sqliteStore) List(ctx context.Context) ([]*SecretListItem, error) {
	var rows []secretListRow
	err := s.ro.SelectContext(ctx, &rows, `
		SELECT id, name, env_key, category, metadata, 1 as has_value, created_at, updated_at
		FROM secrets ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	return toSecretListItems(rows), nil
}

func (s *sqliteStore) ListByCategory(ctx context.Context, category SecretCategory) ([]*SecretListItem, error) {
	var rows []secretListRow
	err := s.ro.SelectContext(ctx, &rows, `
		SELECT id, name, env_key, category, metadata, 1 as has_value, created_at, updated_at
		FROM secrets WHERE category = ? ORDER BY created_at DESC`, string(category))
	if err != nil {
		return nil, fmt.Errorf("list secrets by category: %w", err)
	}
	return toSecretListItems(rows), nil
}

// secretRow is the DB scan target for secret metadata queries.
type secretRow struct {
	ID        string    `db:"id"`
	Name      string    `db:"name"`
	EnvKey    string    `db:"env_key"`
	Category  string    `db:"category"`
	Metadata  string    `db:"metadata"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

func (r *secretRow) toSecret() *Secret {
	s := &Secret{
		ID:        r.ID,
		Name:      r.Name,
		EnvKey:    r.EnvKey,
		Category:  SecretCategory(r.Category),
		CreatedAt: r.CreatedAt,
		UpdatedAt: r.UpdatedAt,
	}
	if r.Metadata != "" {
		_ = json.Unmarshal([]byte(r.Metadata), &s.Metadata)
	}
	return s
}

// secretListRow is the DB scan target for list queries.
type secretListRow struct {
	ID        string    `db:"id"`
	Name      string    `db:"name"`
	EnvKey    string    `db:"env_key"`
	Category  string    `db:"category"`
	Metadata  string    `db:"metadata"`
	HasValue  bool      `db:"has_value"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

func toSecretListItems(rows []secretListRow) []*SecretListItem {
	items := make([]*SecretListItem, len(rows))
	for i, r := range rows {
		item := &SecretListItem{
			ID:        r.ID,
			Name:      r.Name,
			EnvKey:    r.EnvKey,
			Category:  SecretCategory(r.Category),
			HasValue:  r.HasValue,
			CreatedAt: r.CreatedAt,
			UpdatedAt: r.UpdatedAt,
		}
		if r.Metadata != "" {
			_ = json.Unmarshal([]byte(r.Metadata), &item.Metadata)
		}
		items[i] = item
	}
	return items
}
