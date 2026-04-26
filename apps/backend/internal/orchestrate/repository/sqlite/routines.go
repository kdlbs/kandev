package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/orchestrate/models"
)

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
