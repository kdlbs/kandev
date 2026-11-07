package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// sendOperationIndexName is the unique index that decides which concurrent
// first claim owns a delivery. Naming it explicitly keeps
// isSendOperationUniqueViolation attributable to this constraint specifically
// rather than to any unique violation, mirroring isExternalIDUniqueViolation.
const sendOperationIndexName = "uniq_task_message_send_operations_identity"

// sqliteSendOperationViolationMessage is the column-list form go-sqlite3 puts
// in a UNIQUE-constraint error for this index. SQLite exposes no typed access to
// the violated index's name, only the failing index's column list; that triple
// appears on no other index in this table, so matching it attributes the
// violation correctly where a bare "UNIQUE constraint failed" match would also
// fire on the primary key.
const sqliteSendOperationViolationMessage = "UNIQUE constraint failed: task_message_send_operations.sender_task_id, task_message_send_operations.sender_session_id, task_message_send_operations.operation_id"

// sendOperationRetention is how long a settled operation record survives so a
// caller that lost a response can still read its outcome. It is far longer than
// any plausible retry interval and bounds the table's growth on a long-lived
// install.
const sendOperationRetention = 7 * 24 * time.Hour

// sendOperationStalePending is how long a row may stay pending before the age
// sweep settles it as failed/outcome_unknown. A pending row means "may still
// commit", so this must be long enough that no live dispatch outlives it; the
// sweep exists so an abandoned claim cannot pin a row forever.
const sendOperationStalePending = 24 * time.Hour

// SendOperationPruneBatch bounds one retention batch. Pruning runs
// opportunistically off the send path under maintenance admission, so the
// batch is small enough to stay inside that path's latency budget and to leave
// the rest of the backlog for later sends.
const SendOperationPruneBatch = 200

const sendOperationColumns = `id, operation_id, sender_task_id, sender_session_id,
	target_task_id, target_session_id, requested_prompt, requested_delivery_mode,
	requested_session_id, requested_question_id, state, delivery_status, message_id, queued_entry_id,
	failure_code, created_at, updated_at, settled_at`

// sendOperationIdentityColumn matches the byte-wise identity-index comparison.
func (r *Repository) sendOperationIdentityColumn() string {
	if dialect.IsPostgres(r.db.DriverName()) {
		return `operation_id COLLATE "C"`
	}
	return "operation_id COLLATE BINARY"
}

// isSendOperationUniqueViolation reports whether err is a violation of
// uniq_task_message_send_operations_identity specifically. On PostgreSQL it
// inspects the typed pgconn.PgError's constraint name; on SQLite it matches the
// column-list message documented above. Classifying by constraint rather than by
// message text is what keeps this correct under PostgreSQL's identifier-length
// truncation.
func isSendOperationUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && pgErr.ConstraintName == sendOperationIndexName
	}
	return strings.Contains(err.Error(), sqliteSendOperationViolationMessage)
}

// ClaimSendOperation inserts the pending row that owns this delivery. The
// insert commits on its own, before any delivery side effect, so an
// interrupted or timed-out caller always has something to read back. It
// returns models.ErrSendOperationNotClaimed when the identity is already held,
// which is the replay path rather than an error to retry blindly.
//
// No digest or hash of the request is stored: RequestedPrompt and its siblings
// are the request, kept verbatim so a replay can be compared by value.
func (r *Repository) ClaimSendOperation(ctx context.Context, operation *models.TaskMessageSendOperation) error {
	if operation == nil {
		return fmt.Errorf("send operation is nil")
	}
	now := time.Now().UTC()
	operation.State = models.SendOperationStatePending
	operation.CreatedAt = now
	operation.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_message_send_operations (
			id, operation_id, sender_task_id, sender_session_id,
			target_task_id, target_session_id,
			requested_prompt, requested_delivery_mode, requested_session_id, requested_question_id,
			state, delivery_status, message_id, queued_entry_id, failure_code,
			created_at, updated_at, settled_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', '', '', '', ?, ?, NULL)
	`),
		operation.ID,
		operation.OperationID,
		operation.SenderTaskID,
		operation.SenderSessionID,
		operation.TargetTaskID,
		operation.TargetSessionID,
		operation.RequestedPrompt,
		operation.RequestedDeliveryMode,
		operation.RequestedSessionID,
		operation.RequestedQuestionID,
		operation.State,
		now,
		now,
	)
	if isSendOperationUniqueViolation(err) {
		return models.ErrSendOperationNotClaimed
	}
	return err
}

// GetSendOperation reads the operation claimed by this exact sender pair. The
// sender pair is part of the lookup rather than a filter applied afterwards, so
// another session's identity is reported as models.ErrSendOperationNotFound and
// its existence never leaks.
func (r *Repository) GetSendOperation(
	ctx context.Context, senderTaskID, senderSessionID, operationID string,
) (*models.TaskMessageSendOperation, error) {
	operation := &models.TaskMessageSendOperation{}
	var settledAt sql.NullTime

	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT `+sendOperationColumns+`
		  FROM task_message_send_operations
		 WHERE sender_task_id = ? AND sender_session_id = ? AND `+r.sendOperationIdentityColumn()+` = ?
	`), senderTaskID, senderSessionID, operationID)
	err := row.Scan(
		&operation.ID, &operation.OperationID, &operation.SenderTaskID, &operation.SenderSessionID,
		&operation.TargetTaskID, &operation.TargetSessionID,
		&operation.RequestedPrompt, &operation.RequestedDeliveryMode, &operation.RequestedSessionID, &operation.RequestedQuestionID,
		&operation.State, &operation.DeliveryStatus, &operation.MessageID, &operation.QueuedEntryID,
		&operation.FailureCode, &operation.CreatedAt, &operation.UpdatedAt, &settledAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, models.ErrSendOperationNotFound
	}
	if err != nil {
		return nil, err
	}
	if settledAt.Valid {
		settled := settledAt.Time
		operation.SettledAt = &settled
	}
	return operation, nil
}

// SettleSendOperation records the outcome of the delivery this call owned. The
// update is guarded by state = 'pending' so a late or duplicate settle cannot
// rewrite a terminal outcome, and it reports whether a row was updated.
//
// A caller whose process died before settling leaves the row pending, which
// readback reports as still in flight: the record never lies in the unsafe
// direction.
func (r *Repository) SettleSendOperation(
	ctx context.Context,
	senderTaskID, senderSessionID, operationID string,
	state, targetSessionID, deliveryStatus, messageID, queuedEntryID, failureCode string,
) (bool, error) {
	settledAt := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_message_send_operations
		   SET state = ?,
		       target_session_id = ?,
		       delivery_status = ?,
		       message_id = ?,
		       queued_entry_id = ?,
		       failure_code = ?,
		       settled_at = ?,
		       updated_at = ?
		 WHERE sender_task_id = ? AND sender_session_id = ? AND `+r.sendOperationIdentityColumn()+` = ?
		   AND state = ?
	`),
		state, targetSessionID, deliveryStatus, messageID, queuedEntryID, failureCode, settledAt, settledAt,
		senderTaskID, senderSessionID, operationID, models.SendOperationStatePending,
	)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows > 0, err
}

// PruneSendOperations settles abandoned pending rows as failed/outcome_unknown
// and deletes settled rows older than the retention window, both in bounded
// batches. It returns how many rows it settled and removed.
//
// Callers run it opportunistically under maintenance admission (see
// internal/system/maintenance): it is retention, not correctness, so a busy
// lease means the batch is simply deferred to a later send rather than waited
// on. Nothing here can block or fail a live send.
func (r *Repository) PruneSendOperations(ctx context.Context, limit int) (settled int, removed int, err error) {
	if limit <= 0 {
		return 0, 0, nil
	}
	now := time.Now().UTC()
	staleBefore := now.Add(-sendOperationStalePending)
	retentionBefore := now.Add(-sendOperationRetention)

	staleResult, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_message_send_operations
		   SET state = ?, failure_code = ?, settled_at = ?, updated_at = ?
		 WHERE id IN (
			SELECT id FROM task_message_send_operations
			 WHERE state = ? AND created_at < ?
			 ORDER BY created_at ASC
			 LIMIT ?
		 )
	`),
		models.SendOperationStateFailed, models.SendOperationFailureOutcomeUnknown, now, now,
		models.SendOperationStatePending, staleBefore, limit,
	)
	if err != nil {
		return 0, 0, err
	}
	staleRows, err := staleResult.RowsAffected()
	if err != nil {
		return 0, 0, err
	}

	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM task_message_send_operations
		 WHERE id IN (
			SELECT id FROM task_message_send_operations
			 WHERE state <> ? AND created_at < ?
			 ORDER BY created_at ASC
			 LIMIT ?
		 )
	`),
		models.SendOperationStatePending, retentionBefore, limit,
	)
	if err != nil {
		return 0, 0, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return int(staleRows), 0, err
	}
	return int(staleRows), int(rows), nil
}
