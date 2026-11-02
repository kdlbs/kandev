package sqlite

import (
	"context"

	"github.com/kandev/kandev/internal/task/retention"
)

func (r *Repository) migrateWorkspaceProtection(ctx context.Context) error {
	if err := r.migrate.Apply("task_resource_cleanup_jobs.workspace_protection", `ALTER TABLE task_resource_cleanup_jobs ADD COLUMN workspace_protection TEXT NOT NULL DEFAULT 'unclassified'`); err != nil {
		return err
	}
	if _, err := r.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_task_cleanup_workspace_protection ON task_resource_cleanup_jobs(workspace_protection,task_id)`); err != nil {
		return err
	}
	var rows []struct {
		ID       string `db:"id"`
		Snapshot string `db:"resource_snapshot"`
	}
	if err := r.db.SelectContext(ctx, &rows, `SELECT id,resource_snapshot FROM task_resource_cleanup_jobs WHERE workspace_protection='unclassified'`); err != nil {
		return err
	}
	for _, row := range rows {
		if _, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE task_resource_cleanup_jobs SET workspace_protection=? WHERE id=? AND workspace_protection='unclassified' AND resource_snapshot=?`), retention.SnapshotProtection(row.Snapshot), row.ID, row.Snapshot); err != nil {
			return err
		}
	}
	return nil
}
