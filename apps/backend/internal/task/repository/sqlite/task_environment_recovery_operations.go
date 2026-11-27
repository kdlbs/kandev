package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
	"github.com/kandev/kandev/internal/task/recoveryoperation"
)

const recoveryOperationColumns = `task_environment_id, owner_task_id, ownership_generation,
	session_id, operation_id, attempt_id, error_stamp, kind, revision, runner_instance_id,
	state, phase, repository_id, repository_position, repository_total, completed_slots,
	workspace_complete, agent_ready, selected_repository_ids_json, started_at, updated_at,
	ended_at, reason_code`

func (r *Repository) BeginTaskEnvironmentRecoveryOperation(
	ctx context.Context,
	operation models.TaskEnvironmentRecoveryOperation,
) (*models.TaskEnvironmentRecoveryOperation, error) {
	if err := validateRecoveryOperationBegin(operation); err != nil {
		return nil, err
	}
	selectedJSON, err := json.Marshal(operation.SelectedRepositoryIDs)
	if err != nil {
		return nil, fmt.Errorf("encode workspace recovery inventory: %w", err)
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockRecoveryOwnerTask(ctx, r, tx, operation.TaskEnvironmentID, operation.OwnerTaskID); err != nil {
		return nil, err
	}
	if err := validateRecoveryOperationIdentity(ctx, r, tx, operation); err != nil {
		return nil, err
	}
	if err := validateRecoveryOperationClaim(ctx, r, tx, operation); err != nil {
		return nil, err
	}

	previous, err := loadRecoveryOperation(ctx, r, tx, operation.TaskEnvironmentID, true)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if errors.Is(err, sql.ErrNoRows) {
		previous = nil
	}
	operation, err = newRecoveryOperationAttempt(operation, previous, r.nowUTC())
	if err != nil {
		return nil, err
	}
	if err := saveRecoveryOperationAttempt(ctx, r, tx, operation, previous, string(selectedJSON)); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &operation, nil
}

func validateRecoveryOperationBegin(operation models.TaskEnvironmentRecoveryOperation) error {
	if !hasRecoveryOperationIdentity(operation) {
		return errors.New("workspace recovery operation: incomplete identity or phase")
	}
	if operation.State != recoveryoperation.StateRunning {
		return errors.New("workspace recovery operation: begin state must be running")
	}
	if operation.RepositoryTotal < 0 || len(operation.SelectedRepositoryIDs) != operation.RepositoryTotal {
		return errors.New("workspace recovery operation: selected inventory does not match total")
	}
	return nil
}

func hasRecoveryOperationIdentity(operation models.TaskEnvironmentRecoveryOperation) bool {
	return operation.TaskEnvironmentID != "" && operation.OwnerTaskID != "" && operation.SessionID != "" &&
		operation.OperationID != "" && operation.Kind != "" && operation.RunnerInstanceID != "" &&
		operation.OwnershipGeneration > 0 && recoveryoperation.IsKnownPhase(operation.Phase)
}

func validateRecoveryOperationClaim(
	ctx context.Context,
	r *Repository,
	tx *sqlx.Tx,
	operation models.TaskEnvironmentRecoveryOperation,
) error {
	claim, err := recoveryclaim.GetTx(ctx, r.db, tx, operation.TaskEnvironmentID)
	if err != nil {
		return err
	}
	if claim == nil || claim.OwnerTaskID != operation.OwnerTaskID ||
		claim.OwnershipGeneration != operation.OwnershipGeneration || claim.SessionID != operation.SessionID ||
		claim.OperationID != operation.OperationID {
		return recoveryoperation.ErrIdentity
	}
	return nil
}

func newRecoveryOperationAttempt(
	operation models.TaskEnvironmentRecoveryOperation,
	previous *models.TaskEnvironmentRecoveryOperation,
	now time.Time,
) (models.TaskEnvironmentRecoveryOperation, error) {
	operation.AttemptID = uuid.NewString()
	operation.Revision = 1
	if previous != nil {
		if err := validateRecoveryOperationReplacement(operation, previous); err != nil {
			return operation, err
		}
		operation.Revision = previous.Revision + 1
	}
	operation.StartedAt = now
	operation.UpdatedAt = now
	return operation, nil
}

func validateRecoveryOperationReplacement(
	operation models.TaskEnvironmentRecoveryOperation,
	previous *models.TaskEnvironmentRecoveryOperation,
) error {
	if previous.State == recoveryoperation.StateRunning {
		return recoveryoperation.ErrInProgress
	}
	// A new owner may replace recovery history only after the environment generation advances.
	if previous.OwnershipGeneration > operation.OwnershipGeneration ||
		(previous.OwnerTaskID != operation.OwnerTaskID && previous.OwnershipGeneration == operation.OwnershipGeneration) ||
		(previous.OperationID == operation.OperationID &&
			(previous.SessionID != operation.SessionID || previous.ErrorStamp != operation.ErrorStamp || previous.Kind != operation.Kind)) {
		return recoveryoperation.ErrIdentity
	}
	return nil
}

func saveRecoveryOperationAttempt(
	ctx context.Context,
	r *Repository,
	tx *sqlx.Tx,
	operation models.TaskEnvironmentRecoveryOperation,
	previous *models.TaskEnvironmentRecoveryOperation,
	selectedJSON string,
) error {
	if previous == nil {
		return insertRecoveryOperationAttempt(ctx, r, tx, operation, selectedJSON)
	}
	return replaceRecoveryOperationAttempt(ctx, r, tx, operation, previous.Revision, selectedJSON)
}

func insertRecoveryOperationAttempt(
	ctx context.Context,
	r *Repository,
	tx *sqlx.Tx,
	operation models.TaskEnvironmentRecoveryOperation,
	selectedJSON string,
) error {
	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_environment_recovery_operations (
			task_environment_id, owner_task_id, ownership_generation, session_id, operation_id,
			attempt_id, error_stamp, kind, revision, runner_instance_id, state, phase, repository_id,
			repository_position, repository_total, completed_slots, workspace_complete, agent_ready,
			selected_repository_ids_json, started_at, updated_at, ended_at, reason_code
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), operation.TaskEnvironmentID, operation.OwnerTaskID, operation.OwnershipGeneration,
		operation.SessionID, operation.OperationID, operation.AttemptID, operation.ErrorStamp,
		operation.Kind, operation.Revision, operation.RunnerInstanceID, operation.State, operation.Phase,
		operation.RepositoryID, operation.RepositoryPosition, operation.RepositoryTotal,
		operation.CompletedSlots, operation.WorkspaceComplete, operation.AgentReady, selectedJSON,
		operation.StartedAt, operation.UpdatedAt, operation.EndedAt, operation.ReasonCode)
	return err
}

func replaceRecoveryOperationAttempt(
	ctx context.Context,
	r *Repository,
	tx *sqlx.Tx,
	operation models.TaskEnvironmentRecoveryOperation,
	previousRevision int64,
	selectedJSON string,
) error {
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_environment_recovery_operations SET owner_task_id = ?, ownership_generation = ?,
			session_id = ?, operation_id = ?, attempt_id = ?, error_stamp = ?, kind = ?, revision = ?,
		runner_instance_id = ?, state = ?, phase = ?, repository_id = ?, repository_position = ?,
		repository_total = ?, completed_slots = ?, workspace_complete = ?, agent_ready = ?,
		selected_repository_ids_json = ?, started_at = ?, updated_at = ?, ended_at = ?, reason_code = ?
		WHERE task_environment_id = ? AND revision = ?
	`), operation.OwnerTaskID, operation.OwnershipGeneration, operation.SessionID, operation.OperationID,
		operation.AttemptID, operation.ErrorStamp, operation.Kind, operation.Revision, operation.RunnerInstanceID,
		operation.State, operation.Phase, operation.RepositoryID, operation.RepositoryPosition,
		operation.RepositoryTotal, operation.CompletedSlots, operation.WorkspaceComplete, operation.AgentReady,
		selectedJSON, operation.StartedAt, operation.UpdatedAt, operation.EndedAt, operation.ReasonCode,
		operation.TaskEnvironmentID, previousRevision)
	if err != nil {
		return err
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows != 1 {
		return recoveryoperation.ErrStaleWriter
	}
	return nil
}

func (r *Repository) UpdateTaskEnvironmentRecoveryOperation(
	ctx context.Context,
	update models.TaskEnvironmentRecoveryOperationUpdate,
) (*models.TaskEnvironmentRecoveryOperation, error) {
	if err := validateRecoveryOperationUpdate(update); err != nil {
		return nil, err
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	ownerTaskID, err := recoveryOperationOwnerTaskID(ctx, r, tx, update.TaskEnvironmentID)
	if err != nil {
		return nil, err
	}
	if err := lockRecoveryOwnerTask(ctx, r, tx, update.TaskEnvironmentID, ownerTaskID); err != nil {
		return nil, err
	}
	current, err := loadRecoveryOperation(ctx, r, tx, update.TaskEnvironmentID, true)
	if err != nil {
		return nil, err
	}
	if !recoveryOperationUpdateMatches(current, ownerTaskID, update) {
		return nil, recoveryoperation.ErrStaleWriter
	}
	update, err = inheritAndValidateRecoveryProgress(update, current)
	if err != nil {
		return nil, err
	}
	if err := saveRecoveryOperationUpdate(ctx, r, tx, current, update); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return r.GetTaskEnvironmentRecoveryOperation(ctx, update.TaskEnvironmentID)
}

func validateRecoveryOperationUpdate(update models.TaskEnvironmentRecoveryOperationUpdate) error {
	if !hasRecoveryOperationUpdateFence(update) {
		return errors.New("workspace recovery operation: incomplete update fence or invalid state")
	}
	if !validRecoveryOperationReason(update.ReasonCode) {
		return errors.New("workspace recovery operation: invalid reason code")
	}
	return nil
}

func hasRecoveryOperationUpdateFence(update models.TaskEnvironmentRecoveryOperationUpdate) bool {
	return update.TaskEnvironmentID != "" && update.OperationID != "" && update.AttemptID != "" &&
		update.RunnerInstanceID != "" && update.OwnershipGeneration > 0 && update.ExpectedRevision > 0 &&
		recoveryoperation.IsKnownPhase(update.Phase) && validRecoveryOperationState(update.State)
}

func validRecoveryOperationReason(reason string) bool {
	return len(reason) <= 80 && !strings.ContainsAny(reason, "\r\n\x00")
}

func recoveryOperationOwnerTaskID(ctx context.Context, r *Repository, tx *sqlx.Tx, environmentID string) (string, error) {
	var ownerTaskID string
	err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT owner_task_id FROM task_environment_recovery_operations WHERE task_environment_id = ?
	`), environmentID).Scan(&ownerTaskID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", recoveryoperation.ErrStaleWriter
	}
	return ownerTaskID, err
}

func recoveryOperationUpdateMatches(
	current *models.TaskEnvironmentRecoveryOperation,
	ownerTaskID string,
	update models.TaskEnvironmentRecoveryOperationUpdate,
) bool {
	return current.OwnerTaskID == ownerTaskID && current.OwnershipGeneration == update.OwnershipGeneration &&
		current.OperationID == update.OperationID && current.AttemptID == update.AttemptID &&
		current.RunnerInstanceID == update.RunnerInstanceID && current.Revision == update.ExpectedRevision &&
		current.State == recoveryoperation.StateRunning
}

func inheritAndValidateRecoveryProgress(
	update models.TaskEnvironmentRecoveryOperationUpdate,
	current *models.TaskEnvironmentRecoveryOperation,
) (models.TaskEnvironmentRecoveryOperationUpdate, error) {
	if update.RepositoryTotal == 0 {
		update.RepositoryID = current.RepositoryID
		update.RepositoryPosition = current.RepositoryPosition
		update.RepositoryTotal = current.RepositoryTotal
		update.CompletedSlots = current.CompletedSlots
		update.WorkspaceComplete = current.WorkspaceComplete
		update.AgentReady = current.AgentReady
	}
	if update.CompletedSlots > update.RepositoryTotal || update.RepositoryPosition > update.RepositoryTotal ||
		update.RepositoryPosition < 0 || update.CompletedSlots < 0 {
		return update, errors.New("workspace recovery operation: progress is outside selected inventory")
	}
	return update, nil
}

func saveRecoveryOperationUpdate(
	ctx context.Context,
	r *Repository,
	tx *sqlx.Tx,
	current *models.TaskEnvironmentRecoveryOperation,
	update models.TaskEnvironmentRecoveryOperationUpdate,
) error {
	now := r.nowUTC()
	result, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_environment_recovery_operations SET revision = ?, updated_at = ?, state = ?, phase = ?,
			repository_id = ?, repository_position = ?, repository_total = ?, completed_slots = ?,
			workspace_complete = ?, agent_ready = ?, ended_at = ?, reason_code = ?
		WHERE task_environment_id = ? AND owner_task_id = ? AND ownership_generation = ?
			AND operation_id = ? AND attempt_id = ? AND runner_instance_id = ? AND revision = ? AND state = ?
	`), current.Revision+1, now, update.State, update.Phase, update.RepositoryID,
		update.RepositoryPosition, update.RepositoryTotal, update.CompletedSlots, update.WorkspaceComplete,
		update.AgentReady, update.EndedAt, update.ReasonCode, update.TaskEnvironmentID, current.OwnerTaskID,
		update.OwnershipGeneration, update.OperationID, update.AttemptID, update.RunnerInstanceID,
		update.ExpectedRevision, recoveryoperation.StateRunning)
	if err != nil {
		return err
	}
	if rows, rowsErr := result.RowsAffected(); rowsErr != nil || rows != 1 {
		return recoveryoperation.ErrStaleWriter
	}
	return nil
}

func (r *Repository) GetTaskEnvironmentRecoveryOperation(
	ctx context.Context,
	environmentID string,
) (*models.TaskEnvironmentRecoveryOperation, error) {
	if environmentID == "" {
		return nil, nil
	}
	operation, err := loadRecoveryOperation(ctx, r, nil, environmentID, false)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return operation, nil
}

func (r *Repository) InterruptTaskEnvironmentRecoveryOperations(ctx context.Context, currentRunnerID string) (int, error) {
	if currentRunnerID == "" {
		return 0, errors.New("workspace recovery operation: current runner ID is required")
	}
	rows, err := r.db.QueryxContext(ctx, r.db.Rebind(`
		SELECT task_environment_id, operation_id, attempt_id, ownership_generation,
			runner_instance_id, revision, phase
		FROM task_environment_recovery_operations WHERE state = ? AND runner_instance_id <> ?
	`), recoveryoperation.StateRunning, currentRunnerID)
	if err != nil {
		return 0, err
	}
	var stale []models.TaskEnvironmentRecoveryOperationUpdate
	for rows.Next() {
		var update models.TaskEnvironmentRecoveryOperationUpdate
		if err := rows.Scan(&update.TaskEnvironmentID, &update.OperationID, &update.AttemptID,
			&update.OwnershipGeneration, &update.RunnerInstanceID, &update.ExpectedRevision, &update.Phase); err != nil {
			_ = rows.Close()
			return 0, err
		}
		update.State = recoveryoperation.StateInterrupted
		update.ReasonCode = "backend_restarted"
		update.EndedAt = ptrTime(r.nowUTC())
		stale = append(stale, update)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	interrupted := 0
	for _, update := range stale {
		if _, err := r.UpdateTaskEnvironmentRecoveryOperation(ctx, update); err != nil {
			if errors.Is(err, recoveryoperation.ErrStaleWriter) {
				continue
			}
			return interrupted, err
		}
		interrupted++
	}
	return interrupted, nil
}

func loadRecoveryOperation(
	ctx context.Context,
	r *Repository,
	tx *sqlx.Tx,
	environmentID string,
	forUpdate bool,
) (*models.TaskEnvironmentRecoveryOperation, error) {
	query := `SELECT ` + recoveryOperationColumns + ` FROM task_environment_recovery_operations WHERE task_environment_id = ?`
	if forUpdate && dialect.IsPostgres(r.db.DriverName()) {
		query += ` FOR UPDATE`
	}
	var row *sqlx.Row
	if tx != nil {
		row = tx.QueryRowxContext(ctx, r.db.Rebind(query), environmentID)
	} else {
		row = r.db.QueryRowxContext(ctx, r.db.Rebind(query), environmentID)
	}
	var operation models.TaskEnvironmentRecoveryOperation
	var selectedJSON string
	var endedAt sql.NullTime
	if err := row.Scan(&operation.TaskEnvironmentID, &operation.OwnerTaskID, &operation.OwnershipGeneration,
		&operation.SessionID, &operation.OperationID, &operation.AttemptID, &operation.ErrorStamp,
		&operation.Kind, &operation.Revision, &operation.RunnerInstanceID, &operation.State, &operation.Phase,
		&operation.RepositoryID, &operation.RepositoryPosition, &operation.RepositoryTotal,
		&operation.CompletedSlots, &operation.WorkspaceComplete, &operation.AgentReady, &selectedJSON,
		&operation.StartedAt, &operation.UpdatedAt, &endedAt, &operation.ReasonCode); err != nil {
		return nil, err
	}
	if endedAt.Valid {
		operation.EndedAt = &endedAt.Time
	}
	if selectedJSON != "" {
		if err := json.Unmarshal([]byte(selectedJSON), &operation.SelectedRepositoryIDs); err != nil {
			return nil, fmt.Errorf("decode workspace recovery inventory: %w", err)
		}
	}
	return &operation, nil
}

func lockRecoveryOwnerTask(ctx context.Context, r *Repository, tx *sqlx.Tx, environmentID, expectedOwner string) error {
	query := `SELECT task_id FROM task_environments WHERE id = ?`
	var ownerTaskID string
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(query), environmentID).Scan(&ownerTaskID); err != nil {
		return err
	}
	if ownerTaskID != expectedOwner {
		return recoveryoperation.ErrIdentity
	}
	if dialect.IsPostgres(r.db.DriverName()) {
		query = `SELECT id FROM tasks WHERE id = ? FOR UPDATE`
		var id string
		return tx.QueryRowxContext(ctx, r.db.Rebind(query), ownerTaskID).Scan(&id)
	}
	result, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE tasks SET updated_at = updated_at WHERE id = ?`), ownerTaskID)
	if err != nil {
		return err
	}
	if count, err := result.RowsAffected(); err != nil {
		return err
	} else if count != 1 {
		return fmt.Errorf("workspace recovery operation: owner task %s not found", ownerTaskID)
	}
	return nil
}

func validateRecoveryOperationIdentity(ctx context.Context, r *Repository, tx *sqlx.Tx, operation models.TaskEnvironmentRecoveryOperation) error {
	var ownerTaskID, executorType string
	var generation int64
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT task_id, ownership_generation, executor_type FROM task_environments WHERE id = ?
	`), operation.TaskEnvironmentID).Scan(&ownerTaskID, &generation, &executorType); err != nil {
		return err
	}
	if ownerTaskID != operation.OwnerTaskID || generation != operation.OwnershipGeneration ||
		executorType != string(models.ExecutorTypeWorktree) {
		return recoveryoperation.ErrIdentity
	}
	return nil
}

func validRecoveryOperationState(state string) bool {
	return state == recoveryoperation.StateRunning || recoveryoperation.IsTerminal(state)
}

func ptrTime(value time.Time) *time.Time { return &value }
