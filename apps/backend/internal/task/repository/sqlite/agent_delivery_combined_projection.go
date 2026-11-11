package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/kandev/kandev/internal/task/models"
)

// ReceiveAndProjectCanonicalAgentDeliveryEvents admits one contiguous output
// batch into the inbox and projects it in the same writer transaction. Each
// event keeps its own idempotent receipt and effect, while the batch advances
// the received and projected cursors once. A failure rolls the whole batch
// back, so a retry starts from the same SQL state.
//
// The result reports, per event, whether it was already projected before the
// call. Already projected events carry their committed canonical message
// identity so callers can acknowledge without notifying again.
func (r *Repository) ReceiveAndProjectCanonicalAgentDeliveryEvents(
	ctx context.Context,
	events []*models.AgentDeliveryEvent,
	effects []*models.AgentDeliveryEffect,
	remoteHighWater int64,
) ([]models.CanonicalAgentDeliveryProjectionResult, error) {
	streamID, err := validateCanonicalDeliveryBatch(events, effects)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, nil
	}
	contentHashes := make([]string, len(events))
	for index, event := range events {
		if contentHashes[index], err = r.prepareAgentDeliveryEventReceipt(event); err != nil {
			return nil, err
		}
		if err := r.prepareAgentDeliveryOwnerStatement(ctx, event); err != nil {
			return nil, err
		}
	}
	if err := r.prepareAgentDeliveryReceiptStatement(ctx); err != nil {
		return nil, err
	}
	return projectCanonicalDeliveryWithRetry(ctx, func() ([]models.CanonicalAgentDeliveryProjectionResult, error) {
		return r.receiveAndProjectCanonicalDeliveryAttempt(ctx, streamID, events, effects, contentHashes, remoteHighWater)
	})
}

func (r *Repository) receiveAndProjectCanonicalDeliveryAttempt(
	ctx context.Context,
	streamID string,
	events []*models.AgentDeliveryEvent,
	effects []*models.AgentDeliveryEffect,
	contentHashes []string,
	remoteHighWater int64,
) ([]models.CanonicalAgentDeliveryProjectionResult, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockCombinedDeliveryWrites(ctx, tx, streamID, events, effects); err != nil {
		return nil, err
	}
	for index, event := range events {
		if _, err := r.receiveAgentDeliveryEventTx(ctx, tx, event, remoteHighWater, contentHashes[index]); err != nil {
			return nil, err
		}
	}
	if r.agentDeliveryCombinedProjectionFailpoint != nil {
		if err := r.agentDeliveryCombinedProjectionFailpoint("project"); err != nil {
			return nil, err
		}
	}
	projectedSequence, boundary, err := loadCanonicalDeliveryCursorTx(ctx, tx, r.db.Rebind, streamID)
	if err != nil {
		return nil, err
	}
	results := make([]models.CanonicalAgentDeliveryProjectionResult, len(events))
	if err := r.projectCanonicalDeliveryBatchTx(
		ctx, tx, events, effects, results, projectedSequence, boundary,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	r.refreshAgentDeliveryLag(ctx)
	return results, nil
}

// lockCombinedDeliveryWrites takes the union of the receipt and projection lock
// sets once, in the same sorted order both paths already use.
func (r *Repository) lockCombinedDeliveryWrites(
	ctx context.Context,
	tx taskSessionExecutor,
	streamID string,
	events []*models.AgentDeliveryEvent,
	effects []*models.AgentDeliveryEffect,
) error {
	sessionIDs := make([]string, 0, len(events))
	for _, event := range events {
		sessionIDs = append(sessionIDs, event.SessionID)
	}
	return lockAgentDeliveryEventWritesMode(
		ctx, tx, r.db.DriverName(), sessionIDs, []string{streamID}, combinedDeliverySharedSessions(events, effects),
	)
}

func combinedDeliverySharedSessions(
	events []*models.AgentDeliveryEvent,
	effects []*models.AgentDeliveryEffect,
) bool {
	for _, event := range events {
		if event.Terminal {
			return false
		}
	}
	return sharedCanonicalDeliverySession(events, effects)
}

// loadCanonicalDeliveryCursorTx reads the projected cursor and segment
// boundary the batch must continue from.
func loadCanonicalDeliveryCursorTx(
	ctx context.Context,
	tx sqlQueryRower,
	rebind func(string) string,
	streamID string,
) (int64, sql.NullInt64, error) {
	var projectedSequence int64
	var boundary sql.NullInt64
	if err := tx.QueryRowContext(ctx, rebind(`
		SELECT projected_sequence, message_segment_boundary_sequence
		FROM agent_delivery_cursors WHERE stream_id = ?`), streamID).Scan(&projectedSequence, &boundary); err != nil {
		return 0, sql.NullInt64{}, fmt.Errorf("load canonical delivery cursor: %w", err)
	}
	return projectedSequence, boundary, nil
}

type sqlQueryRower interface {
	QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row
}
