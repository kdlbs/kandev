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

type unpublishedPromptTurn struct {
	id        string
	sessionID string
}

const metadataTrueString = "true"

// ReconcileUnpublishedPromptTurns repairs durable prompt reservations first,
// then response-delivery claims left behind before their handoff completed.
func (r *Repository) ReconcileUnpublishedPromptTurns(ctx context.Context) (int, error) {
	turns, err := r.listUnpublishedPromptTurns(ctx)
	if err != nil {
		return 0, err
	}
	reconciled := 0
	var reconcileErrs []error
	for _, turn := range turns {
		changed, reconcileErr := r.reconcileUnpublishedPromptTurn(ctx, turn)
		if reconcileErr != nil {
			reconcileErrs = append(reconcileErrs, fmt.Errorf(
				"reconcile unpublished prompt turn %s for session %s: %w",
				turn.id,
				turn.sessionID,
				reconcileErr,
			))
			continue
		}
		if changed {
			reconciled++
		}
	}
	deliveries, deliveryErr := r.reconcilePendingClarificationResponseDeliveries(ctx)
	if deliveryErr != nil {
		reconcileErrs = append(reconcileErrs, deliveryErr)
	}
	return reconciled + deliveries, errors.Join(reconcileErrs...)
}

// ListTurnsPendingStartEvent returns the durable event outbox in deterministic
// turn order. Startup replays every row before prompt admission, then clears
// the marker through ClearTurnPromptDispatchMetadata.
func (r *Repository) ListTurnsPendingStartEvent(ctx context.Context) ([]*models.Turn, error) {
	query := fmt.Sprintf(`
		SELECT id, task_session_id, task_id, execution_profile_id, route_generation, started_at, completed_at, metadata, created_at, updated_at
		FROM task_session_turns turn_row
		WHERE %s
		ORDER BY turn_row.task_session_id, turn_row.started_at, turn_row.created_at, turn_row.id
	`, turnMetadataFlagPredicate(
		r.ro.DriverName(),
		"turn_row",
		models.TurnMetaKeyPromptDispatchStartEventPending,
	))
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query))
	if err != nil {
		return nil, fmt.Errorf("list turns pending start-event replay: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var turns []*models.Turn
	for rows.Next() {
		turn, scanErr := scanTurn(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan turn pending start-event replay: %w", scanErr)
		}
		turns = append(turns, turn)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate turns pending start-event replay: %w", err)
	}
	return turns, nil
}

func (r *Repository) listUnpublishedPromptTurns(ctx context.Context) ([]unpublishedPromptTurn, error) {
	driverName := r.ro.DriverName()
	query := fmt.Sprintf(`
		SELECT turn_row.id, turn_row.task_session_id
		FROM task_session_turns turn_row
		WHERE %s
		ORDER BY turn_row.task_session_id, turn_row.started_at, turn_row.id
	`, turnDispatchPendingPredicate(driverName, "turn_row"))
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(query))
	if err != nil {
		return nil, fmt.Errorf("list unpublished prompt turns: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var turns []unpublishedPromptTurn
	for rows.Next() {
		var turn unpublishedPromptTurn
		if err := rows.Scan(&turn.id, &turn.sessionID); err != nil {
			return nil, fmt.Errorf("scan unpublished prompt turn: %w", err)
		}
		turns = append(turns, turn)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate unpublished prompt turns: %w", err)
	}
	return turns, nil
}

func (r *Repository) reconcileUnpublishedPromptTurn(
	ctx context.Context,
	turn unpublishedPromptTurn,
) (bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin unpublished prompt turn recovery: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockSessionTurnWrites(ctx, tx, r.db.DriverName(), turn.sessionID); err != nil {
		return false, err
	}
	metadata, err := r.loadUnpublishedPromptTurnMetadata(ctx, tx, turn)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	referenced, err := r.promptTurnHasMessages(ctx, tx, turn.id)
	if err != nil {
		return false, err
	}
	updatedAt := r.nowUTC()
	attempted := metadataFlagIsTrue(metadata[models.TurnMetaKeyPromptDispatchAttempted])
	if attempted || referenced {
		err = r.finalizeReservedClarificationDelivery(ctx, tx, metadata)
		if err == nil {
			models.ClearPromptDispatchMetadata(metadata)
			metadata[models.TurnMetaKeyPromptDispatchStartEventPending] = true
			err = updateTurnMetadata(ctx, tx, r.db, turn.id, metadata, updatedAt)
		}
	} else {
		err = r.restoreReservedClarificationClaim(ctx, tx, metadata, updatedAt)
		if err == nil {
			err = r.deleteEmptyUnpublishedPromptTurn(ctx, tx, turn)
		}
	}
	if err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit unpublished prompt turn recovery: %w", err)
	}
	return true, nil
}

func (r *Repository) finalizeReservedClarificationDelivery(
	ctx context.Context,
	tx *sqlx.Tx,
	metadata map[string]interface{},
) error {
	claim, err := reservedClarificationClaimFromMetadata(metadata)
	if err != nil {
		return err
	}
	if claim == nil {
		return nil
	}
	claimedIDs := make(map[string]struct{}, len(claim.messageIDs))
	for _, messageID := range claim.messageIDs {
		claimedIDs[messageID] = struct{}{}
	}
	messages, err := r.loadPendingClarificationResponseDeliveryBundle(
		ctx,
		tx,
		r.db.DriverName(),
		claim.pendingID,
	)
	if err != nil {
		return err
	}
	if len(messages) > 0 && len(messages) != len(claimedIDs) {
		return fmt.Errorf(
			"response delivery intent expected %d messages, found %d for pending_id %s",
			len(claimedIDs),
			len(messages),
			claim.pendingID,
		)
	}
	for _, message := range messages {
		if _, claimed := claimedIDs[message.ID]; !claimed {
			return fmt.Errorf("response delivery intent includes unreserved message %s", message.ID)
		}
		if message.TurnID != claim.turnID {
			return fmt.Errorf(
				"response delivery intent message %s belongs to turn %s, want %s",
				message.ID,
				message.TurnID,
				claim.turnID,
			)
		}
		if status, _ := message.Metadata["status"].(string); status != clarificationStatusAnswered {
			return fmt.Errorf("response delivery intent message %s has status %q", message.ID, status)
		}
	}
	if len(messages) == 0 {
		return nil
	}
	return r.clearClarificationResponseDeliveryMarkers(
		ctx,
		tx,
		r.db.DriverName(),
		messages,
		clarificationStatusAnswered,
	)
}

type reservedClarificationClaim struct {
	pendingID  string
	turnID     string
	messageIDs []string
}

func reservedClarificationClaimFromMetadata(
	metadata map[string]interface{},
) (*reservedClarificationClaim, error) {
	pendingID, _ := metadata[models.TurnMetaKeyPromptDispatchClarificationPendingID].(string)
	turnID, _ := metadata[models.TurnMetaKeyPromptDispatchClarificationTurnID].(string)
	messageIDs, err := metadataStringSlice(metadata[models.TurnMetaKeyPromptDispatchClarificationMessageIDs])
	if err != nil {
		return nil, fmt.Errorf("decode prompt dispatch clarification message ids: %w", err)
	}
	hasPendingID := pendingID != ""
	hasTurnID := turnID != ""
	hasMessageIDs := len(messageIDs) > 0
	if !hasPendingID && !hasTurnID && !hasMessageIDs {
		return nil, nil
	}
	if !hasPendingID || !hasTurnID || !hasMessageIDs {
		return nil, fmt.Errorf(
			"incomplete prompt dispatch clarification metadata: pending_id=%t turn_id=%t message_ids=%t",
			hasPendingID,
			hasTurnID,
			hasMessageIDs,
		)
	}
	return &reservedClarificationClaim{
		pendingID:  pendingID,
		turnID:     turnID,
		messageIDs: messageIDs,
	}, nil
}

// metadataFlagIsTrue mirrors the SQL JSON predicates across supported
// boolean, string, and numeric encodings of a true metadata flag.
func metadataFlagIsTrue(value interface{}) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return typed == metadataTrueString || typed == "1"
	case float64:
		return typed == 1
	default:
		return false
	}
}

func (r *Repository) loadUnpublishedPromptTurnMetadata(
	ctx context.Context,
	tx *sqlx.Tx,
	turn unpublishedPromptTurn,
) (map[string]interface{}, error) {
	driverName := r.db.DriverName()
	query := fmt.Sprintf(`
		SELECT turn_row.metadata
		FROM task_session_turns turn_row
		WHERE turn_row.id = ? AND turn_row.task_session_id = ? AND %s
	`, turnDispatchPendingPredicate(driverName, "turn_row"))
	var raw string
	if err := tx.GetContext(ctx, &raw, r.db.Rebind(query), turn.id, turn.sessionID); err != nil {
		return nil, err
	}
	metadata := map[string]interface{}{}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
			return nil, fmt.Errorf("decode unpublished prompt turn %s metadata: %w", turn.id, err)
		}
	}
	return metadata, nil
}

func (r *Repository) promptTurnHasMessages(ctx context.Context, tx *sqlx.Tx, turnID string) (bool, error) {
	var referenced bool
	if err := tx.GetContext(ctx, &referenced, r.db.Rebind(`
		SELECT EXISTS (SELECT 1 FROM task_session_messages WHERE turn_id = ?)
	`), turnID); err != nil {
		return false, fmt.Errorf("inspect unpublished prompt turn %s messages: %w", turnID, err)
	}
	return referenced, nil
}

func updateTurnMetadata(
	ctx context.Context,
	tx *sqlx.Tx,
	db *sqlx.DB,
	turnID string,
	metadata map[string]interface{},
	updatedAt time.Time,
) error {
	raw, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode published prompt turn %s metadata: %w", turnID, err)
	}
	if _, err := tx.ExecContext(ctx, db.Rebind(`
		UPDATE task_session_turns SET metadata = ?, updated_at = ? WHERE id = ?
	`), string(raw), updatedAt, turnID); err != nil {
		return fmt.Errorf("publish message-backed prompt turn %s: %w", turnID, err)
	}
	return nil
}

func (r *Repository) restoreReservedClarificationClaim(
	ctx context.Context,
	tx *sqlx.Tx,
	metadata map[string]interface{},
	updatedAt time.Time,
) error {
	pendingID, _ := metadata[models.TurnMetaKeyPromptDispatchClarificationPendingID].(string)
	turnID, _ := metadata[models.TurnMetaKeyPromptDispatchClarificationTurnID].(string)
	messageIDs, err := metadataStringSlice(metadata[models.TurnMetaKeyPromptDispatchClarificationMessageIDs])
	if err != nil {
		return fmt.Errorf("decode prompt dispatch clarification message ids: %w", err)
	}
	if pendingID == "" || turnID == "" || len(messageIDs) == 0 {
		return nil
	}
	for _, messageID := range messageIDs {
		if err := r.restoreReservedClarificationMessage(
			ctx, tx, messageID, pendingID, turnID, updatedAt,
		); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) restoreReservedClarificationMessage(
	ctx context.Context,
	tx *sqlx.Tx,
	messageID, pendingID, turnID string,
	updatedAt time.Time,
) error {
	driverName := r.db.DriverName()
	pendingIDExpr := dialect.JSONExtract(driverName, "metadata", "pending_id")
	query := fmt.Sprintf(`
		SELECT metadata
		FROM task_session_messages
		WHERE id = ? AND turn_id = ? AND type = 'clarification_request' AND %s = ?
	`, pendingIDExpr)
	var raw string
	if err := tx.GetContext(ctx, &raw, r.db.Rebind(query), messageID, turnID, pendingID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("load clarification message %s for prompt recovery: %w", messageID, err)
	}
	metadata := map[string]interface{}{}
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		return fmt.Errorf("decode clarification message %s for prompt recovery: %w", messageID, err)
	}
	// Only an answered bundle was claimed for the unpublished resume prompt.
	// Pending or rejected bundles were never reserved and must remain unchanged.
	if metadata["status"] != clarificationStatusAnswered {
		return nil
	}
	metadata["status"] = clarificationStatusPending
	delete(metadata, "response")
	delete(metadata, clarificationResponseDeliveryPendingKey)
	updated, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode clarification message %s for prompt recovery: %w", messageID, err)
	}
	statusExpr := dialect.JSONExtract(driverName, "metadata", "status")
	result, err := tx.ExecContext(ctx, r.db.Rebind(fmt.Sprintf(`
		UPDATE task_session_messages SET metadata = ?, updated_at = ?
		WHERE id = ? AND %s = ?
	`, statusExpr)), string(updated), updatedAt, messageID, clarificationStatusAnswered)
	if err != nil {
		return fmt.Errorf("restore clarification message %s after unpublished prompt: %w", messageID, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count restored clarification message %s: %w", messageID, err)
	}
	if rows != 1 {
		return fmt.Errorf("clarification message %s changed during prompt recovery", messageID)
	}
	return nil
}

func (r *Repository) deleteEmptyUnpublishedPromptTurn(
	ctx context.Context,
	tx *sqlx.Tx,
	turn unpublishedPromptTurn,
) error {
	query := fmt.Sprintf(`
		DELETE FROM task_session_turns
		WHERE id = ? AND task_session_id = ? AND %s
		  AND NOT EXISTS (SELECT 1 FROM task_session_messages WHERE turn_id = task_session_turns.id)
	`, turnDispatchPendingPredicate(r.db.DriverName(), "task_session_turns"))
	result, err := tx.ExecContext(ctx, r.db.Rebind(query), turn.id, turn.sessionID)
	if err != nil {
		return fmt.Errorf("delete unpublished prompt turn %s: %w", turn.id, err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count deleted unpublished prompt turn %s: %w", turn.id, err)
	}
	if rows != 1 {
		// The session write lock serializes message creation and reservation
		// publication with the checks above and this DELETE. Zero rows therefore
		// means an unexpected unowned write or corrupted reservation, so fail closed.
		return fmt.Errorf("unpublished prompt turn %s could not be removed (rows=%d) during recovery", turn.id, rows)
	}
	return nil
}

func metadataStringSlice(value interface{}) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	if values, ok := value.([]string); ok {
		for index, value := range values {
			if value == "" {
				return nil, fmt.Errorf("entry %d is empty", index)
			}
		}
		return values, nil
	}
	values, ok := value.([]interface{})
	if !ok {
		return nil, fmt.Errorf("expected an array, got %T", value)
	}
	result := make([]string, 0, len(values))
	for index, value := range values {
		text, ok := value.(string)
		if !ok || text == "" {
			return nil, fmt.Errorf("entry %d is not a non-empty string", index)
		}
		result = append(result, text)
	}
	return result, nil
}
