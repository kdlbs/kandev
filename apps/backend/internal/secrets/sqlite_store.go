package secrets

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
)

type sqliteStore struct {
	db     *sqlx.DB // writer
	ro     *sqlx.DB // reader
	crypto *MasterKeyProvider
	ownsDB bool
}

var _ ScopedSecretStore = (*sqliteStore)(nil)

// Provide creates the SQLite secret store using separate writer and reader pools.
func Provide(writer, reader *sqlx.DB, crypto *MasterKeyProvider) (*sqliteStore, func() error, error) {
	store := &sqliteStore{db: writer, ro: reader, crypto: crypto}
	if err := store.initSchema(); err != nil {
		return nil, nil, fmt.Errorf("secrets schema init: %w", err)
	}
	return store, store.Close, nil
}

func (s *sqliteStore) initSchema() error {
	binaryType := dialect.BlobType(s.db.DriverName())
	schema := fmt.Sprintf(`
	CREATE TABLE IF NOT EXISTS secrets (
		id              TEXT PRIMARY KEY,
		name            TEXT NOT NULL,
		user_id         TEXT NOT NULL DEFAULT '',
		scope           TEXT NOT NULL DEFAULT 'global',
		workspace_id    TEXT NOT NULL DEFAULT '',
		encrypted_value %s NOT NULL,
		nonce           %s NOT NULL,
		created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	`, binaryType, binaryType)
	if _, err := s.db.Exec(schema); err != nil {
		return err
	}
	// Existing databases: CREATE TABLE IF NOT EXISTS is a no-op, so the
	// column must also be added via an idempotent migration (ADR 0027).
	migrate := db.NewMigrateLogger(s.db, nil)
	migrate.Apply("secrets.user_id", "ALTER TABLE secrets ADD COLUMN user_id TEXT NOT NULL DEFAULT ''")
	migrate.Apply("secrets.scope", "ALTER TABLE secrets ADD COLUMN scope TEXT NOT NULL DEFAULT 'global'")
	migrate.Apply("secrets.workspace_id", "ALTER TABLE secrets ADD COLUMN workspace_id TEXT NOT NULL DEFAULT ''")
	return nil
}

// scopeOwner returns the per-user scoping ID for a request context.
// Empty = unscoped (internal caller or auth disabled), matching the
// task-service scoping semantics. Rows with user_id=” (created pre-auth or
// by internal flows) stay visible to every caller until the setup wizard
// claims them for the admin.
func scopeOwner(ctx context.Context) string {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || identity.Synthetic {
		return ""
	}
	return identity.UserID
}

// ClaimUnowned assigns every unowned secret to ownerID. Called by the auth
// setup wizard; idempotent.
func (s *sqliteStore) ClaimUnowned(ctx context.Context, ownerID string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE secrets SET user_id = ? WHERE user_id = ''`), ownerID)
	return err
}

func (s *sqliteStore) Close() error {
	if !s.ownsDB {
		return nil
	}
	return s.db.Close()
}

func (s *sqliteStore) Create(ctx context.Context, secret *SecretWithValue) error {
	if secret == nil {
		return fmt.Errorf("secret is required")
	}
	if secret.Scope == "" {
		secret.Scope = ScopeGlobal
	}
	if err := validateSecretScope(secret.Scope, secret.WorkspaceID); err != nil {
		return err
	}
	if secret.ID == "" {
		secret.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	secret.CreatedAt = now
	secret.UpdatedAt = now

	ciphertext, nonce, err := Encrypt([]byte(secret.Value), s.crypto.Key())
	if err != nil {
		return fmt.Errorf("encrypt secret: %w", err)
	}

	_, err = s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO secrets (id, name, user_id, scope, workspace_id, encrypted_value, nonce, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		secret.ID, secret.Name, scopeOwner(ctx), secret.Scope, secret.WorkspaceID, ciphertext, nonce, now, now,
	)
	if err != nil {
		return fmt.Errorf("insert secret: %w", err)
	}
	return nil
}

func (s *sqliteStore) Get(ctx context.Context, id string) (*Secret, error) {
	var row secretRow
	err := s.ro.GetContext(ctx, &row, s.ro.Rebind(`
		SELECT id, name, scope, workspace_id, created_at, updated_at
		FROM secrets WHERE id = ? AND (user_id = '' OR ? = '' OR user_id = ?)`),
		id, scopeOwner(ctx), scopeOwner(ctx))
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("get secret: %w", err)
	}
	return row.toSecret(), nil
}

func (s *sqliteStore) Reveal(ctx context.Context, id string) (string, error) {
	var ciphertext, nonce []byte
	err := s.ro.QueryRowContext(ctx, s.ro.Rebind(`
		SELECT encrypted_value, nonce FROM secrets
		WHERE id = ? AND (user_id = '' OR ? = '' OR user_id = ?)`),
		id, scopeOwner(ctx), scopeOwner(ctx)).
		Scan(&ciphertext, &nonce)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("%w: %s", ErrNotFound, id)
		}
		return "", fmt.Errorf("reveal secret: %w", err)
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

	if req.Value != nil {
		ciphertext, nonce, err := Encrypt([]byte(*req.Value), s.crypto.Key())
		if err != nil {
			return fmt.Errorf("encrypt secret: %w", err)
		}
		_, err = s.db.ExecContext(ctx, s.db.Rebind(`
			UPDATE secrets SET name = ?, encrypted_value = ?, nonce = ?, updated_at = ?
			WHERE id = ? AND (user_id = '' OR ? = '' OR user_id = ?)`),
			existing.Name, ciphertext, nonce, now, id, scopeOwner(ctx), scopeOwner(ctx),
		)
		if err != nil {
			return fmt.Errorf("update secret: %w", err)
		}
	} else {
		_, err = s.db.ExecContext(ctx, s.db.Rebind(`
			UPDATE secrets SET name = ?, updated_at = ?
			WHERE id = ? AND (user_id = '' OR ? = '' OR user_id = ?)`),
			existing.Name, now, id, scopeOwner(ctx), scopeOwner(ctx),
		)
		if err != nil {
			return fmt.Errorf("update secret: %w", err)
		}
	}
	return nil
}

func (s *sqliteStore) Delete(ctx context.Context, id string) error {
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`
		DELETE FROM secrets WHERE id = ? AND (user_id = '' OR ? = '' OR user_id = ?)`),
		id, scopeOwner(ctx), scopeOwner(ctx))
	if err != nil {
		return fmt.Errorf("delete secret: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return nil
}

func (s *sqliteStore) List(ctx context.Context) ([]*SecretListItem, error) {
	return s.ListScoped(ctx, SecretListOptions{Scope: ScopeGlobal})
}

func (s *sqliteStore) ListScoped(ctx context.Context, opts SecretListOptions) ([]*SecretListItem, error) {
	if err := validateListOptions(opts); err != nil {
		return nil, err
	}
	var rows []secretListRow
	query := `
		SELECT id, name, scope, workspace_id, 1 as has_value, created_at, updated_at
		FROM secrets WHERE (user_id = '' OR ? = '' OR user_id = ?)`
	args := []any{scopeOwner(ctx), scopeOwner(ctx)}
	switch opts.Scope {
	case ScopeGlobal:
		query += " AND scope = ?"
		args = append(args, ScopeGlobal)
	case ScopeWorkspace:
		query += " AND ((scope = ? AND workspace_id = ?)"
		args = append(args, ScopeWorkspace, opts.WorkspaceID)
		if opts.IncludeGlobal {
			query += " OR scope = ?"
			args = append(args, ScopeGlobal)
		}
		query += ")"
	}
	query += " ORDER BY created_at DESC"
	err := s.ro.SelectContext(ctx, &rows, s.ro.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	return toSecretListItems(rows), nil
}

func (s *sqliteStore) GetForWorkspace(ctx context.Context, id, workspaceID string) (*Secret, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return nil, fmt.Errorf("workspace id is required")
	}
	secret, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if secret.Scope != ScopeGlobal && (secret.Scope != ScopeWorkspace || secret.WorkspaceID != workspaceID) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return secret, nil
}

func (s *sqliteStore) RevealGlobal(ctx context.Context, id string) (string, error) {
	secret, err := s.Get(ctx, id)
	if err != nil {
		return "", err
	}
	if secret.Scope != ScopeGlobal {
		return "", fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return s.Reveal(ctx, id)
}

func (s *sqliteStore) RevealForWorkspace(ctx context.Context, id, workspaceID string) (string, error) {
	if _, err := s.GetForWorkspace(ctx, id, workspaceID); err != nil {
		return "", err
	}
	return s.Reveal(ctx, id)
}

func (s *sqliteStore) DeleteWorkspaceSecrets(ctx context.Context, workspaceID string) error {
	if strings.TrimSpace(workspaceID) == "" {
		return fmt.Errorf("workspace id is required")
	}
	return s.deleteWorkspaceSecrets(ctx, s.db, workspaceID)
}

// DeleteWorkspaceSecretsTx removes workspace-owned secrets on the supplied
// transaction so workspace and secret deletion can commit or roll back as one.
func (s *sqliteStore) DeleteWorkspaceSecretsTx(ctx context.Context, tx *sqlx.Tx, workspaceID string) error {
	if tx == nil {
		return fmt.Errorf("transaction is required")
	}
	return s.deleteWorkspaceSecrets(ctx, tx, workspaceID)
}

func (s *sqliteStore) deleteWorkspaceSecrets(ctx context.Context, exec sqlx.ExtContext, workspaceID string) error {
	if strings.TrimSpace(workspaceID) == "" {
		return fmt.Errorf("workspace id is required")
	}
	_, err := exec.ExecContext(ctx, s.db.Rebind(`
		DELETE FROM secrets WHERE scope = ? AND workspace_id = ?`), ScopeWorkspace, workspaceID)
	if err != nil {
		return fmt.Errorf("delete workspace secrets: %w", err)
	}
	return nil
}

func validateSecretScope(scope SecretScope, workspaceID string) error {
	switch scope {
	case ScopeGlobal:
		if strings.TrimSpace(workspaceID) != "" {
			return fmt.Errorf("global secrets cannot have a workspace")
		}
	case ScopeWorkspace:
		if strings.TrimSpace(workspaceID) == "" {
			return fmt.Errorf("workspace secrets require a workspace")
		}
	default:
		return fmt.Errorf("invalid secret scope")
	}
	return nil
}

func validateListOptions(opts SecretListOptions) error {
	switch opts.Scope {
	case ScopeGlobal:
		if strings.TrimSpace(opts.WorkspaceID) != "" {
			return fmt.Errorf("global secret listing cannot have a workspace")
		}
	case ScopeWorkspace:
		if strings.TrimSpace(opts.WorkspaceID) == "" {
			return fmt.Errorf("workspace secret listing requires a workspace")
		}
	default:
		return fmt.Errorf("invalid secret scope")
	}
	return nil
}

// secretRow is the DB scan target for secret metadata queries.
type secretRow struct {
	ID          string      `db:"id"`
	Name        string      `db:"name"`
	Scope       SecretScope `db:"scope"`
	WorkspaceID string      `db:"workspace_id"`
	CreatedAt   time.Time   `db:"created_at"`
	UpdatedAt   time.Time   `db:"updated_at"`
}

func (r *secretRow) toSecret() *Secret {
	return &Secret{
		ID:          r.ID,
		Name:        r.Name,
		Scope:       normalizeStoredScope(r.Scope),
		WorkspaceID: r.WorkspaceID,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}

// secretListRow is the DB scan target for list queries.
type secretListRow struct {
	ID          string      `db:"id"`
	Name        string      `db:"name"`
	Scope       SecretScope `db:"scope"`
	WorkspaceID string      `db:"workspace_id"`
	HasValue    bool        `db:"has_value"`
	CreatedAt   time.Time   `db:"created_at"`
	UpdatedAt   time.Time   `db:"updated_at"`
}

func toSecretListItems(rows []secretListRow) []*SecretListItem {
	items := make([]*SecretListItem, len(rows))
	for i, r := range rows {
		items[i] = &SecretListItem{
			ID:          r.ID,
			Name:        r.Name,
			Scope:       normalizeStoredScope(r.Scope),
			WorkspaceID: r.WorkspaceID,
			HasValue:    r.HasValue,
			CreatedAt:   r.CreatedAt,
			UpdatedAt:   r.UpdatedAt,
		}
	}
	return items
}

func normalizeStoredScope(scope SecretScope) SecretScope {
	if scope == "" {
		return ScopeGlobal
	}
	return scope
}
