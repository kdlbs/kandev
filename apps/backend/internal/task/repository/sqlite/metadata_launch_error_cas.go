package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
)

type metadataCASMode uint8

const (
	metadataCASExpected metadataCASMode = iota
	metadataCASDifferent
)

// SetSessionMetadataKeyIfStamp replaces one session metadata value only when
// the current value still has expectedStamp. The row lock and write share one
// transaction, so a recovery request cannot replace a newer error.
func (r *Repository) SetSessionMetadataKeyIfStamp(
	ctx context.Context,
	sessionID, key, expectedStamp string,
	value interface{},
) (bool, error) {
	stored, _, err := r.setMetadataKeyIfStamp(ctx, "task_sessions", "agent session", sessionID, key, expectedStamp, value, metadataCASExpected)
	return stored, err
}

// SetTaskMetadataKeyIfStamp replaces one task metadata value only when the
// current value still has expectedStamp.
func (r *Repository) SetTaskMetadataKeyIfStamp(
	ctx context.Context,
	taskID, key, expectedStamp string,
	value interface{},
) (bool, error) {
	stored, _, err := r.setMetadataKeyIfStamp(ctx, "tasks", "task", taskID, key, expectedStamp, value, metadataCASExpected)
	return stored, err
}

// SetTaskMetadataKeyIfDifferentStamp stores a task metadata value unless the
// current value already has newStamp. It is the atomic idempotency operation
// for repeated PR launch-gate callbacks.
func (r *Repository) SetTaskMetadataKeyIfDifferentStamp(
	ctx context.Context,
	taskID, key, newStamp string,
	value interface{},
) (stored bool, noOp bool, err error) {
	return r.setMetadataKeyIfStamp(ctx, "tasks", "task", taskID, key, newStamp, value, metadataCASDifferent)
}

func (r *Repository) setMetadataKeyIfStamp(
	ctx context.Context,
	table, entityName, entityID, key, expectedStamp string,
	value interface{},
	mode metadataCASMode,
) (bool, bool, error) {
	if strings.TrimSpace(expectedStamp) == "" {
		return false, false, nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return false, false, fmt.Errorf("failed to serialize metadata value: %w", err)
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, false, err
	}
	defer func() { _ = tx.Rollback() }()

	metadata, err := r.lockMetadataRow(ctx, tx, table, entityName, entityID)
	if err != nil {
		return false, false, err
	}
	currentStamp, err := metadataRecordStamp(metadata, key)
	if err != nil {
		return false, false, err
	}
	shouldWrite := currentStamp == expectedStamp
	if mode == metadataCASDifferent {
		shouldWrite = currentStamp != expectedStamp
	}
	if !shouldWrite {
		if err := tx.Commit(); err != nil {
			return false, false, err
		}
		return false, true, nil
	}

	result, err := tx.ExecContext(ctx, r.db.Rebind(metadataKeyUpdateQuery(table, r.db.DriverName())), metadataKeyUpdateArgs(r.db.DriverName(), key, string(payload), r.nowUTC(), entityID)...)
	if err != nil {
		return false, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, false, err
	}
	if rows == 0 {
		return false, false, fmt.Errorf("%s not found: %s", entityName, entityID)
	}
	if err := tx.Commit(); err != nil {
		return false, false, err
	}
	return true, false, nil
}

func (r *Repository) lockMetadataRow(
	ctx context.Context,
	tx *sqlx.Tx,
	table, entityName, entityID string,
) (string, error) {
	query := "SELECT metadata FROM " + table + " WHERE id = ?"
	if dialect.IsPostgres(r.db.DriverName()) {
		query += " FOR UPDATE"
	}
	var raw sql.NullString
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(query), entityID).Scan(&raw); err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("%s not found: %s", entityName, entityID)
		}
		return "", err
	}
	if !raw.Valid {
		return "{}", nil
	}
	return raw.String, nil
}

func metadataRecordStamp(metadataJSON, key string) (string, error) {
	if strings.TrimSpace(metadataJSON) == "" || strings.TrimSpace(metadataJSON) == jsonNull {
		return "", nil
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		return "", fmt.Errorf("failed to parse metadata: %w", err)
	}
	raw, ok := metadata[key]
	if !ok || string(raw) == "null" {
		return "", nil
	}
	var value struct {
		Message    string    `json:"message"`
		OccurredAt time.Time `json:"occurred_at"`
		StampValue string    `json:"stamp"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("failed to parse metadata key %q: %w", key, err)
	}
	if stamp := strings.TrimSpace(value.StampValue); stamp != "" {
		return stamp, nil
	}
	if value.Message == "" {
		return "", nil
	}
	return value.OccurredAt.UTC().Format(time.RFC3339Nano) + ":" + value.Message, nil
}

func metadataKeyUpdateQuery(table, driver string) string {
	if dialect.IsPostgres(driver) {
		return "UPDATE " + table + " SET metadata = jsonb_set(CASE WHEN metadata IS NULL OR metadata = 'null' OR metadata = '' THEN '{}'::jsonb ELSE metadata::jsonb END, ARRAY[?]::text[], ?::jsonb, true)::text, updated_at = ? WHERE id = ?"
	}
	return "UPDATE " + table + " SET metadata = json_set(CASE WHEN metadata IS NULL OR metadata = 'null' OR metadata = '' THEN '{}' ELSE metadata END, ?, json(?)), updated_at = ? WHERE id = ?"
}

func metadataKeyUpdateArgs(driver, key, payload string, updatedAt time.Time, entityID string) []interface{} {
	path := key
	if !dialect.IsPostgres(driver) {
		path = jsonPath(key)
	}
	return []interface{}{path, payload, updatedAt, entityID}
}
