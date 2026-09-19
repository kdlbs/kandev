package sqlite

import (
	"bytes"
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

// updateMessageWithPayloadGuard serializes the metadata read with a concurrent
// replacement. A stale replacement may update display status, but cannot change
// the retained tool identity, normalized summary, or removal receipt.
func (r *Repository) updateMessageWithPayloadGuard(ctx context.Context, message *models.Message, metadataJSON []byte, requestsInput int) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.updateMessageWithPayloadGuardTx(ctx, tx, message, metadataJSON, requestsInput); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) updateMessageWithPayloadGuardTx(ctx context.Context, tx *sqlx.Tx, message *models.Message, metadataJSON []byte, requestsInput int) error {
	if !dialect.IsPostgres(r.db.DriverName()) {
		// SQLite has a database-level writer lock, and this no-op update takes it
		// before the retained metadata is read. PostgreSQL uses FOR UPDATE below;
		// an UPDATE here would fire the conversation revision trigger.
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
	}
	query := `SELECT metadata,type,updated_at FROM task_session_messages WHERE id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += forUpdateClause
	}
	var raw string
	var storedType models.MessageType
	var storedUpdatedAt time.Time
	if err := tx.QueryRowContext(ctx, tx.Rebind(query), message.ID).Scan(&raw, &storedType, &storedUpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) && dialect.IsPostgres(r.db.DriverName()) {
			return fmt.Errorf("message not found: %s", message.ID)
		}
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
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.insertMessageWithPayloadGuardTx(ctx, tx, message, requestsInput, messageType, metadataJSON); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *Repository) insertMessageWithPayloadGuardTx(ctx context.Context, tx *sqlx.Tx, message *models.Message, requestsInput int, messageType, metadataJSON string) error {
	toolCallID, _ := message.Metadata["tool_call_id"].(string)
	if toolCallID == "" || messageType == string(models.MessageTypePermissionRequest) {
		return r.insertMessageRow(ctx, tx, message, requestsInput, messageType, metadataJSON)
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE task_sessions SET id = id WHERE id = ?`), message.TaskSessionID); err != nil {
		return err
	}
	var removed bool
	driver := r.db.DriverName()
	query := fmt.Sprintf(`SELECT EXISTS(SELECT 1 FROM task_session_messages WHERE task_session_id = ? AND %s = ? AND type != 'permission_request' AND %s)`,
		dialect.JSONExtract(driver, "metadata", "tool_call_id"),
		dialect.JSONExtractIsNotNull(driver, "metadata", "payload_retention"),
	)
	err := tx.QueryRowContext(ctx, tx.Rebind(query), message.TaskSessionID, toolCallID).Scan(&removed)
	if err != nil {
		return err
	}
	if removed {
		return fmt.Errorf("tool_payload_removed: tool call already retained")
	}
	if err := r.insertMessageRow(ctx, tx, message, requestsInput, messageType, metadataJSON); err != nil {
		return err
	}
	return nil
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
