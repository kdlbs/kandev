package messagequeue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

func (r *sqliteRepository) readAutoFoldTargetTx(ctx context.Context, tx *sqlx.Tx, source *QueuedMessage, policy *AdmissionPolicy) (*QueuedMessage, *autoMergedValues, error) {
	blocked := func(row *QueuedMessage) (bool, error) {
		return r.editLeaseBlocksEntryTx(ctx, tx, source.SessionID, row.ID)
	}
	sender := ""
	if policy.sortedEnabled() {
		sender = sortedFoldSender(source)
	}
	if sender == "" {
		target, err := readMergeTarget(ctx, r, tx, source.SessionID, sourcePositionBound(source))
		if errors.Is(err, ErrNoMergeTarget) {
			return nil, nil, nil
		}
		if err != nil {
			return nil, nil, err
		}
		values, err := autoFoldValues(target, source, policy, blocked)
		return target, values, err
	}
	return r.readSortedFoldTargetTx(ctx, tx, source, sender, policy, blocked)
}

func (r *sqliteRepository) readSortedFoldTargetTx(
	ctx context.Context, tx *sqlx.Tx, source *QueuedMessage, sender string,
	policy *AdmissionPolicy, blocked func(*QueuedMessage) (bool, error),
) (*QueuedMessage, *autoMergedValues, error) {
	adjacentID, sortedID, crossed, err := r.sortedFoldCandidateIDsTx(ctx, tx, source, sender)
	if err != nil {
		return nil, nil, err
	}
	if sortedID != "" {
		target, err := readQueuedRowByIDTx(ctx, tx, source.SessionID, sortedID)
		if err != nil {
			return nil, nil, err
		}
		values, err := autoFoldValues(target, source, policy, blocked)
		if values != nil && crossed > 0 && policy.log != nil {
			policy.log.Debug("sorted automatic merge target selected", zap.String("session_id", source.SessionID), zap.String("source_id", source.ID), zap.String("target_id", target.ID), zap.Int("crossed", crossed))
		}
		if err != nil || values != nil {
			return target, values, err
		}
	}
	if adjacentID == "" || adjacentID == sortedID {
		return nil, nil, nil
	}
	adjacent, err := readQueuedRowByIDTx(ctx, tx, source.SessionID, adjacentID)
	if err != nil {
		return nil, nil, err
	}
	values, err := autoFoldValues(adjacent, source, policy, blocked)
	if values == nil {
		return nil, nil, err
	}
	return adjacent, values, err
}

// sourcePositionBound is the exclusive upper position for rows above source.
// A full-queue candidate has no persisted position, so every row is above it.
func sourcePositionBound(source *QueuedMessage) int64 {
	if source.Position > 0 {
		return source.Position
	}
	return math.MaxInt64
}

// sortedFoldScanBatch bounds each metadata read so a match near the tail never
// streams the rest of the backlog (PostgreSQL drains unread rows on close).
const sortedFoldScanBatch = 64

type sortedFoldScanRow struct {
	ID           string `db:"id"`
	Position     int64  `db:"position"`
	QueuedBy     string `db:"queued_by"`
	MetadataJSON string `db:"metadata_json"`
}

// sortedFoldCandidateIDsTx walks the persisted rows above source newest-first
// in keyset batches, decoding only identity and metadata, and stops at the
// first barrier or same-sender row. Payloads of backlog rows are never read.
func (r *sqliteRepository) sortedFoldCandidateIDsTx(ctx context.Context, tx *sqlx.Tx, source *QueuedMessage, sender string) (adjacentID, sortedID string, crossed int, err error) {
	bound := sourcePositionBound(source)
	for index := 0; ; {
		var batch []sortedFoldScanRow
		if err := tx.SelectContext(ctx, &batch, tx.Rebind(`SELECT id, position, queued_by, metadata_json FROM queued_messages
 WHERE session_id = ? AND position < ? ORDER BY position DESC LIMIT ?`), source.SessionID, bound, sortedFoldScanBatch); err != nil {
			return "", "", 0, fmt.Errorf("scan sorted fold candidates: %w", err)
		}
		for _, scanned := range batch {
			row := QueuedMessage{ID: scanned.ID, QueuedBy: scanned.QueuedBy}
			if scanned.MetadataJSON != "" && scanned.MetadataJSON != "{}" {
				if err := json.Unmarshal([]byte(scanned.MetadataJSON), &row.Metadata); err != nil {
					return "", "", 0, fmt.Errorf("unmarshal metadata: %w", err)
				}
			}
			if index == 0 {
				adjacentID = row.ID
			}
			if isFoldBarrier(&row) {
				return adjacentID, "", 0, nil
			}
			if isSameFoldSender(&row, sender) {
				return adjacentID, row.ID, index, nil
			}
			index++
		}
		if len(batch) < sortedFoldScanBatch {
			return adjacentID, "", 0, nil
		}
		bound = batch[len(batch)-1].Position
	}
}

func readQueuedRowByIDTx(ctx context.Context, tx *sqlx.Tx, sessionID, id string) (*QueuedMessage, error) {
	row := tx.QueryRowxContext(ctx, tx.Rebind(`SELECT id, session_id, task_id, position, content, model, plan_mode,
 attachments_json, metadata_json, queued_at, queued_by FROM queued_messages WHERE session_id = ? AND id = ?`), sessionID, id)
	message, err := scanQueuedRow(row)
	if err != nil {
		return nil, fmt.Errorf("read fold target: %w", err)
	}
	return message, nil
}

type candidateFold struct {
	target                                  *QueuedMessage
	values                                  *autoMergedValues
	rawContent, rawAttachments, rawMetadata string
}

func (r *sqliteRepository) readCandidateFoldTx(ctx context.Context, tx *sqlx.Tx, candidate *QueuedMessage, policy *AdmissionPolicy) (*candidateFold, error) {
	if !policy.sortedEnabled() {
		target, content, attachments, metadata, err := r.scanTailWithRawJSON(ctx, tx, candidate.SessionID)
		if err != nil || target == nil {
			return nil, err
		}
		values, err := autoFoldValues(target, candidate, policy, func(row *QueuedMessage) (bool, error) {
			return r.editLeaseBlocksEntryTx(ctx, tx, candidate.SessionID, row.ID)
		})
		if err != nil || values == nil {
			return nil, err
		}
		return &candidateFold{target: target, values: values, rawContent: content, rawAttachments: attachments, rawMetadata: metadata}, nil
	}
	target, values, err := r.readAutoFoldTargetTx(ctx, tx, candidate, policy)
	if err != nil || values == nil {
		return nil, err
	}
	row := tx.QueryRowxContext(ctx, tx.Rebind(`SELECT id,session_id,task_id,position,content,model,plan_mode,
 attachments_json,metadata_json,queued_at,queued_by,content,attachments_json,metadata_json
 FROM queued_messages WHERE session_id = ? AND id = ?`), candidate.SessionID, target.ID)
	current, content, attachments, metadata, err := scanQueuedRowWithRawJSON(row)
	if err != nil {
		return nil, err
	}
	// The raw-byte read must still describe the selected snapshot, not a newer edit.
	if !reflect.DeepEqual(current, target) {
		return nil, ErrQueueChanged
	}
	return &candidateFold{target: target, values: values, rawContent: content, rawAttachments: attachments, rawMetadata: metadata}, nil
}
