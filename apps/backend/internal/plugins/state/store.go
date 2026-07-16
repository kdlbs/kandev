// Package state provides a plugin-scoped key/value store backed by SQLite.
// Plugins read and write arbitrary JSON values under a (scope, scope_id, key)
// tuple, always filtered by plugin_id so a plugin can never read or write
// another plugin's state (spec: docs/specs/plugins/spec.md, "plugin_state").
package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
)

// StateEntry is a single row returned by List.
type StateEntry struct {
	Key       string          `db:"state_key" json:"key"`
	Value     json.RawMessage `db:"value_json" json:"value"`
	UpdatedAt time.Time       `db:"updated_at" json:"updated_at"`
}

// Store persists plugin-scoped KV state in the plugin_state table.
type Store struct {
	db *sqlx.DB
	ro *sqlx.DB
}

// NewStore creates a Store and initializes the plugin_state schema if needed.
func NewStore(pool *db.Pool) (*Store, error) {
	s := &Store{db: pool.Writer(), ro: pool.Reader()}
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("plugin state schema init: %w", err)
	}
	return s, nil
}

// initSchema creates the plugin_state table. scope_id is declared NOT NULL
// with an empty-string default (the instance-scope sentinel) rather than a
// nullable column as the spec's illustrative DDL shows. SQLite's UNIQUE
// index treats every NULL as distinct, so a nullable scope_id would let
// ON CONFLICT(plugin_id, scope, scope_id, state_key) silently miss conflicts
// between rows that are both "no scope" (e.g. two instance-scope Set calls
// for the same key), producing duplicate rows instead of an upsert. Empty
// string is unambiguous under ordinary equality/uniqueness and matches this
// codebase's existing convention for optional scope-like columns (see
// base_schema.go: executor_id, repository_id, declared TEXT DEFAULT empty).
func (s *Store) initSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS plugin_state (
			id TEXT PRIMARY KEY,
			plugin_id TEXT NOT NULL,
			scope TEXT NOT NULL DEFAULT 'instance',
			scope_id TEXT NOT NULL DEFAULT '',
			state_key TEXT NOT NULL,
			value_json TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE (plugin_id, scope, scope_id, state_key)
		);
	`)
	return err
}

// Get returns the value stored for the given plugin/scope/scopeID/key.
// scopeID is "" for instance-scoped state. found is false if no row exists.
func (s *Store) Get(ctx context.Context, pluginID, scope, scopeID, key string) (json.RawMessage, bool, error) {
	var raw string
	err := s.ro.QueryRowContext(ctx, s.ro.Rebind(`
		SELECT value_json FROM plugin_state
		WHERE plugin_id = ? AND scope = ? AND scope_id = ? AND state_key = ?
	`), pluginID, scope, scopeID, key).Scan(&raw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return json.RawMessage(raw), true, nil
}

// Set upserts the value for the given plugin/scope/scopeID/key, setting
// updated_at to the current time in RFC3339 UTC.
func (s *Store) Set(ctx context.Context, pluginID, scope, scopeID, key string, value json.RawMessage) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		INSERT INTO plugin_state (id, plugin_id, scope, scope_id, state_key, value_json, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(plugin_id, scope, scope_id, state_key)
		DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at
	`), uuid.New().String(), pluginID, scope, scopeID, key, string(value), now)
	return err
}

// Delete removes the row for the given plugin/scope/scopeID/key. It is not
// an error if no matching row exists.
func (s *Store) Delete(ctx context.Context, pluginID, scope, scopeID, key string) error {
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`
		DELETE FROM plugin_state
		WHERE plugin_id = ? AND scope = ? AND scope_id = ? AND state_key = ?
	`), pluginID, scope, scopeID, key)
	return err
}

// List returns all state entries for the given plugin/scope/scopeID.
func (s *Store) List(ctx context.Context, pluginID, scope, scopeID string) ([]StateEntry, error) {
	rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(`
		SELECT state_key, value_json, updated_at FROM plugin_state
		WHERE plugin_id = ? AND scope = ? AND scope_id = ?
		ORDER BY state_key
	`), pluginID, scope, scopeID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var entries []StateEntry
	for rows.Next() {
		var (
			key, raw, updatedAtStr string
		)
		if err := rows.Scan(&key, &raw, &updatedAtStr); err != nil {
			return nil, err
		}
		updatedAt, err := time.Parse(time.RFC3339, updatedAtStr)
		if err != nil {
			return nil, fmt.Errorf("parse updated_at for key %q: %w", key, err)
		}
		entries = append(entries, StateEntry{Key: key, Value: json.RawMessage(raw), UpdatedAt: updatedAt})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}
