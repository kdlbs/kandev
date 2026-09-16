package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// GetExactProfileAssignment loads the task-owned profile selection. A missing
// assignment is represented by nil, preserving ordinary routing behavior.
func (r *Repository) GetExactProfileAssignment(ctx context.Context, taskID string) (*models.ExactProfileAssignment, error) {
	row := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT task_id, workspace_id, agent_profile_id, profile_revision, generation,
			source_workflow_id, source_workflow_step_id, source_task_state, active, created_at, updated_at
		FROM task_exact_profile_assignments
		WHERE task_id = ?
	`), taskID)
	assignment, err := scanExactProfileAssignment(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return assignment, err
}

// UpsertExactProfileAssignment admits only the next assignment generation.
// Replaying an identical generation is a no-op; a stale, skipped, or changed
// same-generation write is rejected so it cannot silently change a launch.
func (r *Repository) UpsertExactProfileAssignment(ctx context.Context, assignment *models.ExactProfileAssignment) (bool, error) {
	if err := validateExactProfileAssignment(assignment); err != nil {
		return false, err
	}
	now := r.exactProfileAssignmentNow()
	if assignment.CreatedAt.IsZero() {
		assignment.CreatedAt = now
	}
	assignment.UpdatedAt = now

	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_exact_profile_assignments (
			task_id, workspace_id, agent_profile_id, profile_revision, generation,
			source_workflow_id, source_workflow_step_id, source_task_state, active, created_at, updated_at
		)
		SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
		WHERE ? = 1 AND EXISTS (SELECT 1 FROM tasks WHERE id = ? AND workspace_id = ?)
		ON CONFLICT(task_id) DO NOTHING
	`), assignment.TaskID, assignment.WorkspaceID, assignment.AgentProfileID, assignment.ProfileRevision,
		assignment.Generation, assignment.SourceWorkflowID, assignment.SourceWorkflowStepID, assignment.SourceTaskState,
		dialect.BoolToInt(false), assignment.CreatedAt, assignment.UpdatedAt, assignment.Generation,
		assignment.TaskID, assignment.WorkspaceID)
	if err != nil {
		return false, fmt.Errorf("insert exact profile assignment: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows == 1 {
		return true, nil
	}

	result, err = r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_exact_profile_assignments
		SET workspace_id = ?, agent_profile_id = ?, profile_revision = ?,
			generation = ?, source_workflow_id = ?, source_workflow_step_id = ?,
			source_task_state = ?, active = ?, updated_at = ?
		WHERE task_id = ? AND generation = ?
			AND EXISTS (SELECT 1 FROM tasks WHERE id = ? AND workspace_id = ?)
	`), assignment.WorkspaceID, assignment.AgentProfileID, assignment.ProfileRevision,
		assignment.Generation, assignment.SourceWorkflowID, assignment.SourceWorkflowStepID,
		assignment.SourceTaskState, dialect.BoolToInt(false), assignment.UpdatedAt, assignment.TaskID,
		assignment.Generation-1, assignment.TaskID, assignment.WorkspaceID)
	if err != nil {
		return false, fmt.Errorf("replace exact profile assignment: %w", err)
	}
	rows, err = result.RowsAffected()
	if err != nil {
		return false, err
	}
	if rows == 1 {
		return true, nil
	}

	current, err := r.GetExactProfileAssignment(ctx, assignment.TaskID)
	if err != nil {
		return false, err
	}
	if exactProfileAssignmentsEqual(current, assignment) {
		return false, nil
	}
	return false, models.ErrExactProfileAssignmentGeneration
}

// AssignExactProfileAssignment records and activates a generation in one
// transaction. No caller can observe a newly accepted generation inactive, or
// activate an assignment after another writer has superseded it.
//
//nolint:cyclop,funlen // The transaction's three generation states must stay together.
func (r *Repository) AssignExactProfileAssignment(ctx context.Context, assignment *models.ExactProfileAssignment) (bool, error) {
	if err := validateExactProfileAssignment(assignment); err != nil {
		return false, err
	}
	now := r.exactProfileAssignmentNow()
	if assignment.CreatedAt.IsZero() {
		assignment.CreatedAt = now
	}
	assignment.UpdatedAt = now

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin exact profile assignment: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var current models.ExactProfileAssignment
	err = tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT task_id, workspace_id, agent_profile_id, profile_revision, generation,
			source_workflow_id, source_workflow_step_id, source_task_state, active, created_at, updated_at
		FROM task_exact_profile_assignments WHERE task_id = ?
	`), assignment.TaskID).Scan(
		&current.TaskID, &current.WorkspaceID, &current.AgentProfileID, &current.ProfileRevision,
		&current.Generation, &current.SourceWorkflowID, &current.SourceWorkflowStepID, &current.SourceTaskState,
		&current.Active, &current.CreatedAt, &current.UpdatedAt,
	)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("load exact profile assignment: %w", err)
	}

	switch {
	case errors.Is(err, sql.ErrNoRows):
		if assignment.Generation != 1 {
			return false, models.ErrExactProfileAssignmentGeneration
		}
		result, execErr := tx.ExecContext(ctx, r.db.Rebind(`
			INSERT INTO task_exact_profile_assignments (
				task_id, workspace_id, agent_profile_id, profile_revision, generation,
				source_workflow_id, source_workflow_step_id, source_task_state, active, created_at, updated_at
			) SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
			WHERE EXISTS (SELECT 1 FROM tasks WHERE id = ? AND workspace_id = ?)
		`), assignment.TaskID, assignment.WorkspaceID, assignment.AgentProfileID, assignment.ProfileRevision,
			assignment.Generation, assignment.SourceWorkflowID, assignment.SourceWorkflowStepID, assignment.SourceTaskState,
			dialect.BoolToInt(true), assignment.CreatedAt, assignment.UpdatedAt, assignment.TaskID, assignment.WorkspaceID)
		if execErr != nil {
			return false, fmt.Errorf("insert exact profile assignment: %w", execErr)
		}
		rows, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return false, rowsErr
		}
		if rows != 1 {
			return false, models.ErrExactProfileAssignmentGeneration
		}
	case current.Generation == assignment.Generation:
		if !exactProfileAssignmentsEqual(&current, assignment) {
			return false, models.ErrExactProfileAssignmentGeneration
		}
		if !current.Active {
			return false, models.ErrExactProfileAssignmentGeneration
		}
		if err := tx.Commit(); err != nil {
			return false, fmt.Errorf("commit exact profile assignment replay: %w", err)
		}
		return false, nil
	case current.Generation == assignment.Generation-1:
		result, execErr := tx.ExecContext(ctx, r.db.Rebind(`
			UPDATE task_exact_profile_assignments
			SET workspace_id = ?, agent_profile_id = ?, profile_revision = ?, generation = ?,
				source_workflow_id = ?, source_workflow_step_id = ?, source_task_state = ?, active = ?, updated_at = ?
			WHERE task_id = ? AND generation = ?
		`), assignment.WorkspaceID, assignment.AgentProfileID, assignment.ProfileRevision, assignment.Generation,
			assignment.SourceWorkflowID, assignment.SourceWorkflowStepID, assignment.SourceTaskState,
			dialect.BoolToInt(true), assignment.UpdatedAt, assignment.TaskID, current.Generation)
		if execErr != nil {
			return false, fmt.Errorf("replace exact profile assignment: %w", execErr)
		}
		rows, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return false, rowsErr
		}
		if rows != 1 {
			return false, models.ErrExactProfileAssignmentGeneration
		}
	default:
		return false, models.ErrExactProfileAssignmentGeneration
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit exact profile assignment: %w", err)
	}
	return true, nil
}

// ActivateExactProfileAssignment makes one already-recorded generation
// applicable to a transition. A later replacement cannot be activated by a
// stale move because the generation predicate is in the same UPDATE.
func (r *Repository) ActivateExactProfileAssignment(ctx context.Context, taskID string, generation int64) (bool, error) {
	if generation < 1 {
		return false, models.ErrExactProfileAssignmentGeneration
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_exact_profile_assignments
		SET active = ?, updated_at = ?
		WHERE task_id = ? AND generation = ? AND active = ?
	`), dialect.BoolToInt(true), r.exactProfileAssignmentNow(), taskID, generation, dialect.BoolToInt(false))
	if err != nil {
		return false, fmt.Errorf("activate exact profile assignment: %w", err)
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

type exactProfileAssignmentScanner interface {
	Scan(dest ...interface{}) error
}

func scanExactProfileAssignment(row exactProfileAssignmentScanner) (*models.ExactProfileAssignment, error) {
	var assignment models.ExactProfileAssignment
	if err := row.Scan(
		&assignment.TaskID, &assignment.WorkspaceID, &assignment.AgentProfileID, &assignment.ProfileRevision,
		&assignment.Generation, &assignment.SourceWorkflowID, &assignment.SourceWorkflowStepID,
		&assignment.SourceTaskState, &assignment.Active, &assignment.CreatedAt, &assignment.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return &assignment, nil
}

func validateExactProfileAssignment(assignment *models.ExactProfileAssignment) error {
	if assignment == nil || assignment.TaskID == "" || assignment.WorkspaceID == "" ||
		assignment.AgentProfileID == "" || assignment.ProfileRevision.IsZero() {
		return models.ErrExactProfileAssignmentInvalidInput
	}
	if assignment.Generation < 1 {
		return models.ErrExactProfileAssignmentGeneration
	}
	return nil
}

func exactProfileAssignmentsEqual(stored, candidate *models.ExactProfileAssignment) bool {
	return stored != nil &&
		stored.TaskID == candidate.TaskID &&
		stored.WorkspaceID == candidate.WorkspaceID &&
		stored.AgentProfileID == candidate.AgentProfileID &&
		stored.ProfileRevision.Equal(candidate.ProfileRevision) &&
		stored.Generation == candidate.Generation &&
		stored.SourceWorkflowID == candidate.SourceWorkflowID &&
		stored.SourceWorkflowStepID == candidate.SourceWorkflowStepID &&
		stored.SourceTaskState == candidate.SourceTaskState
}

func (r *Repository) exactProfileAssignmentNow() time.Time {
	if r.clockNow != nil {
		return r.clockNow().UTC()
	}
	return time.Now().UTC()
}

// RecordExactProfileLaunchReceipt writes one exact-launch receipt per session.
// A repeated call with the same (task, session) keys is a replay no-op; it
// returns false and never overwrites the original outcome, model, or flags.
func (r *Repository) RecordExactProfileLaunchReceipt(ctx context.Context, receipt *models.ExactProfileLaunchReceipt) (bool, error) {
	if receipt == nil || receipt.TaskID == "" || receipt.SessionID == "" ||
		receipt.AgentProfileID == "" || receipt.ProfileRevision.IsZero() {
		return false, models.ErrExactProfileAssignmentInvalidInput
	}
	if receipt.Generation < 1 {
		return false, models.ErrExactProfileAssignmentGeneration
	}
	if receipt.Outcome != models.ExactProfileLaunchOutcomeApplied &&
		receipt.Outcome != models.ExactProfileLaunchOutcomeFailedClosed {
		return false, models.ErrExactProfileAssignmentInvalidInput
	}
	if receipt.CreatedAt.IsZero() {
		receipt.CreatedAt = r.exactProfileAssignmentNow()
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_exact_profile_launch_receipts (
			task_id, session_id, agent_profile_id, generation, profile_revision_nanos,
			model, outcome, failure_reason, inference_started, substitution_done, created_at
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(task_id, session_id) DO NOTHING
	`), receipt.TaskID, receipt.SessionID, receipt.AgentProfileID, receipt.Generation,
		receipt.ProfileRevision.UnixNano(), receipt.Model, receipt.Outcome, receipt.FailureReason,
		dialect.BoolToInt(receipt.InferenceStarted), dialect.BoolToInt(receipt.SubstitutionDone),
		receipt.CreatedAt)
	if err != nil {
		return false, fmt.Errorf("record exact profile launch receipt: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, nil
}

// GetExactProfileLaunchReceipt loads the launch receipt for one session. A
// missing receipt (pre-exact sessions, or launches that never had an exact
// assignment applied) returns nil.
func (r *Repository) GetExactProfileLaunchReceipt(ctx context.Context, taskID, sessionID string) (*models.ExactProfileLaunchReceipt, error) {
	row := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT task_id, session_id, agent_profile_id, generation, profile_revision_nanos,
			model, outcome, failure_reason, inference_started, substitution_done, created_at
		FROM task_exact_profile_launch_receipts
		WHERE task_id = ? AND session_id = ?
	`), taskID, sessionID)
	var receipt models.ExactProfileLaunchReceipt
	var revisionNanos int64
	if err := row.Scan(
		&receipt.TaskID, &receipt.SessionID, &receipt.AgentProfileID, &receipt.Generation,
		&revisionNanos, &receipt.Model, &receipt.Outcome, &receipt.FailureReason,
		&receipt.InferenceStarted, &receipt.SubstitutionDone, &receipt.CreatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("load exact profile launch receipt: %w", err)
	}
	receipt.ProfileRevision = time.Unix(0, revisionNanos).UTC()
	return &receipt, nil
}

// FindExactProfileReusableSession returns the most recently updated
// nonterminal session on taskID launched against the active exact assignment's
// generation and profile revision. Sessions with a zero generation (created
// before, or outside, an exact assignment) or with a stale generation never
// match, so a launch can never silently continue on a session the wrong
// assignment built.
func (r *Repository) FindExactProfileReusableSession(
	ctx context.Context, taskID string, generation, revision int64,
) (*models.TaskSession, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT `+taskSessionSelectCols+` `+taskSessionFromClause+`
		WHERE ts.task_id = ?
			AND ts.exact_profile_generation = ?
			AND ts.exact_profile_revision = ?
			AND ts.state NOT IN (?, ?, ?)
		ORDER BY ts.updated_at DESC
		LIMIT 1
	`), taskID, generation, revision,
		string(models.TaskSessionStateCompleted), string(models.TaskSessionStateFailed), string(models.TaskSessionStateCancelled))
	session, err := r.scanTaskSession(ctx, row, "no reusable exact-profile session")
	if err != nil {
		if errors.Is(err, models.ErrTaskSessionNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("find exact-profile reusable session: %w", err)
	}
	return session, nil
}
