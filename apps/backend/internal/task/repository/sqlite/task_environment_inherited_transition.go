package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/recoveryclaim"
)

// ClaimInheritedTaskEnvironmentTransition elects or attaches the task-owned
// environment for a child session that cannot reuse its inherited executor.
// The inherited row is only read; it is never re-elected or modified.
//
// The returned environment is non-nil when this operation handled the
// inherited binding. sessionRebound reports that the session previously
// pointed at another environment, so the caller can retire only that
// session's runtime before taking the ordinary workspace launch path.
func (r *Repository) ClaimInheritedTaskEnvironmentTransition(
	ctx context.Context,
	taskID, sessionID, inheritedEnvironmentID, executorType, executorID, executorProfileID string,
) (*models.TaskEnvironment, bool, bool, error) {
	if taskID == "" || sessionID == "" || executorType == "" {
		return nil, false, false, fmt.Errorf("inherited workspace transition requires task, session, and executor identities")
	}

	inheritedOwnerID, err := r.discoverInheritedEnvironmentOwner(ctx, inheritedEnvironmentID)
	if err != nil {
		return nil, false, false, err
	}

	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, false, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockInheritedTransitionTasksTx(ctx, tx, taskID, inheritedOwnerID); err != nil {
		return nil, false, false, err
	}
	if err := r.taskCleanupBarrierLocked(ctx, tx, taskID); err != nil {
		return nil, false, false, err
	}
	if err := recoveryclaim.EnsureTaskAvailableTx(ctx, r.db, tx, taskID); err != nil {
		return nil, false, false, err
	}

	currentEnvironmentID, err := r.loadInheritedTransitionSessionTx(ctx, tx, taskID, sessionID)
	if err != nil {
		return nil, false, false, err
	}

	environmentID, handled, err := r.claimInheritedTransitionEnvironmentTx(
		ctx, tx, taskID, sessionID, currentEnvironmentID, inheritedEnvironmentID,
		inheritedOwnerID, executorType, executorID, executorProfileID,
	)
	if err != nil {
		return nil, false, false, err
	}
	if !handled {
		return nil, false, false, nil
	}

	if err := tx.Commit(); err != nil {
		return nil, false, false, fmt.Errorf("commit inherited environment transition: %w", err)
	}
	environment, err := r.GetTaskEnvironment(ctx, environmentID)
	if err != nil {
		return nil, false, false, fmt.Errorf("load claimed child environment: %w", err)
	}
	return environment, true, currentEnvironmentID != environmentID, nil
}

func (r *Repository) discoverInheritedEnvironmentOwner(ctx context.Context, environmentID string) (string, error) {
	if environmentID == "" {
		return "", nil
	}
	var ownerID string
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`SELECT task_id FROM task_environments WHERE id = ?`), environmentID).Scan(&ownerID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("discover inherited environment owner: %w", err)
	}
	return ownerID, nil
}

func (r *Repository) lockInheritedTransitionTasksTx(ctx context.Context, tx *sqlx.Tx, taskID, ownerID string) error {
	taskIDs := []string{taskID, ownerID}
	sort.Strings(taskIDs)
	for index, id := range taskIDs {
		if id == "" || (index > 0 && id == taskIDs[index-1]) {
			continue
		}
		if err := r.lockTaskRowInTx(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) loadInheritedTransitionSessionTx(ctx context.Context, tx *sqlx.Tx, taskID, sessionID string) (string, error) {
	var currentTaskID, environmentID string
	err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT task_id, COALESCE(task_environment_id, '') FROM task_sessions WHERE id = ?
	`), sessionID).Scan(&currentTaskID, &environmentID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("%w: %s", models.ErrTaskSessionNotFound, sessionID)
	}
	if err != nil {
		return "", fmt.Errorf("load inherited transition session: %w", err)
	}
	if currentTaskID != taskID {
		return "", fmt.Errorf("inherited transition session does not belong to task")
	}
	return environmentID, nil
}

func (r *Repository) claimInheritedTransitionEnvironmentTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID, sessionID, currentEnvironmentID, inheritedEnvironmentID,
	inheritedOwnerID, executorType, executorID, executorProfileID string,
) (string, bool, error) {
	canonical, err := r.loadInheritedTransitionCanonicalTx(ctx, tx, taskID)
	if err != nil {
		return "", false, err
	}
	if canonical != nil {
		if err := r.claimExistingInheritedTransitionEnvironmentTx(
			ctx, tx, taskID, sessionID, currentEnvironmentID, executorType, canonical,
		); err != nil {
			return "", false, err
		}
		return canonical.ID, true, nil
	}
	return r.claimNewInheritedTransitionEnvironmentTx(
		ctx, tx, taskID, sessionID, currentEnvironmentID, inheritedEnvironmentID,
		inheritedOwnerID, executorType, executorID, executorProfileID,
	)
}

func (r *Repository) loadInheritedTransitionCanonicalTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID string,
) (*inheritedTransitionCanonical, error) {
	canonical := &inheritedTransitionCanonical{}
	err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT id, task_id, executor_type, status, COALESCE(materialization_session_id, ''), updated_at
		FROM task_environments WHERE task_id = ?
	`), taskID).Scan(&canonical.ID, &canonical.TaskID, &canonical.ExecutorType, &canonical.Status,
		&canonical.MaterializerID, &canonical.ClaimUpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load canonical child environment: %w", err)
	}
	if err := r.lockTaskEnvironmentMaterializationTx(ctx, tx, canonical.ID); err != nil {
		return nil, err
	}
	err = tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT id, task_id, executor_type, status, COALESCE(materialization_session_id, ''), updated_at
		FROM task_environments WHERE task_id = ?
	`), taskID).Scan(&canonical.ID, &canonical.TaskID, &canonical.ExecutorType, &canonical.Status,
		&canonical.MaterializerID, &canonical.ClaimUpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("reload canonical child environment: %w", err)
	}
	return canonical, nil
}

type inheritedTransitionCanonical struct {
	ID             string
	TaskID         string
	ExecutorType   string
	Status         string
	MaterializerID string
	ClaimUpdatedAt sql.NullTime
}

func (r *Repository) claimExistingInheritedTransitionEnvironmentTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID, sessionID, currentEnvironmentID, executorType string,
	canonical *inheritedTransitionCanonical,
) error {
	if canonical.ExecutorType != "" && canonical.ExecutorType != executorType {
		if err := r.bindRefusedInheritedTransitionSessionTx(ctx, tx, taskID, sessionID, currentEnvironmentID, canonical); err != nil {
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("persist refused session's canonical child environment binding: %w", err)
		}
		return fmt.Errorf("%w: canonical task environment uses executor %q; selected executor is %q",
			models.ErrWorkspaceExecutorMismatch, canonical.ExecutorType, executorType)
	}
	return r.bindInheritedTransitionToExistingEnvironmentTx(
		ctx, tx, taskID, sessionID, canonical.ID, currentEnvironmentID,
		canonical.Status, canonical.MaterializerID, canonical.ClaimUpdatedAt,
	)
}

func (r *Repository) bindRefusedInheritedTransitionSessionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID, sessionID, currentEnvironmentID string,
	canonical *inheritedTransitionCanonical,
) error {
	workspacePath := ""
	status := models.TaskEnvironmentStatus(canonical.Status)
	if status == models.TaskEnvironmentStatusReady || status == models.TaskEnvironmentStatusStopped {
		if err := tx.QueryRowContext(ctx, r.db.Rebind(`
			SELECT COALESCE(workspace_path, '') FROM task_environments WHERE id = ?
		`), canonical.ID).Scan(&workspacePath); err != nil {
			return fmt.Errorf("load canonical child workspace path: %w", err)
		}
	}
	return r.updateInheritedTransitionSessionTx(
		ctx, tx, taskID, sessionID, currentEnvironmentID, canonical.ID, workspacePath,
	)
}

func (r *Repository) claimNewInheritedTransitionEnvironmentTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID, sessionID, currentEnvironmentID, inheritedEnvironmentID,
	inheritedOwnerID, executorType, executorID, executorProfileID string,
) (string, bool, error) {
	if currentEnvironmentID == "" || currentEnvironmentID != inheritedEnvironmentID || inheritedOwnerID == "" {
		return "", false, nil
	}
	if err := r.lockTaskEnvironmentMaterializationTx(ctx, tx, inheritedEnvironmentID); err != nil {
		return "", false, err
	}
	source, err := r.loadInheritedTransitionSourceTx(ctx, tx, inheritedEnvironmentID)
	if err != nil || source == nil {
		return "", false, err
	}
	if source.TaskID != inheritedOwnerID {
		return "", false, fmt.Errorf("%w: inherited environment ownership changed during transition admission", models.ErrWorkspaceReuseUnsafe)
	}
	if source.TaskID == taskID || source.ExecutorType == "" || source.ExecutorType == executorType ||
		executorType == string(models.ExecutorTypeKubernetes) {
		return "", false, nil
	}
	if source.ExecutorType == string(models.ExecutorTypeKubernetes) {
		return "", false, fmt.Errorf("%w: Kubernetes inherited environments cannot transition across task ownership", models.ErrWorkspaceReuseUnsafe)
	}
	if source.ArchivedAt.Valid {
		return "", false, fmt.Errorf("%w: inherited environment owner %s is archived", models.ErrWorkspaceReuseUnsafe, source.TaskID)
	}
	return r.createInheritedTransitionEnvironmentTx(
		ctx, tx, taskID, sessionID, currentEnvironmentID, executorType, executorID, executorProfileID,
	)
}

type inheritedTransitionSource struct {
	TaskID       string
	ExecutorType string
	ArchivedAt   sql.NullTime
}

func (r *Repository) loadInheritedTransitionSourceTx(
	ctx context.Context,
	tx *sqlx.Tx,
	environmentID string,
) (*inheritedTransitionSource, error) {
	source := &inheritedTransitionSource{}
	err := tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT e.task_id, e.executor_type, t.archived_at
		FROM task_environments e JOIN tasks t ON t.id = e.task_id WHERE e.id = ?
	`), environmentID).Scan(&source.TaskID, &source.ExecutorType, &source.ArchivedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load inherited environment owner: %w", err)
	}
	return source, nil
}

func (r *Repository) createInheritedTransitionEnvironmentTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID, sessionID, currentEnvironmentID, executorType, executorID, executorProfileID string,
) (string, bool, error) {
	now := r.nowUTC()
	candidate := &models.TaskEnvironment{
		ID: uuid.NewString(), TaskID: taskID, ExecutorType: executorType,
		ExecutorID: executorID, ExecutorProfileID: executorProfileID,
		Status:                   models.TaskEnvironmentStatusCreating,
		MaterializationSessionID: sessionID,
		CreatedAt:                now, UpdatedAt: now,
	}
	if err := r.insertCreatingWorkspaceEnvironment(ctx, tx, candidate); err != nil {
		return "", false, err
	}
	if err := r.updateInheritedTransitionSessionTx(
		ctx, tx, taskID, sessionID, currentEnvironmentID, candidate.ID, "",
	); err != nil {
		return "", false, err
	}
	return candidate.ID, true, nil
}

func (r *Repository) bindInheritedTransitionToExistingEnvironmentTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID, sessionID, environmentID, currentEnvironmentID,
	status, materializerID string,
	claimUpdatedAt sql.NullTime,
) error {
	if err := recoveryclaim.EnsureAvailableTx(ctx, r.db, tx, environmentID); err != nil {
		return err
	}
	switch models.TaskEnvironmentStatus(status) {
	case models.TaskEnvironmentStatusCreating:
		if materializerID != sessionID {
			outcome, err := r.settleWorkspaceMaterializationClaim(ctx, tx, environmentID, materializerID, claimUpdatedAt.Time, sessionID)
			if err != nil {
				return err
			}
			if outcome == materializationClaimFailed {
				if err := tx.Commit(); err != nil {
					return fmt.Errorf("persist abandoned child materialization failure: %w", err)
				}
				return fmt.Errorf("%w: abandoned child workspace materialization had physical resources", models.ErrWorkspaceReuseUnsafe)
			}
		}
		if err := r.updateInheritedTransitionSessionTx(ctx, tx, taskID, sessionID, currentEnvironmentID, environmentID, ""); err != nil {
			return err
		}
	case models.TaskEnvironmentStatusFailed:
		if err := r.reelectFailedWorkspaceMaterializationTx(ctx, tx, environmentID, sessionID); err != nil {
			return err
		}
		if err := r.updateInheritedTransitionSessionTx(ctx, tx, taskID, sessionID, currentEnvironmentID, environmentID, ""); err != nil {
			return err
		}
	case models.TaskEnvironmentStatusReady, models.TaskEnvironmentStatusStopped:
		var workspacePath string
		if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT COALESCE(workspace_path, '') FROM task_environments WHERE id = ?`), environmentID).Scan(&workspacePath); err != nil {
			return fmt.Errorf("load canonical child workspace path: %w", err)
		}
		if err := r.updateInheritedTransitionSessionTx(ctx, tx, taskID, sessionID, currentEnvironmentID, environmentID, workspacePath); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%w: canonical child environment is not attachable", models.ErrWorkspaceReuseUnsafe)
	}
	return nil
}

func (r *Repository) updateInheritedTransitionSessionTx(
	ctx context.Context,
	tx *sqlx.Tx,
	taskID, sessionID, expectedEnvironmentID, environmentID, workspacePath string,
) error {
	if expectedEnvironmentID == environmentID {
		var currentWorkspacePath string
		if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT COALESCE(workspace_path, '') FROM task_sessions WHERE id = ?`), sessionID).Scan(&currentWorkspacePath); err != nil {
			return fmt.Errorf("load child session workspace path: %w", err)
		}
		if currentWorkspacePath == workspacePath {
			return nil
		}
	}
	res, err := tx.ExecContext(ctx, r.db.Rebind(`
		UPDATE task_sessions SET task_environment_id = ?, workspace_path = ?, updated_at = ?
		WHERE id = ? AND task_id = ? AND COALESCE(task_environment_id, '') = ?
	`), environmentID, workspacePath, r.nowUTC(), sessionID, taskID, expectedEnvironmentID)
	if err != nil {
		return fmt.Errorf("bind child session to canonical environment: %w", err)
	}
	changed, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect child session binding update: %w", err)
	}
	if changed != 1 {
		return fmt.Errorf("%w: child session binding changed during executor transition", models.ErrWorkspaceReuseUnsafe)
	}
	return nil
}
