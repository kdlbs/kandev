package sqlite

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/orchestration/models"
	"time"
)

func (r *Repository) RevokeWorkspaceGrant(ctx context.Context, b *models.AssistantBinding, workspace string, expected int64) error {
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
	var row models.WorkspaceGrant
	if err = tx.GetContext(ctx, &row, tx.Rebind(`SELECT * FROM orchestration_workspace_grants WHERE binding_id=? AND workspace_id=? AND owner_user_id=? AND revision=?`), b.ID, workspace, b.OwnerUserID, expected); err != nil {
		return models.ErrConflict
	}
	if err = json.Unmarshal([]byte(row.ScopeJSON), &row.Scope); err != nil {
		return err
	}
	now := time.Now().UTC()
	row.RevokedAt = &now
	row.UpdatedAt = now
	row.Revision++
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE orchestration_workspace_grants SET revoked_at=?,updated_at=?,revision=revision+1 WHERE id=? AND revision=?`), now, now, row.ID, expected)
	if err = maintenanceRowChanged(result, err); err != nil {
		return err
	}
	if err = recordWorkspaceGrantEvent(ctx, tx, row, "revoked"); err != nil {
		return err
	}
	return tx.Commit()
}

func recordWorkspaceGrantEvent(ctx context.Context, tx *sqlx.Tx, row models.WorkspaceGrant, action string) error {
	raw, err := json.Marshal(row)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO orchestration_workspace_grant_events(id,grant_id,revision,action,snapshot_json,created_at) VALUES(?,?,?,?,?,?)`), uuid.NewString(), row.ID, row.Revision, action, string(raw), row.UpdatedAt)
	return err
}

func (r *Repository) WorkspaceGrantEvents(ctx context.Context, binding, workspace, after string, limit int) ([]models.WorkspaceGrantEvent, error) {
	rows := []models.WorkspaceGrantEvent{}
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`SELECT e.* FROM orchestration_workspace_grant_events e
 JOIN orchestration_workspace_grants g ON g.id=e.grant_id JOIN orchestration_assistant_bindings b ON b.id=g.binding_id AND b.owner_user_id=g.owner_user_id
 WHERE g.binding_id=? AND g.workspace_id=? AND e.id>? ORDER BY e.id LIMIT ?`), binding, workspace, after, min(max(limit, 1), 101))
	if err != nil {
		return nil, err
	}
	for i := range rows {
		if err = json.Unmarshal([]byte(rows[i].SnapshotJSON), &rows[i].Snapshot); err != nil {
			return nil, err
		}
	}
	return rows, nil
}
