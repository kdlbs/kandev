package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	clarificationStatusAnswered = "answered"
	clarificationStatusPending  = "pending"
	clarificationStatusRejected = "rejected"
	// clarificationStatusResponding is an intra-transaction claim marker.
	// CompleteActiveClarificationBundle never commits it independently.
	clarificationStatusResponding = "responding"
)

// CompleteActiveClarificationBundle atomically claims a current-turn pending
// bundle and persists its terminal state. Exactly one concurrent responder can
// transition the rows; superseded or already-terminal bundles return claimed=false.
func (r *Repository) CompleteActiveClarificationBundle(
	ctx context.Context,
	pendingID, status string,
	responses map[string]interface{},
) ([]*models.Message, bool, error) {
	if status != clarificationStatusAnswered && status != clarificationStatusRejected {
		return nil, false, fmt.Errorf("invalid clarification terminal status %q", status)
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()

	drv := r.db.DriverName()
	claimedRows, err := r.claimActiveClarificationBundle(ctx, tx, drv, pendingID)
	if err != nil {
		return nil, false, err
	}
	if claimedRows == 0 {
		return nil, false, nil
	}
	messages, err := r.loadClaimedClarificationBundle(ctx, tx, drv, pendingID)
	if err != nil {
		return nil, false, err
	}
	if int64(len(messages)) != claimedRows {
		return nil, false, fmt.Errorf("claimed %d clarification rows but loaded %d", claimedRows, len(messages))
	}
	if err := r.completeClaimedClarificationMessages(ctx, tx, drv, messages, status, responses); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit clarification bundle completion: %w", err)
	}
	return messages, true, nil
}

// RestoreActiveClarificationBundle reopens a current-turn terminal bundle when
// its detached resume event could not be published. The status check makes the
// rollback idempotent and prevents an older turn from becoming active again.
func (r *Repository) RestoreActiveClarificationBundle(
	ctx context.Context,
	pendingID, terminalStatus string,
	claimedMessages []*models.Message,
) (bool, error) {
	if terminalStatus != clarificationStatusAnswered && terminalStatus != clarificationStatusRejected {
		return false, fmt.Errorf("invalid clarification terminal status %q", terminalStatus)
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()

	claimIDs, err := clarificationClaimIDs(claimedMessages)
	if err != nil {
		return false, err
	}
	messages, err := r.loadRestorableClarificationBundle(ctx, tx, r.db.DriverName(), pendingID)
	if err != nil {
		return false, err
	}
	restorable := make([]*models.Message, 0, len(claimIDs))
	for _, message := range messages {
		if _, claimed := claimIDs[message.ID]; !claimed {
			continue
		}
		if status, _ := message.Metadata["status"].(string); status != terminalStatus {
			return false, nil
		}
		restorable = append(restorable, message)
	}
	if len(restorable) != len(claimIDs) {
		return false, nil
	}
	if err := r.restoreClarificationMessages(
		ctx,
		tx,
		r.db.DriverName(),
		restorable,
		terminalStatus,
	); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit clarification bundle restore: %w", err)
	}
	return true, nil
}

func clarificationClaimIDs(messages []*models.Message) (map[string]struct{}, error) {
	if len(messages) == 0 {
		return nil, fmt.Errorf("clarification restore requires claimed messages")
	}
	ids := make(map[string]struct{}, len(messages))
	for _, message := range messages {
		if message == nil || message.ID == "" {
			return nil, fmt.Errorf("clarification restore received an empty message id")
		}
		if _, duplicate := ids[message.ID]; duplicate {
			return nil, fmt.Errorf("clarification restore received duplicate message %s", message.ID)
		}
		ids[message.ID] = struct{}{}
	}
	return ids, nil
}

func (r *Repository) loadRestorableClarificationBundle(
	ctx context.Context,
	tx *sqlx.Tx,
	drv, pendingID string,
) ([]*models.Message, error) {
	pendingIDExpr := dialect.JSONExtract(drv, "m.metadata", "pending_id")
	bundlePendingIDExpr := dialect.JSONExtract(drv, "bundle.metadata", "pending_id")
	query := fmt.Sprintf(`
		SELECT m.id, m.task_session_id, m.task_id, m.turn_id, m.author_type, m.author_id,
		       m.content, m.requests_input, m.type, m.metadata, m.created_at, m.updated_at
		FROM task_session_messages m
		WHERE %s = ?
		  AND m.type = 'clarification_request'
		  AND m.turn_id = (
			SELECT id
			FROM task_session_turns
			WHERE task_session_id = m.task_session_id
			ORDER BY started_at DESC, created_at DESC, id DESC
			LIMIT 1
		  )
		  AND NOT EXISTS (
			SELECT 1
			FROM task_session_messages bundle
			WHERE %s = ?
			  AND (
				bundle.type != 'clarification_request'
				OR bundle.task_session_id != m.task_session_id
				OR bundle.turn_id != m.turn_id
			  )
		  )
		ORDER BY m.created_at ASC, m.id ASC
	`, pendingIDExpr, bundlePendingIDExpr)
	rows, err := tx.QueryxContext(
		ctx,
		r.db.Rebind(query),
		pendingID,
		pendingID,
	)
	if err != nil {
		return nil, fmt.Errorf("load restorable clarification bundle: %w", err)
	}
	defer func() { _ = rows.Close() }()
	messages, _, scanErr := scanMessageRows(rows, 0)
	if scanErr != nil {
		return nil, fmt.Errorf("scan restorable clarification bundle: %w", scanErr)
	}
	return messages, nil
}

func (r *Repository) restoreClarificationMessages(
	ctx context.Context,
	tx *sqlx.Tx,
	drv string,
	messages []*models.Message,
	terminalStatus string,
) error {
	updatedAt := time.Now().UTC()
	statusExpr := dialect.JSONExtract(drv, "metadata", "status")
	updateQuery := r.db.Rebind(fmt.Sprintf(`
		UPDATE task_session_messages
		SET metadata = ?, updated_at = ?
		WHERE id = ? AND %s = ?
	`, statusExpr))
	for _, message := range messages {
		message.Metadata["status"] = clarificationStatusPending
		delete(message.Metadata, "response")
		metadataJSON, marshalErr := json.Marshal(message.Metadata)
		if marshalErr != nil {
			return fmt.Errorf("marshal clarification message %s for restore: %w", message.ID, marshalErr)
		}
		result, updateErr := tx.ExecContext(
			ctx,
			updateQuery,
			string(metadataJSON),
			updatedAt,
			message.ID,
			terminalStatus,
		)
		if updateErr != nil {
			return fmt.Errorf("restore clarification message %s: %w", message.ID, updateErr)
		}
		updatedRows, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return fmt.Errorf("count restored clarification message %s: %w", message.ID, rowsErr)
		}
		if updatedRows != 1 {
			return fmt.Errorf("clarification message %s lost its terminal claim", message.ID)
		}
	}
	return nil
}

func (r *Repository) claimActiveClarificationBundle(
	ctx context.Context,
	tx *sqlx.Tx,
	drv, pendingID string,
) (int64, error) {
	pendingIDExpr := dialect.JSONExtract(drv, "task_session_messages.metadata", "pending_id")
	statusExpr := dialect.JSONExtract(drv, "task_session_messages.metadata", "status")
	bundlePendingIDExpr := dialect.JSONExtract(drv, "bundle.metadata", "pending_id")
	claimQuery := fmt.Sprintf(`
		UPDATE task_session_messages
		SET metadata = %s, updated_at = CURRENT_TIMESTAMP
		WHERE %s = ?
		  AND type = 'clarification_request'
		  AND COALESCE(%s, '') IN ('', 'pending')
		  AND turn_id = (
			SELECT id
			FROM task_session_turns
			WHERE task_session_id = task_session_messages.task_session_id
			ORDER BY started_at DESC, created_at DESC, id DESC
			LIMIT 1
		  )
		  AND NOT EXISTS (
			SELECT 1
			FROM task_session_messages bundle
			WHERE %s = ?
			  AND (
				bundle.type != 'clarification_request'
				OR bundle.task_session_id != task_session_messages.task_session_id
				OR bundle.turn_id != task_session_messages.turn_id
			  )
		  )
	`, dialect.JSONSet(drv, "metadata", "status", clarificationStatusResponding), pendingIDExpr, statusExpr, bundlePendingIDExpr)
	result, err := tx.ExecContext(ctx, r.db.Rebind(claimQuery), pendingID, pendingID)
	if err != nil {
		return 0, fmt.Errorf("claim active clarification bundle: %w", err)
	}
	claimedRows, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count claimed clarification rows: %w", err)
	}
	return claimedRows, nil
}

func (r *Repository) loadClaimedClarificationBundle(
	ctx context.Context,
	tx *sqlx.Tx,
	drv, pendingID string,
) ([]*models.Message, error) {
	claimedStatusExpr := dialect.JSONExtract(drv, "metadata", "status")
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(fmt.Sprintf(`
		SELECT id, task_session_id, task_id, turn_id, author_type, author_id,
		       content, requests_input, type, metadata, created_at, updated_at
		FROM task_session_messages
		WHERE %s = ? AND %s = ?
		ORDER BY created_at ASC, id ASC
	`, dialect.JSONExtract(drv, "metadata", "pending_id"), claimedStatusExpr)), pendingID, clarificationStatusResponding)
	if err != nil {
		return nil, fmt.Errorf("load claimed clarification bundle: %w", err)
	}
	defer func() { _ = rows.Close() }()
	messages, _, scanErr := scanMessageRows(rows, 0)
	if scanErr != nil {
		return nil, fmt.Errorf("scan claimed clarification bundle: %w", scanErr)
	}
	return messages, nil
}

func (r *Repository) completeClaimedClarificationMessages(
	ctx context.Context,
	tx *sqlx.Tx,
	drv string,
	messages []*models.Message,
	status string,
	responses map[string]interface{},
) error {
	updatedAt := time.Now().UTC()
	claimedStatusExpr := dialect.JSONExtract(drv, "metadata", "status")
	updateQuery := r.db.Rebind(fmt.Sprintf(`
		UPDATE task_session_messages
		SET metadata = ?, updated_at = ?
		WHERE id = ? AND %s = ?
	`, claimedStatusExpr))
	for _, message := range messages {
		questionID, _ := message.Metadata["question_id"].(string)
		if questionID == "" {
			return fmt.Errorf("clarification message %s is missing question_id", message.ID)
		}
		message.Metadata["status"] = status
		if status == clarificationStatusAnswered {
			response, ok := responses[questionID]
			if !ok {
				return fmt.Errorf("missing response for clarification question %s", questionID)
			}
			message.Metadata["response"] = response
		}
		metadataJSON, marshalErr := json.Marshal(message.Metadata)
		if marshalErr != nil {
			return fmt.Errorf("marshal clarification message %s: %w", message.ID, marshalErr)
		}
		updateResult, updateErr := tx.ExecContext(
			ctx,
			updateQuery,
			string(metadataJSON), updatedAt, message.ID, clarificationStatusResponding,
		)
		if updateErr != nil {
			return fmt.Errorf("complete clarification message %s: %w", message.ID, updateErr)
		}
		updatedRows, rowsErr := updateResult.RowsAffected()
		if rowsErr != nil {
			return fmt.Errorf("count completed clarification message %s: %w", message.ID, rowsErr)
		}
		if updatedRows != 1 {
			return fmt.Errorf("clarification message %s lost its claim", message.ID)
		}
		message.UpdatedAt = updatedAt
	}
	return nil
}
