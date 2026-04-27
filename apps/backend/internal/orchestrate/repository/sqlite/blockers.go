package sqlite

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

// CreateTaskBlocker creates a blocker relationship between two tasks.
func (r *Repository) CreateTaskBlocker(ctx context.Context, blocker *models.TaskBlocker) error {
	blocker.CreatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_blockers (task_id, blocker_task_id, created_at)
		VALUES (?, ?, ?)
	`), blocker.TaskID, blocker.BlockerTaskID, blocker.CreatedAt)
	return err
}

// ListTaskBlockers returns all blockers for a task.
func (r *Repository) ListTaskBlockers(ctx context.Context, taskID string) ([]*models.TaskBlocker, error) {
	var blockers []*models.TaskBlocker
	err := r.ro.SelectContext(ctx, &blockers, r.ro.Rebind(
		`SELECT * FROM task_blockers WHERE task_id = ? ORDER BY created_at`), taskID)
	if err != nil {
		return nil, err
	}
	if blockers == nil {
		blockers = []*models.TaskBlocker{}
	}
	return blockers, nil
}

// DeleteTaskBlocker removes a blocker relationship.
func (r *Repository) DeleteTaskBlocker(ctx context.Context, taskID, blockerTaskID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM task_blockers WHERE task_id = ? AND blocker_task_id = ?`), taskID, blockerTaskID)
	return err
}

// ListTasksBlockedBy returns task IDs that are blocked by the given task.
func (r *Repository) ListTasksBlockedBy(ctx context.Context, blockerTaskID string) ([]string, error) {
	var ids []string
	err := r.ro.SelectContext(ctx, &ids, r.ro.Rebind(
		`SELECT task_id FROM task_blockers WHERE blocker_task_id = ?`), blockerTaskID)
	if err != nil {
		return nil, err
	}
	return ids, nil
}

// IsTaskInTerminalStep checks if a task is in a terminal workflow step (Done/Cancelled).
func (r *Repository) IsTaskInTerminalStep(ctx context.Context, taskID string) (bool, error) {
	var state string
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT COALESCE(state, '') FROM tasks WHERE id = ?`), taskID).Scan(&state)
	if err != nil {
		return false, err
	}
	return state == "COMPLETED" || state == "CANCELLED", nil
}

// GetTaskAssignee returns the assignee agent instance ID for a task.
func (r *Repository) GetTaskAssignee(ctx context.Context, taskID string) (string, error) {
	var assignee string
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT COALESCE(assignee_agent_instance_id, '') FROM tasks WHERE id = ?`), taskID).Scan(&assignee)
	if err != nil {
		return "", err
	}
	return assignee, nil
}

// AreAllChildrenTerminal checks if all child tasks of a parent are in terminal state.
func (r *Repository) AreAllChildrenTerminal(ctx context.Context, parentID string) (bool, error) {
	var nonTerminal int
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT COUNT(*) FROM tasks
		WHERE parent_id = ? AND state NOT IN ('COMPLETED', 'CANCELLED')
	`), parentID).Scan(&nonTerminal)
	if err != nil {
		return false, err
	}
	return nonTerminal == 0, nil
}

// ChildSummary holds summary data for a completed child task.
type ChildSummary struct {
	TaskID                  string `db:"id" json:"id"`
	Identifier              string `db:"identifier" json:"identifier"`
	Title                   string `db:"title" json:"title"`
	State                   string `db:"state" json:"state"`
	AssigneeAgentInstanceID string `db:"assignee_agent_instance_id" json:"assignee_agent_instance_id"`
	LastComment             string `db:"last_comment" json:"last_comment,omitempty"`
}

// maxChildSummaries is the maximum number of child summaries returned.
const maxChildSummaries = 20

// maxCommentChars is the maximum length of a last-comment summary.
const maxCommentChars = 500

// GetChildSummaries returns summary data for all children of a parent task.
// Each child includes its last comment (truncated to 500 chars). Returns at
// most 20 rows and a truncated flag indicating whether more exist.
func (r *Repository) GetChildSummaries(ctx context.Context, parentID string) ([]ChildSummary, bool, error) {
	// Count total children first to determine truncation.
	var total int
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT COUNT(*) FROM tasks WHERE parent_id = ?`), parentID).Scan(&total)
	if err != nil {
		return nil, false, err
	}

	var summaries []ChildSummary
	err = r.ro.SelectContext(ctx, &summaries, r.ro.Rebind(`
		SELECT
			t.id,
			COALESCE(t.identifier, '') AS identifier,
			COALESCE(t.title, '') AS title,
			COALESCE(t.state, '') AS state,
			COALESCE(t.assignee_agent_instance_id, '') AS assignee_agent_instance_id,
			COALESCE((
				SELECT SUBSTR(c.body, 1, ?) FROM task_comments c
				WHERE c.task_id = t.id ORDER BY c.created_at DESC LIMIT 1
			), '') AS last_comment
		FROM tasks t
		WHERE t.parent_id = ?
		ORDER BY t.created_at
		LIMIT ?
	`), maxCommentChars, parentID, maxChildSummaries)
	if err != nil {
		return nil, false, err
	}

	return summaries, total > maxChildSummaries, nil
}
