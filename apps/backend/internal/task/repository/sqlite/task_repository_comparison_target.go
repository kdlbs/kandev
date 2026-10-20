package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/securityutil"
	kandevdb "github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// UpdateTaskRepositoryComparisonTarget atomically persists a provider-owned
// target on the exact task-repository attachment. It never changes the
// attachment's checkout or base branch. It mutates the link in place and
// bumps task_repositories.updated_at, so it takes the owning task's row
// lock: a concurrent runner switch's compatibility re-check must resolve
// fully before or fully after this write.
func (r *Repository) UpdateTaskRepositoryComparisonTarget(
	ctx context.Context,
	id string,
	target *models.ComparisonTarget,
	expected *models.ComparisonTarget,
	clearManualOverride bool,
) (*models.TaskRepository, bool, error) {
	if target != nil {
		if err := target.Validate(); err != nil {
			return nil, false, err
		}
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()

	taskRepo, err := r.lockTaskThenGetTaskRepository(ctx, tx, id)
	if err != nil {
		return nil, false, err
	}
	if taskRepo.Metadata == nil {
		taskRepo.Metadata = make(map[string]interface{})
	}
	current, present, err := models.LoadComparisonTarget(taskRepo.Metadata)
	if err != nil {
		return nil, false, err
	}
	var changed bool
	if target == nil {
		changed = removeComparisonTarget(taskRepo.Metadata, current, present, expected, clearManualOverride)
	} else {
		changed, err = applyComparisonTarget(taskRepo.Metadata, current, present, target, clearManualOverride)
	}
	if err != nil {
		return nil, false, err
	}
	if !changed {
		return taskRepo, false, nil
	}

	taskRepo.UpdatedAt = r.nowUTC()
	if err := updateTaskRepositoryMetadata(ctx, tx, taskRepo); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return taskRepo, true, nil
}

func removeComparisonTarget(
	metadata map[string]interface{},
	current models.ComparisonTarget,
	present bool,
	expected *models.ComparisonTarget,
	clearManualOverride bool,
) bool {
	if expected != nil && (!present || !current.ChangeIdentityEqual(*expected)) {
		return false
	}
	changed := present
	if present {
		delete(metadata, models.ComparisonTargetMetadataKey)
	}
	if clearManualOverride && models.HasManualBaseBranchOverride(metadata) {
		delete(metadata, models.ManualBaseBranchOverrideMetadataKey)
		changed = true
	}
	return changed
}

func applyComparisonTarget(
	metadata map[string]interface{},
	current models.ComparisonTarget,
	present bool,
	target *models.ComparisonTarget,
	clearManualOverride bool,
) (bool, error) {
	if present && current.Equal(*target) {
		if clearManualOverride && models.HasManualBaseBranchOverride(metadata) {
			delete(metadata, models.ManualBaseBranchOverrideMetadataKey)
			return true, nil
		}
		return false, nil
	}
	if err := models.PutComparisonTarget(metadata, target); err != nil {
		return false, err
	}
	if clearManualOverride {
		delete(metadata, models.ManualBaseBranchOverrideMetadataKey)
	}
	return true, nil
}

// UpdateTaskRepositoryBaseBranchAndClearComparisonTarget updates a task base
// branch while removing provider-owned target state in the same transaction.
// Manual selections are marked so provider refresh cannot replace them. It
// mutates the link in place and bumps
// task_repositories.updated_at, so it takes the owning task's row lock,
// same reason as UpdateTaskRepositoryComparisonTarget above.
func (r *Repository) UpdateTaskRepositoryBaseBranchAndClearComparisonTarget(
	ctx context.Context,
	id string,
	baseBranch string,
	manualSelection bool,
) (*models.TaskRepository, bool, error) {
	if !securityutil.IsValidBaseBranchRef(baseBranch) {
		return nil, false, fmt.Errorf("invalid base branch: %q", baseBranch)
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()

	taskRepo, err := r.lockTaskThenGetTaskRepository(ctx, tx, id)
	if err != nil {
		return nil, false, err
	}
	if taskRepo.Metadata == nil {
		taskRepo.Metadata = make(map[string]interface{})
	}
	_, present, err := models.LoadComparisonTarget(taskRepo.Metadata)
	if err != nil {
		return nil, false, err
	}
	manualOverride := models.HasManualBaseBranchOverride(taskRepo.Metadata)
	if manualOverride && !manualSelection {
		return taskRepo, false, nil
	}
	if taskRepo.BaseBranch == baseBranch && !present && (!manualSelection || manualOverride) {
		return taskRepo, false, nil
	}
	delete(taskRepo.Metadata, models.ComparisonTargetMetadataKey)
	if manualSelection {
		taskRepo.Metadata[models.ManualBaseBranchOverrideMetadataKey] = true
	}
	taskRepo.BaseBranch = baseBranch
	taskRepo.UpdatedAt = r.nowUTC()
	if err := updateTaskRepositoryMetadata(ctx, tx, taskRepo); err != nil {
		return nil, false, err
	}
	if err := tx.Commit(); err != nil {
		return nil, false, err
	}
	return taskRepo, true, nil
}

// lockTaskThenGetTaskRepository keeps the task-scoped lock order consistent
// with UpdateTaskRepository: task row first, task-repository link second.
// The initial task_id read is unlocked only to identify which task row to
// lock. The link is re-read under FOR UPDATE afterwards and the owner is
// checked again so a concurrent re-parent cannot update through a stale owner.
func (r *Repository) lockTaskThenGetTaskRepository(ctx context.Context, tx *sqlx.Tx, id string) (*models.TaskRepository, error) {
	var taskID string
	err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT task_id FROM task_repositories WHERE id = ?`), id).Scan(&taskID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("task repository not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	if lockErr := kandevdb.LockTaskRowInTx(ctx, tx, r.db.DriverName(), taskID); lockErr != nil &&
		!errors.Is(lockErr, kandevdb.ErrTaskRowNotFound) {
		return nil, lockErr
	}
	taskRepo, err := r.getTaskRepositoryForUpdate(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if taskRepo.TaskID != taskID {
		return nil, fmt.Errorf("task repository %s moved from task %s to task %s during update", id, taskID, taskRepo.TaskID)
	}
	return taskRepo, nil
}

func (r *Repository) getTaskRepositoryForUpdate(ctx context.Context, tx *sqlx.Tx, id string) (*models.TaskRepository, error) {
	query := `
		SELECT id, task_id, repository_id, workspace_relative_path, base_branch, checkout_branch, position, metadata, created_at, updated_at
		FROM task_repositories WHERE id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += " FOR UPDATE"
	}
	taskRepo := &models.TaskRepository{}
	var metadataJSON string
	err := tx.QueryRowxContext(ctx, r.db.Rebind(query), id).Scan(
		&taskRepo.ID,
		&taskRepo.TaskID,
		&taskRepo.RepositoryID,
		&taskRepo.WorkspaceRelativePath,
		&taskRepo.BaseBranch,
		&taskRepo.CheckoutBranch,
		&taskRepo.Position,
		&metadataJSON,
		&taskRepo.CreatedAt,
		&taskRepo.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("task repository not found: %s", id)
	}
	if err != nil {
		return nil, err
	}
	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &taskRepo.Metadata); err != nil {
			return nil, fmt.Errorf("failed to deserialize task repository metadata: %w", err)
		}
	}
	if taskRepo.Metadata == nil {
		taskRepo.Metadata = make(map[string]interface{})
	}
	return taskRepo, nil
}

func updateTaskRepositoryMetadata(ctx context.Context, tx *sqlx.Tx, taskRepo *models.TaskRepository) error {
	metadataJSON, err := json.Marshal(taskRepo.Metadata)
	if err != nil {
		return fmt.Errorf("failed to serialize task repository metadata: %w", err)
	}
	result, err := tx.ExecContext(ctx, tx.Rebind(`
		UPDATE task_repositories SET base_branch = ?, metadata = ?, updated_at = ?
		WHERE id = ?
	`), taskRepo.BaseBranch, string(metadataJSON), taskRepo.UpdatedAt, taskRepo.ID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("task repository not found: %s", taskRepo.ID)
	}
	return nil
}
