package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
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
// The p.archived_at/is_ephemeral/state filters run over every task row
// regardless of Office adoption; taskrepo.IsFromOfficePredicate narrows
// that down to parents this reconciler may actually act on. HasOfficeAdoption
// (the Tick-level gate in ParentWakeReconciler) only proves some workspace
// somewhere has adopted Office — it says nothing about this parent's own
// task or workspace, so without this predicate a Kanban-only parent in an
// unrelated workspace, or in the same workspace as an Office project it has
// no connection to, could still match every other filter and receive an
// unrequested autonomous run. ListUnstartedTasks
// (tasks.go, the sibling query this reconciler mirrors) applies the same
// predicate for the same reason.
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
//     whose stored receipt already matches its current child set and whose
//     delivery evidence still exists. A receipt created by the workflow
//     engine stores a child-set operation id. A legacy receipt stores a run
//     id, and any existing run is evidence that the wake entered the queue.
//     Terminal execution failures stay terminal under the Office runtime
//     contract; they require an explicit user retry and must not become a
//     cron retry loop. A missing referenced run remains eligible because a
//     cleanup or migration can remove the delivery evidence.
//   - the NOT EXISTS against runs drops a candidate with a queued or
//     claimed task_children_completed run (still in flight, regardless of
//     which child set it was requested for — wait for it to resolve rather
//     than race a duplicate) or a terminal one requested at or after
//     newest_child_updated_at (it already reflects the current child set,
//     including a failed or cancelled wake that must remain terminal under
//     the Office runtime contract). A terminal run requested *before* the
//     newest child update does NOT block: that run saw a stale child set, so
//     it must not be treated as having delivered this one — this is R3-A's
//     fix, replacing a plain "any terminal run ever" check that let one
//     finished run permanently immunize a parent against every later
//     child-set change.
//     queueChildrenCompletedRun and cascadeChildrenCompleted (the
//     edge-triggered delivery paths) never write a receipt, so the receipt
//     alone cannot tell a healthy edge-delivered wake from a lost one;
//     evidence of delivery has to come from runs itself.
//   - requiring a non-empty assignee_agent_profile_id drops a candidate
//     with no resolvable runner, and the INNER JOIN against agent_profiles
//     drops one whose runner is paused, stopped, pending approval, or
//     altogether missing (a dangling assignee_agent_profile_id with no
//     matching row) — R3-B's fix: a LEFT JOIN treated "no matching row" as
//     COALESCE'd-to-idle, which passed a dangling reference straight
//     through as a sticky, unresolvable, LIMIT-slot-consuming candidate.
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
				), '') AS child_set_key,
				(
					SELECT MAX(c.updated_at) FROM tasks c
					WHERE c.parent_id = p.id AND c.archived_at IS NULL
				) AS newest_child_updated_at
			FROM tasks p
			WHERE p.archived_at IS NULL
			  AND p.is_ephemeral = 0
			  AND p.state NOT IN ('COMPLETED', 'CANCELLED')
			  AND `+taskrepo.IsFromOfficePredicate("p")+`
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
		INNER JOIN agent_profiles ap ON ap.id = s.assignee_agent_profile_id
		WHERE s.assignee_agent_profile_id != ''
		  AND ap.status NOT IN ('paused', 'stopped', 'pending_approval')
		  AND (
		      r.child_set_key IS NOT s.child_set_key
		      OR (
		          NOT EXISTS (
		              SELECT 1 FROM runs delivered
		              WHERE delivered.id = r.delivered_run_id
		          )
		          AND COALESCE(r.delivery_operation_id, '') = ''
		      )
		  )
		  AND NOT EXISTS (
		      SELECT 1 FROM runs w
		      WHERE json_extract(w.payload, '$.task_id') = s.parent_task_id
		        AND w.reason = ?
		        AND (
		            w.status IN ('queued', 'claimed')
		            OR (
		                w.status IN ('finished', 'failed', 'cancelled')
		                AND w.requested_at >= s.newest_child_updated_at
		            )
		        )
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
	ParentTaskID        string    `db:"parent_task_id"`
	ChildSetKey         string    `db:"child_set_key"`
	DeliveredRunID      string    `db:"delivered_run_id"`
	DeliveryOperationID string    `db:"delivery_operation_id"`
	DeliveredAt         time.Time `db:"delivered_at"`
}

// GetWakeReceipt returns the receipt for a parent task, or nil if none
// has been recorded yet.
func (r *Repository) GetWakeReceipt(ctx context.Context, parentTaskID string) (*WakeReceipt, error) {
	var rec WakeReceipt
	err := r.ro.GetContext(ctx, &rec, r.ro.Rebind(`
		SELECT parent_task_id, child_set_key, delivered_run_id,
		       delivery_operation_id, delivered_at
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
// parent task's current child set, using a transaction the caller owns.
// deliveredRunID is populated by legacy direct-run callers. The workflow
// engine path uses deliveryOperationID because one trigger can fan out to
// several runs and the engine owns their admission.
func (r *Repository) UpsertWakeReceiptTx(
	ctx context.Context, tx *sqlx.Tx,
	parentTaskID, childSetKey, deliveredRunID, deliveryOperationID string,
	deliveredAt time.Time,
) error {
	_, err := tx.ExecContext(ctx, tx.Rebind(`
		INSERT INTO parent_child_wake_receipts (
			parent_task_id, child_set_key, delivered_run_id,
			delivery_operation_id, delivered_at
		) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (parent_task_id) DO UPDATE SET
			child_set_key = excluded.child_set_key,
			delivered_run_id = excluded.delivered_run_id,
			delivery_operation_id = excluded.delivery_operation_id,
			delivered_at = excluded.delivered_at
	`), parentTaskID, childSetKey, deliveredRunID, deliveryOperationID, deliveredAt)
	return err
}

type childSetKeyRow struct {
	ID    string `db:"id"`
	State string `db:"state"`
}

// GetChildSetKey returns the deterministic key for the parent's current
// active child set. It reads child ids and states separately from the
// aggregate query so the same logic works on SQLite and PostgreSQL.
func (r *Repository) GetChildSetKey(ctx context.Context, parentTaskID string) (string, error) {
	var rows []childSetKeyRow
	if err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT id, state
		FROM tasks
		WHERE parent_id = ? AND archived_at IS NULL
		ORDER BY id
	`), parentTaskID); err != nil {
		return "", err
	}
	return formatChildSetKey(rows), nil
}

// GetChildSetKeyTx is the transaction-scoped counterpart to GetChildSetKey.
// Reconciler admission uses it immediately before recording a receipt, so a
// child-set change does not get hidden by a receipt for the old generation.
func (r *Repository) GetChildSetKeyTx(
	ctx context.Context, tx *sqlx.Tx, parentTaskID string,
) (string, error) {
	var rows []childSetKeyRow
	if err := tx.SelectContext(ctx, &rows, tx.Rebind(`
		SELECT id, state
		FROM tasks
		WHERE parent_id = ? AND archived_at IS NULL
		ORDER BY id
	`), parentTaskID); err != nil {
		return "", err
	}
	return formatChildSetKey(rows), nil
}

func formatChildSetKey(rows []childSetKeyRow) string {
	var b strings.Builder
	for i, row := range rows {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(row.ID)
		b.WriteByte(':')
		b.WriteString(row.State)
	}
	return b.String()
}
