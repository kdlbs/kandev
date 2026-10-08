package sqlite

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

// insertExecutorStatusHistoryTx commits a non-actionable lifecycle marker with
// its source fact. Its synthetic turn is never active and emits no completion.
func (r *Repository) insertExecutorStatusHistoryTx(ctx context.Context, tx *sqlx.Tx, id, taskID, sessionID, turnID string, metadata map[string]interface{}, now time.Time) error {
	if _, err := lockTaskSessionRow(ctx, tx, sessionID); err != nil {
		return err
	}
	if turnID == "" {
		turnID = uuid.NewSHA1(uuid.NameSpaceOID, []byte("executor-status-turn:"+id)).String()
		_, err := tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO task_session_turns(id,task_session_id,task_id,started_at,completed_at,created_at,updated_at,metadata) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(id) DO NOTHING`), turnID, sessionID, taskID, now, now, now, now, `{"lifecycle_only":true}`)
		if err != nil {
			return err
		}
	}
	raw, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO task_session_messages(id,task_session_id,task_id,turn_id,author_type,author_id,content,requests_input,type,metadata,created_at,updated_at,prompt_seq,payload_digest,payload_size) VALUES(?,?,?,?,'agent','','',0,'status',?,?,?,0,'',0) ON CONFLICT(id) DO NOTHING`), id, sessionID, taskID, turnID, string(raw), now, now)
	return err
}
