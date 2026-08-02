package azuredevops

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func (s *Store) BeginWorkItemWatchReset(ctx context.Context, watchID string) (*WatchResetResult, error) {
	return s.beginWatchReset(ctx, "azure_devops_work_item_watches", "azure_devops_work_item_watch_tasks", watchID)
}

func (s *Store) BeginPullRequestWatchReset(ctx context.Context, watchID string) (*WatchResetResult, error) {
	return s.beginWatchReset(ctx, "azure_devops_pull_request_watches", "azure_devops_pull_request_watch_tasks", watchID)
}

func (s *Store) beginWatchReset(ctx context.Context, watchTable, taskTable, watchID string) (*WatchResetResult, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET generation = generation + 1, last_polled_at = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND deleting = 0`, watchTable), watchID)
	if err != nil {
		return nil, err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, ErrWatchNotFound
	}
	var generation int64
	if err := tx.QueryRowContext(ctx, fmt.Sprintf(`SELECT generation FROM %s WHERE id = ?`, watchTable), watchID).Scan(&generation); err != nil {
		return nil, err
	}
	var taskIDs []string
	if err := tx.SelectContext(ctx, &taskIDs, fmt.Sprintf(`SELECT task_id FROM %s WHERE watch_id = ? AND generation < ? AND task_id <> '' ORDER BY created_at`, taskTable), watchID, generation); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &WatchResetResult{Generation: generation, TaskIDs: taskIDs}, nil
}

func (s *Store) FinishWorkItemWatchReset(ctx context.Context, watchID string, generation int64) error {
	return s.finishWatchReset(ctx, "azure_devops_work_item_watches", "azure_devops_work_item_watch_tasks", watchID, generation)
}

func (s *Store) FinishPullRequestWatchReset(ctx context.Context, watchID string, generation int64) error {
	return s.finishWatchReset(ctx, "azure_devops_pull_request_watches", "azure_devops_pull_request_watch_tasks", watchID, generation)
}

func (s *Store) finishWatchReset(ctx context.Context, watchTable, taskTable, watchID string, generation int64) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE watch_id = ? AND generation < ?`, taskTable), watchID, generation); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, fmt.Sprintf(`UPDATE %s SET last_polled_at = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND generation = ? AND deleting = 0`, watchTable), watchID, generation)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrWatchOwnershipLost
	}
	return tx.Commit()
}

func (s *Store) ListWorkItemWatchTaskIDs(ctx context.Context, watchID string) ([]string, error) {
	var ids []string
	err := s.ro.SelectContext(ctx, &ids, `SELECT task_id FROM azure_devops_work_item_watch_tasks WHERE watch_id = ? AND task_id <> '' ORDER BY created_at`, watchID)
	return ids, err
}

func (s *Store) ListPullRequestWatchTaskIDs(ctx context.Context, watchID string) ([]string, error) {
	var ids []string
	err := s.ro.SelectContext(ctx, &ids, `SELECT task_id FROM azure_devops_pull_request_watch_tasks WHERE watch_id = ? AND task_id <> '' ORDER BY created_at`, watchID)
	return ids, err
}
