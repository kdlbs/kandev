package messagequeue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// ClearDeliveryAttemptForSession reverses MarkDeliveryAttemptedForSession for
// token-owned receipts whose prompt provably never reached transport. Every
// receipt is cleared, or none is.
func (s *Service) ClearDeliveryAttemptForSession(
	ctx context.Context,
	identity QueueSessionIdentity,
	reserved []QueuedMessage,
) error {
	return s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		return s.repo.ClearDeliveryAttemptForSession(admittedCtx, identity, reserved)
	})
}

func validateDeliveryAttemptClear(reserved []QueuedMessage, identity QueueSessionIdentity) error {
	if len(reserved) == 0 {
		return ErrEntryNotFound
	}
	seen := make(map[string]struct{}, len(reserved))
	for index := range reserved {
		if err := validateDeliveryAttemptCandidate(&reserved[index], identity); err != nil {
			return err
		}
		if _, duplicate := seen[reserved[index].ID]; duplicate {
			return ErrQueueChanged
		}
		seen[reserved[index].ID] = struct{}{}
	}
	return nil
}

// attemptedReservationOwned reports whether stored metadata still belongs to
// the caller's reservation and carries the delivery-attempted marker.
func attemptedReservationOwned(
	metadata map[string]interface{},
	identity QueueSessionIdentity,
	reserved *QueuedMessage,
) bool {
	isReserved, _ := metadata[MetadataLifecycleReserved].(bool)
	attempted, _ := metadata[MetadataDeliveryAttempted].(bool)
	return isReserved && attempted &&
		lifecycleReservationIncarnation(metadata) == identity.SessionIncarnationID &&
		reserved.reservationMatches(metadata)
}

func (r *sqliteRepository) ClearDeliveryAttemptForSession(
	ctx context.Context,
	identity QueueSessionIdentity,
	reserved []QueuedMessage,
) error {
	if err := validateDeliveryAttemptClear(reserved, identity); err != nil {
		return err
	}
	unlock := r.withSessionLock(identity.SessionID)
	defer unlock()
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delivery-attempt clear tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.guardActiveTaskTx(ctx, tx, identity.TaskID); err != nil {
		return err
	}
	if err := r.lockSessionTx(ctx, tx, identity.SessionID); err != nil {
		return err
	}
	if err := r.validateSessionIdentityTx(ctx, tx, identity); err != nil {
		return err
	}
	for index := range reserved {
		if err := r.clearDeliveryAttemptTx(ctx, tx, identity, &reserved[index]); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *sqliteRepository) clearDeliveryAttemptTx(
	ctx context.Context,
	tx *sqlx.Tx,
	identity QueueSessionIdentity,
	reserved *QueuedMessage,
) error {
	var metadataJSON string
	if err := tx.GetContext(ctx, &metadataJSON, r.db.Rebind(`
		SELECT metadata_json FROM queued_messages WHERE id = ? AND session_id = ?
	`), reserved.ID, identity.SessionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrEntryNotFound
		}
		return fmt.Errorf("read delivery-attempt receipt: %w", err)
	}
	metadata := make(map[string]interface{})
	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
			return fmt.Errorf("unmarshal delivery-attempt receipt: %w", err)
		}
	}
	if !attemptedReservationOwned(metadata, identity, reserved) {
		return ErrEntryNotFound
	}
	delete(metadata, MetadataDeliveryAttempted)
	updatedJSON, err := marshalMetadata(metadata)
	if err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE queued_messages SET metadata_json = ?
		WHERE id = ? AND session_id = ? AND metadata_json = ?
	`), updatedJSON, reserved.ID, identity.SessionID, metadataJSON)
	if err != nil {
		return fmt.Errorf("clear delivery attempted: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return ErrEntryNotFound
	}
	return nil
}

func (r *memoryRepository) ClearDeliveryAttemptForSession(
	_ context.Context,
	identity QueueSessionIdentity,
	reserved []QueuedMessage,
) error {
	if err := validateDeliveryAttemptClear(reserved, identity); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := r.bindIdentityLocked(identity); err != nil {
		return err
	}
	byID := make(map[string]*QueuedMessage, len(r.entries[identity.SessionID]))
	for _, msg := range r.entries[identity.SessionID] {
		byID[msg.ID] = msg
	}
	stored := make([]*QueuedMessage, 0, len(reserved))
	for index := range reserved {
		found := byID[reserved[index].ID]
		if found == nil || !attemptedReservationOwned(found.Metadata, identity, &reserved[index]) {
			return ErrEntryNotFound
		}
		stored = append(stored, found)
	}
	for _, msg := range stored {
		delete(msg.Metadata, MetadataDeliveryAttempted)
	}
	return nil
}
