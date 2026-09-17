package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (r *Repository) AssistantForConversation(ctx context.Context, taskID string) (*models.AssistantBinding, error) {
	var row models.AssistantBinding
	err := r.db.GetContext(ctx, &row, r.db.Rebind(`SELECT * FROM orchestration_assistant_bindings WHERE conversation_id=?`), taskID)
	return &row, err
}

// BeginOperation commits the dispatch decision before calling an external
// dependency. The guarded insert is the intent/binding linearization point.
func (r *Repository) BeginOperation(ctx context.Context, row models.Operation) (*models.Operation, bool, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()
	row.ID = uuid.NewString()
	now := time.Now().UTC()
	result, err := tx.ExecContext(ctx, tx.Rebind(`INSERT INTO orchestration_operations
		(id,binding_id,operation_id,conversation_id,run_id,target,request_hash,intent_revision,binding_version,state,created_at,updated_at)
		SELECT ?,b.id,?,?,?,?,?,?,b.version,'prepared',?,? FROM orchestration_assistant_bindings b
		WHERE b.id=? AND b.conversation_id=? AND b.version=?
		AND COALESCE((SELECT revision FROM orchestration_conversation_intents WHERE task_id=?),0)=?
		AND NOT EXISTS (SELECT 1 FROM orchestration_operations pending WHERE pending.binding_id=b.id
		AND pending.intent_revision=? AND pending.target=? AND pending.state='unknown')
		ON CONFLICT(binding_id,operation_id) DO NOTHING`),
		row.ID, row.OperationID, row.ConversationID, row.RunID, row.Target, row.RequestHash, row.IntentRevision, now, now,
		row.BindingID, row.ConversationID, row.BindingVersion, row.ConversationID, row.IntentRevision, row.IntentRevision, row.Target)
	if err != nil {
		return nil, false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return nil, false, err
	}
	var saved models.Operation
	err = tx.GetContext(ctx, &saved, tx.Rebind(`SELECT * FROM orchestration_operations WHERE binding_id=? AND operation_id=?`), row.BindingID, row.OperationID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, models.ErrConflict
	}
	if err != nil {
		return nil, false, err
	}
	if saved.RequestHash != row.RequestHash || saved.Target != row.Target ||
		saved.ConversationID != row.ConversationID || saved.IntentRevision != row.IntentRevision || saved.BindingVersion != row.BindingVersion {
		return nil, false, models.ErrConflict
	}
	return &saved, n == 1, tx.Commit()
}

func (r *Repository) FinishOperation(ctx context.Context, id, state, response string, status int) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE orchestration_operations
		SET state=?,response_json=?,http_status=?,updated_at=? WHERE id=? AND state IN ('prepared','dispatched')`),
		state, response, status, time.Now().UTC(), id)
	return err
}

func (r *Repository) RecoverOperations(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `UPDATE orchestration_operations SET state='unknown',updated_at=CURRENT_TIMESTAMP WHERE state IN ('prepared','dispatched')`)
	return err
}

func (r *Repository) DispatchOperation(ctx context.Context, row *models.Operation) error {
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE orchestration_operations SET state='dispatched',updated_at=?
		WHERE id=? AND state='prepared' AND EXISTS (SELECT 1 FROM orchestration_assistant_bindings b
		WHERE b.id=? AND b.version=? AND b.conversation_id=?)
		AND COALESCE((SELECT revision FROM orchestration_conversation_intents WHERE task_id=?),0)=?`),
		time.Now().UTC(), row.ID, row.BindingID, row.BindingVersion, row.ConversationID, row.ConversationID, row.IntentRevision)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return models.ErrConflict
	}
	return err
}
