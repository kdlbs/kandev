package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// FinalizeClarificationResponseDelivery clears the recovery intent from an
// exact terminal claim after its response has reached either the live waiter
// or the durable detached-resume boundary.
func (r *Repository) FinalizeClarificationResponseDelivery(
	ctx context.Context,
	pendingID, terminalStatus string,
	claimedMessages []*models.Message,
) ([]*models.Message, bool, error) {
	if terminalStatus != clarificationStatusAnswered && terminalStatus != clarificationStatusRejected {
		return nil, false, fmt.Errorf("invalid clarification terminal status %q", terminalStatus)
	}
	claimIDs, err := clarificationClaimIDs(claimedMessages)
	if err != nil {
		return nil, false, err
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
	messages, err := r.loadPendingClarificationResponseDeliveryBundle(ctx, tx, drv, pendingID)
	if err != nil {
		return nil, false, err
	}
	if len(messages) != len(claimIDs) {
		return nil, false, nil
	}
	for _, message := range messages {
		if _, claimed := claimIDs[message.ID]; !claimed {
			return nil, false, nil
		}
		if status, _ := message.Metadata["status"].(string); status != terminalStatus {
			return nil, false, nil
		}
	}
	if err := r.clearClarificationResponseDeliveryMarkers(ctx, tx, drv, messages, terminalStatus); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("commit clarification response delivery: %w", err)
	}
	return messages, true, nil
}

func (r *Repository) reconcilePendingClarificationResponseDeliveries(ctx context.Context) (int, error) {
	pendingIDs, err := r.listPendingClarificationResponseDeliveries(ctx)
	if err != nil {
		return 0, err
	}
	reconciled := 0
	var reconcileErrs []error
	for _, pendingID := range pendingIDs {
		changed, reconcileErr := r.reconcilePendingClarificationResponseDelivery(ctx, pendingID)
		if reconcileErr != nil {
			reconcileErrs = append(reconcileErrs, fmt.Errorf(
				"reconcile pending clarification response delivery %s: %w",
				pendingID,
				reconcileErr,
			))
			continue
		}
		if changed {
			reconciled++
		}
	}
	return reconciled, errors.Join(reconcileErrs...)
}

func (r *Repository) listPendingClarificationResponseDeliveries(ctx context.Context) ([]string, error) {
	drv := r.ro.DriverName()
	pendingIDExpr := dialect.JSONExtract(drv, "m.metadata", "pending_id")
	reservedPendingIDExpr := dialect.JSONExtract(
		drv,
		"turn_row.metadata",
		models.TurnMetaKeyPromptDispatchClarificationPendingID,
	)
	query := fmt.Sprintf(`
		SELECT DISTINCT %s
		FROM task_session_messages m
		WHERE m.type = 'clarification_request'
		  AND %s
		  AND NOT EXISTS (
		    SELECT 1
		    FROM task_session_turns turn_row
		    WHERE %s
		      AND %s = %s
		  )
		ORDER BY %s
	`, pendingIDExpr,
		clarificationResponseDeliveryPendingPredicate(drv, "m"),
		turnDispatchPendingPredicate(drv, "turn_row"),
		reservedPendingIDExpr,
		pendingIDExpr,
		pendingIDExpr,
	)
	var pendingIDs []string
	if err := r.ro.SelectContext(ctx, &pendingIDs, r.ro.Rebind(query)); err != nil {
		return nil, fmt.Errorf("list pending clarification response deliveries: %w", err)
	}
	for _, pendingID := range pendingIDs {
		if pendingID == "" {
			return nil, errors.New("pending clarification response delivery is missing pending_id")
		}
	}
	return pendingIDs, nil
}

func (r *Repository) reconcilePendingClarificationResponseDelivery(
	ctx context.Context,
	pendingID string,
) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin clarification response delivery recovery: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	drv := r.db.DriverName()
	if err := r.lockClarificationBundleTurnWrites(ctx, tx, drv, pendingID); err != nil {
		return false, err
	}
	messages, err := r.loadPendingClarificationResponseDeliveryBundle(ctx, tx, drv, pendingID)
	if err != nil {
		return false, err
	}
	if len(messages) == 0 {
		return false, nil
	}
	terminalStatus, err := clarificationDeliveryTerminalStatus(messages)
	if err != nil {
		return false, err
	}

	restorableMessages, err := r.loadRestorableClarificationBundle(ctx, tx, drv, pendingID)
	if err != nil {
		return false, err
	}
	canRestore := clarificationDeliveryCanRestore(messages, restorableMessages)
	if canRestore {
		err = r.restoreClarificationMessages(ctx, tx, drv, messages, terminalStatus)
	} else {
		// A terminal session or a newer durable turn makes the old response
		// historical. Preserve its terminal status but retire the stale intent.
		err = r.clearClarificationResponseDeliveryMarkers(ctx, tx, drv, messages, terminalStatus)
	}
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit clarification response delivery recovery: %w", err)
	}
	return true, nil
}

func clarificationDeliveryTerminalStatus(messages []*models.Message) (string, error) {
	terminalStatus, _ := messages[0].Metadata["status"].(string)
	if terminalStatus != clarificationStatusAnswered && terminalStatus != clarificationStatusRejected {
		return "", fmt.Errorf("delivery claim has invalid terminal status %q", terminalStatus)
	}
	for _, message := range messages[1:] {
		if status, _ := message.Metadata["status"].(string); status != terminalStatus {
			return "", fmt.Errorf("delivery claim mixes terminal statuses %q and %q", terminalStatus, status)
		}
	}
	return terminalStatus, nil
}

func clarificationDeliveryCanRestore(
	deliveryMessages, restorableMessages []*models.Message,
) bool {
	if len(restorableMessages) == 0 {
		return false
	}
	restorableByID := make(map[string]struct{}, len(restorableMessages))
	for _, message := range restorableMessages {
		restorableByID[message.ID] = struct{}{}
	}
	for _, message := range deliveryMessages {
		if _, ok := restorableByID[message.ID]; !ok {
			return false
		}
	}
	return true
}

func (r *Repository) loadPendingClarificationResponseDeliveryBundle(
	ctx context.Context,
	tx *sqlx.Tx,
	drv, pendingID string,
) ([]*models.Message, error) {
	pendingIDExpr := dialect.JSONExtract(drv, "m.metadata", "pending_id")
	query := fmt.Sprintf(`
		SELECT m.id, m.task_session_id, m.task_id, m.turn_id, m.author_type, m.author_id,
		       m.content, m.requests_input, m.type, m.metadata, m.created_at, m.updated_at
		FROM task_session_messages m
		WHERE m.type = 'clarification_request'
		  AND %s = ?
		  AND %s
		ORDER BY m.created_at ASC, m.id ASC
	`, pendingIDExpr, clarificationResponseDeliveryPendingPredicate(drv, "m"))
	rows, err := tx.QueryxContext(ctx, r.db.Rebind(query), pendingID)
	if err != nil {
		return nil, fmt.Errorf("load pending clarification response delivery: %w", err)
	}
	defer func() { _ = rows.Close() }()
	messages, _, scanErr := scanMessageRows(rows, 0)
	if scanErr != nil {
		return nil, fmt.Errorf("scan pending clarification response delivery: %w", scanErr)
	}
	return messages, nil
}

// clearClarificationResponseDeliveryMarkers deliberately leaves updated_at
// untouched: it only clears the internal delivery-recovery marker, not the
// substantive answer (status/response), which completeClaimedClarificationMessages
// already committed at the claim. Bumping updated_at here as well used to let
// a concurrent loser's R2b reconstruction (resolver.go's reconstructWinnerResolution,
// keyed on the same column) observe an earlier updated_at than the value the
// winner itself gets stamped with afterward (finalizeClarificationResponseDelivery),
// so the two sides could report different responded_at instants for one
// answer -- caught by the REST-vs-MCP race in question_handlers_cross_entry_race_test.go.
// Leaving the column fixed at the claim's timestamp makes it stable and
// identical for every reader from the moment the claim commits.
func (r *Repository) clearClarificationResponseDeliveryMarkers(
	ctx context.Context,
	tx *sqlx.Tx,
	drv string,
	messages []*models.Message,
	terminalStatus string,
) error {
	statusExpr := dialect.JSONExtract(drv, "metadata", "status")
	query := r.db.Rebind(fmt.Sprintf(`
		UPDATE task_session_messages
		SET metadata = ?
		WHERE id = ? AND %s = ? AND %s
	`, statusExpr, clarificationResponseDeliveryPendingPredicate(drv, "task_session_messages")))
	finalizedMetadata := make([]map[string]interface{}, len(messages))
	for i, message := range messages {
		metadata := maps.Clone(message.Metadata)
		delete(metadata, clarificationResponseDeliveryPendingKey)
		raw, marshalErr := json.Marshal(metadata)
		if marshalErr != nil {
			return fmt.Errorf("marshal clarification message %s for delivery finalization: %w", message.ID, marshalErr)
		}
		result, updateErr := tx.ExecContext(ctx, query, string(raw), message.ID, terminalStatus)
		if updateErr != nil {
			return fmt.Errorf("finalize clarification response delivery for message %s: %w", message.ID, updateErr)
		}
		rows, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return fmt.Errorf("count finalized clarification response delivery for message %s: %w", message.ID, rowsErr)
		}
		if rows != 1 {
			return fmt.Errorf("clarification message %s lost its delivery intent", message.ID)
		}
		finalizedMetadata[i] = metadata
	}
	for i, message := range messages {
		message.Metadata = finalizedMetadata[i]
	}
	return nil
}
