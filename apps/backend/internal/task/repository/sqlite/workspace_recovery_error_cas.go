package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// CommitWorkspaceRecoveryErrorIfCurrent stores the recovery error while the
// captured session, execution, environment, owner, generation, and error stamp
// remain current. A repeated refusal reuses an existing active relocation
// stamp without changing session metadata.
func (r *Repository) CommitWorkspaceRecoveryErrorIfCurrent(
	ctx context.Context,
	observation models.WorkspaceRecoveryErrorObservation,
	errorValue models.LastAgentError,
) (bool, string, error) {
	if !workspaceRecoveryObservationReady(observation) {
		return false, "", nil
	}
	payload, err := json.Marshal(errorValue)
	if err != nil {
		return false, "", fmt.Errorf("serialize workspace recovery error: %w", err)
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return false, "", err
	}
	defer func() { _ = tx.Rollback() }()

	current, err := r.workspaceRecoveryObservationCurrent(ctx, tx, observation)
	if err != nil {
		return false, "", err
	}
	if !current {
		return false, "", nil
	}
	return r.commitWorkspaceRecoveryErrorMetadata(ctx, tx, observation, errorValue, string(payload))
}

func workspaceRecoveryObservationReady(observation models.WorkspaceRecoveryErrorObservation) bool {
	return observation.TaskID != "" && observation.SessionID != "" &&
		observation.TaskEnvironmentID != "" && observation.EnvironmentOwnerTaskID != "" &&
		observation.OwnershipGeneration > 0 && observation.SessionState != "" &&
		workspaceRecoverySnapshotMatchesObservation(observation.SelectionSnapshot, observation)
}

func (r *Repository) workspaceRecoveryObservationCurrent(
	ctx context.Context,
	tx *sqlx.Tx,
	observation models.WorkspaceRecoveryErrorObservation,
) (bool, error) {
	if err := db.LockTaskRowInTx(ctx, tx, r.db.DriverName(), observation.TaskID); err != nil {
		if errors.Is(err, db.ErrTaskRowNotFound) {
			return false, nil
		}
		return false, err
	}
	found, err := lockTaskSessionRow(ctx, tx, observation.SessionID)
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}
	current, err := r.workspaceRecoverySessionMatches(ctx, tx, observation)
	if err != nil || !current {
		return current, err
	}
	current, err = r.workspaceRecoverySelectionMatches(ctx, tx, observation)
	if err != nil || !current {
		return current, err
	}
	return r.workspaceRecoveryExecutionMatches(ctx, tx, observation)
}

func (r *Repository) workspaceRecoverySessionMatches(
	ctx context.Context,
	tx *sqlx.Tx,
	observation models.WorkspaceRecoveryErrorObservation,
) (bool, error) {
	var taskID, environmentID string
	var state models.TaskSessionState
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT task_id, COALESCE(task_environment_id, ''), state
		FROM task_sessions WHERE id = ?
	`), observation.SessionID).Scan(&taskID, &environmentID, &state); err != nil {
		return false, err
	}
	return taskID == observation.TaskID && environmentID == observation.TaskEnvironmentID && state == observation.SessionState, nil
}

func (r *Repository) workspaceRecoverySelectionMatches(
	ctx context.Context,
	tx *sqlx.Tx,
	observation models.WorkspaceRecoveryErrorObservation,
) (bool, error) {
	var ownerTaskID, executorType, executorID, executorProfileID, environmentStatus, taskDirName, workspacePath string
	var generation int64
	environmentQuery := `
		SELECT task_id, ownership_generation, COALESCE(executor_type, ''),
			COALESCE(executor_id, ''), COALESCE(executor_profile_id, ''),
			COALESCE(status, ''), COALESCE(task_dir_name, ''), COALESCE(workspace_path, '')
		FROM task_environments WHERE id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		environmentQuery += ` FOR UPDATE`
	}
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(environmentQuery), observation.TaskEnvironmentID).Scan(
		&ownerTaskID, &generation, &executorType, &executorID, &executorProfileID,
		&environmentStatus, &taskDirName, &workspacePath,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if ownerTaskID != observation.EnvironmentOwnerTaskID || generation != observation.OwnershipGeneration {
		return false, nil
	}
	currentSelection := models.WorkspaceRecoverySelectionSnapshot{
		TaskID: observation.TaskID, SessionID: observation.SessionID, SessionPersisted: true,
		SessionTaskEnvironmentID: observation.TaskEnvironmentID, TaskEnvironmentID: observation.TaskEnvironmentID,
		EnvironmentOwnerTaskID: ownerTaskID, OwnershipGeneration: generation,
		ExecutorType: executorType, ExecutorID: executorID, ExecutorProfileID: executorProfileID,
		EnvironmentStatus: environmentStatus, TaskDirName: taskDirName, WorkspacePath: workspacePath,
	}
	slots, err := r.readWorkspaceRecoveryInventoryForUpdate(ctx, tx, observation.TaskEnvironmentID)
	if err != nil {
		return false, err
	}
	currentSelection.Slots = slots
	return observation.SelectionSnapshot.Equal(currentSelection), nil
}

func (r *Repository) workspaceRecoveryExecutionMatches(
	ctx context.Context,
	tx *sqlx.Tx,
	observation models.WorkspaceRecoveryErrorObservation,
) (bool, error) {
	var executionID, executionTaskID string
	executionErr := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT agent_execution_id, task_id FROM executors_running WHERE session_id = ?
	`), observation.SessionID).Scan(&executionID, &executionTaskID)
	if executionErr != nil && !errors.Is(executionErr, sql.ErrNoRows) {
		return false, executionErr
	}
	if errors.Is(executionErr, sql.ErrNoRows) {
		executionID = ""
	} else if executionTaskID != observation.TaskID {
		return false, nil
	}
	return executionID == observation.AgentExecutionID, nil
}

func (r *Repository) commitWorkspaceRecoveryErrorMetadata(
	ctx context.Context,
	tx *sqlx.Tx,
	observation models.WorkspaceRecoveryErrorObservation,
	errorValue models.LastAgentError,
	payload string,
) (bool, string, error) {
	metadata, err := r.lockMetadataRow(ctx, tx, "task_sessions", "agent session", observation.SessionID)
	if err != nil {
		return false, "", err
	}
	currentStamp, err := metadataRecordStamp(metadata, models.SessionMetaKeyLastAgentError)
	if err != nil {
		return false, "", err
	}
	if current, ok := sessionMetadataLastAgentError(metadata); ok &&
		!current.IsDismissed() && current.Code == models.LaunchErrorCategoryManagedCloneRelocationRequired &&
		len(current.RecoveryActions) == 1 && current.RecoveryActions[0] == models.RecoveryActionRelocateAndResume {
		return commitUnchangedWorkspaceRecoveryError(tx, current.Stamp())
	}
	if currentStamp != observation.ExpectedErrorStamp {
		return commitUnchangedWorkspaceRecoveryError(tx, "")
	}

	result, err := tx.ExecContext(ctx, r.db.Rebind(metadataKeyUpdateQuery("task_sessions", r.db.DriverName())),
		metadataKeyUpdateArgs(r.db.DriverName(), models.SessionMetaKeyLastAgentError, payload, r.nowUTC(), observation.SessionID)...)
	if err != nil {
		return false, "", err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, "", err
	}
	if rows == 0 {
		return false, "", nil
	}
	if err := tx.Commit(); err != nil {
		return false, "", err
	}
	return true, errorValue.Stamp(), nil
}

func commitUnchangedWorkspaceRecoveryError(tx *sqlx.Tx, stamp string) (bool, string, error) {
	if err := tx.Commit(); err != nil {
		return false, "", err
	}
	return false, stamp, nil
}

func sessionMetadataLastAgentError(metadataJSON string) (models.LastAgentError, bool) {
	if strings.TrimSpace(metadataJSON) == "" || strings.TrimSpace(metadataJSON) == jsonNull {
		return models.LastAgentError{}, false
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(metadataJSON), &metadata); err != nil {
		return models.LastAgentError{}, false
	}
	return models.LoadLastAgentError(metadata)
}
