// Package provideraccess persists narrow provider credential grants and leases.
package provideraccess

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
)

const grantColumns = `id, scope_key, plugin_installation_id, plugin_id, workspace_id,
 conversation_key, target_task_id, repository_id, provider, purpose, generation,
 created_by_user_id, expires_at, revoked_at, created_at, updated_at`

// GrantScope identifies one provider-access authority boundary.
type GrantScope struct {
	PluginInstallationID string
	PluginID             string
	WorkspaceID          string
	ConversationKey      string
	TargetTaskID         string
	RepositoryID         string
	Provider             string
	Purpose              string
}

// Grant is an administrator-created, expiring provider-access approval.
type Grant struct {
	GrantScope
	ID              string
	Generation      int64
	CreatedByUserID string
	ExpiresAt       time.Time
	RevokedAt       *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Scope returns the grant's exact scope.
func (g Grant) Scope() GrantScope { return g.GrantScope }

type grantRow struct {
	ID                   string        `db:"id"`
	ScopeKey             string        `db:"scope_key"`
	PluginInstallationID string        `db:"plugin_installation_id"`
	PluginID             string        `db:"plugin_id"`
	WorkspaceID          string        `db:"workspace_id"`
	ConversationKey      string        `db:"conversation_key"`
	TargetTaskID         string        `db:"target_task_id"`
	RepositoryID         string        `db:"repository_id"`
	Provider             string        `db:"provider"`
	Purpose              string        `db:"purpose"`
	Generation           int64         `db:"generation"`
	CreatedByUserID      string        `db:"created_by_user_id"`
	ExpiresAt            int64         `db:"expires_at"`
	RevokedAt            sql.NullInt64 `db:"revoked_at"`
	CreatedAt            int64         `db:"created_at"`
	UpdatedAt            int64         `db:"updated_at"`
}

func (r grantRow) grant() *Grant {
	g := &Grant{GrantScope: GrantScope{
		PluginInstallationID: r.PluginInstallationID, PluginID: r.PluginID,
		WorkspaceID: r.WorkspaceID, ConversationKey: r.ConversationKey,
		TargetTaskID: r.TargetTaskID, RepositoryID: r.RepositoryID,
		Provider: r.Provider, Purpose: r.Purpose,
	}, ID: r.ID, Generation: r.Generation, CreatedByUserID: r.CreatedByUserID,
		ExpiresAt: time.Unix(r.ExpiresAt, 0).UTC(),
		CreatedAt: time.Unix(r.CreatedAt, 0).UTC(), UpdatedAt: time.Unix(r.UpdatedAt, 0).UTC()}
	if r.RevokedAt.Valid {
		revoked := time.Unix(r.RevokedAt.Int64, 0).UTC()
		g.RevokedAt = &revoked
	}
	return g
}

// Store persists provider-access grants.
type Store struct{ db *sqlx.DB }

// NewStore creates replayable grant storage on SQLite or PostgreSQL.
func NewStore(db *sqlx.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("provider access database is required")
	}
	statements := []string{
		`CREATE TABLE IF NOT EXISTS provider_access_grants (
   id TEXT PRIMARY KEY, scope_key TEXT NOT NULL,
   plugin_installation_id TEXT NOT NULL, plugin_id TEXT NOT NULL,
   workspace_id TEXT NOT NULL, conversation_key TEXT NOT NULL,
   target_task_id TEXT NOT NULL, repository_id TEXT NOT NULL,
   provider TEXT NOT NULL, purpose TEXT NOT NULL, generation BIGINT NOT NULL,
   created_by_user_id TEXT NOT NULL, expires_at BIGINT NOT NULL,
   revoked_at BIGINT, created_at BIGINT NOT NULL, updated_at BIGINT NOT NULL,
   UNIQUE(scope_key, generation))`,
		`CREATE UNIQUE INDEX IF NOT EXISTS provider_access_grants_active
   ON provider_access_grants(scope_key) WHERE revoked_at IS NULL`,
		`CREATE INDEX IF NOT EXISTS provider_access_grants_workspace
   ON provider_access_grants(workspace_id, updated_at)`,
	}
	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return nil, fmt.Errorf("initialize provider access grants: %w", err)
		}
	}
	return &Store{db: db}, nil
}

// ReplaceGrant revokes one active exact-scope grant and inserts its successor atomically.
func (s *Store) ReplaceGrant(ctx context.Context, grant *Grant) error {
	if err := validateGrant(grant); err != nil {
		return err
	}
	key, err := scopeKey(grant.Scope())
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if dialect.IsPostgres(s.db.DriverName()) {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "provider-access-grant:"+key); err != nil {
			return err
		}
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE provider_access_grants
  SET revoked_at = ?, updated_at = ? WHERE scope_key = ? AND revoked_at IS NULL`), now.Unix(), now.Unix(), key); err != nil {
		return err
	}
	if err := tx.GetContext(ctx, &grant.Generation, tx.Rebind(`SELECT COALESCE(MAX(generation), 0) + 1
  FROM provider_access_grants WHERE scope_key = ?`), key); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO provider_access_grants (`+grantColumns+`)
  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		grant.ID, key, grant.PluginInstallationID, grant.PluginID,
		grant.WorkspaceID, grant.ConversationKey, grant.TargetTaskID,
		grant.RepositoryID, grant.Provider, grant.Purpose, grant.Generation,
		grant.CreatedByUserID, grant.ExpiresAt.Unix(), nil, now.Unix(), now.Unix())
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	grant.CreatedAt, grant.UpdatedAt = now, now
	return nil
}

func validateGrant(grant *Grant) error {
	if grant == nil || grant.ID == "" || grant.CreatedByUserID == "" ||
		!grant.ExpiresAt.After(time.Now()) || grant.RevokedAt != nil {
		return errors.New("complete active provider access grant is required")
	}
	_, err := scopeKey(grant.Scope())
	return err
}

func scopeKey(scope GrantScope) (string, error) {
	values := []string{scope.PluginInstallationID, scope.PluginID, scope.WorkspaceID,
		scope.ConversationKey, scope.TargetTaskID, scope.RepositoryID, scope.Provider, scope.Purpose}
	for _, value := range values {
		if value == "" || value != strings.TrimSpace(value) {
			return "", errors.New("complete provider access scope is required")
		}
	}
	canonical, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

// GetGrant returns a grant by immutable identity.
func (s *Store) GetGrant(ctx context.Context, id string) (*Grant, error) {
	var row grantRow
	err := s.db.GetContext(ctx, &row, s.db.Rebind(`SELECT `+grantColumns+` FROM provider_access_grants WHERE id = ?`), id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return row.grant(), nil
}

// GetActiveGrant returns the one nonexpired active exact-scope grant, if any.
func (s *Store) GetActiveGrant(ctx context.Context, scope GrantScope) (*Grant, error) {
	key, err := scopeKey(scope)
	if err != nil {
		return nil, err
	}
	var row grantRow
	err = s.db.GetContext(ctx, &row, s.db.Rebind(`SELECT `+grantColumns+`
  FROM provider_access_grants WHERE scope_key = ? AND revoked_at IS NULL AND expires_at > ?`), key, time.Now().Unix())
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return row.grant(), nil
}

// RevokeGrant fences the exact workspace's grant from later admissions.
func (s *Store) RevokeGrant(ctx context.Context, workspaceID, id string, at time.Time) error {
	if workspaceID == "" || id == "" || at.IsZero() {
		return errors.New("workspace, grant, and revocation time are required")
	}
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`UPDATE provider_access_grants
  SET revoked_at = ?, updated_at = ? WHERE id = ? AND workspace_id = ? AND revoked_at IS NULL`),
		at.Unix(), at.Unix(), id, workspaceID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return sql.ErrNoRows
	}
	return nil
}
