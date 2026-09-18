package sqlite

import (
	"context"
	"encoding/json"
	"github.com/kandev/kandev/internal/orchestration/models"
	"time"
)

func (r *Repository) ForgetWorkspaceContext(ctx context.Context, b *models.AssistantBinding, workspace string, expected int64) error {
	if b == nil || expected < 1 {
		return models.ErrConflict
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err = lockAssistantBinding(ctx, tx, b); err != nil {
		return err
	}
	var g models.WorkspaceGrant
	err = tx.GetContext(ctx, &g, tx.Rebind(`SELECT * FROM orchestration_workspace_grants WHERE binding_id=? AND owner_user_id=? AND workspace_id=? AND revision=?`), b.ID, b.OwnerUserID, workspace, expected)
	if err != nil {
		return models.ErrConflict
	}
	if err = json.Unmarshal([]byte(g.ScopeJSON), &g.Scope); err != nil {
		return err
	}
	g.Revision++
	g.UpdatedAt = time.Now().UTC()
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE orchestration_workspace_grants SET revision=revision+1,updated_at=? WHERE id=? AND revision=?`), g.UpdatedAt, g.ID, expected)
	if err = maintenanceRowChanged(result, err); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`DELETE FROM orchestration_context_packets WHERE binding_id=? AND objective_id IN (SELECT id FROM orchestration_objectives WHERE binding_id=? AND workspace_id=?)`), b.ID, b.ID, workspace)
	if err != nil {
		return err
	}
	if err = recordWorkspaceGrantEvent(ctx, tx, g, "forgotten"); err != nil {
		return err
	}
	return tx.Commit()
}
