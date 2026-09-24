package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/agent/mcpconfig/registry"
	"github.com/kandev/kandev/internal/db/dialect"
)

var _ registry.CacheStore = (*sqliteRepository)(nil)

func (r *sqliteRepository) ListMCPRegistryEntries(ctx context.Context, query string) ([]registry.Entry, error) {
	query = strings.ToLower(strings.TrimSpace(query))
	rows, err := r.ro.QueryxContext(ctx, r.ro.Rebind(`SELECT payload_json FROM mcp_registry_entries WHERE lower(name) LIKE ? ORDER BY name, version`), "%"+query+"%")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	entries := make([]registry.Entry, 0)
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		entry, err := decodeRegistryEntry(payload)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	return entries, rows.Err()
}

func (r *sqliteRepository) GetMCPRegistryEntry(ctx context.Context, identity string) (*registry.Entry, error) {
	var payload string
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`SELECT payload_json FROM mcp_registry_entries WHERE identity = ?`), identity).Scan(&payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, registry.ErrRegistryEntryNotFound
	}
	if err != nil {
		return nil, err
	}
	entry, err := decodeRegistryEntry(payload)
	if err != nil {
		return nil, err
	}
	return &entry, nil
}

func (r *sqliteRepository) ReplaceMCPRegistryEntries(ctx context.Context, entries []registry.Entry) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM mcp_registry_entries`)); err != nil {
		return err
	}
	for _, entry := range entries {
		if err := upsertRegistryEntry(ctx, tx, entry); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *sqliteRepository) UpsertMCPRegistryEntries(ctx context.Context, entries []registry.Entry) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, entry := range entries {
		if err := upsertRegistryEntry(ctx, tx, entry); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *sqliteRepository) GetMCPRegistrySyncState(ctx context.Context) (registry.SyncState, error) {
	var state registry.SyncState
	var lastSuccessful, lastAttempt, updatedSince sql.NullTime
	var degraded int
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`SELECT last_successful_at, last_attempt_at, updated_since, degraded, last_error FROM mcp_registry_sync_state WHERE id = 1`)).Scan(&lastSuccessful, &lastAttempt, &updatedSince, &degraded, &state.LastError)
	if errors.Is(err, sql.ErrNoRows) {
		return registry.SyncState{}, registry.ErrSyncStateNotFound
	}
	if err != nil {
		return registry.SyncState{}, err
	}
	state.LastSuccessfulAt = nullTimeValue(lastSuccessful)
	state.LastAttemptAt = nullTimeValue(lastAttempt)
	state.UpdatedSince = nullTimeValue(updatedSince)
	state.Degraded = degraded == 1
	return state, nil
}

func (r *sqliteRepository) SaveMCPRegistrySyncState(ctx context.Context, state registry.SyncState) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO mcp_registry_sync_state (id, last_successful_at, last_attempt_at, updated_since, degraded, last_error)
		VALUES (1, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			last_successful_at = excluded.last_successful_at,
			last_attempt_at = excluded.last_attempt_at,
			updated_since = excluded.updated_since,
			degraded = excluded.degraded,
			last_error = excluded.last_error
	`), nullableTime(state.LastSuccessfulAt), nullableTime(state.LastAttemptAt), nullableTime(state.UpdatedSince), dialect.BoolToInt(state.Degraded), state.LastError)
	return err
}

type registryExecer interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowxContext(context.Context, string, ...any) *sqlx.Row
	Rebind(string) string
}

func upsertRegistryEntry(ctx context.Context, execer registryExecer, entry registry.Entry) error {
	if strings.TrimSpace(entry.Name) == "" || strings.TrimSpace(entry.Version) == "" {
		return errors.New("registry entry requires name and version")
	}
	identity := entry.Identity()
	previousPayload, previousRevision, found, err := existingRegistryPayload(ctx, execer, identity)
	if err != nil {
		return err
	}
	comparisonPayload, err := registryPayload(entry)
	if err != nil {
		return err
	}
	revision := int64(1)
	if found {
		revision = previousRevision
		if previousPayload != comparisonPayload {
			revision++
		}
	}
	entry.Revision = revision
	if entry.UpdatedAt.IsZero() {
		entry.UpdatedAt = time.Now().UTC()
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal registry entry: %w", err)
	}
	_, err = execer.ExecContext(ctx, execer.Rebind(`
		INSERT INTO mcp_registry_entries (identity, name, version, status, status_message, payload_json, revision, updated_at, synced_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(identity) DO UPDATE SET
			name = excluded.name, version = excluded.version, status = excluded.status,
			status_message = excluded.status_message, payload_json = excluded.payload_json,
			revision = excluded.revision, updated_at = excluded.updated_at, synced_at = excluded.synced_at
	`), identity, entry.Name, entry.Version, entry.Status, entry.StatusMessage, string(payload), entry.Revision, entry.UpdatedAt, time.Now().UTC())
	return err
}

func existingRegistryPayload(ctx context.Context, execer registryExecer, identity string) (string, int64, bool, error) {
	row := execer.QueryRowxContext(ctx, execer.Rebind(`SELECT payload_json, revision FROM mcp_registry_entries WHERE identity = ?`), identity)
	var payload string
	var revision int64
	err := row.Scan(&payload, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, false, nil
	}
	return payload, revision, err == nil, err
}

func registryPayload(entry registry.Entry) (string, error) {
	entry.Revision = 0
	entry.UpdatedAt = time.Time{}
	payload, err := json.Marshal(entry)
	return string(payload), err
}

func decodeRegistryEntry(payload string) (registry.Entry, error) {
	var entry registry.Entry
	if err := json.Unmarshal([]byte(payload), &entry); err != nil {
		return registry.Entry{}, fmt.Errorf("unmarshal registry entry: %w", err)
	}
	return entry, nil
}

func nullTimeValue(value sql.NullTime) time.Time {
	if !value.Valid {
		return time.Time{}
	}
	return value.Time
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}
