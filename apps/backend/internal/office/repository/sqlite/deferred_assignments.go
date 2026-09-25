package sqlite

import (
	"context"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

func (r *Repository) createDeferredAssignmentsTable() error {
	_, err := r.db.Exec(`
	CREATE TABLE IF NOT EXISTS office_deferred_assignments (
		task_id               TEXT PRIMARY KEY,
		workspace_id          TEXT NOT NULL,
		agent_profile_id      TEXT NOT NULL,
		assignment_generation INTEGER NOT NULL DEFAULT 0,
		pause_id              TEXT NOT NULL DEFAULT '',
		created_at            TIMESTAMP NOT NULL,
		resolved_at           TIMESTAMP,
		outcome               TEXT NOT NULL DEFAULT ''
	);

	CREATE INDEX IF NOT EXISTS idx_office_deferred_assignment_workspace
		ON office_deferred_assignments(workspace_id);
	`)
	return err
}

// RecordDeferredAssignment upserts the deferred-assignment row for taskID
// (task_id is the primary key, so one row exists per task): inserts when
// none exists, or overwrites an existing row — resetting resolved_at/
// outcome back to pending — when one does. A later occurrence (a
// reassignment during the same pause, or a fresh pause after an earlier
// deferral already resolved) replaces the row, so replay acts on the
// latest assigning occurrence (see models.DeferredAssignment). The
// overwrite only applies when the stored row is already resolved, or its
// assignment_generation is <= the incoming one: two paused assignments can
// interleave (A reads gen1, B commits and records gen2, A's write lands
// last), and an unconditional overwrite would let A's stale write regress
// a still-pending gen2 row back to gen1 — silently losing B's assignment
// (R1-F3). The returned bool reports whether this call actually wrote a
// row: false (with a nil error) means the generation guard suppressed a
// stale write, letting the caller skip logging an activity entry for an
// assignment that was never recorded — the write itself remains a
// best-effort no-op rather than a propagated error.
func (r *Repository) RecordDeferredAssignment(
	ctx context.Context, taskID, workspaceID, agentProfileID string, assignmentGeneration int64, pauseID string,
) (bool, error) {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO office_deferred_assignments (
			task_id, workspace_id, agent_profile_id, assignment_generation, pause_id, created_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`), taskID, workspaceID, agentProfileID, assignmentGeneration, pauseID, now)
	if err == nil {
		return true, nil
	}
	if !isUniqueConstraintErr(err) {
		return false, err
	}
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE office_deferred_assignments
		SET workspace_id = ?, agent_profile_id = ?, assignment_generation = ?, pause_id = ?, created_at = ?,
		    resolved_at = NULL, outcome = ''
		WHERE task_id = ? AND (resolved_at IS NOT NULL OR assignment_generation <= ?)
	`), workspaceID, agentProfileID, assignmentGeneration, pauseID, now, taskID, assignmentGeneration)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	return rows > 0, err
}

// ListPendingDeferredAssignmentsForWorkspace returns every pending
// deferred assignment for a workspace, oldest first. Used by the
// pause.Service.Resume hook, which already knows the workspace it just
// released and needs no cross-workspace pause-state check.
func (r *Repository) ListPendingDeferredAssignmentsForWorkspace(ctx context.Context, workspaceID string) ([]models.DeferredAssignment, error) {
	var rows []models.DeferredAssignment
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT * FROM office_deferred_assignments
		WHERE workspace_id = ? AND resolved_at IS NULL
		ORDER BY created_at ASC
	`), workspaceID)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []models.DeferredAssignment{}
	}
	return rows, nil
}

// ListReplayablePendingDeferredAssignments returns pending deferred
// assignments whose workspace has no active pause right now, oldest
// first, capped at limit. This is the recovery tick's backstop query: it
// covers a Resume hook that never ran (a process restart between release
// and replay) or that failed transiently, by re-deriving "replayable" from
// current pause state instead of trusting that Resume always fires it.
func (r *Repository) ListReplayablePendingDeferredAssignments(ctx context.Context, limit int) ([]models.DeferredAssignment, error) {
	var rows []models.DeferredAssignment
	err := r.ro.SelectContext(ctx, &rows, r.ro.Rebind(`
		SELECT da.* FROM office_deferred_assignments da
		WHERE da.resolved_at IS NULL
		  AND NOT EXISTS (
		      SELECT 1 FROM office_workspace_pauses p
		      WHERE p.workspace_id = da.workspace_id AND p.released_at IS NULL
		  )
		ORDER BY da.created_at ASC
		LIMIT ?
	`), limit)
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []models.DeferredAssignment{}
	}
	return rows, nil
}

// ResolveDeferredAssignment CAS-resolves a pending row with the given
// outcome ("replayed" or "dropped"). Returns whether this call won the
// CAS — false means another writer (a concurrent Resume hook and recovery
// tick, or two overlapping ticks) already resolved this row, OR the row
// was overwritten (RecordDeferredAssignment) with a different identity
// since the caller read it. The CAS keys on the full identity the caller
// read — task_id, assignment_generation, agent_profile_id, and pause_id —
// not task_id alone (R1-F2): a replay that read row (task, gen1) can race
// a fresh RecordDeferredAssignment that upserts the same task_id to gen2
// before the replay resolves; keying on task_id alone would let the gen1
// resolve mark the gen2 row "replayed"/"dropped" and lose gen2 entirely.
func (r *Repository) ResolveDeferredAssignment(
	ctx context.Context, taskID string, assignmentGeneration int64, agentProfileID, pauseID, outcome string,
) (bool, error) {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE office_deferred_assignments SET resolved_at = ?, outcome = ?
		WHERE task_id = ? AND assignment_generation = ? AND agent_profile_id = ? AND pause_id = ? AND resolved_at IS NULL
	`), time.Now().UTC(), outcome, taskID, assignmentGeneration, agentProfileID, pauseID)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	return rows > 0, err
}
