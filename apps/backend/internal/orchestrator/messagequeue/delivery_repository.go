package messagequeue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/adminmetrics"
)

// DeliveryLedger persists sender-independent cross-task delivery work. Queue
// admission is intentionally a later step: a full target queue cannot erase a
// receipt that was already accepted from the source turn.
type DeliveryLedger interface {
	CreateOrGetDelivery(ctx context.Context, delivery Delivery) (*Delivery, bool, error)
	GetDelivery(ctx context.Context, deliveryID string) (*Delivery, error)
	ClaimDueDeliveries(ctx context.Context, now time.Time, leaseOwner string, leaseDuration time.Duration, limit int) ([]Delivery, error)
	RescheduleDelivery(ctx context.Context, deliveryID, leaseOwner string, nextAttemptAt time.Time, lastError string) (*Delivery, error)
	MarkDeliveryQueued(ctx context.Context, deliveryID, leaseOwner, queueEntryID string) (*Delivery, error)
	AcknowledgeDelivery(ctx context.Context, deliveryID, queueEntryID string, deliveredAt time.Time) (*Delivery, error)
	AcknowledgeDeliveryByQueueEntry(ctx context.Context, queueEntryID string, deliveredAt time.Time) (*Delivery, error)
	MarkDeliveryRecoverable(ctx context.Context, deliveryID, leaseOwner, lastError string) (*Delivery, error)
	RetryDelivery(ctx context.Context, deliveryID string, nextAttemptAt time.Time) (*Delivery, error)
}

// RetryDelivery explicitly reopens a retained recovery receipt. Delivered,
// cancelled, and terminal rows cannot be resurrected by an operator retry.
func (r *sqliteRepository) RetryDelivery(ctx context.Context, deliveryID string, nextAttemptAt time.Time) (*Delivery, error) {
	if nextAttemptAt.IsZero() {
		nextAttemptAt = time.Now().UTC()
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE message_deliveries
		SET state = ?, next_attempt_at = ?, last_error = '', lease_owner = NULL, lease_expires_at = NULL, updated_at = ?
		WHERE id = ? AND state = ?
	`), DeliveryPendingCapacity, nextAttemptAt, time.Now().UTC(), deliveryID, DeliveryRecoverable)
	if err != nil {
		return nil, fmt.Errorf("retry delivery: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, ErrEntryNotFound
	}
	return r.GetDelivery(ctx, deliveryID)
}

// deliveryQueueAcknowledger commits FIFO removal and its delivery-ledger
// terminal transition together. It is intentionally optional so older test
// repositories retain their legacy queue behavior, while the production SQL
// repository never leaves a queued receipt behind after deleting its only
// queue entry.
type deliveryQueueAcknowledger interface {
	AcknowledgeQueueEntryAndDelivery(ctx context.Context, sessionID, queueEntryID string, deliveredAt time.Time) error
}

// AcknowledgeQueueEntryAndDelivery atomically deletes a reserved FIFO entry
// and records its durable receipt as delivered. The transaction is the commit
// point after executor acceptance: a crash or injected failure rolls back both
// mutations, so retrying the acknowledgement cannot lose or duplicate work.
func (r *sqliteRepository) AcknowledgeQueueEntryAndDelivery(
	ctx context.Context, sessionID, queueEntryID string, deliveredAt time.Time,
) error {
	if deliveredAt.IsZero() {
		deliveredAt = time.Now().UTC()
	}
	unlock := r.withSessionLock(sessionID)
	defer unlock()
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delivery queue acknowledgement: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockSessionTx(ctx, tx, sessionID); err != nil {
		return err
	}

	// Not every queued entry has a delivery receipt. Preserve the ordinary
	// acknowledgement behavior while requiring a receipt transition whenever
	// a durable delivery owns this exact entry.
	var deliveryID string
	err = tx.GetContext(ctx, &deliveryID, r.db.Rebind(`
		SELECT id FROM message_deliveries WHERE queue_entry_id = ?
	`), queueEntryID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("load delivery queue acknowledgement: %w", err)
	}
	if err == nil {
		result, updateErr := tx.ExecContext(ctx, r.db.Rebind(`
			UPDATE message_deliveries
			SET state = ?, delivered_at = ?, updated_at = ?
			WHERE id = ? AND queue_entry_id = ? AND state = ?
		`), DeliveryDelivered, deliveredAt, deliveredAt, deliveryID, queueEntryID, DeliveryQueued)
		if updateErr != nil {
			return fmt.Errorf("acknowledge delivery in queue transaction: %w", updateErr)
		}
		affected, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return rowsErr
		}
		if affected != 1 {
			return ErrEntryNotFound
		}
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		DELETE FROM queued_messages WHERE id = ? AND session_id = ?
	`), queueEntryID, sessionID)
	if err != nil {
		return fmt.Errorf("acknowledge queued delivery entry: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrEntryNotFound
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delivery queue acknowledgement: %w", err)
	}
	if deliveryID != "" {
		adminmetrics.RecordMessageDeliveryOutcome(string(DeliveryDelivered), 1)
	}
	return nil
}

// CreateOrGetDelivery atomically creates a source-turn idempotency receipt or
// returns the existing immutable receipt for a repeated send.
func (r *sqliteRepository) CreateOrGetDelivery(ctx context.Context, delivery Delivery) (*Delivery, bool, error) {
	if err := validateDeliveryIdentity(delivery); err != nil {
		return nil, false, err
	}
	prepareNewDelivery(&delivery)
	metadataJSON, err := json.Marshal(delivery.Metadata)
	if err != nil {
		return nil, false, fmt.Errorf("marshal delivery metadata: %w", err)
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO message_deliveries (
			id, sender_task_id, sender_session_id, source_turn_id, idempotency_key,
			target_task_id, target_session_id, delivery_mode, content, metadata_json,
			state, queue_entry_id, attempts, next_attempt_at, last_error, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', 0, ?, '', ?, ?)
		ON CONFLICT(sender_session_id, source_turn_id, idempotency_key) DO NOTHING
	`), delivery.ID, delivery.SenderTaskID, delivery.SenderSessionID, delivery.SourceTurnID,
		delivery.IdempotencyKey, delivery.TargetTaskID, delivery.TargetSessionID, delivery.DeliveryMode,
		delivery.Content, string(metadataJSON), delivery.State, delivery.NextAttemptAt, delivery.CreatedAt, delivery.UpdatedAt)
	if err != nil {
		return nil, false, fmt.Errorf("insert delivery receipt: %w", err)
	}
	created, err := result.RowsAffected()
	if err != nil {
		return nil, false, fmt.Errorf("insert delivery receipt rows affected: %w", err)
	}
	stored, err := r.getDeliveryBySourceKey(ctx, delivery.SenderSessionID, delivery.SourceTurnID, delivery.IdempotencyKey)
	if err != nil {
		return nil, false, err
	}
	if created == 1 {
		adminmetrics.RecordMessageDeliveryOutcome(string(stored.State), 1)
	} else {
		adminmetrics.RecordMessageDeliveryDuplicate()
	}
	return stored, created == 1, nil
}

// GetDelivery loads one durable receipt by immutable delivery identity.
func (r *sqliteRepository) GetDelivery(ctx context.Context, deliveryID string) (*Delivery, error) {
	delivery, err := scanDelivery(r.ro.QueryRowxContext(ctx, r.ro.Rebind(deliverySelectByID), deliveryID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get delivery: %w", err)
	}
	return delivery, nil
}

// ClaimDueDeliveries leases a bounded batch. The compare-and-set update makes
// the lease authoritative across backend instances; an expired reservation is
// reclaimable after a process crash.
func (r *sqliteRepository) ClaimDueDeliveries(ctx context.Context, now time.Time, leaseOwner string, leaseDuration time.Duration, limit int) ([]Delivery, error) {
	if leaseOwner == "" || leaseDuration <= 0 || limit <= 0 {
		return nil, nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin delivery claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	ids, err := r.selectDueDeliveryIDs(ctx, tx, now, limit)
	if err != nil {
		return nil, err
	}
	claimed, err := r.claimDeliveryIDs(ctx, tx, ids, now, leaseOwner, leaseDuration)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit delivery claim: %w", err)
	}
	return claimed, nil
}

// RescheduleDelivery releases a claimed receipt for a bounded later retry.
func (r *sqliteRepository) RescheduleDelivery(ctx context.Context, deliveryID, leaseOwner string, nextAttemptAt time.Time, lastError string) (*Delivery, error) {
	if nextAttemptAt.IsZero() {
		return nil, errors.New("delivery retry time is required")
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE message_deliveries
		SET state = ?, next_attempt_at = ?, last_error = ?, lease_owner = NULL, lease_expires_at = NULL, updated_at = ?
		WHERE id = ? AND state = ? AND lease_owner = ?
	`), DeliveryRetryWait, nextAttemptAt, lastError, time.Now().UTC(), deliveryID, DeliveryReserved, leaseOwner)
	if err != nil {
		return nil, fmt.Errorf("reschedule delivery: %w", err)
	}
	stored, err := r.deliveryAfterLeasedUpdate(ctx, deliveryID, result)
	if err == nil {
		adminmetrics.RecordMessageDeliveryRetry()
	}
	return stored, err
}

// MarkDeliveryQueued records that a leased receipt reached the target FIFO.
func (r *sqliteRepository) MarkDeliveryQueued(ctx context.Context, deliveryID, leaseOwner, queueEntryID string) (*Delivery, error) {
	if queueEntryID == "" {
		return nil, errors.New("delivery queue entry is required")
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE message_deliveries
		SET state = ?, queue_entry_id = ?, lease_owner = NULL, lease_expires_at = NULL, updated_at = ?
		WHERE id = ? AND state = ? AND lease_owner = ?
	`), DeliveryQueued, queueEntryID, time.Now().UTC(), deliveryID, DeliveryReserved, leaseOwner)
	if err != nil {
		return nil, fmt.Errorf("mark delivery queued: %w", err)
	}
	stored, err := r.deliveryAfterLeasedUpdate(ctx, deliveryID, result)
	if err == nil {
		adminmetrics.RecordMessageDeliveryOutcome(string(DeliveryQueued), 1)
	}
	return stored, err
}

// AcknowledgeDelivery marks the receipt terminal only after the queue entry
// was accepted by the executor. Duplicate executor acknowledgement is safe.
func (r *sqliteRepository) AcknowledgeDelivery(ctx context.Context, deliveryID, queueEntryID string, deliveredAt time.Time) (*Delivery, error) {
	if deliveredAt.IsZero() {
		deliveredAt = time.Now().UTC()
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE message_deliveries
		SET state = ?, delivered_at = ?, updated_at = ?
		WHERE id = ? AND queue_entry_id = ? AND state = ?
	`), DeliveryDelivered, deliveredAt, deliveredAt, deliveryID, queueEntryID, DeliveryQueued)
	if err != nil {
		return nil, fmt.Errorf("acknowledge delivery: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("acknowledge delivery rows affected: %w", err)
	}
	stored, err := r.GetDelivery(ctx, deliveryID)
	if err != nil {
		return nil, err
	}
	if affected == 0 && (stored == nil || stored.State != DeliveryDelivered || stored.QueueEntryID != queueEntryID) {
		return nil, ErrEntryNotFound
	}
	if affected == 1 {
		adminmetrics.RecordMessageDeliveryOutcome(string(DeliveryDelivered), 1)
	}
	return stored, nil
}

// AcknowledgeDeliveryByQueueEntry joins executor acknowledgement to the
// receipt without trusting caller-provided delivery identity.
func (r *sqliteRepository) AcknowledgeDeliveryByQueueEntry(ctx context.Context, queueEntryID string, deliveredAt time.Time) (*Delivery, error) {
	if deliveredAt.IsZero() {
		deliveredAt = time.Now().UTC()
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE message_deliveries
		SET state = ?, delivered_at = ?, updated_at = ?
		WHERE queue_entry_id = ? AND state = ?
	`), DeliveryDelivered, deliveredAt, deliveredAt, queueEntryID, DeliveryQueued)
	if err != nil {
		return nil, fmt.Errorf("acknowledge delivery by queue entry: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("acknowledge delivery by queue entry rows affected: %w", err)
	}
	if affected == 0 {
		return nil, nil
	}
	adminmetrics.RecordMessageDeliveryOutcome(string(DeliveryDelivered), 1)
	return r.getDeliveryByQueueEntry(ctx, queueEntryID)
}

// MarkDeliveryRecoverable ends automatic retries while retaining the payload
// and error for an authorized explicit recovery operation.
func (r *sqliteRepository) MarkDeliveryRecoverable(ctx context.Context, deliveryID, leaseOwner, lastError string) (*Delivery, error) {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE message_deliveries
		SET state = ?, last_error = ?, lease_owner = NULL, lease_expires_at = NULL, updated_at = ?
		WHERE id = ? AND state = ? AND lease_owner = ?
	`), DeliveryRecoverable, lastError, time.Now().UTC(), deliveryID, DeliveryReserved, leaseOwner)
	if err != nil {
		return nil, fmt.Errorf("mark delivery recoverable: %w", err)
	}
	stored, err := r.deliveryAfterLeasedUpdate(ctx, deliveryID, result)
	if err == nil {
		adminmetrics.RecordMessageDeliveryOutcome(string(DeliveryRecoverable), 1)
	}
	return stored, err
}

func validateDeliveryIdentity(delivery Delivery) error {
	if delivery.SenderSessionID == "" || delivery.SourceTurnID == "" || delivery.IdempotencyKey == "" {
		return errors.New("delivery source session, turn, and idempotency key are required")
	}
	if delivery.TargetTaskID == "" || delivery.TargetSessionID == "" {
		return errors.New("delivery target task and session are required")
	}
	return nil
}

func prepareNewDelivery(delivery *Delivery) {
	now := time.Now().UTC()
	if delivery.ID == "" {
		delivery.ID = uuid.NewString()
	}
	if delivery.State == "" {
		delivery.State = DeliveryPendingCapacity
	}
	if delivery.NextAttemptAt.IsZero() {
		delivery.NextAttemptAt = now
	}
	if delivery.CreatedAt.IsZero() {
		delivery.CreatedAt = now
	}
	if delivery.UpdatedAt.IsZero() {
		delivery.UpdatedAt = now
	}
}

func (r *sqliteRepository) getDeliveryBySourceKey(ctx context.Context, senderSessionID, sourceTurnID, idempotencyKey string) (*Delivery, error) {
	delivery, err := scanDelivery(r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT `+deliveryColumns+` FROM message_deliveries
		WHERE sender_session_id = ? AND source_turn_id = ? AND idempotency_key = ?
	`), senderSessionID, sourceTurnID, idempotencyKey))
	if err != nil {
		return nil, fmt.Errorf("get delivery receipt: %w", err)
	}
	return delivery, nil
}

func (r *sqliteRepository) getDeliveryByQueueEntry(ctx context.Context, queueEntryID string) (*Delivery, error) {
	delivery, err := scanDelivery(r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT `+deliveryColumns+` FROM message_deliveries WHERE queue_entry_id = ?
	`), queueEntryID))
	if err != nil {
		return nil, fmt.Errorf("get delivery by queue entry: %w", err)
	}
	return delivery, nil
}

func (r *sqliteRepository) deliveryAfterLeasedUpdate(ctx context.Context, deliveryID string, result sql.Result) (*Delivery, error) {
	affected, err := result.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("delivery update rows affected: %w", err)
	}
	if affected == 0 {
		return nil, ErrDeliveryLeaseLost
	}
	return r.GetDelivery(ctx, deliveryID)
}

func (r *sqliteRepository) selectDueDeliveryIDs(ctx context.Context, tx queryer, now time.Time, limit int) ([]string, error) {
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(`
		SELECT id FROM message_deliveries
		WHERE (state IN (?, ?) AND next_attempt_at <= ?)
		   OR (state = ? AND lease_expires_at IS NOT NULL AND lease_expires_at <= ?)
		ORDER BY next_attempt_at ASC, created_at ASC
		LIMIT ?
	`), DeliveryPendingCapacity, DeliveryRetryWait, now, DeliveryReserved, now, limit)
	if err != nil {
		return nil, fmt.Errorf("select due deliveries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan due delivery id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate due deliveries: %w", err)
	}
	return ids, nil
}

func (r *sqliteRepository) claimDeliveryIDs(ctx context.Context, tx *sqlx.Tx, ids []string, now time.Time, leaseOwner string, leaseDuration time.Duration) ([]Delivery, error) {
	leaseExpiresAt := now.Add(leaseDuration)
	claimed := make([]Delivery, 0, len(ids))
	for _, id := range ids {
		result, err := tx.ExecContext(ctx, r.db.Rebind(`
			UPDATE message_deliveries
			SET state = ?, lease_owner = ?, lease_expires_at = ?, attempts = attempts + 1, updated_at = ?
			WHERE id = ? AND (
				(state IN (?, ?) AND next_attempt_at <= ?)
				OR (state = ? AND lease_expires_at IS NOT NULL AND lease_expires_at <= ?)
			)
		`), DeliveryReserved, leaseOwner, leaseExpiresAt, now, id,
			DeliveryPendingCapacity, DeliveryRetryWait, now, DeliveryReserved, now)
		if err != nil {
			return nil, fmt.Errorf("claim delivery: %w", err)
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return nil, fmt.Errorf("claim delivery rows affected: %w", err)
		}
		if affected == 0 {
			continue
		}
		delivery, err := scanDelivery(tx.QueryRowxContext(ctx, r.db.Rebind(deliverySelectByID), id))
		if err != nil {
			return nil, fmt.Errorf("load claimed delivery: %w", err)
		}
		claimed = append(claimed, *delivery)
	}
	return claimed, nil
}

const deliveryColumns = `
	id, sender_task_id, sender_session_id, source_turn_id, idempotency_key,
	target_task_id, target_session_id, delivery_mode, content, metadata_json,
	state, queue_entry_id, attempts, next_attempt_at, lease_owner, lease_expires_at,
	last_error, created_at, updated_at, delivered_at
`

const deliverySelectByID = `SELECT ` + deliveryColumns + ` FROM message_deliveries WHERE id = ?`

type deliveryScanner interface {
	Scan(dest ...interface{}) error
}

type queryer interface {
	QueryxContext(ctx context.Context, query string, args ...interface{}) (*sqlx.Rows, error)
}

func scanDelivery(row deliveryScanner) (*Delivery, error) {
	var delivery Delivery
	var metadataJSON string
	var leaseOwner, lastError, queueEntryID sql.NullString
	var leaseExpiresAt, deliveredAt sql.NullTime
	err := row.Scan(
		&delivery.ID, &delivery.SenderTaskID, &delivery.SenderSessionID, &delivery.SourceTurnID, &delivery.IdempotencyKey,
		&delivery.TargetTaskID, &delivery.TargetSessionID, &delivery.DeliveryMode, &delivery.Content, &metadataJSON,
		&delivery.State, &queueEntryID, &delivery.Attempts, &delivery.NextAttemptAt, &leaseOwner, &leaseExpiresAt,
		&lastError, &delivery.CreatedAt, &delivery.UpdatedAt, &deliveredAt,
	)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(metadataJSON), &delivery.Metadata); err != nil {
		return nil, fmt.Errorf("decode delivery metadata: %w", err)
	}
	delivery.QueueEntryID = queueEntryID.String
	delivery.LeaseOwner = leaseOwner.String
	delivery.LeaseExpiresAt = leaseExpiresAt.Time
	delivery.LastError = lastError.String
	delivery.DeliveredAt = deliveredAt.Time
	return &delivery, nil
}
