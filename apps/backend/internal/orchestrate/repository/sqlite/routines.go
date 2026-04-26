package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/orchestrate/models"
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
		INSERT INTO orchestrate_routine_triggers (
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
		`SELECT * FROM orchestrate_routine_triggers WHERE routine_id = ? ORDER BY created_at`), routineID)
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
		`SELECT * FROM orchestrate_routine_triggers WHERE public_id = ?`), publicID).StructScan(&t)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("trigger not found: %s", publicID)
	}
	return &t, err
}

// GetDueTriggers returns cron triggers that are due to fire.
func (r *Repository) GetDueTriggers(ctx context.Context, now time.Time) ([]*models.RoutineTrigger, error) {
	var triggers []*models.RoutineTrigger
	err := r.ro.SelectContext(ctx, &triggers, r.ro.Rebind(`
		SELECT t.* FROM orchestrate_routine_triggers t
		JOIN orchestrate_routines r ON t.routine_id = r.id
		WHERE t.kind = 'cron' AND t.enabled = 1
		  AND r.status = 'active'
		  AND t.next_run_at IS NOT NULL AND t.next_run_at <= ?
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
		UPDATE orchestrate_routine_triggers
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
		UPDATE orchestrate_routine_triggers SET next_run_at = ?, updated_at = ? WHERE id = ?
	`), nextRunAt, time.Now().UTC(), triggerID)
	return err
}

// DeleteRoutineTrigger deletes a trigger by ID.
func (r *Repository) DeleteRoutineTrigger(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM orchestrate_routine_triggers WHERE id = ?`), id)
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

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO orchestrate_routines (
			id, workspace_id, name, description, task_template,
			assignee_agent_instance_id, status, concurrency_policy,
			variables, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), routine.ID, routine.WorkspaceID, routine.Name, routine.Description,
		routine.TaskTemplate, routine.AssigneeAgentInstanceID, routine.Status,
		routine.ConcurrencyPolicy, routine.Variables, routine.CreatedAt, routine.UpdatedAt)
	return err
}

// GetRoutine returns a routine by ID.
func (r *Repository) GetRoutine(ctx context.Context, id string) (*models.Routine, error) {
	var routine models.Routine
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT * FROM orchestrate_routines WHERE id = ?`), id).StructScan(&routine)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("routine not found: %s", id)
	}
	return &routine, err
}

// ListRoutines returns all routines for a workspace.
func (r *Repository) ListRoutines(ctx context.Context, workspaceID string) ([]*models.Routine, error) {
	var routines []*models.Routine
	err := r.ro.SelectContext(ctx, &routines, r.ro.Rebind(
		`SELECT * FROM orchestrate_routines WHERE workspace_id = ? ORDER BY name`), workspaceID)
	if err != nil {
		return nil, err
	}
	if routines == nil {
		routines = []*models.Routine{}
	}
	return routines, nil
}

// UpdateRoutine updates an existing routine.
func (r *Repository) UpdateRoutine(ctx context.Context, routine *models.Routine) error {
	routine.UpdatedAt = time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE orchestrate_routines SET
			name = ?, description = ?, task_template = ?,
			assignee_agent_instance_id = ?, status = ?, concurrency_policy = ?,
			variables = ?, last_run_at = ?, updated_at = ?
		WHERE id = ?
	`), routine.Name, routine.Description, routine.TaskTemplate,
		routine.AssigneeAgentInstanceID, routine.Status, routine.ConcurrencyPolicy,
		routine.Variables, routine.LastRunAt, routine.UpdatedAt, routine.ID)
	return err
}

// DeleteRoutine deletes a routine by ID.
func (r *Repository) DeleteRoutine(ctx context.Context, id string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(
		`DELETE FROM orchestrate_routines WHERE id = ?`), id)
	return err
}

// CreateRoutineRun creates a new routine run record.
func (r *Repository) CreateRoutineRun(ctx context.Context, run *models.RoutineRun) error {
	if run.ID == "" {
		run.ID = uuid.New().String()
	}
	run.CreatedAt = time.Now().UTC()

	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO orchestrate_routine_runs (
			id, routine_id, trigger_id, source, status, trigger_payload,
			linked_task_id, coalesced_into_run_id, dispatch_fingerprint,
			started_at, completed_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), run.ID, run.RoutineID, run.TriggerID, run.Source, run.Status,
		run.TriggerPayload, run.LinkedTaskID, run.CoalescedIntoRunID,
		run.DispatchFingerprint, run.StartedAt, run.CompletedAt, run.CreatedAt)
	return err
}

// ListRuns returns paginated routine runs for a routine.
func (r *Repository) ListRuns(ctx context.Context, routineID string, limit, offset int) ([]*models.RoutineRun, error) {
	var runs []*models.RoutineRun
	err := r.ro.SelectContext(ctx, &runs, r.ro.Rebind(`
		SELECT * FROM orchestrate_routine_runs
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

// ListAllRuns returns recent runs across all routines in a workspace.
func (r *Repository) ListAllRuns(ctx context.Context, wsID string, limit int) ([]*models.RoutineRun, error) {
	var runs []*models.RoutineRun
	err := r.ro.SelectContext(ctx, &runs, r.ro.Rebind(`
		SELECT rr.* FROM orchestrate_routine_runs rr
		JOIN orchestrate_routines rt ON rr.routine_id = rt.id
		WHERE rt.workspace_id = ?
		ORDER BY rr.created_at DESC LIMIT ?
	`), wsID, limit)
	if err != nil {
		return nil, err
	}
	if runs == nil {
		runs = []*models.RoutineRun{}
	}
	return runs, nil
}

// GetActiveRunForFingerprint returns an active run matching the fingerprint.
func (r *Repository) GetActiveRunForFingerprint(
	ctx context.Context, routineID, fingerprint string,
) (*models.RoutineRun, error) {
	var run models.RoutineRun
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT * FROM orchestrate_routine_runs
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

// UpdateRunStatus updates a run's status and optionally its linked task.
func (r *Repository) UpdateRunStatus(
	ctx context.Context, runID, status, linkedTaskID string,
) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE orchestrate_routine_runs
		SET status = ?, linked_task_id = ?, completed_at = ?
		WHERE id = ?
	`), status, linkedTaskID, now, runID)
	return err
}

// UpdateRunCoalesced marks a run as coalesced into another run.
func (r *Repository) UpdateRunCoalesced(ctx context.Context, runID, coalescedIntoRunID string) error {
	now := time.Now().UTC()
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE orchestrate_routine_runs
		SET status = 'coalesced', coalesced_into_run_id = ?, completed_at = ?
		WHERE id = ?
	`), coalescedIntoRunID, now, runID)
	return err
}
