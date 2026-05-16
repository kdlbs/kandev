package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/office/models"
)

func (r *Repository) createTreeHoldTables() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS office_task_tree_holds (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		root_task_id TEXT NOT NULL,
		mode TEXT NOT NULL,
		release_policy TEXT NOT NULL DEFAULT '{"strategy":"manual"}',
		released_at DATETIME,
		released_by TEXT DEFAULT '',
		released_reason TEXT DEFAULT '',
		created_at DATETIME NOT NULL,
		FOREIGN KEY (root_task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);

	CREATE TABLE IF NOT EXISTS office_task_tree_hold_members (
		hold_id TEXT NOT NULL,
		task_id TEXT NOT NULL,
		depth INTEGER NOT NULL,
		task_status TEXT NOT NULL DEFAULT '',
		skip_reason TEXT NOT NULL DEFAULT '',
		PRIMARY KEY (hold_id, task_id),
		FOREIGN KEY (hold_id) REFERENCES office_task_tree_holds(id) ON DELETE CASCADE,
		FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_tree_holds_root_mode ON office_task_tree_holds(root_task_id, mode, released_at);
	CREATE INDEX IF NOT EXISTS idx_tree_hold_members_task ON office_task_tree_hold_members(task_id);
	`)
	return err
}

func (r *Repository) FindSubtree(ctx context.Context, rootTaskID string) ([]models.SubtreeMember, error) {
	var members []models.SubtreeMember
	err := r.ro.SelectContext(ctx, &members, r.ro.Rebind(`
		WITH RECURSIVE subtree(id, depth) AS (
			SELECT id, 0 FROM tasks WHERE id = ?
			UNION ALL
			SELECT t.id, s.depth + 1
			FROM tasks t JOIN subtree s ON t.parent_id = s.id
			WHERE t.archived_at IS NULL
		)
		SELECT id, depth FROM subtree ORDER BY depth, id
	`), rootTaskID)
	if err != nil {
		return nil, err
	}
	if members == nil {
		members = []models.SubtreeMember{}
	}
	return members, nil
}

func (r *Repository) CreateTreeHold(ctx context.Context, hold *models.TreeHold) error {
	if hold.ID == "" {
		hold.ID = uuid.New().String()
	}
	if hold.ReleasePolicy == "" {
		hold.ReleasePolicy = `{"strategy":"manual"}`
	}
	hold.CreatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO office_task_tree_holds (
			id, workspace_id, root_task_id, mode, release_policy,
			released_at, released_by, released_reason, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), hold.ID, hold.WorkspaceID, hold.RootTaskID, hold.Mode, hold.ReleasePolicy,
		hold.ReleasedAt, hold.ReleasedBy, hold.ReleasedReason, hold.CreatedAt)
	return err
}

func (r *Repository) CreateTreeHoldMembers(
	ctx context.Context,
	holdID string,
	members []models.TreeHoldMember,
) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PreparexContext(ctx, r.db.Rebind(`
		INSERT INTO office_task_tree_hold_members (
			hold_id, task_id, depth, task_status, skip_reason
		) VALUES (?, ?, ?, ?, ?)
	`))
	if err != nil {
		return err
	}
	defer func() { _ = stmt.Close() }()
	for _, m := range members {
		if _, err := stmt.ExecContext(ctx, holdID, m.TaskID, m.Depth, m.TaskStatus, m.SkipReason); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (r *Repository) GetActiveHold(ctx context.Context, rootTaskID, mode string) (*models.TreeHold, error) {
	var hold models.TreeHold
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT * FROM office_task_tree_holds
		WHERE root_task_id = ? AND mode = ? AND released_at IS NULL
		ORDER BY created_at DESC
		LIMIT 1
	`), rootTaskID, mode).StructScan(&hold)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &hold, nil
}

func (r *Repository) GetActiveHoldForMember(ctx context.Context, taskID string) (*models.TreeHold, error) {
	var hold models.TreeHold
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT h.* FROM office_task_tree_holds h
		JOIN office_task_tree_hold_members m ON m.hold_id = h.id
		WHERE m.task_id = ? AND h.released_at IS NULL
		ORDER BY h.created_at DESC
		LIMIT 1
	`), taskID).StructScan(&hold)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &hold, nil
}

func (r *Repository) ReleaseHold(ctx context.Context, holdID, releasedBy, reason string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE office_task_tree_holds
		SET released_at = ?, released_by = ?, released_reason = ?
		WHERE id = ? AND released_at IS NULL
	`), time.Now().UTC(), releasedBy, reason, holdID)
	return err
}

func (r *Repository) ListHoldMembers(ctx context.Context, holdID string) ([]models.TreeHoldMember, error) {
	var members []models.TreeHoldMember
	err := r.ro.SelectContext(ctx, &members, r.ro.Rebind(`
		SELECT * FROM office_task_tree_hold_members
		WHERE hold_id = ?
		ORDER BY depth, task_id
	`), holdID)
	if err != nil {
		return nil, err
	}
	if members == nil {
		members = []models.TreeHoldMember{}
	}
	return members, nil
}

func (r *Repository) PreviewSubtree(ctx context.Context, rootTaskID string) ([]*models.TreePreviewTask, error) {
	var tasks []*models.TreePreviewTask
	err := r.ro.SelectContext(ctx, &tasks, r.ro.Rebind(`
		WITH RECURSIVE subtree(id, depth) AS (
			SELECT id, 0 FROM tasks WHERE id = ?
			UNION ALL
			SELECT t.id, s.depth + 1
			FROM tasks t JOIN subtree s ON t.parent_id = s.id
			WHERE t.archived_at IS NULL
		)
		SELECT t.id, COALESCE(t.title, '') AS title, COALESCE(t.state, '') AS status, s.depth
		FROM subtree s JOIN tasks t ON t.id = s.id
		ORDER BY s.depth, t.id
	`), rootTaskID)
	if err != nil {
		return nil, err
	}
	if tasks == nil {
		tasks = []*models.TreePreviewTask{}
	}
	return tasks, nil
}

func (r *Repository) CountActiveRunsForTasks(ctx context.Context, taskIDs []string) (int, error) {
	if len(taskIDs) == 0 {
		return 0, nil
	}
	query, args := taskIDInQuery(`
		SELECT COUNT(*) FROM runs
		WHERE status IN ('queued', 'claimed')
		  AND json_extract(payload, '$.task_id') IN (%s)
	`, taskIDs)
	var count int
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(query), args...).Scan(&count)
	return count, err
}

func (r *Repository) CancelRunsForTasks(ctx context.Context, taskIDs []string, reason string) (int, error) {
	if len(taskIDs) == 0 {
		return 0, nil
	}
	query, args := taskIDInQuery(`
		UPDATE runs
		SET status = 'cancelled', cancel_reason = ?, finished_at = ?
		WHERE status IN ('queued', 'claimed')
		  AND json_extract(payload, '$.task_id') IN (%s)
	`, taskIDs)
	args = append([]interface{}{reason, time.Now().UTC()}, args...)
	res, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...)
	if err != nil {
		return 0, err
	}
	rows, _ := res.RowsAffected()
	return int(rows), nil
}

func (r *Repository) BulkUpdateTaskState(ctx context.Context, taskIDs []string, state string) error {
	if len(taskIDs) == 0 {
		return nil
	}
	query, args := taskIDInQuery(`UPDATE tasks SET state = ?, updated_at = CURRENT_TIMESTAMP WHERE id IN (%s)`, taskIDs)
	args = append([]interface{}{state}, args...)
	_, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...)
	return err
}

func (r *Repository) BulkReleaseTaskCheckout(ctx context.Context, taskIDs []string) error {
	if len(taskIDs) == 0 {
		return nil
	}
	query, args := taskIDInQuery(`
		UPDATE tasks SET checkout_agent_id = '', checkout_at = NULL, updated_at = CURRENT_TIMESTAMP
		WHERE id IN (%s)
	`, taskIDs)
	_, err := r.db.ExecContext(ctx, r.db.Rebind(query), args...)
	return err
}

func (r *Repository) GetSubtreeCostSummary(
	ctx context.Context,
	rootTaskID string,
) (*models.SubtreeCostSummary, error) {
	var summary models.SubtreeCostSummary
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		WITH RECURSIVE subtree(id) AS (
			SELECT id FROM tasks WHERE id = ?
			UNION ALL
			SELECT t.id FROM tasks t JOIN subtree s ON t.parent_id = s.id
			WHERE t.archived_at IS NULL
		)
		SELECT
			COUNT(DISTINCT t.id) AS task_count,
			COALESCE(SUM(e.cost_subcents), 0) AS cost_subcents,
			COALESCE(SUM(e.tokens_in), 0) AS tokens_in,
			COALESCE(SUM(e.tokens_cached_in), 0) AS tokens_cached_in,
			COALESCE(SUM(e.tokens_out), 0) AS tokens_out
		FROM subtree t
		LEFT JOIN office_cost_events e ON e.task_id = t.id
	`), rootTaskID).StructScan(&summary)
	if err != nil {
		return nil, err
	}
	summary.RootTaskID = rootTaskID
	summary.IncludeDescendants = true
	return &summary, nil
}

func taskIDInQuery(template string, taskIDs []string) (string, []interface{}) {
	placeholders := make([]string, len(taskIDs))
	args := make([]interface{}, len(taskIDs))
	for i, id := range taskIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	return fmt.Sprintf(template, strings.Join(placeholders, ",")), args
}
