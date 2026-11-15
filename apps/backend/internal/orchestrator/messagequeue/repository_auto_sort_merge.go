package messagequeue

import (
	"context"
	"errors"
	"reflect"

	"github.com/jmoiron/sqlx"
)

func (r *sqliteRepository) readAutoFoldTargetTx(ctx context.Context, tx *sqlx.Tx, source *QueuedMessage, policy *AdmissionPolicy) (*QueuedMessage, *autoMergedValues, error) {
	blocked := func(row *QueuedMessage) (bool, error) {
		return r.editLeaseBlocksEntryTx(ctx, tx, source.SessionID, row.ID)
	}
	if !policy.sortedEnabled() {
		target, err := readMergeTarget(ctx, r, tx, source.SessionID, source.Position)
		if errors.Is(err, ErrNoMergeTarget) {
			return nil, nil, nil
		}
		if err != nil {
			return nil, nil, err
		}
		values, err := autoFoldValues(target, source, policy, blocked)
		return target, values, err
	}
	rows, err := r.listBySessionTx(ctx, tx, source.SessionID)
	if err != nil {
		return nil, nil, err
	}
	if source.Position > 0 {
		for i := range rows {
			if rows[i].Position >= source.Position {
				rows = rows[:i]
				break
			}
		}
	}
	return autoFoldTarget(rows, source, policy, blocked)
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
