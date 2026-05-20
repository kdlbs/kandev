package sqlite

import "context"

// GetRequireApprovalForNewAgents returns the governance setting for new agent approval.
func (r *Repository) GetRequireApprovalForNewAgents(ctx context.Context, workspaceID string) (bool, error) {
	return r.getGovernanceBool(ctx, workspaceID, "require_approval_for_new_agents")
}

// SetRequireApprovalForNewAgents persists the governance setting for new agent approval.
func (r *Repository) SetRequireApprovalForNewAgents(ctx context.Context, workspaceID string, required bool) error {
	return r.setGovernanceBool(ctx, workspaceID, "require_approval_for_new_agents", required)
}

// GetRequireApprovalForTaskCompletion returns the governance setting for task-completion approval.
func (r *Repository) GetRequireApprovalForTaskCompletion(ctx context.Context, workspaceID string) (bool, error) {
	return r.getGovernanceBool(ctx, workspaceID, "require_approval_for_task_completion")
}

// SetRequireApprovalForTaskCompletion persists the governance setting for task-completion approval.
func (r *Repository) SetRequireApprovalForTaskCompletion(ctx context.Context, workspaceID string, required bool) error {
	return r.setGovernanceBool(ctx, workspaceID, "require_approval_for_task_completion", required)
}

// GetRequireApprovalForSkillChanges returns the governance setting for skill-change approval.
func (r *Repository) GetRequireApprovalForSkillChanges(ctx context.Context, workspaceID string) (bool, error) {
	return r.getGovernanceBool(ctx, workspaceID, "require_approval_for_skill_changes")
}

// SetRequireApprovalForSkillChanges persists the governance setting for skill-change approval.
func (r *Repository) SetRequireApprovalForSkillChanges(ctx context.Context, workspaceID string, required bool) error {
	return r.setGovernanceBool(ctx, workspaceID, "require_approval_for_skill_changes", required)
}

func (r *Repository) getGovernanceBool(ctx context.Context, workspaceID, key string) (bool, error) {
	var val int
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT COALESCE(value, 0) FROM office_workspace_governance
		WHERE workspace_id = ? AND key = ?
	`), workspaceID, key).Scan(&val)
	if err != nil {
		// Row not found means default (false).
		return false, nil
	}
	return val != 0, nil
}

func (r *Repository) setGovernanceBool(ctx context.Context, workspaceID, key string, value bool) error {
	v := 0
	if value {
		v = 1
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO office_workspace_governance (workspace_id, key, value)
		VALUES (?, ?, ?)
		ON CONFLICT(workspace_id, key) DO UPDATE SET value = excluded.value
	`), workspaceID, key, v)
	return err
}
