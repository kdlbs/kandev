package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/office/models"
)

// -- Routine Triggers --

// CreateRoutineTrigger creates a new routine trigger.
func (r *Repository) CreateRoutineTrigger(ctx context.Context, t *models.RoutineTrigger) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	t.CreatedAt = now
	t.UpdatedAt = now

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO office_routine_triggers (
			id, routine_id, kind, cron_expression, timezone,
			public_id, signing_mode, secret, next_run_at, last_fired_at,
			enabled, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), t.ID, t.RoutineID, t.Kind, t.CronExpression, t.Timezone,
		t.PublicID, t.SigningMode, t.Secret, t.NextRunAt, t.LastFiredAt,
		t.Enabled, t.CreatedAt, t.UpdatedAt)
	return err
}

// ListTriggersByRoutineID returns all triggers for a routine.
func (r *Repository) ListTriggersByRoutineID(ctx context.Context, routineID string) ([]*models.RoutineTrigger, error) {
	var triggers []*models.RoutineTrigger
	err := r.ro.SelectContext(ctx, &triggers, r.ro.Rebind(
		`SELECT * FROM office_routine_triggers WHERE routine_id = ? ORDER BY created_at`), routineID)
	if err != nil {
		return nil, err
	}
	if triggers == nil {
		triggers = []*models.RoutineTrigger{}
	}
	return triggers, nil
}

// GetTriggerByPublicID returns a trigger by its public ID (for webhook lookup).
func (r *Repository) GetTriggerByPublicID(ctx context.Context, publicID string) (*models.RoutineTrigger, error) {
	var t models.RoutineTrigger
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT * FROM office_routine_triggers WHERE public_id = ?`), publicID).StructScan(&t)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("trigger not found: %s", publicID)
	}
	return &t, err
}

// GetDueTriggers returns cron triggers that are due to fire, ordered by
// next_run_at then id so that TickScheduledTriggers processes the same
// trigger set in a stable order across ticks (AC-OFFICE-ROUTINE-CATCHUP-001.7).
// Routine status filtering is done in the service layer via ConfigLoader.
func (r *Repository) GetDueTriggers(ctx context.Context, now time.Time) ([]*models.RoutineTrigger, error) {
	var triggers []*models.RoutineTrigger
	err := r.ro.SelectContext(ctx, &triggers, r.ro.Rebind(`
		SELECT * FROM office_routine_triggers
		WHERE kind = 'cron' AND enabled = 1
		  AND next_run_at IS NOT NULL AND next_run_at <= ?
		ORDER BY next_run_at ASC, id ASC
	`), now)
	if err != nil {
		return nil, err
	}
	if triggers == nil {
		triggers = []*models.RoutineTrigger{}
	}
	return triggers, nil
}

// ClaimTrigger atomically claims a trigger by CAS on next_run_at.
// Clears next_run_at to prevent double-fire; caller must set new next_run_at.
// Returns true if the claim succeeded.
func (r *Repository) ClaimTrigger(ctx context.Context, triggerID string, oldNextRunAt time.Time) (bool, error) {
	now := time.Now().UTC()
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE office_routine_triggers
		SET last_fired_at = ?, next_run_at = NULL, updated_at = ?
		WHERE id = ? AND next_run_at = ?
	`), now, now, triggerID, oldNextRunAt)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	return rows > 0, err
}

// UpdateTriggerNextRun updates the next_run_at for a trigger.
func (r *Repository) UpdateTriggerNextRun(ctx context.Context, triggerID string, nextRunAt *time.Time) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE office_routine_triggers SET next_run_at = ?, updated_at = ? WHERE id = ?
	`), nextRunAt, time.Now().UTC(), triggerID)
	return err
}

// ListStrandedTriggers returns enabled cron triggers whose next_run_at is
// null (a claim was taken via ClaimTrigger) and whose updated_at is older
// than olderThan. A claim taken this tick or the previous one is never
// returned; only a claim abandoned before that — a process that stopped
// between ClaimTrigger and the re-arm write — is stale
// (AC-OFFICE-ROUTINE-CATCHUP-001.9).
func (r *Repository) ListStrandedTriggers(ctx context.Context, olderThan time.Time) ([]*models.RoutineTrigger, error) {
	var triggers []*models.RoutineTrigger
	err := r.ro.SelectContext(ctx, &triggers, r.ro.Rebind(`
		SELECT * FROM office_routine_triggers
		WHERE kind = 'cron' AND enabled = 1
		  AND next_run_at IS NULL AND updated_at < ?
		ORDER BY id ASC
	`), olderThan)
	if err != nil {
		return nil, err
	}
	if triggers == nil {
		triggers = []*models.RoutineTrigger{}
	}
	return triggers, nil
}

// ReconcileTriggerNextRun arms a stranded trigger's next_run_at. Unlike
// UpdateTriggerNextRun (the live claimant's own unconditional re-arm), this
// write is a CAS on `next_run_at IS NULL AND enabled = 1`: a second
// reconciling processor affects zero rows, and a trigger disabled or armed
// between the read and this write is left alone. Returns true if this call
// won the CAS.
func (r *Repository) ReconcileTriggerNextRun(ctx context.Context, triggerID string, nextRunAt time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE office_routine_triggers
		SET next_run_at = ?, updated_at = ?
		WHERE id = ? AND next_run_at IS NULL AND enabled = 1
	`), nextRunAt, time.Now().UTC(), triggerID)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	return rows > 0, err
}

// DeleteRoutineTrigger deletes a trigger by ID.
func (r *Repository) DeleteRoutineTrigger(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM office_routine_triggers WHERE id = ?`), id)
	return err
}

// CreateRoutine creates a new routine.
func (r *Repository) CreateRoutine(ctx context.Context, routine *models.Routine) error {
	if routine.ID == "" {
		routine.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	routine.CreatedAt = now
	routine.UpdatedAt = now
	return r.insertRoutine(ctx, r.db, routine)
}

// CreateRoutineTx is CreateRoutine scoped to a caller-owned transaction,
// letting config sync write a new routine and its ownership manifest row
// atomically (AC-OFFICE-CONFIG-SYNC-003.14).
func (r *Repository) CreateRoutineTx(ctx context.Context, tx *sqlx.Tx, routine *models.Routine) error {
	if routine.ID == "" {
		routine.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	routine.CreatedAt = now
	routine.UpdatedAt = now
	return r.insertRoutine(ctx, tx, routine)
}

// insertRoutine is the single write-time normalization funnel for
// catch_up_policy and catch_up_max: it reassigns both fields on the
// passed-in struct, before the statement binds them, so a handler that
// serializes the same *Routine back to the caller reports the normalized
// value rather than the submitted one (AC-OFFICE-ROUTINE-CATCHUP-001.13,
// AC-OFFICE-ROUTINE-CATCHUP-003.1). See
// docs/specs/office/system-design/routine-catch-up-01.md.
func (r *Repository) insertRoutine(ctx context.Context, ext sqlx.ExtContext, routine *models.Routine) error {
	routine.CatchUpPolicy = models.NormaliseCatchUpPolicy(string(routine.CatchUpPolicy))
	routine.CatchUpMax = models.NormaliseCatchUpMax(routine.CatchUpMax)
	_, err := ext.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO office_routines (
			id, workspace_id, name, description, task_template,
			assignee_agent_profile_id, status, concurrency_policy,
			catch_up_policy, catch_up_max,
			variables, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), routine.ID, routine.WorkspaceID, routine.Name, routine.Description,
		routine.TaskTemplate, routine.AssigneeAgentProfileID, routine.Status,
		routine.ConcurrencyPolicy, routine.CatchUpPolicy, routine.CatchUpMax,
		routine.Variables, routine.CreatedAt, routine.UpdatedAt)
	return err
}

// GetRoutine returns a routine by ID.
func (r *Repository) GetRoutine(ctx context.Context, id string) (*models.Routine, error) {
	var routine models.Routine
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT * FROM office_routines WHERE id = ?`), id).StructScan(&routine)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("routine not found: %s", id)
	}
	return &routine, err
}

// ListRoutines returns all routines for a workspace.
// An empty workspaceID returns rows from all workspaces.
func (r *Repository) ListRoutines(ctx context.Context, workspaceID string) ([]*models.Routine, error) {
	var (
		routines []*models.Routine
		err      error
	)
	if workspaceID == "" {
		err = r.ro.SelectContext(ctx, &routines,
			`SELECT * FROM office_routines ORDER BY name`)
	} else {
		err = r.ro.SelectContext(ctx, &routines, r.ro.Rebind(
			`SELECT * FROM office_routines WHERE workspace_id = ? ORDER BY name`), workspaceID)
	}
	if err != nil {
		return nil, err
	}
	if routines == nil {
		routines = []*models.Routine{}
	}
	return routines, nil
}

// UpdateRoutine updates an existing routine. It applies the same write-time
// catch_up_policy/catch_up_max normalization funnel as insertRoutine, on the
// passed-in struct, so this second, independent writer of both columns
// cannot diverge from the create path (AC-OFFICE-ROUTINE-CATCHUP-001.13,
// AC-OFFICE-ROUTINE-CATCHUP-003.1).
func (r *Repository) UpdateRoutine(ctx context.Context, routine *models.Routine) error {
	routine.UpdatedAt = time.Now().UTC()
	routine.CatchUpPolicy = models.NormaliseCatchUpPolicy(string(routine.CatchUpPolicy))
	routine.CatchUpMax = models.NormaliseCatchUpMax(routine.CatchUpMax)
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE office_routines SET
			name = ?, description = ?, task_template = ?,
			assignee_agent_profile_id = ?, status = ?, concurrency_policy = ?,
			catch_up_policy = ?, catch_up_max = ?,
			variables = ?, last_run_at = ?, updated_at = ?
		WHERE id = ?
	`), routine.Name, routine.Description, routine.TaskTemplate,
		routine.AssigneeAgentProfileID, routine.Status, routine.ConcurrencyPolicy,
		routine.CatchUpPolicy, routine.CatchUpMax,
		routine.Variables, routine.LastRunAt, routine.UpdatedAt, routine.ID)
	return err
}

// RoutineConfigFields is the subset of office_routines columns a config
// import owns (see UpdateRoutineConfigFields).
type RoutineConfigFields struct {
	Description       string
	TaskTemplate      string
	ConcurrencyPolicy models.RoutineConcurrencyPolicy
}

// UpdateRoutineConfigFields updates only the columns a config import owns
// (description, task template, concurrency policy), leaving
// assignee_agent_profile_id, status, catch_up_policy, catch_up_max,
// variables, and last_run_at untouched instead of reverting them to a
// stale read-then-write snapshot.
func (r *Repository) UpdateRoutineConfigFields(
	ctx context.Context, id string, fields RoutineConfigFields,
) error {
	return r.updateRoutineConfigFields(ctx, r.db, id, fields)
}

// UpdateRoutineConfigFieldsTx is UpdateRoutineConfigFields scoped to a
// caller-owned transaction.
func (r *Repository) UpdateRoutineConfigFieldsTx(
	ctx context.Context, tx *sqlx.Tx, id string, fields RoutineConfigFields,
) error {
	return r.updateRoutineConfigFields(ctx, tx, id, fields)
}

func (r *Repository) updateRoutineConfigFields(
	ctx context.Context, ext sqlx.ExtContext, id string, fields RoutineConfigFields,
) error {
	now := time.Now().UTC()
	_, err := ext.ExecContext(ctx, r.db.Rebind(`
		UPDATE office_routines SET
			description = ?, task_template = ?, concurrency_policy = ?, updated_at = ?
		WHERE id = ?
	`), fields.Description, fields.TaskTemplate, fields.ConcurrencyPolicy, now, id)
	return err
}

// DeleteRoutine deletes a routine by ID.
func (r *Repository) DeleteRoutine(ctx context.Context, id string) error {
	return r.deleteRoutine(ctx, r.db, id)
}

// DeleteRoutineTx is DeleteRoutine scoped to a caller-owned transaction.
func (r *Repository) DeleteRoutineTx(ctx context.Context, tx *sqlx.Tx, id string) error {
	return r.deleteRoutine(ctx, tx, id)
}

func (r *Repository) deleteRoutine(ctx context.Context, ext sqlx.ExtContext, id string) error {
	_, err := ext.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM office_routines WHERE id = ?`), id)
	return err
}

// CreateRoutineRun creates a new routine run record. The three catch-up
// gap-summary columns are written once, here, from whatever the caller set
// on run (nil when the claim measured no gap); nothing updates them
// afterward, so a run that later transitions to coalesced or failed keeps
// the gap that was measured for its tick (AC-OFFICE-ROUTINE-CATCHUP-002.10).
func (r *Repository) CreateRoutineRun(ctx context.Context, run *models.RoutineRun) error {
	if run.ID == "" {
		run.ID = uuid.New().String()
	}
	run.CreatedAt = time.Now().UTC()

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO office_routine_runs (
			id, routine_id, trigger_id, source, status, trigger_payload,
			linked_task_id, coalesced_into_run_id, dispatch_fingerprint,
			catch_up_missed_ticks, catch_up_first_missed_at, catch_up_truncated,
			started_at, completed_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), run.ID, run.RoutineID, run.TriggerID, run.Source, run.Status,
		run.TriggerPayload, run.LinkedTaskID, run.CoalescedIntoRunID,
		run.DispatchFingerprint, run.CatchUpMissedTicks, run.CatchUpFirstMissedAt,
		run.CatchUpTruncated, run.StartedAt, run.CompletedAt, run.CreatedAt)
	return err
}

// ListRoutineRuns returns paginated routine runs for a routine.
func (r *Repository) ListRoutineRuns(ctx context.Context, routineID string, limit, offset int) ([]*models.RoutineRun, error) {
	var runs []*models.RoutineRun
	err := r.ro.SelectContext(ctx, &runs, r.ro.Rebind(`
		SELECT * FROM office_routine_runs
		WHERE routine_id = ? ORDER BY created_at DESC LIMIT ? OFFSET ?
	`), routineID, limit, offset)
	if err != nil {
		return nil, err
	}
	if runs == nil {
		runs = []*models.RoutineRun{}
	}
	return runs, nil
}

// ListAllRuns returns recent runs across all routines, filtered by workspace.
func (r *Repository) ListAllRuns(ctx context.Context, workspaceID string, limit int) ([]*models.RoutineRun, error) {
	var runs []*models.RoutineRun
	err := r.ro.SelectContext(ctx, &runs, r.ro.Rebind(`
		SELECT r.* FROM office_routine_runs r
		JOIN office_routines rt ON rt.id = r.routine_id
		WHERE rt.workspace_id = ?
		ORDER BY r.created_at DESC LIMIT ?
	`), workspaceID, limit)
	if err != nil {
		return nil, err
	}
	if runs == nil {
		runs = []*models.RoutineRun{}
	}
	return runs, nil
}

// GetActiveRunForFingerprint returns the newest active run matching the
// fingerprint, if any. "Active" means status = task_created (the only
// status a run can gate another fire from — see
// routines.RoutineService's materialise* methods). The caller repairs
// terminal, archived, or missing linked tasks before it applies policy.
func (r *Repository) GetActiveRunForFingerprint(
	ctx context.Context, routineID, fingerprint string,
) (*models.RoutineRun, error) {
	var run models.RoutineRun
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT * FROM office_routine_runs
		WHERE routine_id = ? AND dispatch_fingerprint = ?
		  AND status = 'task_created'
		ORDER BY created_at DESC LIMIT 1
	`), routineID, fingerprint).StructScan(&run)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// GetRoutineRunByLinkedTaskID returns the most recent run linked to
// taskID, or nil if no run is linked to it (most tasks aren't
// routine-created). Used by SyncRunStatus to find the run to close out
// when its task reaches a terminal step. taskID must be non-empty:
// linked_task_id defaults to "" for every lightweight run, so an empty
// taskID would match (and let a caller rewrite) an arbitrary lightweight
// run instead of correctly finding nothing.
func (r *Repository) GetRoutineRunByLinkedTaskID(
	ctx context.Context, taskID string,
) (*models.RoutineRun, error) {
	if taskID == "" {
		return nil, nil
	}
	var run models.RoutineRun
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT * FROM office_routine_runs
		WHERE linked_task_id = ?
		ORDER BY created_at DESC LIMIT 1
	`), taskID).StructScan(&run)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &run, nil
}

// GetTaskTerminalStatus reports the linked task outcome: "done" for
// COMPLETED, "failed" for FAILED, "cancelled" for CANCELLED or archived
// tasks, "missing" when the task row was deleted, and "" otherwise.
// Reads the shared tasks table directly because the routines gate must
// distinguish an active task from a terminal, archived, or missing task.
func (r *Repository) GetTaskTerminalStatus(ctx context.Context, taskID string) (string, error) {
	var state string
	var archivedAt sql.NullTime
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT COALESCE(state, ''), archived_at FROM tasks WHERE id = ?`), taskID).Scan(&state, &archivedAt)
	if err == sql.ErrNoRows {
		return "missing", nil
	}
	if err != nil {
		return "", err
	}
	if archivedAt.Valid {
		return "cancelled", nil
	}
	switch state {
	case taskStateCompleted:
		return "done", nil
	case taskStateFailed:
		return "failed", nil
	case taskStateCancelled:
		return "cancelled", nil
	default:
		return "", nil
	}
}

// TouchRoutineLastRun updates only last_run_at (+ updated_at) on a
// routine. Deliberately narrower than UpdateRoutine, whose whole-row
// snapshot write can revert concurrent edits to other columns (see
// UpdateRoutineConfigFields above) — a dispatch in flight should never
// clobber a config change made while it ran, or vice versa.
func (r *Repository) TouchRoutineLastRun(ctx context.Context, routineID string, at time.Time) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE office_routines SET last_run_at = ?, updated_at = ? WHERE id = ?
	`), at, time.Now().UTC(), routineID)
	return err
}

// UpdateRunStatus updates a run's status and optionally its linked task.
func (r *Repository) UpdateRunStatus(
	ctx context.Context, runID string, status models.RoutineRunStatus, linkedTaskID string,
) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE office_routine_runs
		SET status = ?, linked_task_id = ?, completed_at = ?
		WHERE id = ?
	`), status, linkedTaskID, now, runID)
	return err
}

// UpdateRunStatusIfTaskCreated closes a run out with a terminal status,
// but only while it is still 'task_created' — the one status a routine
// run can gate its fingerprint from. The WHERE clause makes this an
// atomic claim: SyncRunStatus (TaskMoved-driven) and the concurrency
// gate's own inline check (applyConcurrencyPolicy) can both observe the
// same terminal task, and this is what lets exactly one of them win
// instead of a read-then-write race rewriting an already-closed run's
// status or completed_at. Returns whether this call was the one that
// closed it.
func (r *Repository) UpdateRunStatusIfTaskCreated(
	ctx context.Context, runID string, status models.RoutineRunStatus, linkedTaskID string,
) (bool, error) {
	now := time.Now().UTC()
	res, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE office_routine_runs
		SET status = ?, linked_task_id = ?, completed_at = ?
		WHERE id = ? AND status = 'task_created'
	`), status, linkedTaskID, now, runID)
	if err != nil {
		return false, err
	}
	rows, err := res.RowsAffected()
	return rows > 0, err
}

// UpdateRunCoalesced marks a run as coalesced into another run.
func (r *Repository) UpdateRunCoalesced(ctx context.Context, runID, coalescedIntoRunID string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE office_routine_runs
		SET status = 'coalesced', coalesced_into_run_id = ?, completed_at = ?
		WHERE id = ?
	`), coalescedIntoRunID, now, runID)
	return err
}
