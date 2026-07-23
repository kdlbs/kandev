package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
)

// ListTaskWorkspaceFoldersByTaskIDs loads folders for a task batch in one query.
func (r *Repository) ListTaskWorkspaceFoldersByTaskIDs(ctx context.Context, taskIDs []string) (map[string][]*models.TaskWorkspaceFolder, error) {
	result := make(map[string][]*models.TaskWorkspaceFolder, len(taskIDs))
	if len(taskIDs) == 0 {
		return result, nil
	}
	placeholders := make([]string, len(taskIDs))
	args := make([]interface{}, len(taskIDs))
	for i, id := range taskIDs {
		placeholders[i], args[i] = "?", id
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(fmt.Sprintf(`
		SELECT id, task_id, local_path, display_name, position, created_at, updated_at
		FROM task_workspace_folders WHERE task_id IN (%s)
		ORDER BY position ASC, created_at ASC
	`, strings.Join(placeholders, ","))), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		folder, scanErr := scanTaskWorkspaceFolder(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result[folder.TaskID] = append(result[folder.TaskID], folder)
	}
	return result, rows.Err()
}

// ListTaskWorkspaceFolders returns task-owned folders in durable source order.
func (r *Repository) ListTaskWorkspaceFolders(ctx context.Context, taskID string) ([]*models.TaskWorkspaceFolder, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT id, task_id, local_path, display_name, position, created_at, updated_at
		FROM task_workspace_folders
		WHERE task_id = ?
		ORDER BY position ASC, created_at ASC
	`), taskID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	folders := make([]*models.TaskWorkspaceFolder, 0)
	for rows.Next() {
		folder, err := scanTaskWorkspaceFolder(rows)
		if err != nil {
			return nil, err
		}
		folders = append(folders, folder)
	}
	return folders, rows.Err()
}

// CreateWorkspaceSourceBatch atomically writes repository and folder links.
// It owns a single position sequence across both relations so later callers
// can project an unambiguous source order.
func (r *Repository) CreateWorkspaceSourceBatch(ctx context.Context, batch *models.WorkspaceSourceBatch) error {
	if batch == nil || batch.TaskID == "" {
		return fmt.Errorf("workspace source batch requires task_id")
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	if err := r.createWorkspaceSourceBatchTx(ctx, tx, batch); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return fmt.Errorf("rollback workspace source batch: %w", rollbackErr)
		}
		return err
	}
	return tx.Commit()
}

func (r *Repository) createWorkspaceSourceBatchTx(ctx context.Context, tx *sqlx.Tx, batch *models.WorkspaceSourceBatch) error {
	if err := r.updateWorkspaceSourceBranchesTx(ctx, tx, batch.RepositoryUpdates); err != nil {
		return err
	}
	nextPosition, err := r.nextWorkspaceSourcePositionTx(ctx, tx, batch.TaskID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, source := range batch.Sources {
		nextPosition, err = r.persistWorkspaceSourceTx(ctx, tx, batch.TaskID, source, nextPosition, now)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) updateWorkspaceSourceBranchesTx(ctx context.Context, tx *sqlx.Tx, updates []models.WorkspaceSourceRepositoryUpdate) error {
	for _, update := range updates {
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE task_repositories SET base_branch = ?, updated_at = ? WHERE id = ?`), update.BaseBranch, time.Now().UTC(), update.TaskRepositoryID); err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) persistWorkspaceSourceTx(ctx context.Context, tx *sqlx.Tx, taskID string, source models.WorkspaceSource, position int, now time.Time) (int, error) {
	if source.Folder != nil && source.Repository != nil {
		return position, fmt.Errorf("workspace source must contain exactly one kind")
	}
	if source.Folder == nil && source.Repository == nil {
		return position, fmt.Errorf("workspace source kind is required")
	}
	if folder := source.Folder; folder != nil {
		if err := r.insertWorkspaceFolderTx(ctx, tx, taskID, folder, position, now); err != nil {
			return position, err
		}
		return position + 1, nil
	}
	if err := r.insertWorkspaceRepositoryTx(ctx, tx, taskID, source.Repository, position, now); err != nil {
		return position, err
	}
	return position + 1, nil
}

func (r *Repository) insertWorkspaceFolderTx(ctx context.Context, tx *sqlx.Tx, taskID string, folder *models.TaskWorkspaceFolder, position int, now time.Time) error {
	if folder.ID == "" {
		folder.ID = uuid.New().String()
	}
	folder.TaskID, folder.Position = taskID, position
	folder.CreatedAt, folder.UpdatedAt = now, now
	_, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_workspace_folders
			(id, task_id, local_path, display_name, position, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`), folder.ID, folder.TaskID, folder.LocalPath, folder.DisplayName, folder.Position, folder.CreatedAt, folder.UpdatedAt)
	return err
}

func (r *Repository) insertWorkspaceRepositoryTx(ctx context.Context, tx *sqlx.Tx, taskID string, taskRepo *models.TaskRepository, position int, now time.Time) error {
	if taskRepo.ID == "" {
		taskRepo.ID = uuid.New().String()
	}
	taskRepo.TaskID, taskRepo.Position = taskID, position
	taskRepo.CreatedAt, taskRepo.UpdatedAt = now, now
	metadata, err := json.Marshal(taskRepo.Metadata)
	if err != nil {
		metadata = []byte("{}")
	}
	_, err = tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_repositories
			(id, task_id, repository_id, base_branch, checkout_branch, position, metadata, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), taskRepo.ID, taskRepo.TaskID, taskRepo.RepositoryID, taskRepo.BaseBranch, taskRepo.CheckoutBranch, taskRepo.Position, string(metadata), taskRepo.CreatedAt, taskRepo.UpdatedAt)
	return err
}

func (r *Repository) nextWorkspaceSourcePositionTx(ctx context.Context, tx *sqlx.Tx, taskID string) (int, error) {
	var maxPosition sql.NullInt64
	if err := tx.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT MAX(position) FROM (
			SELECT position FROM task_repositories WHERE task_id = ?
			UNION ALL
			SELECT position FROM task_workspace_folders WHERE task_id = ?
		)
	`), taskID, taskID).Scan(&maxPosition); err != nil {
		return 0, err
	}
	if !maxPosition.Valid {
		return 0, nil
	}
	return int(maxPosition.Int64) + 1, nil
}

// CompensateWorkspaceSourceBatch removes only rows created by batch. It is
// idempotent so retrying cleanup after a partial runtime failure is safe.
func (r *Repository) CompensateWorkspaceSourceBatch(ctx context.Context, batch *models.WorkspaceSourceBatch) error {
	if batch == nil {
		return nil
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	for _, source := range batch.Sources {
		if folder := source.Folder; folder != nil && folder.ID != "" {
			if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_workspace_folders WHERE id = ?`), folder.ID); err != nil {
				_ = tx.Rollback()
				return err
			}
		}
		if taskRepo := source.Repository; taskRepo != nil && taskRepo.ID != "" {
			if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_repositories WHERE id = ?`), taskRepo.ID); err != nil {
				_ = tx.Rollback()
				return err
			}
		}
	}
	for _, update := range batch.RepositoryUpdates {
		if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE task_repositories SET base_branch = ?, updated_at = ? WHERE id = ?`), update.PreviousBaseBranch, time.Now().UTC(), update.TaskRepositoryID); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func scanTaskWorkspaceFolder(scanner interface{ Scan(...any) error }) (*models.TaskWorkspaceFolder, error) {
	folder := &models.TaskWorkspaceFolder{}
	if err := scanner.Scan(
		&folder.ID,
		&folder.TaskID,
		&folder.LocalPath,
		&folder.DisplayName,
		&folder.Position,
		&folder.CreatedAt,
		&folder.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return folder, nil
}
