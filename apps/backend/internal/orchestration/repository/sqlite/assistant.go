package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (r *Repository) AssistantBinding(ctx context.Context, owner string) (*models.AssistantBinding, error) {
	var row models.AssistantBinding
	err := r.db.GetContext(ctx, &row, r.db.Rebind(`SELECT b.* FROM orchestration_assistant_bindings b
		JOIN workspace_orchestrators o ON o.agent_id=b.orchestrator_id AND o.workspace_id=b.workspace_id
		JOIN tasks t ON t.id=b.conversation_id AND t.workspace_id=b.workspace_id
		WHERE b.owner_user_id=?`), owner)
	return &row, err
}

func (r *Repository) SelectAssistant(ctx context.Context, row *models.AssistantBinding, expected int64) error {
	if row.ExecutionMode == "" {
		row.ExecutionMode = "inspect"
	}
	switch row.ExecutionMode {
	case "answer", "inspect", "design", "execute":
	default:
		return models.ErrConflict
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := claimConversation(ctx, tx, row); err != nil {
		return err
	}
	now := time.Now().UTC()
	var result sql.Result
	if expected == 0 {
		row.ID, row.Version, row.CreatedAt, row.UpdatedAt = uuid.NewString(), 1, now, now
		result, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO orchestration_assistant_bindings
			(id,owner_user_id,orchestrator_id,workspace_id,conversation_id,execution_mode,version,created_at,updated_at)
			VALUES(?,?,?,?,?,?,1,?,?) ON CONFLICT DO NOTHING`),
			row.ID, row.OwnerUserID, row.OrchestratorID, row.WorkspaceID, row.ConversationID, row.ExecutionMode, now, now)
	} else {
		result, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE orchestration_assistant_bindings
			SET orchestrator_id=?,workspace_id=?,conversation_id=?,execution_mode=?,version=version+1,updated_at=?
			WHERE owner_user_id=? AND version=?
			AND NOT EXISTS (SELECT 1 FROM orchestration_assistant_bindings WHERE orchestrator_id=? AND owner_user_id<>?)`),
			row.OrchestratorID, row.WorkspaceID, row.ConversationID, row.ExecutionMode, now,
			row.OwnerUserID, expected, row.OrchestratorID, row.OwnerUserID)
	}
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return models.ErrConflict
	}
	if err = tx.GetContext(ctx, row, tx.Rebind(`SELECT * FROM orchestration_assistant_bindings WHERE owner_user_id=?`), row.OwnerUserID); err != nil {
		return err
	}
	if err := supersedePrivateIntake(ctx, tx, row); err != nil {
		return err
	}
	return tx.Commit()
}

// ConversationUserOwner reads retained ownership, never the mutable default
// pointer. Only conversations that have never been claimed are shared.
func (r *Repository) ConversationUserOwner(ctx context.Context, taskID string) (string, error) {
	var owner string
	err := r.db.GetContext(ctx, &owner, r.db.Rebind(`SELECT COALESCE(MAX(owner_user_id),'')
		FROM orchestration_conversations WHERE task_id=?`), taskID)
	return owner, err
}

func claimConversation(ctx context.Context, tx *sqlx.Tx, row *models.AssistantBinding) error {
	if row.OwnerUserID == "" {
		return models.ErrConflict
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE orchestration_conversations SET owner_user_id=?
		WHERE task_id=? AND agent_profile_id=? AND workspace_id=? AND platform='web'
		AND (owner_user_id='' OR owner_user_id=?)`),
		row.OwnerUserID, row.ConversationID, row.OrchestratorID, row.WorkspaceID, row.OwnerUserID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err == nil && n != 1 {
		return models.ErrConflict
	}
	return err
}
