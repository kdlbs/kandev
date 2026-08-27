package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/jmoiron/sqlx"
)

// StuckParentCandidate is a parent task whose non-archived children are
// all terminal but which may not yet have received its
// task_children_completed wake (or received it for a since-changed child
// set). ChildSetKey is the deterministic "id:state" concatenation compared
// directly against the last-delivered receipt — no hashing, SQLite has no
// built-in hash function — computed here, in SQL, so the sweep stays a
// single query per tick.
type StuckParentCandidate struct {
	ParentTaskID           string `db:"parent_task_id"`
	AssigneeAgentProfileID string `db:"assignee_agent_profile_id"`
	WorkflowStepID         string `db:"workflow_step_id"`
	ChildSetKey            string `db:"child_set_key"`
}

// ListStuckParents finds parent tasks that look done from their children's
// perspective — not archived, not ephemeral, not already terminal, with at
// least one non-archived child, and with no non-archived child left in a
// non-terminal state — and that have not yet had a task_children_completed
// wake delivered for their current child set, by anyone.
//
// Archived children are deliberately excluded from both the "has a child"
// and "any non-terminal" checks below — this is a divergence from
// AreAllChildrenTerminal (blockers.go), which counts every child
// regardless of archived_at and so can be blocked forever by an archived
// child stuck mid-flight. Do not "fix" this back to match
// AreAllChildrenTerminal; the divergence is intentional so an archived
// child can never wedge the reconciler, and a parent whose only children
// are archived is never swept with a spurious empty child-set key.
//
// Every remaining filter runs in SQL, ahead of LIMIT, not in the caller
// afterward:
//   - the LEFT JOIN against parent_child_wake_receipts drops a candidate
//     whose stored receipt already matches its current child set — the
//     receipt is purely this "has the child set changed" discriminator.
//   - the NOT EXISTS against runs drops a candidate that already has an
//     active or finished task_children_completed run, regardless of who
//     queued it — queueChildrenCompletedRun and cascadeChildrenCompleted
//     (the edge-triggered delivery paths) never write a receipt, so the
//     receipt alone cannot tell a healthy edge-delivered wake from a lost
//     one; evidence of delivery has to come from runs itself.
//   - requiring a non-empty assignee_agent_profile_id drops a candidate
//     with no resolvable runner, and the LEFT JOIN against agent_profiles
//     drops one whose runner is paused, stopped, or pending approval.
//
// This is an invariant, not four independent filters: any predicate that
// can stay true for the same parent across consecutive ticks MUST be
// applied here, before LIMIT — never in Go after ListStuckParents returns.
// A parent with no runner, or a paused runner, does not resolve itself on
// its own; left as a Go-side rejection it would occupy a LIMIT slot on
// every tick forever, permanently starving any genuinely actionable
// candidate behind it. Go-side rejection is only admissible for a
// condition that can change between this SELECT and the write a moment
// later (guardAgentStatus in the caller is exactly that — a cheap,
// redundant closing of that race window, not the primary filter).
func (r *Repository) ListStuckParents(ctx context.Context, reason string, limit int) ([]StuckParentCandidate, error) {
	var rows []StuckParentCandidate
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		WITH stuck AS (
			SELECT
				p.id AS parent_task_id,
				`+RunnerProjection("p")+` AS assignee_agent_profile_id,
				p.workflow_step_id AS workflow_step_id,
				COALESCE((
					SELECT GROUP_CONCAT(c.id || ':' || c.state, ',')
					FROM (
						SELECT id, state FROM tasks
						WHERE parent_id = p.id AND archived_at IS NULL
						ORDER BY id
					) c
				), '') AS child_set_key
			FROM tasks p
			WHERE p.archived_at IS NULL
			  AND p.is_ephemeral = 0
			  AND p.state NOT IN ('COMPLETED', 'CANCELLED')
			  AND EXISTS (
			      SELECT 1 FROM tasks c
			      WHERE c.parent_id = p.id AND c.archived_at IS NULL
			  )
			  AND NOT EXISTS (
			      SELECT 1 FROM tasks c
			      WHERE c.parent_id = p.id
			        AND c.archived_at IS NULL
			        AND c.state NOT IN ('COMPLETED', 'CANCELLED')
			  )
		)
		SELECT s.parent_task_id, s.assignee_agent_profile_id, s.workflow_step_id, s.child_set_key
		FROM stuck s
		LEFT JOIN parent_child_wake_receipts r ON r.parent_task_id = s.parent_task_id
		LEFT JOIN agent_profiles ap ON ap.id = s.assignee_agent_profile_id
		WHERE s.assignee_agent_profile_id != ''
		  AND COALESCE(ap.status, 'idle') NOT IN ('paused', 'stopped', 'pending_approval')
		  AND r.child_set_key IS NOT s.child_set_key
		  AND NOT EXISTS (
		      SELECT 1 FROM runs w
		      WHERE json_extract(w.payload, '$.task_id') = s.parent_task_id
		        AND w.reason = ?
		        AND w.status IN ('queued', 'claimed', 'finished')
		  )
		ORDER BY s.parent_task_id
		LIMIT ?
	`), reason, limit)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []StuckParentCandidate{}
	}
	return rows, nil
}

// WakeReceipt is the last-delivered task_children_completed wake for a
// parent task, keyed by the child set it was delivered for.
type WakeReceipt struct {
	ParentTaskID   string    `db:"parent_task_id"`
	ChildSetKey    string    `db:"child_set_key"`
	DeliveredRunID string    `db:"delivered_run_id"`
	DeliveredAt    time.Time `db:"delivered_at"`
}

// GetWakeReceipt returns the receipt for a parent task, or nil if none
// has been recorded yet.
func (r *Repository) GetWakeReceipt(ctx context.Context, parentTaskID string) (*WakeReceipt, error) {
	var rec WakeReceipt
	err := r.ro.GetContext(ctx, &rec, r.ro.Rebind(`
		SELECT parent_task_id, child_set_key, delivered_run_id, delivered_at
		FROM parent_child_wake_receipts
		WHERE parent_task_id = ?
	`), parentTaskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &rec, nil
}

// UpsertWakeReceiptTx records (or updates) the delivery receipt for a
// parent task's current child set, using a transaction the caller owns so
// the receipt write commits atomically with the run insert it accompanies.
func (r *Repository) UpsertWakeReceiptTx(
	ctx context.Context, tx *sqlx.Tx,
	parentTaskID, childSetKey, deliveredRunID string, deliveredAt time.Time,
) error {
	_, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO parent_child_wake_receipts (
			parent_task_id, child_set_key, delivered_run_id, delivered_at
		) VALUES (?, ?, ?, ?)
		ON CONFLICT (parent_task_id) DO UPDATE SET
			child_set_key = excluded.child_set_key,
			delivered_run_id = excluded.delivered_run_id,
			delivered_at = excluded.delivered_at
	`), parentTaskID, childSetKey, deliveredRunID, deliveredAt)
	return err
}
