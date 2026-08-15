package secrets

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
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

	// failAfterInsert is a test-only hook. When set, it is invoked after the
	// transfer inserts the target row and before the Move source delete runs,
	// proving Move is transactional (the inserted copy must roll back).
	failAfterInsert func() error

	// afterSourceLock is a test-only hook invoked right after the source row
	// is selected with FOR UPDATE (Move on PostgreSQL), giving tests a
	// deterministic point where the source row lock is known to be held.
	afterSourceLock func() error

	// beforeTransferInsert is a test-only hook invoked after the conflict
	// check and before the conditional insert, letting tests commit a
	// competing row at the exact point where the atomic NOT EXISTS guard must
	// observe it (PostgreSQL: ordinary creates do not take the transfer lock).
	beforeTransferInsert func() error
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

// initSchema creates the secrets table if needed and applies idempotent column migrations.
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

// Close closes the underlying writer connection when the store owns it.
func (s *sqliteStore) Close() error {
	if !s.ownsDB {
		return nil
	}
	return s.db.Close()
}

// Create validates the scope, encrypts the value, and inserts a new secret.
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

// Get returns a secret's metadata by id, scoped to the caller; ErrNotFound if absent.
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

// Reveal returns the decrypted plaintext value of the secret with the given id.
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

// Update applies the requested name and/or value changes to the secret with the given id.
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

// Delete removes the secret with the given id, scoped to the caller; ErrNotFound if absent.
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

// List returns all global secrets visible to the caller.
func (s *sqliteStore) List(ctx context.Context) ([]*SecretListItem, error) {
	return s.ListScoped(ctx, SecretListOptions{Scope: ScopeGlobal})
}

// ListScoped returns secrets matching the given scope options, newest first.
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

// GetForWorkspace returns the secret with id if it is global or belongs to the given workspace.
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

// RevealGlobal returns the decrypted value of the global secret with the given id.
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

// RevealForWorkspace returns the decrypted value of a secret visible in the given workspace.
func (s *sqliteStore) RevealForWorkspace(ctx context.Context, id, workspaceID string) (string, error) {
	if _, err := s.GetForWorkspace(ctx, id, workspaceID); err != nil {
		return "", err
	}
	return s.Reveal(ctx, id)
}

// DeleteWorkspaceSecrets removes all workspace-owned secrets for the given workspace in a transaction.
func (s *sqliteStore) DeleteWorkspaceSecrets(ctx context.Context, workspaceID string) error {
	if strings.TrimSpace(workspaceID) == "" {
		return fmt.Errorf("workspace id is required")
	}
	// Run under a transaction so the PostgreSQL advisory lock is held to
	// commit, keeping concurrent secret transfers from inserting into a
	// workspace that is being deleted.
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin workspace secret cleanup: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := s.DeleteWorkspaceSecretsTx(ctx, tx, workspaceID); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteWorkspaceSecretsTx removes workspace-owned secrets on the supplied
// transaction so workspace and secret deletion can commit or roll back as one.
// On PostgreSQL it first acquires the workspace's advisory lock (shared with
// secret transfers), guaranteeing a concurrent transfer cannot insert after
// this cleanup has run.
func (s *sqliteStore) DeleteWorkspaceSecretsTx(ctx context.Context, tx *sqlx.Tx, workspaceID string) error {
	if tx == nil {
		return fmt.Errorf("transaction is required")
	}
	if dialect.IsPostgres(s.db.DriverName()) {
		if _, err := tx.ExecContext(ctx, s.db.Rebind("SELECT pg_advisory_xact_lock(?)"), WorkspaceLockKey(workspaceID)); err != nil {
			return fmt.Errorf("acquire workspace lock: %w", err)
		}
	}
	return s.deleteWorkspaceSecrets(ctx, tx, workspaceID)
}

// deleteWorkspaceSecrets runs the workspace-owned secret deletion on the supplied executor.
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

// validateSecretScope rejects scope/workspace combinations that are not allowed.
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

// validateListOptions rejects list options with an invalid scope or mismatched workspace.
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

// toSecret converts the scanned row into a Secret with its stored scope normalized.
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

// toSecretListItems converts scanned list rows into SecretListItems with normalized scopes.
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

// normalizeStoredScope maps a legacy empty stored scope to Global.
func normalizeStoredScope(scope SecretScope) SecretScope {
	if scope == "" {
		return ScopeGlobal
	}
	return scope
}

// transferExec is the minimal transaction-bound executor used by
// transferBody. Both *sql.Conn (SQLite BEGIN IMMEDIATE path) and *sqlx.Tx
// (PostgreSQL advisory-lock path) satisfy it, so every transfer statement
// runs on the same transaction and never on the store's reader handle.
type transferExec interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// CopyScoped copies a secret into the target scope under a new name and returns the copy.
func (s *sqliteStore) CopyScoped(ctx context.Context, sourceID, sourceWorkspaceID string, targetScope SecretScope, targetWorkspaceID string, requestedName *string, verifyDestination func(context.Context) error) (*Secret, error) {
	return s.transferScoped(ctx, sourceID, sourceWorkspaceID, targetScope, targetWorkspaceID, requestedName, verifyDestination, false)
}

// MoveScoped moves a secret into the target scope under a new name, deleting the source.
func (s *sqliteStore) MoveScoped(ctx context.Context, sourceID, sourceWorkspaceID string, targetScope SecretScope, targetWorkspaceID string, requestedName *string, verifyDestination func(context.Context) error) (*Secret, error) {
	return s.transferScoped(ctx, sourceID, sourceWorkspaceID, targetScope, targetWorkspaceID, requestedName, verifyDestination, true)
}

// transferScoped runs the atomic copy/move transfer: conflict check, insert, and optional source delete inside one transaction.
func (s *sqliteStore) transferScoped(ctx context.Context, sourceID, sourceWorkspaceID string, targetScope SecretScope, targetWorkspaceID string, requestedName *string, verifyDestination func(context.Context) error, deleteSource bool) (*Secret, error) {
	if err := validateSecretScope(targetScope, targetWorkspaceID); err != nil {
		return nil, fmt.Errorf("scope validation: %w", err)
	}
	if requestedName != nil && strings.TrimSpace(*requestedName) == "" {
		return nil, fmt.Errorf("target name is required")
	}
	if dialect.IsPostgres(s.db.DriverName()) {
		return s.transferPostgres(ctx, sourceID, sourceWorkspaceID, targetScope, targetWorkspaceID, requestedName, verifyDestination, deleteSource)
	}
	return s.transferSQLite(ctx, sourceID, targetScope, targetWorkspaceID, requestedName, verifyDestination, deleteSource)
}

// transferSQLite runs the transfer on the single writer connection with
// BEGIN IMMEDIATE, taking the SQLite write lock up front so competing
// transfers serialize before any read.
func (s *sqliteStore) transferSQLite(ctx context.Context, sourceID string, targetScope SecretScope, targetWorkspaceID string, requestedName *string, verifyDestination func(context.Context) error, deleteSource bool) (*Secret, error) {
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire writer connection: %w", err)
	}
	defer func() { _ = conn.Close() }()

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return nil, fmt.Errorf("begin immediate: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			// The rollback must reach SQLite even when the caller canceled ctx:
			// a canceled ROLLBACK would leave the BEGIN IMMEDIATE transaction
			// and its write lock open on the pooled connection.
			_, _ = conn.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
		}
	}()

	// SQLite's single writer serializes all writers (BEGIN IMMEDIATE), so the
	// source row cannot change under us; no row lock is needed.
	secret, err := s.transferBody(ctx, conn, func(q string) string { return q }, sourceID, targetScope, targetWorkspaceID, requestedName, verifyDestination, deleteSource, false)
	if err != nil {
		return nil, err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return nil, fmt.Errorf("commit transfer: %w", err)
	}
	committed = true
	return secret, nil
}

// transferPostgres runs the transfer in a transaction that acquires the
// destination's advisory lock (plus the source workspace's lock for Move) in
// sorted order before any read, serializing competing transfers and
// coordinating with workspace deletion under READ COMMITTED.
func (s *sqliteStore) transferPostgres(ctx context.Context, sourceID, sourceWorkspaceID string, targetScope SecretScope, targetWorkspaceID string, requestedName *string, verifyDestination func(context.Context) error, deleteSource bool) (*Secret, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin transfer: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	keys := []int64{transferTargetLockKey(targetScope, targetWorkspaceID)}
	if deleteSource && strings.TrimSpace(sourceWorkspaceID) != "" {
		keys = append(keys, WorkspaceLockKey(sourceWorkspaceID))
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	var prev int64
	first := true
	for _, key := range keys {
		if !first && key == prev {
			continue
		}
		if _, err := tx.ExecContext(ctx, s.db.Rebind("SELECT pg_advisory_xact_lock(?)"), key); err != nil {
			return nil, fmt.Errorf("acquire transfer lock: %w", err)
		}
		prev = key
		first = false
	}

	// A Move destroys the source, so the source row is locked with FOR UPDATE
	// (deleteSource == true): a concurrent Update cannot commit a newer value
	// between our read and delete, which would otherwise lose that value.
	secret, err := s.transferBody(ctx, tx, s.db.Rebind, sourceID, targetScope, targetWorkspaceID, requestedName, verifyDestination, deleteSource, deleteSource)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit transfer: %w", err)
	}
	return secret, nil
}

// transferTargetLockKey returns the advisory-lock key for a transfer target:
// the workspace key for Workspace targets, the Global constant otherwise.
func transferTargetLockKey(targetScope SecretScope, targetWorkspaceID string) int64 {
	if normalizeStoredScope(targetScope) != ScopeWorkspace {
		return GlobalSecretTransferLockKey
	}
	return WorkspaceLockKey(targetWorkspaceID)
}

// transferBody executes the shared transfer steps on the transaction-bound
// executor: destination verification (while the lock is held), source select,
// normalized-scope conflict check, insert of the copy (reusing the source's
// encrypted value), optional source delete, and the returned-row read. Every
// statement uses the same executor and the per-user visibility predicate.
func (s *sqliteStore) transferBody(ctx context.Context, exec transferExec, rebind func(string) string, sourceID string, targetScope SecretScope, targetWorkspaceID string, requestedName *string, verifyDestination func(context.Context) error, deleteSource, lockSource bool) (*Secret, error) {
	owner := scopeOwner(ctx)

	if verifyDestination != nil {
		if err := verifyDestination(ctx); err != nil {
			return nil, err
		}
	}

	src, err := s.transferSource(ctx, exec, rebind, sourceID, owner, lockSource)
	if err != nil {
		return nil, err
	}
	// An omitted name resolves from the transactional source row: a concurrent
	// rename can never split the copied name from its value (or, for Move,
	// delete the renamed row and recreate it under a stale name).
	targetName := ""
	if requestedName == nil {
		targetName = src.Name
	} else {
		targetName = *requestedName
	}

	if err := s.transferConflictCheck(ctx, exec, rebind, targetScope, targetWorkspaceID, targetName, owner); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	storedScope := normalizeStoredScope(targetScope)
	copyID := uuid.New().String()
	if s.beforeTransferInsert != nil {
		if err := s.beforeTransferInsert(); err != nil {
			return nil, err
		}
	}
	// The insert is guarded by the same conflict predicate atomically: on
	// PostgreSQL, an ordinary create or rename does not take the transfer's
	// advisory lock, so the eager check alone could race a late commit of the
	// same name. NOT EXISTS turns that race into a clean conflict instead of a
	// duplicate row.
	predicate, predicateArgs := transferConflictPredicate(targetScope, targetWorkspaceID, targetName, owner)
	result, err := exec.ExecContext(ctx, rebind(`
		INSERT INTO secrets (id, name, user_id, scope, workspace_id, encrypted_value, nonce, created_at, updated_at)
		SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?
		WHERE NOT EXISTS (SELECT 1 FROM secrets WHERE `+predicate+`)`),
		append([]any{copyID, targetName, owner, storedScope, targetWorkspaceID, src.Ciphertext, src.Nonce, now, now}, predicateArgs...)...)
	if err != nil {
		return nil, fmt.Errorf("insert transferred secret: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return nil, ErrSecretNameConflict
	}

	if deleteSource {
		if err := s.deleteTransferredSource(ctx, exec, rebind, sourceID, owner); err != nil {
			return nil, err
		}
	}

	var meta secretRow
	if err := exec.QueryRowContext(ctx, rebind(`
		SELECT id, name, scope, workspace_id, created_at, updated_at FROM secrets WHERE id = ?`),
		copyID).Scan(&meta.ID, &meta.Name, &meta.Scope, &meta.WorkspaceID, &meta.CreatedAt, &meta.UpdatedAt); err != nil {
		return nil, fmt.Errorf("read transferred secret: %w", err)
	}
	return meta.toSecret(), nil
}

// deleteTransferredSource removes the source row using the same per-user
// visibility predicate as the source select. A missing row means the source
// vanished concurrently and the whole transfer rolls back.
func (s *sqliteStore) deleteTransferredSource(ctx context.Context, exec transferExec, rebind func(string) string, sourceID, owner string) error {
	if s.failAfterInsert != nil {
		if err := s.failAfterInsert(); err != nil {
			return err
		}
	}
	result, err := exec.ExecContext(ctx, rebind(`
		DELETE FROM secrets WHERE id = ? AND (user_id = '' OR ? = '' OR user_id = ?)`),
		sourceID, owner, owner)
	if err != nil {
		return fmt.Errorf("delete transferred source: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("%w: %s", ErrNotFound, sourceID)
	}
	return nil
}

// transferSourceRow is the decrypted-source scan target for transferBody.
type transferSourceRow struct {
	ID          string
	Name        string
	Scope       SecretScope
	WorkspaceID string
	Ciphertext  []byte
	Nonce       []byte
}

// transferSource selects the source row with the per-user visibility
// predicate. On PostgreSQL Moves (lockSource) it appends FOR UPDATE so a
// concurrent Update cannot interleave between the read and the delete; the
// afterSourceLock test hook fires once the row lock is known to be held.
func (s *sqliteStore) transferSource(ctx context.Context, exec transferExec, rebind func(string) string, sourceID, owner string, lockSource bool) (*transferSourceRow, error) {
	query := `
		SELECT id, name, scope, workspace_id, encrypted_value, nonce
		FROM secrets WHERE id = ? AND (user_id = '' OR ? = '' OR user_id = ?)`
	if lockSource {
		query += " FOR UPDATE"
	}
	var src transferSourceRow
	err := exec.QueryRowContext(ctx, rebind(query), sourceID, owner, owner).
		Scan(&src.ID, &src.Name, &src.Scope, &src.WorkspaceID, &src.Ciphertext, &src.Nonce)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, sourceID)
		}
		return nil, fmt.Errorf("transfer source lookup: %w", err)
	}
	if lockSource && s.afterSourceLock != nil {
		if err := s.afterSourceLock(); err != nil {
			return nil, err
		}
	}
	return &src, nil
}

// transferConflictPredicate returns the WHERE predicate and arguments that
// identify an existing secret with the given name in the destination scope,
// comparing the NORMALIZED scope so a legacy empty stored scope (which reads
// as Global) can never be bypassed. Shared by the eager conflict check and the
// atomic NOT EXISTS guard on the transfer insert.
func transferConflictPredicate(targetScope SecretScope, targetWorkspaceID, targetName, owner string) (string, []any) {
	if normalizeStoredScope(targetScope) != ScopeWorkspace {
		return `name = ? AND (scope = ? OR scope = '') AND (user_id = '' OR ? = '' OR user_id = ?)`,
			[]any{targetName, ScopeGlobal, owner, owner}
	}
	return `name = ? AND scope = ? AND workspace_id = ? AND (user_id = '' OR ? = '' OR user_id = ?)`,
		[]any{targetName, ScopeWorkspace, targetWorkspaceID, owner, owner}
}

// transferConflictCheck rejects a target name that already exists in the
// destination scope. It is the eager, pre-insert check; the insert itself is
// guarded by the same predicate atomically (see transferBody), so a competing
// create or rename that commits after this check can still never produce a
// duplicate name from the transfer.
func (s *sqliteStore) transferConflictCheck(ctx context.Context, exec transferExec, rebind func(string) string, targetScope SecretScope, targetWorkspaceID, targetName, owner string) error {
	predicate, args := transferConflictPredicate(targetScope, targetWorkspaceID, targetName, owner)
	query := `SELECT 1 FROM secrets WHERE ` + predicate + ` LIMIT 1`
	var found int
	if err := exec.QueryRowContext(ctx, rebind(query), args...).Scan(&found); err == nil {
		return ErrSecretNameConflict
	} else if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("transfer conflict check: %w", err)
	}
	return nil
}
