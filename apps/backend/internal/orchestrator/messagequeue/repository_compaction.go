package messagequeue

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

type compactionRawPayload struct{ content, attachments, metadata string }

func (r *sqliteRepository) compactSessionQueueTx(ctx context.Context, tx *sqlx.Tx, identity QueueSessionIdentity, leased map[string]struct{}) (int, error) {
	leased, err := r.compactionLeasesTx(ctx, tx, identity.SessionID, leased)
	if err != nil {
		return 0, err
	}
	rows, raw, err := r.readCompactionRowsTx(ctx, tx, identity.SessionID)
	if err != nil {
		return 0, err
	}
	fences, err := r.compactionFencesTx(ctx, tx, identity.SessionID)
	if err != nil {
		return 0, err
	}
	compacted := compactQueueRows(rows, leased, fences)
	for i := range compacted.kept {
		entry := &compacted.kept[i]
		if !entry.dirty {
			continue
		}
		payload := raw[entry.originalIndex]
		values := &autoMergedValues{content: entry.merged.Content, attachments: entry.merged.Attachments, metadata: entry.merged.Metadata, queuedAt: entry.merged.QueuedAt}
		if err := updateAdmissionMergeTargetTx(ctx, r, tx, entry.original, payload.content, payload.attachments, payload.metadata, values); err != nil {
			return 0, err
		}
	}
	for _, index := range compacted.removed {
		source, payload := &rows[index], raw[index]
		result, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM queued_messages WHERE id = ? AND session_id = ? AND position = ?
 AND content = ? AND attachments_json = ? AND metadata_json = ? AND queued_at = ?`), source.ID, source.SessionID, source.Position, payload.content, payload.attachments, payload.metadata, source.QueuedAt)
		if err != nil {
			return 0, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		if count != 1 {
			return 0, ErrQueueChanged
		}
	}
	return len(compacted.removed), nil
}

func (r *sqliteRepository) readCompactionRowsTx(ctx context.Context, tx *sqlx.Tx, sessionID string) ([]QueuedMessage, []compactionRawPayload, error) {
	rows, err := tx.QueryxContext(ctx, tx.Rebind(`SELECT id,session_id,task_id,position,content,model,plan_mode,
 attachments_json,metadata_json,queued_at,queued_by,content,attachments_json,metadata_json
 FROM queued_messages WHERE session_id = ? ORDER BY position ASC, id ASC`), sessionID)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()
	var messages []QueuedMessage
	var raw []compactionRawPayload
	for rows.Next() {
		message, content, attachments, metadata, err := scanQueuedRowWithRawJSON(rows)
		if err != nil {
			return nil, nil, err
		}
		messages = append(messages, *message)
		raw = append(raw, compactionRawPayload{content: content, attachments: attachments, metadata: metadata})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return messages, raw, nil
}

func (r *sqliteRepository) compactionLeasesTx(ctx context.Context, tx *sqlx.Tx, sessionID string, local map[string]struct{}) (map[string]struct{}, error) {
	var durable []string
	if err := tx.SelectContext(ctx, &durable, tx.Rebind(`SELECT entry_id FROM queue_edit_leases WHERE session_id = ? AND expires_at > ?`), sessionID, time.Now().UTC()); err != nil {
		return nil, err
	}
	if len(durable) == 0 {
		return local, nil
	}
	leased := make(map[string]struct{}, len(local)+len(durable))
	for id := range local {
		leased[id] = struct{}{}
	}
	for _, id := range durable {
		leased[id] = struct{}{}
	}
	return leased, nil
}

func (r *sqliteRepository) compactionFencesTx(ctx context.Context, tx *sqlx.Tx, sessionID string) (map[string]string, error) {
	var claims []struct {
		QueueID        string `db:"queue_id"`
		TaskID         string `db:"task_id"`
		InstallationID string `db:"installation_id"`
		InstanceKey    string `db:"instance_key"`
		Generation     int64  `db:"generation"`
	}
	if err := tx.SelectContext(ctx, &claims, tx.Rebind(`SELECT f.queue_id, f.task_id, f.installation_id, f.instance_key, f.generation
 FROM queue_task_management_fences f JOIN queued_messages m ON m.id = f.queue_id
 WHERE m.session_id = ?`), sessionID); err != nil {
		return nil, err
	}
	if len(claims) == 0 {
		return nil, nil
	}
	fences := make(map[string]string, len(claims))
	for _, claim := range claims {
		fences[claim.QueueID] = strings.Join([]string{
			claim.TaskID, claim.InstallationID, claim.InstanceKey, strconv.FormatInt(claim.Generation, 10),
		}, "\x00")
	}
	return fences, nil
}
