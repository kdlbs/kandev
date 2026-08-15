package sqlite

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
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

// DetachActiveClarificationMessagesBySessionID atomically marks only pending,
// current-turn clarification rows as detached and returns the rows that changed.
func (r *Repository) DetachActiveClarificationMessagesBySessionID(
	ctx context.Context,
	sessionID string,
) ([]*models.Message, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin clarification detach: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	drv := r.db.DriverName()
	if err := lockSessionTurnWrites(ctx, tx, drv, sessionID); err != nil {
		return nil, err
	}
	updatedAt := time.Now().UTC()
	query := fmt.Sprintf(`
		UPDATE task_session_messages
		SET metadata = %s, updated_at = ?
		WHERE task_session_id = ?
		  AND type = 'clarification_request'
		  AND COALESCE(%s, '') IN ('', 'pending')
		  AND %s
		  AND turn_id = (
			SELECT id
			FROM task_session_turns
			WHERE task_session_id = task_session_messages.task_session_id
			ORDER BY started_at DESC, created_at DESC, id DESC
			LIMIT 1
		  )
		RETURNING id, task_session_id, task_id, turn_id, author_type, author_id,
		          content, requests_input, type, metadata, created_at, updated_at
	`, clarificationDetachedMetadataExpr(drv),
		dialect.JSONExtract(drv, "task_session_messages.metadata", "status"),
		clarificationNotDetachedPredicate(drv))
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(query), updatedAt, sessionID)
	if err != nil {
		return nil, fmt.Errorf("detach active clarification messages: %w", err)
	}
	messages, _, err := scanMessageRows(rows, 0)
	closeErr := rows.Close()
	if err != nil {
		return nil, fmt.Errorf("scan detached clarification messages: %w", err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close detached clarification rows: %w", closeErr)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit clarification detach: %w", err)
	}
	return messages, nil
}

// ExpireActiveClarificationBundle atomically expires one exact pending bundle
// only while it belongs to the session's current durable turn. The status and
// pending-ID predicates are evaluated by the UPDATE, so a stale expiry can
// never overwrite a concurrent answer or a newer bundle.
func (r *Repository) ExpireActiveClarificationBundle(
	ctx context.Context,
	sessionID, pendingID string,
) ([]*models.Message, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin clarification expiry: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	drv := r.db.DriverName()
	if err := lockSessionTurnWrites(ctx, tx, drv, sessionID); err != nil {
		return nil, err
	}
	updatedAt := time.Now().UTC()
	query := fmt.Sprintf(`
		UPDATE task_session_messages
		SET metadata = %s, updated_at = ?
		WHERE task_session_id = ?
		  AND type = 'clarification_request'
		  AND COALESCE(%s, '') IN ('', 'pending')
		  AND %s = ?
		  AND turn_id = (
			SELECT id
			FROM task_session_turns
			WHERE task_session_id = task_session_messages.task_session_id
			ORDER BY started_at DESC, created_at DESC, id DESC
			LIMIT 1
		  )
		RETURNING id, task_session_id, task_id, turn_id, author_type, author_id,
		          content, requests_input, type, metadata, created_at, updated_at
	`, clarificationExpiredMetadataExpr(drv),
		dialect.JSONExtract(drv, "task_session_messages.metadata", "status"),
		dialect.JSONExtract(drv, "task_session_messages.metadata", "pending_id"))
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(query), updatedAt, sessionID, pendingID)
	if err != nil {
		return nil, fmt.Errorf("expire active clarification messages: %w", err)
	}
	messages, _, err := scanMessageRows(rows, 0)
	closeErr := rows.Close()
	if err != nil {
		return nil, fmt.Errorf("scan expired clarification messages: %w", err)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close expired clarification rows: %w", closeErr)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit clarification expiry: %w", err)
	}
	return messages, nil
}

func clarificationDetachedMetadataExpr(driverName string) string {
	if dialect.IsPostgres(driverName) {
		return "jsonb_set(metadata::jsonb, '{agent_disconnected}', 'true'::jsonb)::text"
	}
	return "json_set(metadata, '$.agent_disconnected', json('true'))"
}

func clarificationExpiredMetadataExpr(driverName string) string {
	if dialect.IsPostgres(driverName) {
		return "jsonb_set(jsonb_set(metadata::jsonb, '{agent_disconnected}', 'true'::jsonb), " +
			"'{status}', '\"expired\"'::jsonb)::text"
	}
	return "json_set(metadata, '$.agent_disconnected', json('true'), '$.status', 'expired')"
}

func clarificationNotDetachedPredicate(driverName string) string {
	value := dialect.JSONExtract(driverName, "task_session_messages.metadata", "agent_disconnected")
	if dialect.IsPostgres(driverName) {
		return fmt.Sprintf("COALESCE(%s, '') NOT IN ('true', '1')", value)
	}
	return fmt.Sprintf("COALESCE(%s, 0) != 1", value)
}

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
	if err := r.lockClarificationBundleTurnWrites(ctx, tx, drv, pendingID); err != nil {
		return nil, false, err
	}
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
	drv := r.db.DriverName()
	if err := r.lockClarificationBundleTurnWrites(ctx, tx, drv, pendingID); err != nil {
		return false, err
	}
	messages, err := r.loadRestorableClarificationBundle(ctx, tx, drv, pendingID)
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

func (r *Repository) lockClarificationBundleTurnWrites(
	ctx context.Context,
	tx *sqlx.Tx,
	driverName, pendingID string,
) error {
	if !dialect.IsPostgres(driverName) {
		return nil
	}
	pendingIDExpr := dialect.JSONExtract(driverName, "metadata", "pending_id")
	query := fmt.Sprintf(`
		SELECT DISTINCT task_session_id
		FROM task_session_messages
		WHERE type = 'clarification_request'
		  AND %s = ?
	`, pendingIDExpr)
	var sessionIDs []string
	if err := tx.SelectContext(ctx, &sessionIDs, r.db.Rebind(query), pendingID); err != nil {
		return fmt.Errorf("load clarification bundle sessions: %w", err)
	}
	return lockSessionTurnWrites(ctx, tx, driverName, sessionIDs...)
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
	// A pending ID spanning message types, sessions, or turns is malformed. The
	// NOT EXISTS guard intentionally makes the whole bundle ineligible to restore.
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
		  AND turn_id = (
			SELECT id
			FROM task_session_turns
			WHERE task_session_id = task_session_messages.task_session_id
			ORDER BY started_at DESC, created_at DESC, id DESC
			LIMIT 1
		  )
	`, statusExpr))
	for _, message := range messages {
		restoredMetadata := maps.Clone(message.Metadata)
		restoredMetadata["status"] = clarificationStatusPending
		delete(restoredMetadata, "response")
		metadataJSON, marshalErr := json.Marshal(restoredMetadata)
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
	// A pending ID spanning message types, sessions, or turns is malformed. The
	// NOT EXISTS guard intentionally makes the whole bundle ineligible to claim.
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
