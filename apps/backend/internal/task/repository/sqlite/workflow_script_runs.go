package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

const workflowScriptRunColumns = `
	id, occurrence_key, task_id, workflow_id, workflow_step_id, workflow_step_name,
	trigger, action_position, session_id, execution_id, command, timeout_seconds,
	failure_policy, process_request_id, message_id, process_id, status,
	admission_attempted_at, exit_code, output, output_truncated, failure_reason,
	created_at, updated_at, started_at, completed_at`

// ClaimWorkflowScriptRun atomically inserts a pending run or returns the
// immutable row previously claimed for the same occurrence key.
func (r *Repository) ClaimWorkflowScriptRun(ctx context.Context, run *models.WorkflowScriptRun) (*models.WorkflowScriptRun, bool, error) {
	if err := validateWorkflowScriptRun(run); err != nil {
		return nil, false, err
	}
	now := r.nowUTC()
	if run.ID == "" {
		run.ID = uuid.NewString()
	}
	if run.Status == "" {
		run.Status = models.WorkflowScriptRunPending
	}
	if run.CreatedAt.IsZero() {
		run.CreatedAt = now
	}
	run.UpdatedAt = now
	if run.ProcessRequestID == "" {
		run.ProcessRequestID = run.ID
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO workflow_script_runs (`+workflowScriptRunColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (occurrence_key) DO NOTHING
	`), run.ID, run.OccurrenceKey, run.TaskID, run.WorkflowID, run.WorkflowStepID,
		run.WorkflowStepName, string(run.Trigger), run.ActionPosition, run.SessionID, run.ExecutionID,
		run.Command, run.TimeoutSeconds, run.FailurePolicy, run.ProcessRequestID,
		run.MessageID, run.ProcessID, string(run.Status), run.AdmissionAttemptedAt,
		run.ExitCode, run.Output, dialect.BoolToInt(run.OutputTruncated), run.FailureReason,
		run.CreatedAt, run.UpdatedAt, run.StartedAt, run.CompletedAt)
	if err != nil {
		return nil, false, fmt.Errorf("create workflow script run: %w", err)
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return nil, false, fmt.Errorf("inspect workflow script run claim: %w", err)
	}
	claimed, err := r.GetWorkflowScriptRunByOccurrence(ctx, run.OccurrenceKey)
	if err != nil {
		return nil, false, err
	}
	return claimed, inserted == 1, nil
}

func validateWorkflowScriptRun(run *models.WorkflowScriptRun) error {
	if run == nil {
		return fmt.Errorf("%w: run is nil", models.ErrWorkflowScriptRunInvalid)
	}
	missing := make([]string, 0, 9)
	if strings.TrimSpace(run.OccurrenceKey) == "" {
		missing = append(missing, "occurrence_key")
	}
	if strings.TrimSpace(run.TaskID) == "" {
		missing = append(missing, "task_id")
	}
	if strings.TrimSpace(run.WorkflowStepID) == "" {
		missing = append(missing, "workflow_step_id")
	}
	if !run.Trigger.IsValid() {
		missing = append(missing, "trigger")
	}
	if run.ActionPosition < 0 {
		missing = append(missing, "action_position")
	}
	if strings.TrimSpace(run.SessionID) == "" {
		missing = append(missing, "session_id")
	}
	if strings.TrimSpace(run.ExecutionID) == "" {
		missing = append(missing, "execution_id")
	}
	if strings.TrimSpace(run.Command) == "" {
		missing = append(missing, "command")
	}
	if run.TimeoutSeconds <= 0 {
		missing = append(missing, "timeout_seconds")
	}
	if !models.IsValidWorkflowScriptFailurePolicy(run.FailurePolicy) {
		missing = append(missing, "failure_policy")
	}
	if len(missing) > 0 {
		return fmt.Errorf("%w: missing or invalid %s", models.ErrWorkflowScriptRunInvalid, strings.Join(missing, ", "))
	}
	if run.Status != "" && run.Status != models.WorkflowScriptRunPending {
		return fmt.Errorf("%w: new runs must be pending", models.ErrWorkflowScriptRunInvalid)
	}
	return nil
}

// GetWorkflowScriptRun returns one run by its durable ID.
func (r *Repository) GetWorkflowScriptRun(ctx context.Context, id string) (*models.WorkflowScriptRun, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT `+workflowScriptRunColumns+` FROM workflow_script_runs WHERE id = ?`), id)
	run, err := scanWorkflowScriptRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", models.ErrWorkflowScriptRunNotFound, id)
	}
	return run, err
}

// GetWorkflowScriptRunByOccurrence returns the winner of an occurrence claim.
func (r *Repository) GetWorkflowScriptRunByOccurrence(ctx context.Context, key string) (*models.WorkflowScriptRun, error) {
	row := r.ro.QueryRowContext(ctx, r.ro.Rebind(
		`SELECT `+workflowScriptRunColumns+` FROM workflow_script_runs WHERE occurrence_key = ?`), key)
	run, err := scanWorkflowScriptRun(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: occurrence %s", models.ErrWorkflowScriptRunNotFound, key)
	}
	return run, err
}

// ListNonTerminalWorkflowScriptRuns returns runs that need normal dispatch or
// startup reconciliation, in their original admission order.
func (r *Repository) ListNonTerminalWorkflowScriptRuns(ctx context.Context) ([]*models.WorkflowScriptRun, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT `+workflowScriptRunColumns+`
		FROM workflow_script_runs
		WHERE status IN (?, ?, ?)
		ORDER BY created_at ASC, id ASC
	`), models.WorkflowScriptRunPending, models.WorkflowScriptRunStarting, models.WorkflowScriptRunRunning)
	if err != nil {
		return nil, fmt.Errorf("list non-terminal workflow script runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	runs := make([]*models.WorkflowScriptRun, 0)
	for rows.Next() {
		run, scanErr := scanWorkflowScriptRun(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

// MarkWorkflowScriptRunStarting durably records the admission attempt and
// message projection before any managed process start request is sent.
func (r *Repository) MarkWorkflowScriptRunStarting(ctx context.Context, id, messageID string) (bool, error) {
	if strings.TrimSpace(messageID) == "" {
		return false, fmt.Errorf("%w: message_id is required", models.ErrWorkflowScriptRunInvalid)
	}
	now := r.nowUTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE workflow_script_runs
		SET status = ?, message_id = ?, admission_attempted_at = ?,
			started_at = ?, updated_at = ?
		WHERE id = ? AND status = ?
	`), models.WorkflowScriptRunStarting, messageID, now, now, now, id, models.WorkflowScriptRunPending)
	if err != nil {
		return false, fmt.Errorf("mark workflow script run starting: %w", err)
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

// MarkWorkflowScriptRunRunning records the managed process identity. A retry
// with the same process ID is harmless; another process ID is never accepted.
func (r *Repository) MarkWorkflowScriptRunRunning(ctx context.Context, id, processID string) (bool, error) {
	if strings.TrimSpace(processID) == "" {
		return false, fmt.Errorf("%w: process_id is required", models.ErrWorkflowScriptRunInvalid)
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE workflow_script_runs
		SET status = ?, process_id = ?, updated_at = ?
		WHERE id = ? AND status = ? AND (process_id = '' OR process_id = ?)
	`), models.WorkflowScriptRunRunning, processID, r.nowUTC(), id,
		models.WorkflowScriptRunStarting, processID)
	if err != nil {
		return false, fmt.Errorf("mark workflow script run running: %w", err)
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

// CompleteWorkflowScriptRun writes the terminal process result exactly once.
// It updates only execution fields; the claimed workflow snapshot is never
// read from or written by a later workflow edit.
func (r *Repository) CompleteWorkflowScriptRun(ctx context.Context, id string, completion models.WorkflowScriptRunCompletion) (bool, error) {
	if !completion.Status.IsTerminal() {
		return false, fmt.Errorf("%w: %s is not terminal", models.ErrWorkflowScriptRunInvalidTransition, completion.Status)
	}
	completedAt := completion.CompletedAt
	if completedAt.IsZero() {
		completedAt = r.nowUTC()
	}
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE workflow_script_runs
		SET status = ?,
			process_id = CASE WHEN ? = '' THEN process_id ELSE ? END,
			exit_code = ?, output = ?, output_truncated = ?, failure_reason = ?,
			completed_at = ?, updated_at = ?
		WHERE id = ? AND status IN (?, ?, ?)
	`), completion.Status, completion.ProcessID, completion.ProcessID,
		completion.ExitCode, completion.Output, dialect.BoolToInt(completion.OutputTruncated),
		completion.FailureReason, completedAt, r.nowUTC(), id,
		models.WorkflowScriptRunPending, models.WorkflowScriptRunStarting, models.WorkflowScriptRunRunning)
	if err != nil {
		return false, fmt.Errorf("complete workflow script run: %w", err)
	}
	count, err := result.RowsAffected()
	return count == 1, err
}

// InterruptWorkflowScriptRuns closes only non-terminal runs that crossed the
// admission boundary. Pending claims without an admission attempt remain
// eligible for normal dispatch after restart.
func (r *Repository) InterruptWorkflowScriptRuns(ctx context.Context, reason string) (int, error) {
	now := r.nowUTC()
	result, err := r.db.ExecContext(ctx, r.db.Rebind(`
		UPDATE workflow_script_runs
		SET status = ?, failure_reason = ?, completed_at = ?, updated_at = ?
		WHERE status IN (?, ?)
	`), models.WorkflowScriptRunInterrupted, reason, now, now,
		models.WorkflowScriptRunStarting, models.WorkflowScriptRunRunning)
	if err != nil {
		return 0, fmt.Errorf("interrupt workflow script runs: %w", err)
	}
	count, err := result.RowsAffected()
	return int(count), err
}

type workflowScriptRunScanner interface {
	Scan(dest ...any) error
}

func scanWorkflowScriptRun(scanner workflowScriptRunScanner) (*models.WorkflowScriptRun, error) {
	run := &models.WorkflowScriptRun{}
	var trigger, status string
	var admissionAttemptedAt, startedAt, completedAt sql.NullTime
	var exitCode sql.NullInt64
	var outputTruncated int
	err := scanner.Scan(
		&run.ID, &run.OccurrenceKey, &run.TaskID, &run.WorkflowID, &run.WorkflowStepID,
		&run.WorkflowStepName, &trigger, &run.ActionPosition, &run.SessionID, &run.ExecutionID, &run.Command,
		&run.TimeoutSeconds, &run.FailurePolicy, &run.ProcessRequestID, &run.MessageID,
		&run.ProcessID, &status, &admissionAttemptedAt, &exitCode, &run.Output,
		&outputTruncated, &run.FailureReason, &run.CreatedAt, &run.UpdatedAt,
		&startedAt, &completedAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return nil, fmt.Errorf("scan workflow script run: %w", err)
	}
	run.Trigger = models.WorkflowScriptRunTrigger(trigger)
	run.Status = models.WorkflowScriptRunStatus(status)
	run.OutputTruncated = outputTruncated != 0
	if admissionAttemptedAt.Valid {
		t := admissionAttemptedAt.Time.UTC()
		run.AdmissionAttemptedAt = &t
	}
	if exitCode.Valid {
		code := int(exitCode.Int64)
		run.ExitCode = &code
	}
	if startedAt.Valid {
		t := startedAt.Time.UTC()
		run.StartedAt = &t
	}
	if completedAt.Valid {
		t := completedAt.Time.UTC()
		run.CompletedAt = &t
	}
	return run, nil
}
