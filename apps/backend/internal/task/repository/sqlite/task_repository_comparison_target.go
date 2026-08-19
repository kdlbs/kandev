package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/securityutil"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// UpdateTaskRepositoryComparisonTarget atomically persists a provider-owned
// target on the exact task-repository attachment. It never changes the
// attachment's checkout or base branch.
func (r *Repository) UpdateTaskRepositoryComparisonTarget(
	ctx context.Context,
	id string,
	target *models.ComparisonTarget,
	expected *models.ComparisonTarget,
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

	taskRepo, err := r.getTaskRepositoryForUpdate(ctx, tx, id)
	if err != nil {
		return nil, false, err
	}
	current, present, err := models.LoadComparisonTarget(taskRepo.Metadata)
	if err != nil {
		return nil, false, err
	}
	var changed bool
	if target == nil {
		changed = removeComparisonTarget(taskRepo.Metadata, current, present, expected)
	} else {
		changed, err = applyComparisonTarget(taskRepo.Metadata, current, present, target)
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
) bool {
	if !present {
		return false
	}
	if expected != nil && !current.ChangeIdentityEqual(*expected) {
		return false
	}
	delete(metadata, models.ComparisonTargetMetadataKey)
	return true
}

func applyComparisonTarget(
	metadata map[string]interface{},
	current models.ComparisonTarget,
	present bool,
	target *models.ComparisonTarget,
) (bool, error) {
	if present && current.Equal(*target) {
		return false, nil
	}
	if err := models.PutComparisonTarget(metadata, target); err != nil {
		return false, err
	}
	return true, nil
}

// UpdateTaskRepositoryBaseBranchAndClearComparisonTarget applies a user
// selected comparison branch while removing provider-owned target state in
// the same transaction. This also handles selecting the same visible branch.
func (r *Repository) UpdateTaskRepositoryBaseBranchAndClearComparisonTarget(
	ctx context.Context,
	id string,
	baseBranch string,
) (*models.TaskRepository, bool, error) {
	if !securityutil.IsValidBaseBranchRef(baseBranch) {
		return nil, false, fmt.Errorf("invalid base branch: %q", baseBranch)
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()

	taskRepo, err := r.getTaskRepositoryForUpdate(ctx, tx, id)
	if err != nil {
		return nil, false, err
	}
	_, present, err := models.LoadComparisonTarget(taskRepo.Metadata)
	if err != nil {
		return nil, false, err
	}
	if taskRepo.BaseBranch == baseBranch && !present {
		return taskRepo, false, nil
	}
	delete(taskRepo.Metadata, models.ComparisonTargetMetadataKey)
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

func (r *Repository) getTaskRepositoryForUpdate(ctx context.Context, tx *sqlx.Tx, id string) (*models.TaskRepository, error) {
	query := `
		SELECT id, task_id, repository_id, base_branch, checkout_branch, position, metadata, created_at, updated_at
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
