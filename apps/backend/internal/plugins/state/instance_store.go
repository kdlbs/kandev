package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
)

// InstanceStateEntry is a revisioned value owned by one plugin instance.
// Deleted entries are retained as tombstones so a delete followed by a stale
// recreate cannot reset the revision back to one.
type InstanceStateEntry struct {
	InstanceID string
	Key        string
	Value      json.RawMessage
	Revision   int64
	WriterKind string
	UpdatedAt  time.Time
	Deleted    bool
}

// ConflictError reports the revision that won a conditional state write.
// The current value is intentionally not included. Browser callers only get
// it when they still have a separate read permission.
type ConflictError struct {
	CurrentRevision int64
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("plugin instance state: revision conflict (current revision %d)", e.CurrentRevision)
}

func (e *ConflictError) Unwrap() error { return ErrConflict }

// InstanceStore persists browser and agent-authored state for a plugin
// instance. The mutex makes the read/check/write sequence atomic for every
// caller in this process; the PostgreSQL row lock covers concurrent processes.
type InstanceStore struct {
	db *sqlx.DB
	ro *sqlx.DB
	mu sync.Mutex
}

// NewInstanceStore creates the revisioned instance-state table if needed.
func NewInstanceStore(pool *db.Pool) (*InstanceStore, error) {
	if pool == nil || pool.Writer() == nil || pool.Reader() == nil {
		return nil, errors.New("plugin instance state: database pool is required")
	}
	s := &InstanceStore{db: pool.Writer(), ro: pool.Reader()}
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("plugin instance state schema init: %w", err)
	}
	return s, nil
}

func (s *InstanceStore) initSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS plugin_instance_state (
			id TEXT PRIMARY KEY,
			plugin_instance_id TEXT NOT NULL,
			state_key TEXT NOT NULL,
			value_json TEXT NOT NULL,
			revision INTEGER NOT NULL,
			writer_kind TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			deleted INTEGER NOT NULL DEFAULT 0,
			UNIQUE (plugin_instance_id, state_key)
		);
	`)
	return err
}

// Get returns the current value. found is false for a missing key or a
// tombstone, but the returned entry still carries its revision when a
// tombstone exists so callers can report a precise conflict.
func (s *InstanceStore) Get(ctx context.Context, instanceID, key string) (InstanceStateEntry, bool, error) {
	var entry InstanceStateEntry
	var raw, updatedAt string
	var deleted int
	err := s.ro.QueryRowContext(ctx, s.ro.Rebind(`
		SELECT plugin_instance_id, state_key, value_json, revision, writer_kind, updated_at, deleted
		FROM plugin_instance_state
		WHERE plugin_instance_id = ? AND state_key = ?
	`), instanceID, key).Scan(
		&entry.InstanceID, &entry.Key, &raw, &entry.Revision, &entry.WriterKind, &updatedAt, &deleted,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return InstanceStateEntry{}, false, nil
	}
	if err != nil {
		return InstanceStateEntry{}, false, err
	}
	entry.Value = json.RawMessage(raw)
	entry.Deleted = deleted != 0
	entry.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return InstanceStateEntry{}, false, fmt.Errorf("parse instance state updated_at: %w", err)
	}
	return entry, !entry.Deleted, nil
}

// List returns live values in stable key order. Tombstones are omitted.
func (s *InstanceStore) List(ctx context.Context, instanceID string) ([]InstanceStateEntry, error) {
	rows, err := s.ro.QueryxContext(ctx, s.ro.Rebind(`
		SELECT plugin_instance_id, state_key, value_json, revision, writer_kind, updated_at, deleted
		FROM plugin_instance_state
		WHERE plugin_instance_id = ? AND deleted = 0
		ORDER BY state_key
	`), instanceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var entries []InstanceStateEntry
	for rows.Next() {
		var entry InstanceStateEntry
		var raw, updatedAt string
		var deleted int
		if err := rows.Scan(&entry.InstanceID, &entry.Key, &raw, &entry.Revision, &entry.WriterKind, &updatedAt, &deleted); err != nil {
			return nil, err
		}
		entry.Value = json.RawMessage(raw)
		entry.Deleted = deleted != 0
		entry.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
		if err != nil {
			return nil, fmt.Errorf("parse instance state updated_at: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// Set writes a value at the expected revision. A nil expectation is an
// unconditional internal write; a pointer to zero creates a new key, and a
// positive value must match the current revision.
func (s *InstanceStore) Set(
	ctx context.Context,
	instanceID, key string,
	value json.RawMessage,
	expectedRevision *int64,
	writerKind string,
) (InstanceStateEntry, error) {
	if instanceID == "" || key == "" {
		return InstanceStateEntry{}, errors.New("plugin instance state: instance id and key are required")
	}
	if !json.Valid(value) {
		return InstanceStateEntry{}, errors.New("plugin instance state: value must be valid JSON")
	}
	if writerKind == "" {
		writerKind = "unknown"
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return InstanceStateEntry{}, err
	}
	defer func() { _ = tx.Rollback() }()

	current, found, err := selectInstanceState(ctx, tx, s.db.DriverName(), instanceID, key)
	if err != nil {
		return InstanceStateEntry{}, err
	}
	if err := checkExpectedRevision(expectedRevision, current, found); err != nil {
		return InstanceStateEntry{}, err
	}
	now := time.Now().UTC()
	if !found {
		current = InstanceStateEntry{
			InstanceID: instanceID,
			Key:        key,
			Value:      append(json.RawMessage(nil), value...),
			Revision:   1,
			WriterKind: writerKind,
			UpdatedAt:  now,
		}
		_, err = tx.ExecContext(ctx, tx.Rebind(`
			INSERT INTO plugin_instance_state
				(id, plugin_instance_id, state_key, value_json, revision, writer_kind, updated_at, deleted)
			VALUES (?, ?, ?, ?, ?, ?, ?, 0)
		`), uuid.New().String(), instanceID, key, string(value), current.Revision, writerKind, now.Format(time.RFC3339Nano))
	} else {
		current.Value = append(json.RawMessage(nil), value...)
		current.Revision++
		current.WriterKind = writerKind
		current.UpdatedAt = now
		current.Deleted = false
		_, err = tx.ExecContext(ctx, tx.Rebind(`
			UPDATE plugin_instance_state
			SET value_json = ?, revision = ?, writer_kind = ?, updated_at = ?, deleted = 0
			WHERE plugin_instance_id = ? AND state_key = ?
		`), string(value), current.Revision, writerKind, now.Format(time.RFC3339Nano), instanceID, key)
	}
	if err != nil {
		return InstanceStateEntry{}, err
	}
	if err := tx.Commit(); err != nil {
		return InstanceStateEntry{}, err
	}
	return current, nil
}

// Delete removes the visible value and advances its revision. The tombstone
// keeps the revision monotonic. Deleting an already-deleted key is idempotent.
func (s *InstanceStore) Delete(ctx context.Context, instanceID, key string, expectedRevision *int64, writerKind string) (int64, error) {
	if instanceID == "" || key == "" {
		return 0, errors.New("plugin instance state: instance id and key are required")
	}
	if writerKind == "" {
		writerKind = "unknown"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()
	current, found, err := selectInstanceState(ctx, tx, s.db.DriverName(), instanceID, key)
	if err != nil {
		return 0, err
	}
	if err := checkExpectedRevision(expectedRevision, current, found); err != nil {
		return 0, err
	}
	if found && current.Deleted {
		return current.Revision, tx.Commit()
	}
	now := time.Now().UTC()
	if !found {
		_, err = tx.ExecContext(ctx, tx.Rebind(`
			INSERT INTO plugin_instance_state
				(id, plugin_instance_id, state_key, value_json, revision, writer_kind, updated_at, deleted)
			VALUES (?, ?, ?, 'null', 1, ?, ?, 1)
		`), uuid.New().String(), instanceID, key, writerKind, now.Format(time.RFC3339Nano))
		if err != nil {
			return 0, err
		}
		if err := tx.Commit(); err != nil {
			return 0, err
		}
		return 1, nil
	}
	current.Revision++
	_, err = tx.ExecContext(ctx, tx.Rebind(`
		UPDATE plugin_instance_state
		SET value_json = 'null', revision = ?, writer_kind = ?, updated_at = ?, deleted = 1
		WHERE plugin_instance_id = ? AND state_key = ?
	`), current.Revision, writerKind, now.Format(time.RFC3339Nano), instanceID, key)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return current.Revision, nil
}

// DeleteInstance removes all state owned by an isolated web-app instance.
// Instance deletion is a lifecycle boundary, so unlike Delete it removes the
// complete key/tombstone set instead of retaining per-key revisions.
func (s *InstanceStore) DeleteInstance(ctx context.Context, instanceID string) error {
	if strings.TrimSpace(instanceID) == "" {
		return errors.New("plugin instance state: instance id is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.ExecContext(ctx, s.db.Rebind(`DELETE FROM plugin_instance_state WHERE plugin_instance_id = ?`), instanceID)
	return err
}

func selectInstanceState(
	ctx context.Context,
	tx *sqlx.Tx,
	driver, instanceID, key string,
) (InstanceStateEntry, bool, error) {
	query := `
		SELECT plugin_instance_id, state_key, value_json, revision, writer_kind, updated_at, deleted
		FROM plugin_instance_state
		WHERE plugin_instance_id = ? AND state_key = ?
	`
	if dialect.IsPostgres(driver) {
		query += " FOR UPDATE"
	}
	var entry InstanceStateEntry
	var raw, updatedAt string
	var deleted int
	err := tx.QueryRowxContext(ctx, tx.Rebind(query), instanceID, key).Scan(
		&entry.InstanceID, &entry.Key, &raw, &entry.Revision, &entry.WriterKind, &updatedAt, &deleted,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return InstanceStateEntry{}, false, nil
	}
	if err != nil {
		return InstanceStateEntry{}, false, err
	}
	entry.Value = json.RawMessage(raw)
	entry.Deleted = deleted != 0
	entry.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt)
	if err != nil {
		return InstanceStateEntry{}, false, fmt.Errorf("parse instance state updated_at: %w", err)
	}
	return entry, true, nil
}

func checkExpectedRevision(expected *int64, current InstanceStateEntry, found bool) error {
	if expected == nil {
		return nil
	}
	currentRevision := int64(0)
	if found {
		currentRevision = current.Revision
	}
	if *expected != currentRevision {
		return &ConflictError{CurrentRevision: currentRevision}
	}
	return nil
}
