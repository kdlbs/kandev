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
		return r.foldTargetBlockedTx(ctx, tx, source.SessionID, row)
	}
	sender := ""
	if policy.sortedEnabled() {
		sender = sortedFoldSender(source)
	}
	if sender == "" {
		target, err := readMergeTarget(ctx, r, tx, source.SessionID, sourcePositionBound(source), source.ID)
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

// sourcePositionBound is the upper position for rows above source. A
// full-queue candidate has no persisted position, so every row is above it.
// The bound is the largest INTEGER column value: PostgreSQL stores position as
// int4 and rejects a wider bind parameter.
func sourcePositionBound(source *QueuedMessage) int64 {
	if source.Position > 0 {
		return source.Position
	}
	return math.MaxInt32
}

// sortedFoldScanBatch bounds each metadata read so a match near the tail never
// streams the rest of the backlog (PostgreSQL drains unread rows on close).
const sortedFoldScanBatch = 64

// sortedFoldScanQuery reads one batch below a (position, id) boundary. The
// row-value bound seeks idx_queued_messages_session_order directly, so each
// batch continues where the previous one ended instead of rescanning.
const sortedFoldScanQuery = `SELECT id, position, queued_by, metadata_json FROM queued_messages
 WHERE session_id = ? AND (position, id) < (?, ?) ORDER BY position DESC, id DESC LIMIT ?`

type sortedFoldScanRow struct {
	ID           string `db:"id"`
	Position     int64  `db:"position"`
	QueuedBy     string `db:"queued_by"`
	MetadataJSON string `db:"metadata_json"`
}

// sortedFoldCandidateIDsTx walks the persisted rows above source newest-first
// in keyset batches, decoding only identity and metadata, and stops at the
// first barrier or same-sender row. Payloads of backlog rows are never read.
// Positions are not unique (a restored reservation keeps its old slot), so the
// keyset continues on (position, id).
func (r *sqliteRepository) sortedFoldCandidateIDsTx(ctx context.Context, tx *sqlx.Tx, source *QueuedMessage, sender string) (adjacentID, sortedID string, crossed int, err error) {
	var last *sortedFoldScanRow
	for index := 0; ; {
		batch, err := r.sortedFoldScanBatchTx(ctx, tx, source, last)
		if err != nil {
			return "", "", 0, err
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
		last = &batch[len(batch)-1]
	}
}

func (r *sqliteRepository) sortedFoldScanBatchTx(ctx context.Context, tx *sqlx.Tx, source *QueuedMessage, last *sortedFoldScanRow) ([]sortedFoldScanRow, error) {
	position, id := sourcePositionBound(source), source.ID
	if last != nil {
		position, id = last.Position, last.ID
	}
	var batch []sortedFoldScanRow
	if err := tx.SelectContext(ctx, &batch, tx.Rebind(sortedFoldScanQuery),
		source.SessionID, position, id, sortedFoldScanBatch); err != nil {
		return nil, fmt.Errorf("scan sorted fold candidates: %w", err)
	}
	return batch, nil
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
			return r.foldTargetBlockedTx(ctx, tx, candidate.SessionID, row)
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

// foldTargetBlockedTx reports whether an automatic fold must leave target
// alone. A live edit lease holds the entry, and a task-management fence binds
// its content to a claim, so folding a new message into it would change what
// the claim delivers. Compaction applies the same rule.
func (r *sqliteRepository) foldTargetBlockedTx(ctx context.Context, tx *sqlx.Tx, sessionID string, target *QueuedMessage) (bool, error) {
	leased, err := r.editLeaseBlocksEntryTx(ctx, tx, sessionID, target.ID)
	if err != nil || leased {
		return leased, err
	}
	var fenced bool
	if err := tx.GetContext(ctx, &fenced, tx.Rebind(`
		SELECT EXISTS (SELECT 1 FROM queue_task_management_fences WHERE queue_id = ?)
	`), target.ID); err != nil {
		return false, fmt.Errorf("read fold target task-management fence: %w", err)
	}
	return fenced, nil
}
