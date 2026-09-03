package sqlite

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/office/models"
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
// This is the reverse direction of ListTaskBlockers; results are ordered by
// insertion time so callers see a stable list.
func (r *Repository) ListTasksBlockedBy(ctx context.Context, blockerTaskID string) ([]string, error) {
	var ids []string
	err := r.ro.SelectContext(ctx, &ids, r.ro.Rebind(
		`SELECT task_id FROM task_blockers WHERE blocker_task_id = ? ORDER BY created_at, task_id`),
		blockerTaskID)
	if err != nil {
		return nil, err
	}
	if ids == nil {
		ids = []string{}
	}
	return ids, nil
}

// ListBlockersForTasks returns a map of task ID → list of blocker IDs for a set of tasks.
// Uses IN query over the given task IDs.
func (r *Repository) ListBlockersForTasks(ctx context.Context, taskIDs []string) (map[string][]string, error) {
	if len(taskIDs) == 0 {
		return map[string][]string{}, nil
	}
	query, args, err := sqlx.In(
		`SELECT task_id, blocker_task_id FROM task_blockers WHERE task_id IN (?)`, taskIDs)
	if err != nil {
		return nil, err
	}
	query = r.ro.Rebind(query)
	type row struct {
		TaskID        string `db:"task_id"`
		BlockerTaskID string `db:"blocker_task_id"`
	}
	var rows []row
	if err := r.ro.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	result := make(map[string][]string, len(taskIDs))
	for _, rw := range rows {
		result[rw.TaskID] = append(result[rw.TaskID], rw.BlockerTaskID)
	}
	return result, nil
}

// ListDependentsForTasks returns a map of blocker task ID → list of task IDs
// blocked by it, for a set of blocker task IDs. This is the batched reverse of
// ListBlockersForTasks and backs the "blocks" direction of the dependency
// payload; a per-task query would add one round trip per card to a board read.
func (r *Repository) ListDependentsForTasks(ctx context.Context, blockerTaskIDs []string) (map[string][]string, error) {
	if len(blockerTaskIDs) == 0 {
		return map[string][]string{}, nil
	}
	query, args, err := sqlx.In(
		`SELECT task_id, blocker_task_id FROM task_blockers WHERE blocker_task_id IN (?) ORDER BY created_at, task_id`,
		blockerTaskIDs)
	if err != nil {
		return nil, err
	}
	query = r.ro.Rebind(query)
	type row struct {
		TaskID        string `db:"task_id"`
		BlockerTaskID string `db:"blocker_task_id"`
	}
	var rows []row
	if err := r.ro.SelectContext(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	result := make(map[string][]string, len(blockerTaskIDs))
	for _, rw := range rows {
		result[rw.BlockerTaskID] = append(result[rw.BlockerTaskID], rw.TaskID)
	}
	return result, nil
}

// ListTasksWithDependencies returns the distinct dependent task IDs that have at
// least one dependency edge. Used by startup reconciliation to bound its sweep to
// tasks that could be chain steps rather than scanning the whole board.
func (r *Repository) ListTasksWithDependencies(ctx context.Context) ([]string, error) {
	var ids []string
	err := r.ro.SelectContext(ctx, &ids,
		`SELECT DISTINCT task_id FROM task_blockers ORDER BY task_id`)
	if err != nil {
		return nil, err
	}
	if ids == nil {
		ids = []string{}
	}
	return ids, nil
}

// DeleteTaskBlockersForTask removes every dependency edge touching taskID, in
// both directions. Called when a task is deleted: task_blockers predates the
// tasks foreign key, so there is no ON DELETE CASCADE to rely on and orphaned
// edges would keep dependents blocked on a task that no longer exists.
func (r *Repository) DeleteTaskBlockersForTask(ctx context.Context, taskID string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM task_blockers WHERE task_id = ? OR blocker_task_id = ?`), taskID, taskID)
	return err
}

func (r *Repository) IsTaskInTerminalStep(ctx context.Context, taskID string) (bool, error) {
	var state string
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT COALESCE(state, '') FROM tasks WHERE id = ?`), taskID).Scan(&state)
	if err != nil {
		return false, err
	}
	return state == "COMPLETED" || state == "CANCELLED", nil
}

// GetTaskAssignee returns the agent currently driving a task. Resolves
// through the runner participant projection (ADR 0005 Wave F): per-task
// runner row > step's primary > "".
func (r *Repository) GetTaskAssignee(ctx context.Context, taskID string) (string, error) {
	var assignee string
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT `+RunnerProjection("tasks")+` FROM tasks WHERE id = ?`), taskID).Scan(&assignee)
	if err != nil {
		return "", err
	}
	return assignee, nil
}

// GetTaskAssigneeTx is GetTaskAssignee scoped to a caller-owned transaction,
// for a caller that must read the effective runner immediately before an
// insert whose validity depends on it not having changed since an earlier,
// separate read (ParentWakeReconciler.recordReceipt, scheduler_wake_reconciler.go:
// closing the race between ListStuckParents' SELECT and this transaction's
// run insert, where a reassignment in between would otherwise queue a run
// for a runner that is no longer this parent's assignee).
func (r *Repository) GetTaskAssigneeTx(ctx context.Context, tx *sqlx.Tx, taskID string) (string, error) {
	var assignee string
	err := tx.QueryRowxContext(ctx, tx.Rebind(
		`SELECT `+RunnerProjection("tasks")+` FROM tasks WHERE id = ?`), taskID).Scan(&assignee)
	if err != nil {
		return "", err
	}
	return assignee, nil
}

// AreAllChildrenTerminal checks if all child tasks of a parent are in
// terminal state. Unlike ListStuckParents (wake_receipts.go), this counts
// every child regardless of archived_at, so an archived child stuck
// mid-flight can block it forever — that divergence is intentional on the
// reconciler's side (see ListStuckParents), not a bug here; do not add an
// archived_at filter to this query to "match" it.
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

// ChildState is a minimal id+state pair over a parent's child tasks.
type ChildState struct {
	TaskID string `db:"id"`
	State  string `db:"state"`
}

// ListChildStates returns id+state for every child of a parent task,
// ordered by id. Callers that wake a parent once AreAllChildrenTerminal
// reports true use this to build a wake-idempotency key that changes
// across delegation waves — a parent that fans out again after one wave
// completes gets a distinct child set (and therefore a distinct key), so
// it can be woken again instead of colliding with the permanently-unique
// {reason}:{taskID}:{agentID} key QueueRunCtx mints by default.
func (r *Repository) ListChildStates(ctx context.Context, parentID string) ([]ChildState, error) {
	var rows []ChildState
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT id, COALESCE(state, '') AS state FROM tasks
		WHERE parent_id = ?
		ORDER BY id
	`), parentID)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []ChildState{}
	}
	return rows, nil
}

// ChildSummary holds summary data for a completed child task.
type ChildSummary struct {
	TaskID                 string `db:"id" json:"id"`
	Identifier             string `db:"identifier" json:"identifier"`
	Title                  string `db:"title" json:"title"`
	State                  string `db:"state" json:"state"`
	AssigneeAgentProfileID string `db:"assignee_agent_profile_id" json:"assignee_agent_profile_id"`
	LastComment            string `db:"last_comment" json:"last_comment,omitempty"`
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
			`+RunnerProjection("t")+` AS assignee_agent_profile_id,
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
