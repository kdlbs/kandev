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
// set). ChildSetKey is the deterministic "id:state" concatenation the
// caller hashes to compare against the last-delivered receipt — computed
// here, in SQL, so the sweep stays a single query per tick.
type StuckParentCandidate struct {
	ParentTaskID           string `db:"parent_task_id"`
	AssigneeAgentProfileID string `db:"assignee_agent_profile_id"`
	ChildSetKey            string `db:"child_set_key"`
}

// ListStuckParents finds parent tasks that look done from their children's
// perspective: not archived, not ephemeral, not already terminal, with at
// least one child, and with no non-archived child left in a non-terminal
// state.
//
// Archived children are deliberately excluded from the "any non-terminal"
// check below — this is a divergence from AreAllChildrenTerminal
// (blockers.go), which counts every child regardless of archived_at and
// so can be blocked forever by an archived child stuck mid-flight. Do not
// "fix" this back to match AreAllChildrenTerminal; the divergence is
// intentional so an archived child can never wedge the reconciler.
func (r *Repository) ListStuckParents(ctx context.Context, limit int) ([]StuckParentCandidate, error) {
	var rows []StuckParentCandidate
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT
			p.id AS parent_task_id,
			`+RunnerProjection("p")+` AS assignee_agent_profile_id,
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
		  AND EXISTS (SELECT 1 FROM tasks c WHERE c.parent_id = p.id)
		  AND NOT EXISTS (
		      SELECT 1 FROM tasks c
		      WHERE c.parent_id = p.id
		        AND c.archived_at IS NULL
		        AND c.state NOT IN ('COMPLETED', 'CANCELLED')
		  )
		LIMIT ?
	`), limit)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []StuckParentCandidate{}
	}
	return rows, nil
}

// WakeReceipt is the last-delivered task_children_completed wake for a
// parent task, keyed by the hash of the child set it was delivered for.
type WakeReceipt struct {
	ParentTaskID   string    `db:"parent_task_id"`
	ChildSetHash   string    `db:"child_set_hash"`
	DeliveredRunID string    `db:"delivered_run_id"`
	DeliveredAt    time.Time `db:"delivered_at"`
}

// GetWakeReceipt returns the receipt for a parent task, or nil if none
// has been recorded yet.
func (r *Repository) GetWakeReceipt(ctx context.Context, parentTaskID string) (*WakeReceipt, error) {
	var rec WakeReceipt
	err := r.ro.GetContext(ctx, &rec, r.ro.Rebind(`
		SELECT parent_task_id, child_set_hash, delivered_run_id, delivered_at
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
	parentTaskID, childSetHash, deliveredRunID string, deliveredAt time.Time,
) error {
	_, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO parent_child_wake_receipts (
			parent_task_id, child_set_hash, delivered_run_id, delivered_at
		) VALUES (?, ?, ?, ?)
		ON CONFLICT (parent_task_id) DO UPDATE SET
			child_set_hash = excluded.child_set_hash,
			delivered_run_id = excluded.delivered_run_id,
			delivered_at = excluded.delivered_at
	`), parentTaskID, childSetHash, deliveredRunID, deliveredAt)
	return err
}
