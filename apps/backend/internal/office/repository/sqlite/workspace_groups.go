package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/office/models"
)

// createWorkspaceGroupTables creates the task_workspace_groups and
// task_workspace_group_members tables used by office task handoffs.
//
// Cleanup safety: owned_by_kandev defaults to 0 and cleanup_policy
// defaults to never_delete. Only the materializer that actually creates
// the workspace on disk is allowed to flip these — see
// MarkWorkspaceMaterialized.
func (r *Repository) createWorkspaceGroupTables() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS task_workspace_groups (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		owner_task_id TEXT NOT NULL,
		materialized_path TEXT NOT NULL DEFAULT '',
		materialized_environment_id TEXT NOT NULL DEFAULT '',
		materialized_kind TEXT NOT NULL,
		owned_by_kandev INTEGER NOT NULL DEFAULT 0,
		cleanup_policy TEXT NOT NULL DEFAULT 'never_delete',
		cleanup_status TEXT NOT NULL DEFAULT 'active',
		cleaned_at DATETIME,
		cleanup_error TEXT NOT NULL DEFAULT '',
		restore_status TEXT NOT NULL DEFAULT 'not_needed',
		restore_error TEXT NOT NULL DEFAULT '',
		restore_config_json TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_task_workspace_groups_workspace
		ON task_workspace_groups(workspace_id);
	CREATE INDEX IF NOT EXISTS idx_task_workspace_groups_owner
		ON task_workspace_groups(owner_task_id);

	CREATE TABLE IF NOT EXISTS task_workspace_group_members (
		workspace_group_id TEXT NOT NULL,
		task_id TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'member',
		released_at DATETIME,
		release_reason TEXT NOT NULL DEFAULT '',
		released_by_cascade_id TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL,
		PRIMARY KEY (workspace_group_id, task_id),
		FOREIGN KEY (workspace_group_id) REFERENCES task_workspace_groups(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_task_workspace_group_members_task
		ON task_workspace_group_members(task_id);
	CREATE INDEX IF NOT EXISTS idx_task_workspace_group_members_cascade
		ON task_workspace_group_members(released_by_cascade_id);
	`)
	return err
}

// CreateWorkspaceGroup inserts a fresh workspace group. Callers SHOULD
// leave OwnedByKandev=false and CleanupPolicy="" — the table defaults
// give a never-delete row. Use MarkWorkspaceMaterialized once a
// materializer actually creates a workspace on disk.
func (r *Repository) CreateWorkspaceGroup(ctx context.Context, g *models.WorkspaceGroup) error {
	if g.ID == "" {
		g.ID = uuid.New().String()
	}
	if g.MaterializedKind == "" {
		return errors.New("workspace group: MaterializedKind is required")
	}
	now := time.Now().UTC()
	if g.CreatedAt.IsZero() {
		g.CreatedAt = now
	}
	g.UpdatedAt = now
	if g.CleanupPolicy == "" {
		g.CleanupPolicy = models.WorkspaceCleanupPolicyNeverDelete
	}
	if g.CleanupStatus == "" {
		g.CleanupStatus = models.WorkspaceCleanupStatusActive
	}
	if g.RestoreStatus == "" {
		g.RestoreStatus = models.WorkspaceRestoreStatusNotNeeded
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_workspace_groups (
			id, workspace_id, owner_task_id, materialized_path,
			materialized_environment_id, materialized_kind, owned_by_kandev,
			cleanup_policy, cleanup_status, cleaned_at, cleanup_error,
			restore_status, restore_error, restore_config_json,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), g.ID, g.WorkspaceID, g.OwnerTaskID, g.MaterializedPath,
		g.MaterializedEnvironmentID, g.MaterializedKind, g.OwnedByKandev,
		g.CleanupPolicy, g.CleanupStatus, g.CleanedAt, g.CleanupError,
		g.RestoreStatus, g.RestoreError, g.RestoreConfigJSON,
		g.CreatedAt, g.UpdatedAt)
	return err
}

// GetWorkspaceGroup returns a single group by ID, or (nil, nil) when not found.
func (r *Repository) GetWorkspaceGroup(ctx context.Context, id string) (*models.WorkspaceGroup, error) {
	var g models.WorkspaceGroup
	err := r.ro.GetContext(ctx, &g, r.ro.Rebind(`
		SELECT id, workspace_id, owner_task_id, materialized_path,
			materialized_environment_id, materialized_kind, owned_by_kandev,
			cleanup_policy, cleanup_status, cleaned_at, cleanup_error,
			restore_status, restore_error, restore_config_json,
			created_at, updated_at
		FROM task_workspace_groups
		WHERE id = ?
	`), id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// MarkWorkspaceMaterialized records the materialized path/environment AND
// flips the ownership fields atomically. It is the ONLY supported path
// for setting owned_by_kandev=true: callers must pass m.OwnedByKandev=true
// only after they have actually created the workspace on disk. Whenever
// OwnedByKandev=true the cleanup policy is set to
// delete_when_last_member_archived_or_deleted; otherwise it is left as
// never_delete.
func (r *Repository) MarkWorkspaceMaterialized(ctx context.Context, id string, m models.MaterializedWorkspace) error {
	if id == "" {
		return errors.New("workspace group: id required")
	}
	if m.Kind == "" {
		return errors.New("workspace group: kind required")
	}
	policy := models.WorkspaceCleanupPolicyNeverDelete
	if m.OwnedByKandev {
		policy = models.WorkspaceCleanupPolicyDeleteWhenLastMemberArchivedOrDel
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE task_workspace_groups SET
			materialized_path = ?,
			materialized_environment_id = ?,
			materialized_kind = ?,
			owned_by_kandev = ?,
			cleanup_policy = ?,
			restore_config_json = ?,
			updated_at = ?
		WHERE id = ?
	`), m.Path, m.EnvironmentID, m.Kind, m.OwnedByKandev, policy,
		m.RestoreConfig, time.Now().UTC(), id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("workspace group %s: not found", id)
	}
	return tx.Commit()
}

// UpdateWorkspaceGroupCleanupStatus updates the cleanup_status (+ optional
// error and cleaned_at) and bumps updated_at. cleanedAt may be nil.
func (r *Repository) UpdateWorkspaceGroupCleanupStatus(ctx context.Context, id, status, errStr string, cleanedAt *time.Time) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_workspace_groups
		SET cleanup_status = ?, cleanup_error = ?, cleaned_at = ?, updated_at = ?
		WHERE id = ?
	`), status, errStr, cleanedAt, time.Now().UTC(), id)
	return err
}

// UpdateWorkspaceGroupRestoreStatus updates the restore_status (+ optional
// error) and bumps updated_at.
func (r *Repository) UpdateWorkspaceGroupRestoreStatus(ctx context.Context, id, status, errStr string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_workspace_groups
		SET restore_status = ?, restore_error = ?, updated_at = ?
		WHERE id = ?
	`), status, errStr, time.Now().UTC(), id)
	return err
}

// AddWorkspaceGroupMember inserts a membership row. role defaults to "member"
// when empty. INSERT OR IGNORE: re-adding the same task is a no-op.
func (r *Repository) AddWorkspaceGroupMember(ctx context.Context, groupID, taskID, role string) error {
	if groupID == "" || taskID == "" {
		return errors.New("workspace group member: groupID and taskID required")
	}
	if role == "" {
		role = models.WorkspaceMemberRoleMember
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT OR IGNORE INTO task_workspace_group_members (
			workspace_group_id, task_id, role, created_at
		) VALUES (?, ?, ?, ?)
	`), groupID, taskID, role, time.Now().UTC())
	return err
}

// ReleaseWorkspaceGroupMember stamps released_at + release_reason and
// optionally records the cascade ID that initiated the release. Calling
// it on an already-released membership is a no-op (the WHERE clause
// filters by released_at IS NULL) so cascades stay idempotent.
func (r *Repository) ReleaseWorkspaceGroupMember(ctx context.Context, groupID, taskID, reason, cascadeID string) error {
	if groupID == "" || taskID == "" {
		return errors.New("workspace group member: groupID and taskID required")
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_workspace_group_members
		SET released_at = ?, release_reason = ?, released_by_cascade_id = ?
		WHERE workspace_group_id = ? AND task_id = ? AND released_at IS NULL
	`), time.Now().UTC(), reason, cascadeID, groupID, taskID)
	return err
}

// RestoreWorkspaceGroupMemberByCascade clears released_at/release_reason
// only when the membership was released by the given cascade. Memberships
// released by other cascades or manual archive are left alone.
func (r *Repository) RestoreWorkspaceGroupMemberByCascade(ctx context.Context, taskID, cascadeID string) error {
	if taskID == "" || cascadeID == "" {
		return errors.New("restore membership: taskID and cascadeID required")
	}
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_workspace_group_members
		SET released_at = NULL, release_reason = '', released_by_cascade_id = ''
		WHERE task_id = ? AND released_by_cascade_id = ?
	`), taskID, cascadeID)
	return err
}

// ListWorkspaceGroupMembers returns every membership row for the group,
// including released members (audit history).
func (r *Repository) ListWorkspaceGroupMembers(ctx context.Context, groupID string) ([]models.WorkspaceGroupMember, error) {
	var members []models.WorkspaceGroupMember
	err := r.ro.SelectContext(ctx, &members, r.ro.Rebind(`
		SELECT workspace_group_id, task_id, role, released_at,
			release_reason, released_by_cascade_id, created_at
		FROM task_workspace_group_members
		WHERE workspace_group_id = ?
		ORDER BY created_at, task_id
	`), groupID)
	if err != nil {
		return nil, err
	}
	if members == nil {
		members = []models.WorkspaceGroupMember{}
	}
	return members, nil
}

// ListActiveWorkspaceGroupMembers returns members that are NOT released
// AND whose backing task is NOT archived. The JOIN against tasks lives
// here because the office and task repos share one SQLite DB; no
// cross-DB callback is needed.
func (r *Repository) ListActiveWorkspaceGroupMembers(ctx context.Context, groupID string) ([]models.WorkspaceGroupMember, error) {
	var members []models.WorkspaceGroupMember
	err := r.ro.SelectContext(ctx, &members, r.ro.Rebind(`
		SELECT m.workspace_group_id, m.task_id, m.role, m.released_at,
			m.release_reason, m.released_by_cascade_id, m.created_at
		FROM task_workspace_group_members m
		JOIN tasks t ON t.id = m.task_id
		WHERE m.workspace_group_id = ?
			AND m.released_at IS NULL
			AND t.archived_at IS NULL
		ORDER BY m.created_at, m.task_id
	`), groupID)
	if err != nil {
		return nil, err
	}
	if members == nil {
		members = []models.WorkspaceGroupMember{}
	}
	return members, nil
}

// GetWorkspaceGroupForTask returns the active (non-released) workspace
// group for a task, or (nil, nil) if the task is not a member of any
// active group.
func (r *Repository) GetWorkspaceGroupForTask(ctx context.Context, taskID string) (*models.WorkspaceGroup, error) {
	var g models.WorkspaceGroup
	err := r.ro.GetContext(ctx, &g, r.ro.Rebind(`
		SELECT g.id, g.workspace_id, g.owner_task_id, g.materialized_path,
			g.materialized_environment_id, g.materialized_kind, g.owned_by_kandev,
			g.cleanup_policy, g.cleanup_status, g.cleaned_at, g.cleanup_error,
			g.restore_status, g.restore_error, g.restore_config_json,
			g.created_at, g.updated_at
		FROM task_workspace_groups g
		JOIN task_workspace_group_members m ON m.workspace_group_id = g.id
		WHERE m.task_id = ? AND m.released_at IS NULL
		LIMIT 1
	`), taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}
