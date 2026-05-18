package sqlite

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"
)

// WorkspaceDeletionCounts summarizes workspace-owned rows for destructive confirmation.
type WorkspaceDeletionCounts struct {
	Agents int `json:"agents"`
	Skills int `json:"skills"`
}

// GetWorkspaceDeletionCounts returns counts shown before deleting a workspace.
func (r *Repository) GetWorkspaceDeletionCounts(ctx context.Context, workspaceID string) (*WorkspaceDeletionCounts, error) {
	counts := &WorkspaceDeletionCounts{}
	if err := r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT COUNT(*) FROM agent_profiles WHERE workspace_id = ? AND workspace_id != '' AND deleted_at IS NULL`),
		workspaceID,
	).Scan(&counts.Agents); err != nil {
		return nil, fmt.Errorf("count agents: %w", err)
	}
	if err := r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT COUNT(*) FROM office_skills WHERE workspace_id = ?`),
		workspaceID,
	).Scan(&counts.Skills); err != nil {
		return nil, fmt.Errorf("count skills: %w", err)
	}
	return counts, nil
}

// DeleteWorkspaceData deletes all office-domain rows owned by a workspace.
func (r *Repository) DeleteWorkspaceData(ctx context.Context, workspaceID string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	if err := r.deleteWorkspaceDataTx(ctx, tx, workspaceID); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return fmt.Errorf("%w; rollback: %v", err, rollbackErr)
		}
		return err
	}
	return tx.Commit()
}

func (r *Repository) deleteWorkspaceDataTx(ctx context.Context, tx *sqlx.Tx, workspaceID string) error {
	// Office agents are now rows in agent_profiles with workspace_id != ''.
	// We delete them via the merged table; shallow kanban profiles
	// (workspace_id = '') are unaffected.
	statements := []string{
		`DELETE FROM runs WHERE agent_profile_id IN (SELECT id FROM agent_profiles WHERE workspace_id = ?)`,
		`DELETE FROM office_agent_memory WHERE agent_profile_id IN (SELECT id FROM agent_profiles WHERE workspace_id = ?)`,
		`DELETE FROM office_agent_instructions WHERE agent_profile_id IN (SELECT id FROM agent_profiles WHERE workspace_id = ?)`,
		`DELETE FROM office_agent_runtime WHERE agent_id IN (SELECT id FROM agent_profiles WHERE workspace_id = ?)`,
		`DELETE FROM office_cost_events WHERE agent_profile_id IN (SELECT id FROM agent_profiles WHERE workspace_id = ?)`,
		`DELETE FROM office_cost_events WHERE project_id IN (SELECT id FROM office_projects WHERE workspace_id = ?)`,
		`DELETE FROM office_budget_policies WHERE workspace_id = ?`,
		`DELETE FROM office_routine_runs WHERE routine_id IN (SELECT id FROM office_routines WHERE workspace_id = ?)`,
		`DELETE FROM office_routine_triggers WHERE routine_id IN (SELECT id FROM office_routines WHERE workspace_id = ?)`,
		`DELETE FROM office_task_labels WHERE label_id IN (SELECT id FROM office_labels WHERE workspace_id = ?)`,
		`DELETE FROM office_labels WHERE workspace_id = ?`,
		`DELETE FROM office_channels WHERE workspace_id = ?`,
		`DELETE FROM office_approvals WHERE workspace_id = ?`,
		`DELETE FROM office_activity_log WHERE workspace_id = ?`,
		`DELETE FROM office_routines WHERE workspace_id = ?`,
		`DELETE FROM office_skills WHERE workspace_id = ?`,
		`DELETE FROM office_projects WHERE workspace_id = ?`,
		`DELETE FROM agent_profiles WHERE workspace_id = ? AND workspace_id != ''`,
		`DELETE FROM office_workspace_governance WHERE workspace_id = ?`,
		`DELETE FROM office_onboarding WHERE workspace_id = ?`,
	}
	for _, stmt := range statements {
		if _, err := tx.ExecContext(ctx, r.db.Rebind(stmt), workspaceID); err != nil {
			return fmt.Errorf("delete workspace data: %w", err)
		}
	}
	return nil
}
