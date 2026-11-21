package promptsync

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/authcircuit"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/reposync"
)

// configRowID is the key of the single install-wide configuration row.
const configRowID = "default"

// Store persists the install-wide prompt sync configuration and status.
type Store struct {
	db *sqlx.DB
	ro *sqlx.DB
}

// NewStore creates the store and initializes its schema.
func NewStore(writer, reader *sqlx.DB) (*Store, error) {
	s := &Store{db: writer, ro: reader}
	if _, err := s.db.Exec(dialect.MustRenderSchema(s.db.DriverName(), createTableSQL)); err != nil {
		return nil, fmt.Errorf("promptsync schema init: %w", err)
	}
	return s, nil
}

const createTableSQL = `
	CREATE TABLE IF NOT EXISTS prompt_sync_config (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		provider TEXT NOT NULL,
		github_host TEXT NOT NULL DEFAULT '',
		repo_owner TEXT NOT NULL DEFAULT '',
		repo_name TEXT NOT NULL DEFAULT '',
		project_path TEXT NOT NULL DEFAULT '',
		branch TEXT NOT NULL,
		path TEXT NOT NULL,
		interval_seconds INTEGER NOT NULL,
		poll_enabled {{boolean}} NOT NULL DEFAULT TRUE,
		last_synced_at TIMESTAMP,
		last_ok {{boolean}} NOT NULL DEFAULT FALSE,
		last_error TEXT NOT NULL DEFAULT '',
		last_warnings TEXT NOT NULL DEFAULT '[]',
		last_hash TEXT NOT NULL DEFAULT '',
		failure_class TEXT NOT NULL DEFAULT '',
		consecutive_failures INTEGER NOT NULL DEFAULT 0,
		next_attempt_at TIMESTAMP,
		last_error_class TEXT NOT NULL DEFAULT '',
		poll_suspended {{boolean}} NOT NULL DEFAULT FALSE,
		poll_suspension_reason TEXT NOT NULL DEFAULT '',
		credential_fingerprint TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL
	);
`

const configColumns = `workspace_id, provider, github_host, repo_owner, repo_name, project_path, branch, path,
	interval_seconds, poll_enabled, last_synced_at, last_ok, last_error, last_warnings,
	failure_class, consecutive_failures, next_attempt_at, last_error_class, poll_suspended,
	poll_suspension_reason, credential_fingerprint, created_at, updated_at`

// Get returns the configuration, or nil when none is stored.
func (s *Store) Get(ctx context.Context) (*Config, error) {
	cfg := &Config{}
	var lastSyncedAt, nextAttemptAt sql.NullTime
	var warningsJSON, failureClass string
	err := s.ro.QueryRowContext(ctx, s.ro.Rebind(`SELECT `+configColumns+` FROM prompt_sync_config WHERE id = ?`), configRowID).Scan(
		&cfg.WorkspaceID, &cfg.Provider, &cfg.GitHubHost, &cfg.RepoOwner, &cfg.RepoName, &cfg.ProjectPath,
		&cfg.Branch, &cfg.Path, &cfg.IntervalSeconds, &cfg.PollEnabled, &lastSyncedAt, &cfg.LastOk,
		&cfg.LastError, &warningsJSON, &failureClass, &cfg.ConsecutiveFailures,
		&nextAttemptAt, &cfg.LastErrorClass, &cfg.PollSuspended, &cfg.PollSuspensionReason,
		&cfg.CredentialFingerprint, &cfg.CreatedAt, &cfg.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	cfg.FailureClass = authcircuit.FailureClass(failureClass)
	if lastSyncedAt.Valid {
		t := lastSyncedAt.Time
		cfg.LastSyncedAt = &t
	}
	if nextAttemptAt.Valid {
		t := nextAttemptAt.Time
		cfg.NextAttemptAt = &t
	}
	// Corrupt JSON degrades to no warnings rather than failing the read.
	_ = json.Unmarshal([]byte(warningsJSON), &cfg.LastWarnings)
	return cfg, nil
}

// Upsert creates or replaces the configuration. Status and recovery state
// reset so the next sync starts fresh; the credential fingerprint is kept
// because it tracks the connection, not this configuration.
func (s *Store) Upsert(ctx context.Context, req *SetConfigRequest) (*Config, error) {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO prompt_sync_config (
			id, workspace_id, provider, github_host, repo_owner, repo_name, project_path, branch, path,
			interval_seconds, poll_enabled, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			workspace_id = excluded.workspace_id, provider = excluded.provider,
			github_host = excluded.github_host, repo_owner = excluded.repo_owner,
			repo_name = excluded.repo_name, project_path = excluded.project_path,
			branch = excluded.branch, path = excluded.path,
			interval_seconds = excluded.interval_seconds, poll_enabled = excluded.poll_enabled,
			last_synced_at = NULL, last_ok = FALSE, last_error = '', last_warnings = '[]', last_hash = '',
			failure_class = '', consecutive_failures = 0, next_attempt_at = NULL,
			last_error_class = '', poll_suspended = FALSE, poll_suspension_reason = '',
			updated_at = excluded.updated_at
	`), configRowID, req.WorkspaceID, req.Provider, req.GitHubHost, req.RepoOwner, req.RepoName,
		req.ProjectPath, req.Branch, req.Path, req.IntervalSeconds, req.PollEnabled != nil && *req.PollEnabled, now, now)
	if err != nil {
		return nil, err
	}
	return s.Get(ctx)
}

// RecordSuccess stores a successful run and clears recovery state.
func (s *Store) RecordSuccess(ctx context.Context, warnings []string, hash string, at time.Time) error {
	if warnings == nil {
		warnings = []string{}
	}
	warningsJSON, err := json.Marshal(warnings)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE prompt_sync_config
		SET last_synced_at = ?, last_ok = TRUE, last_error = '', last_warnings = ?, last_hash = ?,
			failure_class = '', consecutive_failures = 0, next_attempt_at = NULL,
			last_error_class = '', poll_suspended = FALSE, poll_suspension_reason = '', updated_at = ?
		WHERE id = ?
	`), at, string(warningsJSON), hash, at, configRowID)
	return err
}

// RecordFailure stores a failed run and its recovery directive.
func (s *Store) RecordFailure(ctx context.Context, errMsg string, directive reposync.FailureDirective, at time.Time) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		UPDATE prompt_sync_config
		SET last_synced_at = ?, last_ok = FALSE, last_error = ?, last_warnings = '[]', last_hash = '',
			failure_class = ?, consecutive_failures = ?, next_attempt_at = ?, last_error_class = ?,
			poll_suspended = ?, poll_suspension_reason = ?, updated_at = ?
		WHERE id = ?
	`), at, errMsg, string(directive.CircuitClass), directive.Consecutive, directive.NextAttemptAt,
		directive.Class, directive.Suspended, directive.SuspensionReason, at, configRowID)
	return err
}

// RecordCredentialFingerprint stores the connection fingerprint. When
// resetRecovery is set, a changed credential clears backoff and suspension.
func (s *Store) RecordCredentialFingerprint(ctx context.Context, fingerprint string, resetRecovery bool) error {
	query := `UPDATE prompt_sync_config SET credential_fingerprint = ? WHERE id = ?`
	if resetRecovery {
		query = `UPDATE prompt_sync_config
			SET credential_fingerprint = ?, failure_class = '', consecutive_failures = 0, next_attempt_at = NULL,
				last_error_class = '', poll_suspended = FALSE, poll_suspension_reason = ''
			WHERE id = ?`
	}
	_, err := s.db.ExecContext(ctx, s.db.Rebind(query), fingerprint, configRowID)
	return err
}

// Delete removes the configuration. Deleting a missing row is a no-op.
func (s *Store) Delete(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`DELETE FROM prompt_sync_config WHERE id = ?`), configRowID)
	return err
}
