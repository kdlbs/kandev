package sqlite

import (
	"context"
	"database/sql"
	"errors"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *Repository) initWorkspaceCollaborationSchema() error {
	_, err := r.db.ExecContext(r.migrationContext(), `
		CREATE TABLE IF NOT EXISTS workspace_collaboration_grants (
		    caller_task_id TEXT NOT NULL,
		    source_workspace_id TEXT NOT NULL,
		    target_workspace_id TEXT NOT NULL,
		    can_read INTEGER NOT NULL DEFAULT 0,
		    can_report INTEGER NOT NULL DEFAULT 0,
		    can_prompt INTEGER NOT NULL DEFAULT 0,
		    can_control INTEGER NOT NULL DEFAULT 0,
		    updated_by TEXT NOT NULL,
		    updated_at TIMESTAMP NOT NULL,
		    PRIMARY KEY (caller_task_id, source_workspace_id, target_workspace_id),
		    FOREIGN KEY (caller_task_id) REFERENCES tasks(id) ON DELETE CASCADE,
		    FOREIGN KEY (source_workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE,
		    FOREIGN KEY (target_workspace_id) REFERENCES workspaces(id) ON DELETE CASCADE
		)
	`)
	return err
}

func (r *Repository) GetWorkspaceCollaborationGrant(ctx context.Context, taskID, sourceID, targetID string) (*models.WorkspaceCollaborationGrant, error) {
	var grant models.WorkspaceCollaborationGrant
	err := r.ro.GetContext(ctx, &grant, r.ro.Rebind(`
		SELECT * FROM workspace_collaboration_grants
		WHERE caller_task_id = ? AND source_workspace_id = ? AND target_workspace_id = ?
	`), taskID, sourceID, targetID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &grant, nil
}

func (r *Repository) ListWorkspaceCollaborationGrants(ctx context.Context, taskID, sourceID string) ([]*models.WorkspaceCollaborationGrant, error) {
	grants := []*models.WorkspaceCollaborationGrant{}
	err := r.ro.SelectContext(ctx, &grants, r.ro.Rebind(`
		SELECT * FROM workspace_collaboration_grants
		WHERE caller_task_id = ? AND source_workspace_id = ? ORDER BY target_workspace_id
	`), taskID, sourceID)
	return grants, err
}

func (r *Repository) UpsertWorkspaceCollaborationGrant(ctx context.Context, g *models.WorkspaceCollaborationGrant) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO workspace_collaboration_grants (
		    caller_task_id, source_workspace_id, target_workspace_id,
		    can_read, can_report, can_prompt, can_control, updated_by, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (caller_task_id, source_workspace_id, target_workspace_id)
		DO UPDATE SET can_read = excluded.can_read, can_report = excluded.can_report,
		    can_prompt = excluded.can_prompt, can_control = excluded.can_control,
		    updated_by = excluded.updated_by, updated_at = excluded.updated_at
	`), g.CallerTaskID, g.SourceWorkspaceID, g.TargetWorkspaceID,
		dialect.BoolToInt(g.Read), dialect.BoolToInt(g.Report), dialect.BoolToInt(g.Prompt), dialect.BoolToInt(g.Control), g.UpdatedBy, g.UpdatedAt)
	return err
}
