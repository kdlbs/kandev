package sqlite

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// updateMessageWithPayloadGuard takes the writer before reading the tombstone.
// A stale replacement may update display status, but cannot change the retained
// tool identity, normalized summary, or removal receipt.
func (r *Repository) updateMessageWithPayloadGuard(ctx context.Context, message *models.Message, metadataJSON []byte, requestsInput int) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE task_session_messages SET id = id WHERE id = ?`), message.ID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("message not found: %s", message.ID)
	}
	var raw string
	var storedType models.MessageType
	var storedUpdatedAt time.Time
	if err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT metadata,type,updated_at FROM task_session_messages WHERE id = ?`), message.ID).Scan(&raw, &storedType, &storedUpdatedAt); err != nil {
		return err
	}
	metadataJSON, sanitized, err := mergeRetainedMessageMetadata([]byte(raw), metadataJSON)
	if err != nil {
		return err
	}
	nextType := message.Type
	if sanitized != nil {
		nextType = storedType
	}
	updatedAt := message.UpdatedAt
	if sanitized != nil {
		updatedAt = storedUpdatedAt
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE task_session_messages SET content = ?, requests_input = ?, type = ?, metadata = ?, updated_at = ? WHERE id = ?`), message.Content, requestsInput, string(nextType), string(metadataJSON), updatedAt, message.ID)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if sanitized != nil {
		message.Metadata = sanitized
		message.Type = nextType
		message.UpdatedAt = storedUpdatedAt
	}
	return nil
}

// insertMessageWithPayloadGuard prevents fallback-create and repeated start
// frames from giving a removed tool call a fresh row identity.
func (r *Repository) insertMessageWithPayloadGuard(ctx context.Context, message *models.Message, requestsInput int, messageType, metadataJSON string) error {
	toolCallID, _ := message.Metadata["tool_call_id"].(string)
	if toolCallID == "" || messageType == string(models.MessageTypePermissionRequest) {
		return r.insertMessageRow(ctx, r.db, message, requestsInput, messageType, metadataJSON)
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE task_sessions SET id = id WHERE id = ?`), message.TaskSessionID); err != nil {
		return err
	}
	var removed bool
	err = tx.QueryRowContext(ctx, tx.Rebind(`SELECT EXISTS(SELECT 1 FROM task_session_messages WHERE task_session_id = ? AND json_extract(metadata, '$.tool_call_id') = ? AND type != 'permission_request' AND json_type(metadata, '$.payload_retention') IS NOT NULL)`), message.TaskSessionID, toolCallID).Scan(&removed)
	if err != nil {
		return err
	}
	if removed {
		return fmt.Errorf("tool_payload_removed: tool call already retained")
	}
	if err := r.insertMessageRow(ctx, tx, message, requestsInput, messageType, metadataJSON); err != nil {
		return err
	}
	return tx.Commit()
}

func mergeRetainedMessageMetadata(stored, incoming []byte) ([]byte, map[string]any, error) {
	var current map[string]json.RawMessage
	if err := json.Unmarshal(stored, &current); err != nil {
		return nil, nil, fmt.Errorf("decode stored message metadata: %w", err)
	}
	if _, removed := current["payload_retention"]; !removed {
		return incoming, nil, nil
	}
	var replacement map[string]json.RawMessage
	if err := json.Unmarshal(incoming, &replacement); err != nil {
		return nil, nil, err
	}
	for _, key := range []string{"status", "title"} {
		if value, ok := replacement[key]; ok {
			current[key] = value
		}
	}
	encoded, err := json.Marshal(current)
	if err != nil {
		return nil, nil, err
	}
	var sanitized map[string]any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&sanitized); err != nil {
		return nil, nil, err
	}
	return encoded, sanitized, nil
}
