package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// GetWorkspaceChief hides deleted or moved identities rather than returning a stale link.
func (r *Repository) GetWorkspaceChief(ctx context.Context, workspaceID string) (string, error) {
	var id string
	err := r.ro.GetContext(ctx, &id, r.ro.Rebind(`SELECT c.agent_profile_id FROM office_workspace_chief c
 JOIN agent_profiles a ON a.id = c.agent_profile_id AND a.workspace_id = c.workspace_id
 WHERE c.workspace_id = ? AND a.deleted_at IS NULL`), workspaceID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return id, err
}

// SetWorkspaceChief stores one explicit choice per workspace, without scheduling a run.
func (r *Repository) SetWorkspaceChief(ctx context.Context, workspaceID, agentID string) error {
	if agentID == "" {
		_, err := r.db.ExecContext(ctx, r.db.Rebind(`DELETE FROM office_workspace_chief WHERE workspace_id = ?`), workspaceID)
		return err
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`INSERT INTO office_workspace_chief (workspace_id, agent_profile_id)
 SELECT workspace_id, id FROM agent_profiles WHERE id = ? AND workspace_id = ? AND role <> '' AND deleted_at IS NULL
 ON CONFLICT (workspace_id) DO UPDATE SET agent_profile_id = excluded.agent_profile_id`), agentID, workspaceID)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return fmt.Errorf("chief must be an agent in this workspace")
	}
	return nil
}

// ValidateWorkspace checks existence without changing workflow configuration.
func (r *Repository) ValidateWorkspace(ctx context.Context, workspaceID string) error {
	var id string
	return r.ro.GetContext(ctx, &id, r.ro.Rebind(`SELECT id FROM workspaces WHERE id = ?`), workspaceID)
}
