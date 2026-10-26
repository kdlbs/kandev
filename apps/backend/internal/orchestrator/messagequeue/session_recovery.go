package messagequeue

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	internaldb "github.com/kandev/kandev/internal/db"
)

var (
	ErrQueueRecoveryUnauthorized      = errors.New("queue recovery authorization is no longer valid")
	ErrQueueRecoveryTargetNotTerminal = errors.New("queue recovery source session is not terminal")
	ErrQueueRecoveryConflict          = errors.New("queue source already recovered to a different session")
	ErrQueueRecoverySnapshotExpired   = errors.New("queue recovery snapshot has expired")
)

type QueueRecoveryScope struct {
	TaskID               string
	WorkspaceID          string
	SourceSessionID      string
	DestinationSessionID string
}

type QueueRecoveryReceipt struct {
	ID                   string    `db:"id" json:"id"`
	TaskID               string    `db:"task_id" json:"task_id"`
	WorkspaceID          string    `db:"workspace_id" json:"workspace_id"`
	SourceSessionID      string    `db:"source_session_id" json:"source_session_id"`
	DestinationSessionID string    `db:"destination_session_id" json:"destination_session_id"`
	EntryCount           int       `db:"entry_count" json:"entry_count"`
	SnapshotSHA256       string    `db:"snapshot_sha256" json:"snapshot_sha256"`
	OccurredAt           time.Time `db:"occurred_at" json:"occurred_at"`
}

type QueueRecoveryResult struct {
	Receipt QueueRecoveryReceipt
	Entries []QueuedMessage
}

type QueueRecoveryEntry struct {
	ID               string              `json:"id"`
	SessionID        string              `json:"session_id"`
	TaskID           string              `json:"task_id"`
	Position         int64               `json:"position"`
	Content          string              `json:"content"`
	ContentSHA256    string              `json:"content_sha256"`
	ContentBytes     int                 `json:"content_bytes"`
	Model            string              `json:"model"`
	PlanMode         bool                `json:"plan_mode"`
	Attachments      []MessageAttachment `json:"attachments"`
	QueuedAt         string              `json:"queued_at"`
	QueuedBy         string              `json:"queued_by"`
	Origin           string              `json:"origin,omitempty"`
	SenderTaskID     string              `json:"sender_task_id,omitempty"`
	ReservedInFlight bool                `json:"reserved_in_flight"`
}

func RecoveryEntries(entries []QueuedMessage) []QueueRecoveryEntry {
	result := make([]QueueRecoveryEntry, 0, len(entries))
	for i := range entries {
		entry := &entries[i]
		digest := sha256.Sum256([]byte(entry.Content))
		result = append(result, QueueRecoveryEntry{
			ID: entry.ID, SessionID: entry.SessionID, TaskID: entry.TaskID,
			Position: entry.Position, Content: entry.Content,
			ContentSHA256: fmt.Sprintf("%x", digest), ContentBytes: len(entry.Content),
			Model: entry.Model, PlanMode: entry.PlanMode, Attachments: entry.Attachments,
			QueuedAt: entry.QueuedAt.UTC().Format(time.RFC3339Nano), QueuedBy: entry.QueuedBy,
			Origin:           metadataString(entry.Metadata, "origin"),
			SenderTaskID:     metadataString(entry.Metadata, MetadataSenderTaskID),
			ReservedInFlight: entry.IsReservedInFlight(),
		})
	}
	return result
}

type storedQueueRecoveryReceipt struct {
	QueueRecoveryReceipt
	SnapshotJSON     string `db:"snapshot_json"`
	SnapshotRedacted bool   `db:"snapshot_redacted"`
}

// RecoverSessionQueueInTransaction moves the original FIFO under the task
// repository's authorization lock and records an exact replay receipt.
func RecoverSessionQueueInTransaction(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, scope QueueRecoveryScope) (*QueueRecoveryResult, error) {
	r := &sqliteRepository{db: db}
	first, second := scope.SourceSessionID, scope.DestinationSessionID
	if first > second {
		first, second = second, first
	}
	if err := r.lockSessionTx(ctx, tx, first); err != nil {
		return nil, err
	}
	if err := r.lockSessionTx(ctx, tx, second); err != nil {
		return nil, err
	}
	if err := rejectRecoveryInFlightStateTx(ctx, tx, db, scope.SourceSessionID); err != nil {
		return nil, err
	}
	var previous storedQueueRecoveryReceipt
	err := tx.GetContext(ctx, &previous, db.Rebind(`
		SELECT id, task_id, workspace_id, source_session_id, destination_session_id,
			entry_count, snapshot_sha256, snapshot_json, snapshot_redacted, occurred_at
		FROM queue_recovery_receipts WHERE source_session_id = ?
	`), scope.SourceSessionID)
	if err == nil {
		return replayQueueRecovery(scope, previous)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("read queue recovery receipt: %w", err)
	}
	return r.moveSessionQueueForRecovery(ctx, tx, db, scope)
}

func (r *sqliteRepository) moveSessionQueueForRecovery(
	ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, scope QueueRecoveryScope,
) (*QueueRecoveryResult, error) {
	ordered, _, err := r.listOrderedStoredSessionEntries(ctx, tx, scope.SourceSessionID)
	if err != nil {
		return nil, err
	}
	for _, stored := range ordered {
		if stored.message.TaskID != scope.TaskID {
			return nil, ErrQueueRecoveryUnauthorized
		}
	}
	attachmentsPresent, err := internaldb.TableExists(tx, "task_message_attachments")
	if err != nil {
		return nil, fmt.Errorf("inspect queue recovery attachments: %w", err)
	}
	if attachmentsPresent {
		if err := r.transferQueuedAttachmentClaimsTx(ctx, tx, scope.SourceSessionID, scope.DestinationSessionID); err != nil {
			return nil, err
		}
	}
	var destinationMax sql.NullInt64
	if err := tx.GetContext(ctx, &destinationMax, db.Rebind(`
		SELECT MAX(position) FROM queued_messages WHERE session_id = ?
	`), scope.DestinationSessionID); err != nil {
		return nil, fmt.Errorf("read recovery destination position: %w", err)
	}
	entries := make([]QueuedMessage, 0, len(ordered))
	for index, stored := range ordered {
		entry := *cloneQueuedMessage(stored.message)
		entries = append(entries, entry)
		metadata := clearReservedMetadata(entry.Metadata)
		if _, exists := metadata["recovery_source_session_id"]; !exists {
			metadata["recovery_source_session_id"] = scope.SourceSessionID
			metadata["recovery_source_position"] = entry.Position
		}
		encoded, err := marshalMetadata(metadata)
		if err != nil {
			return nil, err
		}
		result, err := tx.ExecContext(ctx, db.Rebind(`
			UPDATE queued_messages SET session_id = ?, position = ?, metadata_json = ?
			WHERE id = ? AND session_id = ? AND metadata_json = ?
		`), scope.DestinationSessionID, destinationMax.Int64+int64(index)+1,
			encoded, entry.ID, scope.SourceSessionID, stored.raw)
		if err != nil {
			return nil, fmt.Errorf("move recovered queue entry: %w", err)
		}
		if rows, err := result.RowsAffected(); err != nil || rows != 1 {
			return nil, ErrQueueChanged
		}
	}
	if err := r.bumpQueuePositionTx(ctx, tx, scope.DestinationSessionID, destinationMax.Int64+int64(len(ordered))); err != nil {
		return nil, err
	}
	return insertQueueRecoveryReceiptTx(ctx, tx, db, scope, entries)
}

func insertQueueRecoveryReceiptTx(
	ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, scope QueueRecoveryScope, entries []QueuedMessage,
) (*QueueRecoveryResult, error) {
	snapshotJSON, err := json.Marshal(entries)
	if err != nil {
		return nil, fmt.Errorf("encode queue recovery receipt: %w", err)
	}
	receipt := QueueRecoveryReceipt{
		ID: uuid.NewString(), TaskID: scope.TaskID, WorkspaceID: scope.WorkspaceID,
		SourceSessionID: scope.SourceSessionID, DestinationSessionID: scope.DestinationSessionID,
		EntryCount: len(entries), SnapshotSHA256: fmt.Sprintf("%x", sha256.Sum256(snapshotJSON)),
		OccurredAt: time.Now().UTC(),
	}
	_, err = tx.ExecContext(ctx, db.Rebind(`
		INSERT INTO queue_recovery_receipts
			(id, task_id, workspace_id, source_session_id, destination_session_id,
			 entry_count, snapshot_sha256, snapshot_json, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), receipt.ID, receipt.TaskID, receipt.WorkspaceID, receipt.SourceSessionID,
		receipt.DestinationSessionID, receipt.EntryCount, receipt.SnapshotSHA256,
		string(snapshotJSON), receipt.OccurredAt)
	if err != nil {
		return nil, fmt.Errorf("record queue recovery receipt: %w", err)
	}
	return &QueueRecoveryResult{Receipt: receipt, Entries: entries}, nil
}

func replayQueueRecovery(scope QueueRecoveryScope, stored storedQueueRecoveryReceipt) (*QueueRecoveryResult, error) {
	if stored.TaskID != scope.TaskID || stored.WorkspaceID != scope.WorkspaceID ||
		stored.DestinationSessionID != scope.DestinationSessionID {
		return nil, ErrQueueRecoveryConflict
	}
	if stored.SnapshotRedacted {
		return nil, ErrQueueRecoverySnapshotExpired
	}
	if fmt.Sprintf("%x", sha256.Sum256([]byte(stored.SnapshotJSON))) != stored.SnapshotSHA256 {
		return nil, errors.New("queue recovery receipt snapshot hash mismatch")
	}
	var entries []QueuedMessage
	if err := json.Unmarshal([]byte(stored.SnapshotJSON), &entries); err != nil {
		return nil, fmt.Errorf("decode queue recovery receipt: %w", err)
	}
	if len(entries) != stored.EntryCount {
		return nil, errors.New("queue recovery receipt entry count mismatch")
	}
	return &QueueRecoveryResult{Receipt: stored.QueueRecoveryReceipt, Entries: entries}, nil
}

func rejectRecoveryInFlightStateTx(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, sessionID string) error {
	for _, table := range []string{"queue_dispatch_claims", "queue_send_now_claims", "pending_moves"} {
		present, err := internaldb.TableExists(tx, table)
		if err != nil {
			return fmt.Errorf("inspect queue recovery claim table: %w", err)
		}
		if !present {
			continue
		}
		var count int
		err = tx.GetContext(ctx, &count, db.Rebind(`SELECT COUNT(*) FROM `+table+` WHERE session_id = ?`), sessionID)
		if err != nil {
			return fmt.Errorf("inspect queue recovery claims: %w", err)
		}
		if count != 0 {
			return fmt.Errorf("%w: %d unresolved %s records", ErrQueueRecoveryConflict, count, table)
		}
	}
	return nil
}

func redactRecoveryReceiptsIfEmptyTx(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, destinationSessionID string) error {
	present, err := internaldb.TableExists(tx, "queue_recovery_receipts")
	if err != nil {
		return fmt.Errorf("inspect recovery receipt table: %w", err)
	}
	if !present {
		return nil
	}
	var receipts []storedQueueRecoveryReceipt
	err = tx.SelectContext(ctx, &receipts, db.Rebind(`
		WITH RECURSIVE lineage(task_id, session_id) AS (
			SELECT task_id, destination_session_id FROM queue_recovery_receipts
			WHERE destination_session_id = ?
			UNION
			SELECT r.task_id, r.source_session_id FROM queue_recovery_receipts r
			JOIN lineage l ON r.task_id = l.task_id AND r.destination_session_id = l.session_id
		)
		SELECT r.id, r.task_id, r.workspace_id, r.source_session_id, r.destination_session_id,
			r.entry_count, r.snapshot_sha256, r.snapshot_json, r.snapshot_redacted, r.occurred_at
		FROM queue_recovery_receipts r
		JOIN lineage l ON r.task_id = l.task_id AND r.destination_session_id = l.session_id
		WHERE r.snapshot_redacted = 0
	`), destinationSessionID)
	if err != nil {
		return fmt.Errorf("list recovery receipt lineage: %w", err)
	}
	if len(receipts) == 0 {
		return nil
	}
	claimIDs, err := recoverySendNowClaimEntryIDsTx(ctx, tx)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, receipt := range receipts {
		var entries []QueuedMessage
		if err := json.Unmarshal([]byte(receipt.SnapshotJSON), &entries); err != nil {
			return fmt.Errorf("decode recovery receipt entries: %w", err)
		}
		pending, err := recoveryReceiptEntriesPendingTx(ctx, tx, db, entries, claimIDs)
		if err != nil {
			return err
		}
		if !pending {
			if err := redactRecoveryReceiptTx(ctx, tx, db, receipt.QueueRecoveryReceipt, "destination_dispositioned", now); err != nil {
				return err
			}
		}
	}
	return nil
}

func recoverySendNowClaimEntryIDsTx(ctx context.Context, tx *sqlx.Tx) (map[string]struct{}, error) {
	ids := make(map[string]struct{})
	present, err := internaldb.TableExists(tx, "queue_send_now_claims")
	if err != nil || !present {
		return ids, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT claim_json FROM queue_send_now_claims`)
	if err != nil {
		return nil, fmt.Errorf("read recovery Send Now claims: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, fmt.Errorf("scan recovery Send Now claim: %w", err)
		}
		var claim SendNowClaim
		if err := json.Unmarshal([]byte(raw), &claim); err != nil {
			return nil, fmt.Errorf("decode recovery Send Now claim: %w", err)
		}
		for _, source := range claim.Sources {
			ids[source.ID] = struct{}{}
		}
		ids[claim.Dispatch.ID] = struct{}{}
	}
	return ids, rows.Err()
}

func recoveryReceiptEntriesPendingTx(
	ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, entries []QueuedMessage, sendNowClaimIDs map[string]struct{},
) (bool, error) {
	for _, entry := range entries {
		if _, pending := sendNowClaimIDs[entry.ID]; pending {
			return true, nil
		}
		for _, table := range []struct{ name, column string }{
			{name: "queued_messages", column: "id"},
			{name: "queue_dispatch_claims", column: "entry_id"},
		} {
			present, err := internaldb.TableExists(tx, table.name)
			if err != nil {
				return false, fmt.Errorf("inspect recovery entry state: %w", err)
			}
			if !present {
				continue
			}
			var count int
			if err := tx.GetContext(ctx, &count, db.Rebind(`SELECT COUNT(*) FROM `+table.name+` WHERE `+table.column+` = ?`), entry.ID); err != nil {
				return false, fmt.Errorf("count recovery entry state: %w", err)
			}
			if count != 0 {
				return true, nil
			}
		}
	}
	return false, nil
}

func redactRecoveryReceiptsTx(ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, taskID, destinationSessionID, reason string) error {
	present, err := internaldb.TableExists(tx, "queue_recovery_receipts")
	if err != nil {
		return fmt.Errorf("inspect recovery receipt table: %w", err)
	}
	if !present {
		return nil
	}
	var receipts []QueueRecoveryReceipt
	if err := tx.SelectContext(ctx, &receipts, db.Rebind(`
		SELECT id, task_id, workspace_id, source_session_id, destination_session_id,
			entry_count, snapshot_sha256, occurred_at
		FROM queue_recovery_receipts
		WHERE (? = '' OR task_id = ?) AND (? = '' OR destination_session_id = ?)
			AND snapshot_redacted = 0
	`), taskID, taskID, destinationSessionID, destinationSessionID); err != nil {
		return fmt.Errorf("list recovery receipts for cleanup: %w", err)
	}
	now := time.Now().UTC()
	for _, receipt := range receipts {
		if err := redactRecoveryReceiptTx(ctx, tx, db, receipt, reason, now); err != nil {
			return err
		}
	}
	return nil
}

func redactRecoveryReceiptTx(
	ctx context.Context, tx *sqlx.Tx, db *sqlx.DB, receipt QueueRecoveryReceipt, reason string, now time.Time,
) error {
	if _, err := tx.ExecContext(ctx, db.Rebind(`
			UPDATE queue_recovery_receipts
			SET snapshot_json = '[]', snapshot_redacted = 1, redacted_at = ?
			WHERE id = ? AND snapshot_redacted = 0
		`), now, receipt.ID); err != nil {
		return fmt.Errorf("redact recovery receipt: %w", err)
	}
	if _, err := tx.ExecContext(ctx, db.Rebind(`
			INSERT INTO queue_recovery_cleanup_events
				(receipt_id, task_id, workspace_id, source_session_id,
				 destination_session_id, reason, occurred_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`), receipt.ID, receipt.TaskID, receipt.WorkspaceID, receipt.SourceSessionID,
		receipt.DestinationSessionID, reason, now); err != nil {
		return fmt.Errorf("audit recovery receipt cleanup: %w", err)
	}
	return nil
}
