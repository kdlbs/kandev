package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// SetTaskInterruptedSessionIfNotArchived records one interruption without
// replacing a sibling session's marker. A retry for the same episode token is
// idempotent; a later episode in the same incarnation rotates the token.
func (r *Repository) SetTaskInterruptedSessionIfNotArchived(
	ctx context.Context,
	taskID, sessionID string,
	marker models.InterruptedSessionMarker,
) (bool, error) {
	if taskID == "" || sessionID == "" || marker.IncarnationID == "" || marker.Token == "" || marker.InterruptedAt == "" {
		return false, errors.New("task interruption identity and marker are required")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	changed, err := r.setTaskInterruptedSessionTx(ctx, tx, taskID, sessionID, marker, nil)
	if err != nil || !changed {
		return changed, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

// SetTaskInterruptedSessionIfRecoveryCurrent also fences recovery sweeps by
// the exact settled session generation and token.
func (r *Repository) SetTaskInterruptedSessionIfRecoveryCurrent(
	ctx context.Context,
	taskID, sessionID string,
	expectedSessionUpdatedAt time.Time,
	expectedRecoveryToken string,
	marker models.InterruptedSessionMarker,
) (bool, error) {
	if taskID == "" || sessionID == "" || expectedRecoveryToken == "" {
		return false, nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	changed, err := r.setTaskInterruptedSessionTx(ctx, tx, taskID, sessionID, marker, &taskInterruptionRecoveryGuard{
		updatedAt: expectedSessionUpdatedAt,
		token:     expectedRecoveryToken,
	})
	if err != nil || !changed {
		return changed, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

type taskInterruptionRecoveryGuard struct {
	updatedAt time.Time
	token     string
}

var errMalformedTaskInterruptionEntries = errors.New("malformed task interruption entries")

func (r *Repository) lockTaskInterruptionMetadata(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
) (map[string]json.RawMessage, bool, bool, error) {
	if err := r.lockTaskRowInTx(ctx, tx, taskID); err != nil {
		if errors.Is(err, ErrTaskNotFound) {
			return nil, false, false, nil
		}
		return nil, false, false, err
	}
	metadata, archived, err := readTaskInterruptionMetadata(ctx, tx, taskID)
	if errors.Is(err, ErrTaskNotFound) {
		return nil, false, false, nil
	}
	if err != nil {
		return nil, false, false, err
	}
	return metadata, true, archived, nil
}

func (r *Repository) setTaskInterruptedSessionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID, sessionID string,
	marker models.InterruptedSessionMarker,
	recovery *taskInterruptionRecoveryGuard,
) (bool, error) {
	if err := validateInterruptedSessionMarker(marker); err != nil {
		return false, err
	}
	metadata, found, archived, err := r.lockTaskInterruptionMetadata(ctx, tx, taskID)
	if err != nil || !found || archived {
		return false, err
	}
	entries, err := interruptedSessionEntries(metadata)
	if err != nil {
		return false, err
	}
	existing, exists := entries[sessionID]
	if sameInterruptionEpisode(existing, exists, marker) {
		return false, nil
	}
	preserveLegacyInterruption(metadata, entries)
	entries[sessionID] = marker
	if err := setInterruptedSessionEntries(metadata, entries); err != nil {
		return false, err
	}
	setTaskInterruptionProjection(metadata, entries)
	return r.writeTaskInterruptionMetadataGuarded(
		ctx, tx, taskID, metadata, r.nowUTC(), sessionID, marker.IncarnationID, recovery,
	)
}

func validateInterruptedSessionMarker(marker models.InterruptedSessionMarker) error {
	if marker.IncarnationID == "" || marker.Token == "" || marker.InterruptedAt == "" {
		return errors.New("task interruption identity and marker are required")
	}
	return nil
}

func sameInterruptionEpisode(existing models.InterruptedSessionMarker, exists bool, marker models.InterruptedSessionMarker) bool {
	return exists && existing.IncarnationID == marker.IncarnationID && existing.Token == marker.Token
}

func preserveLegacyInterruption(metadata map[string]json.RawMessage, entries map[string]models.InterruptedSessionMarker) {
	if len(entries) != 0 || interruptionMetadataString(metadata, models.MetaKeyLegacyInterruptedAt) != "" {
		return
	}
	legacy := interruptionMetadataString(metadata, models.MetaKeyInterruptedAt)
	if legacy != "" {
		metadata[models.MetaKeyLegacyInterruptedAt] = interruptionJSONString(legacy)
	}
}

// ResolveTaskInterruptedSession removes only captured scoped or legacy state.
func (r *Repository) ResolveTaskInterruptedSession(
	ctx context.Context,
	taskID, sessionID, incarnationID, token, legacyTimestamp string,
) (bool, error) {
	if !hasInterruptedTaskResolution(taskID, token, legacyTimestamp) {
		return false, nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	metadata, found, archived, err := r.lockTaskInterruptionMetadata(ctx, tx, taskID)
	if err != nil || !found || archived {
		return false, err
	}
	entries, err := interruptedSessionEntries(metadata)
	if err != nil {
		return false, err
	}
	changed := removeCapturedInterruption(metadata, entries, sessionID, incarnationID, token, legacyTimestamp)
	if !changed {
		return false, nil
	}
	written, err := r.persistResolvedInterruption(ctx, tx, taskID, metadata, entries, r.nowUTC())
	if err != nil || !written {
		return written, err
	}
	return true, nil
}

func hasInterruptedTaskResolution(taskID, token, legacyTimestamp string) bool {
	return taskID != "" && (token != "" || legacyTimestamp != "")
}

func removeCapturedInterruption(
	metadata map[string]json.RawMessage,
	entries map[string]models.InterruptedSessionMarker,
	sessionID, incarnationID, token, legacyTimestamp string,
) bool {
	changed := false
	if token != "" {
		entry, exists := entries[sessionID]
		if !exists || entry.IncarnationID != incarnationID || entry.Token != token {
			return false
		}
		delete(entries, sessionID)
		changed = true
	}
	if removeCapturedLegacyInterruption(metadata, entries, legacyTimestamp) {
		changed = true
	}
	return changed
}

func removeCapturedLegacyInterruption(
	metadata map[string]json.RawMessage,
	entries map[string]models.InterruptedSessionMarker,
	captured string,
) bool {
	if captured == "" {
		return false
	}
	legacy := interruptionMetadataString(metadata, models.MetaKeyLegacyInterruptedAt)
	if legacy != "" {
		if legacy != captured {
			return false
		}
		delete(metadata, models.MetaKeyLegacyInterruptedAt)
		return true
	}
	if len(entries) == 0 && interruptionMetadataString(metadata, models.MetaKeyInterruptedAt) == captured {
		delete(metadata, models.MetaKeyInterruptedAt)
		return true
	}
	return false
}

func (r *Repository) persistResolvedInterruption(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
	metadata map[string]json.RawMessage,
	entries map[string]models.InterruptedSessionMarker,
	updatedAt time.Time,
) (bool, error) {
	if len(entries) == 0 {
		delete(metadata, models.MetaKeyInterruptedSessions)
	} else if err := setInterruptedSessionEntries(metadata, entries); err != nil {
		return false, err
	}
	setTaskInterruptionProjection(metadata, entries)
	written, err := r.writeTaskInterruptionMetadataGuarded(ctx, tx, taskID, metadata, updatedAt, "", "", nil)
	if err != nil {
		return false, err
	}
	if !written {
		return false, nil
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Repository) removeTaskInterruptedSessionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID, sessionID, incarnationID string,
) (bool, error) {
	if taskID == "" || sessionID == "" || incarnationID == "" {
		return false, nil
	}
	metadata, found, _, err := r.lockTaskInterruptionMetadata(ctx, tx, taskID)
	if err != nil || !found {
		return false, err
	}
	entries, err := interruptedSessionEntries(metadata)
	if err != nil {
		return false, err
	}
	entry, exists := entries[sessionID]
	if !exists || entry.IncarnationID != incarnationID {
		return false, nil
	}
	delete(entries, sessionID)
	if len(entries) == 0 {
		delete(metadata, models.MetaKeyInterruptedSessions)
	} else {
		if err := setInterruptedSessionEntries(metadata, entries); err != nil {
			return false, err
		}
	}
	setTaskInterruptionProjection(metadata, entries)
	written, err := r.writeTaskInterruptionMetadataGuarded(ctx, tx, taskID, metadata, r.nowUTC(), "", "", nil)
	if err != nil {
		return false, err
	}
	return written, nil
}

func readTaskInterruptionMetadata(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
) (map[string]json.RawMessage, bool, error) {
	var raw string
	var archived bool
	err := tx.QueryRowContext(ctx, tx.Rebind(
		`SELECT COALESCE(metadata, '{}'), archived_at IS NOT NULL FROM tasks WHERE id = ?`,
	), taskID).Scan(&raw, &archived)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, ErrTaskNotFound
	}
	if err != nil {
		return nil, false, fmt.Errorf("read task interruption metadata: %w", err)
	}
	metadata := map[string]json.RawMessage{}
	if raw != "" && raw != "null" {
		if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
			return nil, false, fmt.Errorf("decode task interruption metadata: %w", err)
		}
	}
	if metadata == nil {
		metadata = map[string]json.RawMessage{}
	}
	return metadata, archived, nil
}

func interruptedSessionEntries(metadata map[string]json.RawMessage) (map[string]models.InterruptedSessionMarker, error) {
	entries := map[string]models.InterruptedSessionMarker{}
	value, ok := metadata[models.MetaKeyInterruptedSessions]
	if !ok || string(value) == "null" {
		return entries, nil
	}
	if err := json.Unmarshal(value, &entries); err != nil {
		return nil, fmt.Errorf("%w: decode task interruption entries: %v", errMalformedTaskInterruptionEntries, err)
	}
	if entries == nil {
		entries = map[string]models.InterruptedSessionMarker{}
	}
	return entries, nil
}

func setInterruptedSessionEntries(
	metadata map[string]json.RawMessage,
	entries map[string]models.InterruptedSessionMarker,
) error {
	encoded, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("encode task interruption entries: %w", err)
	}
	metadata[models.MetaKeyInterruptedSessions] = encoded
	return nil
}

func setTaskInterruptionProjection(
	metadata map[string]json.RawMessage,
	entries map[string]models.InterruptedSessionMarker,
) {
	latest := interruptionMetadataString(metadata, models.MetaKeyLegacyInterruptedAt)
	latestTime := interruptionTime(latest)
	for _, entry := range entries {
		candidate := entry.InterruptedAt
		candidateTime := interruptionTime(candidate)
		if latest == "" || candidateTime.After(latestTime) {
			latest = candidate
			latestTime = candidateTime
		}
	}
	if latest == "" {
		delete(metadata, models.MetaKeyInterruptedAt)
	} else {
		metadata[models.MetaKeyInterruptedAt] = interruptionJSONString(latest)
	}
}

func interruptionTime(value string) time.Time {
	parsed, _ := time.Parse(time.RFC3339Nano, value)
	return parsed
}

func interruptionMetadataString(metadata map[string]json.RawMessage, key string) string {
	var value string
	_ = json.Unmarshal(metadata[key], &value)
	return value
}

func interruptionJSONString(value string) json.RawMessage {
	encoded, _ := json.Marshal(value)
	return encoded
}

func (r *Repository) writeTaskInterruptionMetadataGuarded(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
	metadata map[string]json.RawMessage,
	updatedAt time.Time,
	sessionID string,
	sessionIncarnationID string,
	recovery *taskInterruptionRecoveryGuard,
) (bool, error) {
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return false, fmt.Errorf("encode task interruption metadata: %w", err)
	}
	query := `UPDATE tasks SET metadata = ?, updated_at = ? WHERE id = ?`
	args := []interface{}{string(encoded), updatedAt, taskID}
	if sessionID != "" {
		query += ` AND EXISTS (SELECT 1 FROM task_sessions s WHERE s.id = ? AND s.task_id = tasks.id AND s.queue_incarnation_id = ?)`
		args = append(args, sessionID, sessionIncarnationID)
	}
	if recovery != nil {
		var tokenPath string
		if dialect.IsPostgres(r.db.DriverName()) {
			tokenPath = models.SessionMetaKeyRecoverySettlementPending
			query += ` AND EXISTS (SELECT 1 FROM task_sessions s WHERE s.id = ? AND s.task_id = tasks.id AND s.state = ? AND s.updated_at = ? AND jsonb_extract_path_text(CASE WHEN s.metadata IS NULL OR s.metadata = 'null' OR s.metadata = '' THEN '{}'::jsonb ELSE s.metadata::jsonb END, ?, 'token') = ?)`
		} else {
			tokenPath = jsonPath(models.SessionMetaKeyRecoverySettlementPending + ".token")
			query += ` AND EXISTS (SELECT 1 FROM task_sessions s WHERE s.id = ? AND s.task_id = tasks.id AND s.state = ? AND s.updated_at = ? AND json_extract(CASE WHEN s.metadata IS NULL OR s.metadata = 'null' OR s.metadata = '' THEN '{}' ELSE s.metadata END, ?) = ?)`
		}
		args = append(args, sessionID, string(models.TaskSessionStateWaitingForInput), recovery.updatedAt)
		args = append(args, tokenPath, recovery.token)
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(query), args...)
	if err != nil {
		return false, fmt.Errorf("write task interruption metadata: %w", err)
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}
