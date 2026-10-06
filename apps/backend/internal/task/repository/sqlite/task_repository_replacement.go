package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

// ReplaceTaskRepositories commits one complete association set. The task guard
// precedes canonical reads so a waiter observes the preceding committed set.
func (r *Repository) ReplaceTaskRepositories(ctx context.Context, taskID string,
	build func(models.TaskRepositoryReplacementSnapshot) ([]*models.TaskRepository, error),
) ([]*models.TaskRepository, error) {
	var options *sql.TxOptions
	if dialect.IsPostgres(r.db.DriverName()) {
		options = &sql.TxOptions{Isolation: sql.LevelReadCommitted}
	}
	tx, err := r.db.BeginTxx(ctx, options)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	if err := r.lockTaskRepositoryReplacement(ctx, tx, taskID); err != nil {
		if cancelErr := ctx.Err(); cancelErr != nil {
			return nil, cancelErr
		}
		return nil, err
	}
	canonical, err := listTaskRepositoriesByTaskIDs(ctx, tx, []string{taskID})
	if err != nil {
		return nil, err
	}
	snapshot := models.TaskRepositoryReplacementSnapshot{Repositories: canonical[taskID]}
	err = tx.QueryRowContext(ctx, tx.Rebind(`SELECT EXISTS (SELECT 1 FROM task_environments WHERE task_id = ?)`), taskID).Scan(&snapshot.EnvironmentExists)
	if err != nil {
		return nil, err
	}
	rows, err := build(snapshot)
	if err != nil {
		return nil, err
	}
	prepared, metadata, err := prepareReplacementRows(taskID, rows)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := persistReplacementSet(ctx, tx, taskID, prepared, metadata); err != nil {
		return nil, err
	}
	committed, err := listTaskRepositoriesByTaskIDs(ctx, tx, []string{taskID})
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return committed[taskID], nil
}

func persistReplacementSet(ctx context.Context, tx *sqlx.Tx, taskID string, prepared []*models.TaskRepository, metadata []string) error {
	if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM task_repositories WHERE task_id = ?`), taskID); err != nil {
		return err
	}
	for i, row := range prepared {
		if err := insertReplacementRow(ctx, tx, row, metadata[i]); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) lockTaskRepositoryReplacement(ctx context.Context, tx *sqlx.Tx, taskID string) error {
	if dialect.IsPostgres(r.db.DriverName()) {
		return r.lockTaskRowInTx(ctx, tx, taskID)
	}
	// Acquire SQLite's writer before the first read, including across independent pools.
	result, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE tasks SET updated_at = updated_at WHERE id = ?`), taskID)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("%w: %s", ErrTaskNotFound, taskID)
	}
	return nil
}

func prepareReplacementRows(taskID string, rows []*models.TaskRepository) ([]*models.TaskRepository, []string, error) {
	prepared := make([]*models.TaskRepository, 0, len(rows))
	metadata := make([]string, 0, len(rows))
	now := time.Now().UTC()
	for _, row := range rows {
		if row == nil {
			return nil, nil, fmt.Errorf("task repository row is required")
		}
		if row.TaskID != "" && row.TaskID != taskID {
			return nil, nil, fmt.Errorf("task repository belongs to a different task")
		}
		encoded, err := json.Marshal(row.Metadata)
		if err != nil {
			return nil, nil, fmt.Errorf("encode task repository metadata: %w", err)
		}
		copy := *row
		copy.ID, copy.TaskID = uuid.NewString(), taskID
		copy.CreatedAt, copy.UpdatedAt = now, now
		prepared = append(prepared, &copy)
		metadata = append(metadata, string(encoded))
	}
	return prepared, metadata, nil
}

func insertReplacementRow(ctx context.Context, tx *sqlx.Tx, row *models.TaskRepository, metadata string) error {
	_, err := tx.ExecContext(ctx, tx.Rebind(`
 INSERT INTO task_repositories (
 id, task_id, repository_id, base_branch, checkout_branch, branch_policy_id, branch_policy_name,
 branch_policy_base_branch, branch_policy_branch_template, branch_policy_pull_request_target,
 position, metadata, created_at, updated_at
 ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`),
		row.ID, row.TaskID, row.RepositoryID, row.BaseBranch, row.CheckoutBranch,
		row.BranchPolicyID, row.BranchPolicyName, row.BranchPolicyBaseBranch,
		row.BranchPolicyBranchTemplate, row.BranchPolicyPullRequestTarget,
		row.Position, metadata, row.CreatedAt, row.UpdatedAt)
	return err
}
