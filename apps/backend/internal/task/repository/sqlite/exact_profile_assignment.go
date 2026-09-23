package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
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
//nolint:cyclop,funlen,gocognit // The transaction's three generation states must stay together.
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
		WHERE EXISTS (
			SELECT 1 FROM tasks
			WHERE id = ? AND workspace_id = ?
				AND (? = '' OR workflow_id = ?)
				AND (? = '' OR workflow_step_id = ?)
				AND (? = '' OR state = ?)
				AND archived_at IS NULL
		)
		`), assignment.TaskID, assignment.WorkspaceID, assignment.AgentProfileID, assignment.ProfileRevision,
			assignment.Generation, assignment.SourceWorkflowID, assignment.SourceWorkflowStepID, assignment.SourceTaskState,
			dialect.BoolToInt(true), assignment.CreatedAt, assignment.UpdatedAt,
			assignment.TaskID, assignment.WorkspaceID,
			assignment.SourceWorkflowID, assignment.SourceWorkflowID,
			assignment.SourceWorkflowStepID, assignment.SourceWorkflowStepID,
			assignment.SourceTaskState, assignment.SourceTaskState)
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
		result, execErr := tx.ExecContext(ctx, r.db.Rebind(`
			UPDATE task_exact_profile_assignments SET updated_at = updated_at
			WHERE task_id = ? AND generation = ? AND active = ?
				AND EXISTS (
					SELECT 1 FROM tasks
					WHERE id = ? AND workspace_id = ?
						AND (? = '' OR workflow_id = ?)
						AND (? = '' OR workflow_step_id = ?)
						AND (? = '' OR state = ?)
						AND archived_at IS NULL
				)
		`), assignment.TaskID, assignment.Generation, dialect.BoolToInt(true),
			assignment.TaskID, assignment.WorkspaceID,
			assignment.SourceWorkflowID, assignment.SourceWorkflowID,
			assignment.SourceWorkflowStepID, assignment.SourceWorkflowStepID,
			assignment.SourceTaskState, assignment.SourceTaskState)
		if execErr != nil {
			return false, fmt.Errorf("verify exact profile assignment replay: %w", execErr)
		}
		rows, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return false, rowsErr
		}
		if rows != 1 {
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
				AND EXISTS (
					SELECT 1 FROM tasks
					WHERE id = ? AND workspace_id = ?
						AND (? = '' OR workflow_id = ?)
						AND (? = '' OR workflow_step_id = ?)
						AND (? = '' OR state = ?)
						AND archived_at IS NULL
				)
		`), assignment.WorkspaceID, assignment.AgentProfileID, assignment.ProfileRevision, assignment.Generation,
			assignment.SourceWorkflowID, assignment.SourceWorkflowStepID, assignment.SourceTaskState,
			dialect.BoolToInt(true), assignment.UpdatedAt, assignment.TaskID, current.Generation,
			assignment.TaskID, assignment.WorkspaceID,
			assignment.SourceWorkflowID, assignment.SourceWorkflowID,
			assignment.SourceWorkflowStepID, assignment.SourceWorkflowStepID,
			assignment.SourceTaskState, assignment.SourceTaskState)
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

func (r *Repository) BindExactProfileLaunchAttempt(ctx context.Context, binding *models.ExactProfileLaunchAttemptBinding) (bool, error) {
	if !validExactProfileLaunchAttemptBinding(binding) {
		return false, models.ErrExactProfileAssignmentInvalidInput
	}
	if binding.CreatedAt.IsZero() {
		binding.CreatedAt = r.exactProfileAssignmentNow()
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("begin exact profile launch attempt: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.validateExactProfileLaunchAttemptCurrentTx(ctx, tx, binding); err != nil {
		return false, err
	}
	var current models.ExactProfileLaunchAttemptBinding
	var revisionNanos int64
	bindingQuery := `SELECT task_id, session_id, execution_id, attempt_id, session_incarnation_id, agent_profile_id, model, profile_revision_nanos, generation, created_at FROM task_exact_profile_launch_attempt_bindings WHERE task_id=? AND session_id=?`
	if dialect.IsPostgres(r.db.DriverName()) {
		bindingQuery += ` FOR UPDATE`
	}
	err = tx.QueryRowxContext(ctx, r.db.Rebind(bindingQuery), binding.TaskID, binding.SessionID).Scan(&current.TaskID, &current.SessionID, &current.ExecutionID, &current.AttemptID, &current.SessionIncarnationID, &current.AgentProfileID, &current.Model, &revisionNanos, &current.Generation, &current.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		if binding.ExpectedPrior != nil {
			return false, models.ErrExactProfileAssignmentGeneration
		}
		_, err = tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO task_exact_profile_launch_attempt_bindings (task_id, session_id, execution_id, attempt_id, session_incarnation_id, agent_profile_id, model, profile_revision_nanos, generation, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`), binding.TaskID, binding.SessionID, binding.ExecutionID, binding.AttemptID, binding.SessionIncarnationID, binding.AgentProfileID, binding.Model, binding.ProfileRevision.UnixNano(), binding.Generation, binding.CreatedAt)
		if err != nil {
			return false, fmt.Errorf("insert exact profile launch attempt: %w", err)
		}
	} else if err != nil {
		return false, fmt.Errorf("load exact profile launch attempt: %w", err)
	} else {
		current.ProfileRevision = time.Unix(0, revisionNanos).UTC()
		if binding.ExpectedPrior == nil || !exactProfileLaunchAttemptBindingsEqual(&current, binding.ExpectedPrior) {
			return false, models.ErrExactProfileAssignmentGeneration
		}
		result, execErr := tx.ExecContext(ctx, r.db.Rebind(`UPDATE task_exact_profile_launch_attempt_bindings SET execution_id=?, attempt_id=?, session_incarnation_id=?, agent_profile_id=?, model=?, profile_revision_nanos=?, generation=?, created_at=? WHERE task_id=? AND session_id=? AND execution_id=? AND attempt_id=? AND session_incarnation_id=? AND agent_profile_id=? AND model=? AND profile_revision_nanos=? AND generation=?`), binding.ExecutionID, binding.AttemptID, binding.SessionIncarnationID, binding.AgentProfileID, binding.Model, binding.ProfileRevision.UnixNano(), binding.Generation, binding.CreatedAt, binding.TaskID, binding.SessionID, current.ExecutionID, current.AttemptID, current.SessionIncarnationID, current.AgentProfileID, current.Model, current.ProfileRevision.UnixNano(), current.Generation)
		if execErr != nil {
			return false, fmt.Errorf("replace exact profile launch attempt: %w", execErr)
		}
		rows, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return false, rowsErr
		}
		if rows != 1 {
			return false, models.ErrExactProfileAssignmentGeneration
		}
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit exact profile launch attempt: %w", err)
	}
	return true, nil
}

// GetCurrentExactProfileLaunchAttempt returns one admitted binding only while
// its assignment and session incarnation remain current. Recovery callers must
// match the returned execution identity before using it as event evidence.
func (r *Repository) GetCurrentExactProfileLaunchAttempt(ctx context.Context, taskID, sessionID string) (*models.ExactProfileLaunchAttemptBinding, error) {
	if taskID == "" || sessionID == "" {
		return nil, models.ErrExactProfileAssignmentInvalidInput
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin exact profile launch attempt lookup: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	binding := &models.ExactProfileLaunchAttemptBinding{}
	var revisionNanos int64
	err = tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT task_id, session_id, execution_id, attempt_id, session_incarnation_id, agent_profile_id, model, profile_revision_nanos, generation, created_at FROM task_exact_profile_launch_attempt_bindings WHERE task_id=? AND session_id=?`), taskID, sessionID).Scan(&binding.TaskID, &binding.SessionID, &binding.ExecutionID, &binding.AttemptID, &binding.SessionIncarnationID, &binding.AgentProfileID, &binding.Model, &revisionNanos, &binding.Generation, &binding.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load exact profile launch attempt: %w", err)
	}
	binding.ProfileRevision = time.Unix(0, revisionNanos).UTC()
	if !validExactProfileLaunchAttemptBinding(binding) {
		return nil, nil
	}
	if err := r.validateExactProfileLaunchAttemptCurrentTx(ctx, tx, binding); err != nil {
		if errors.Is(err, models.ErrExactProfileAssignmentGeneration) {
			return nil, nil
		}
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit exact profile launch attempt lookup: %w", err)
	}
	return binding, nil
}

func (r *Repository) RecordExactProfileLaunchReceiptForAttempt(ctx context.Context, binding *models.ExactProfileLaunchAttemptBinding, receipt *models.ExactProfileLaunchReceipt) (bool, error) {
	if !validExactProfileLaunchAttemptBinding(binding) || receipt == nil {
		return false, models.ErrExactProfileAssignmentInvalidInput
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.validateExactProfileLaunchAttemptCurrentTx(ctx, tx, binding); err != nil {
		return false, err
	}
	var current int
	bindingQuery := `SELECT 1 FROM task_exact_profile_launch_attempt_bindings WHERE task_id=? AND session_id=? AND execution_id=? AND attempt_id=? AND session_incarnation_id=? AND agent_profile_id=? AND model=? AND profile_revision_nanos=? AND generation=?`
	if dialect.IsPostgres(r.db.DriverName()) {
		bindingQuery += ` FOR UPDATE`
	}
	err = tx.GetContext(ctx, &current, r.db.Rebind(bindingQuery), binding.TaskID, binding.SessionID, binding.ExecutionID, binding.AttemptID, binding.SessionIncarnationID, binding.AgentProfileID, binding.Model, binding.ProfileRevision.UnixNano(), binding.Generation)
	if errors.Is(err, sql.ErrNoRows) {
		return false, models.ErrExactProfileAssignmentGeneration
	}
	if err != nil {
		return false, err
	}
	if receipt.TaskID != binding.TaskID || receipt.SessionID != binding.SessionID || receipt.AgentProfileID != binding.AgentProfileID || receipt.Generation != binding.Generation || receipt.Model != binding.Model || !receipt.ProfileRevision.Equal(binding.ProfileRevision) {
		return false, models.ErrExactProfileAssignmentGeneration
	}
	if receipt.CreatedAt.IsZero() {
		receipt.CreatedAt = r.exactProfileAssignmentNow()
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO task_exact_profile_launch_attempt_receipts (task_id,session_id,execution_id,attempt_id,session_incarnation_id,agent_profile_id,profile_revision_nanos,generation,model,outcome,failure_reason,inference_started,substitution_done,created_at) SELECT ?,?,?,?,?,?,?,?,?,?,?,?,?,? WHERE EXISTS (SELECT 1 FROM task_exact_profile_launch_attempt_bindings WHERE task_id=? AND session_id=? AND execution_id=? AND attempt_id=? AND session_incarnation_id=? AND agent_profile_id=? AND model=? AND profile_revision_nanos=? AND generation=?) AND EXISTS (SELECT 1 FROM task_exact_profile_assignments WHERE task_id=? AND agent_profile_id=? AND profile_revision=? AND generation=? AND active=?) AND EXISTS (SELECT 1 FROM task_sessions WHERE id=? AND task_id=? AND queue_incarnation_id=?) ON CONFLICT(task_id,session_id,execution_id,attempt_id,session_incarnation_id,agent_profile_id,profile_revision_nanos,generation) DO NOTHING`), receipt.TaskID, receipt.SessionID, binding.ExecutionID, binding.AttemptID, binding.SessionIncarnationID, receipt.AgentProfileID, receipt.ProfileRevision.UnixNano(), receipt.Generation, binding.Model, receipt.Outcome, receipt.FailureReason, dialect.BoolToInt(receipt.InferenceStarted), dialect.BoolToInt(receipt.SubstitutionDone), receipt.CreatedAt, binding.TaskID, binding.SessionID, binding.ExecutionID, binding.AttemptID, binding.SessionIncarnationID, binding.AgentProfileID, binding.Model, binding.ProfileRevision.UnixNano(), binding.Generation, binding.TaskID, binding.AgentProfileID, binding.ProfileRevision, binding.Generation, dialect.BoolToInt(true), binding.SessionID, binding.TaskID, binding.SessionIncarnationID)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows == 1, tx.Commit()
}

func validExactProfileLaunchAttemptBinding(binding *models.ExactProfileLaunchAttemptBinding) bool {
	return binding != nil && binding.TaskID != "" && binding.SessionID != "" && binding.ExecutionID != "" && binding.AttemptID != "" && binding.SessionIncarnationID != "" && binding.AgentProfileID != "" && binding.Model != "" && !binding.ProfileRevision.IsZero() && binding.Generation > 0
}

func exactProfileLaunchAttemptBindingsEqual(left, right *models.ExactProfileLaunchAttemptBinding) bool {
	return left != nil && right != nil && left.TaskID == right.TaskID && left.SessionID == right.SessionID && left.ExecutionID == right.ExecutionID && left.AttemptID == right.AttemptID && left.SessionIncarnationID == right.SessionIncarnationID && left.AgentProfileID == right.AgentProfileID && left.Model == right.Model && left.ProfileRevision.Equal(right.ProfileRevision) && left.Generation == right.Generation
}

func (r *Repository) validateExactProfileLaunchAttemptCurrentTx(ctx context.Context, tx *sqlx.Tx, binding *models.ExactProfileLaunchAttemptBinding) error {
	var found int
	assignmentQuery := `SELECT 1 FROM task_exact_profile_assignments WHERE task_id=? AND agent_profile_id=? AND profile_revision=? AND generation=? AND active=?`
	sessionQuery := `SELECT 1 FROM task_sessions WHERE id=? AND task_id=? AND queue_incarnation_id=?`
	if dialect.IsPostgres(r.db.DriverName()) {
		assignmentQuery += ` FOR UPDATE`
		sessionQuery += ` FOR UPDATE`
	}
	err := tx.GetContext(ctx, &found, r.db.Rebind(assignmentQuery), binding.TaskID, binding.AgentProfileID, binding.ProfileRevision, binding.Generation, dialect.BoolToInt(true))
	if errors.Is(err, sql.ErrNoRows) {
		return models.ErrExactProfileAssignmentGeneration
	}
	if err != nil {
		return fmt.Errorf("load current exact profile assignment: %w", err)
	}
	err = tx.GetContext(ctx, &found, r.db.Rebind(sessionQuery), binding.SessionID, binding.TaskID, binding.SessionIncarnationID)
	if errors.Is(err, sql.ErrNoRows) {
		return models.ErrExactProfileAssignmentGeneration
	}
	if err != nil {
		return fmt.Errorf("load current exact profile session: %w", err)
	}
	return nil
}

// GetExactProfileLaunchReceipt loads the launch receipt for one session. A
// missing receipt (pre-exact sessions, or launches that never had an exact
// assignment applied) returns nil.
func (r *Repository) GetExactProfileLaunchReceipt(ctx context.Context, taskID, sessionID string) (*models.ExactProfileLaunchReceipt, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin exact profile launch receipt lookup: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	assignmentLockQuery := `SELECT 1 FROM task_exact_profile_assignments WHERE task_id = ?`
	sessionLockQuery := `SELECT 1 FROM task_sessions WHERE id = ? AND task_id = ?`
	bindingLockQuery := `SELECT 1 FROM task_exact_profile_launch_attempt_bindings WHERE task_id = ? AND session_id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		assignmentLockQuery += ` FOR SHARE`
		sessionLockQuery += ` FOR SHARE`
		bindingLockQuery += ` FOR SHARE`
	}
	var found int
	err = tx.GetContext(ctx, &found, r.db.Rebind(assignmentLockQuery), taskID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("lock exact profile receipt assignment: %w", err)
	}
	err = tx.GetContext(ctx, &found, r.db.Rebind(sessionLockQuery), sessionID, taskID)
	if err == nil {
		err = tx.GetContext(ctx, &found, r.db.Rebind(bindingLockQuery), taskID, sessionID)
		if err == nil {
			return r.getCurrentExactProfileAttemptReceiptTx(ctx, tx, taskID, sessionID)
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("load exact profile launch attempt binding: %w", err)
		}
		if r.exactProfileReceiptLegacyFallbackHook != nil {
			r.exactProfileReceiptLegacyFallbackHook()
		}
		return r.getLegacyExactProfileLaunchReceiptTx(ctx, tx, taskID, sessionID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("lock exact profile receipt session: %w", err)
	}
	// A missing session cannot acquire a binding, so legacy lookup is safe.
	return r.getLegacyExactProfileLaunchReceiptTx(ctx, tx, taskID, sessionID)
}

func (r *Repository) getLegacyExactProfileLaunchReceiptTx(ctx context.Context, tx *sqlx.Tx, taskID, sessionID string) (*models.ExactProfileLaunchReceipt, error) {
	row := tx.QueryRowxContext(ctx, r.db.Rebind(`
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
			if err := tx.Commit(); err != nil {
				return nil, fmt.Errorf("commit empty legacy exact profile receipt lookup: %w", err)
			}
			return nil, nil
		}
		return nil, fmt.Errorf("load exact profile launch receipt: %w", err)
	}
	receipt.ProfileRevision = time.Unix(0, revisionNanos).UTC()
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit legacy exact profile receipt lookup: %w", err)
	}
	return &receipt, nil
}

func (r *Repository) getCurrentExactProfileAttemptReceiptTx(ctx context.Context, tx *sqlx.Tx, taskID, sessionID string) (*models.ExactProfileLaunchReceipt, error) {
	var binding models.ExactProfileLaunchAttemptBinding
	var bindingRevision int64
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT task_id, session_id, execution_id, attempt_id, session_incarnation_id, agent_profile_id, model, profile_revision_nanos, generation, created_at FROM task_exact_profile_launch_attempt_bindings WHERE task_id=? AND session_id=?`), taskID, sessionID).Scan(&binding.TaskID, &binding.SessionID, &binding.ExecutionID, &binding.AttemptID, &binding.SessionIncarnationID, &binding.AgentProfileID, &binding.Model, &bindingRevision, &binding.Generation, &binding.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load current exact profile attempt binding: %w", err)
	}
	binding.ProfileRevision = time.Unix(0, bindingRevision).UTC()
	if !validExactProfileLaunchAttemptBinding(&binding) {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	var current int
	err = tx.GetContext(ctx, &current, r.db.Rebind(`SELECT 1 FROM task_exact_profile_assignments WHERE task_id=? AND agent_profile_id=? AND profile_revision=? AND generation=? AND active=?`), binding.TaskID, binding.AgentProfileID, binding.ProfileRevision, binding.Generation, dialect.BoolToInt(true))
	if errors.Is(err, sql.ErrNoRows) {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("validate current exact profile assignment receipt: %w", err)
	}
	err = tx.GetContext(ctx, &current, r.db.Rebind(`SELECT 1 FROM task_sessions WHERE id=? AND task_id=? AND queue_incarnation_id=?`), binding.SessionID, binding.TaskID, binding.SessionIncarnationID)
	if errors.Is(err, sql.ErrNoRows) {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("validate current exact profile session receipt: %w", err)
	}
	if r.exactProfileReceiptCurrentLookupHook != nil {
		r.exactProfileReceiptCurrentLookupHook()
	}
	row := tx.QueryRowxContext(ctx, r.db.Rebind(`SELECT task_id, session_id, agent_profile_id, generation, profile_revision_nanos, model, outcome, failure_reason, inference_started, substitution_done, created_at FROM task_exact_profile_launch_attempt_receipts WHERE task_id=? AND session_id=? AND execution_id=? AND attempt_id=? AND session_incarnation_id=? AND agent_profile_id=? AND model=? AND profile_revision_nanos=? AND generation=?`), binding.TaskID, binding.SessionID, binding.ExecutionID, binding.AttemptID, binding.SessionIncarnationID, binding.AgentProfileID, binding.Model, binding.ProfileRevision.UnixNano(), binding.Generation)
	var receipt models.ExactProfileLaunchReceipt
	var revisionNanos int64
	if err := row.Scan(&receipt.TaskID, &receipt.SessionID, &receipt.AgentProfileID, &receipt.Generation, &revisionNanos, &receipt.Model, &receipt.Outcome, &receipt.FailureReason, &receipt.InferenceStarted, &receipt.SubstitutionDone, &receipt.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			if err := tx.Commit(); err != nil {
				return nil, err
			}
			return nil, nil
		}
		return nil, fmt.Errorf("load current exact profile attempt receipt: %w", err)
	}
	receipt.ProfileRevision = time.Unix(0, revisionNanos).UTC()
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit current exact profile receipt lookup: %w", err)
	}
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
