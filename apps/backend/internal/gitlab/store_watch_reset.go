package gitlab

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
)

type watchInvalidation struct {
	Generation int64
	TaskIDs    []string
	Missing    bool
}

func (s *Store) BeginReviewWatchReset(ctx context.Context, watchID string) (*watchInvalidation, error) {
	return s.beginWatchInvalidation(ctx, "gitlab_review_watches", "gitlab_review_mr_tasks", "review_watch_id", watchID, false)
}

func (s *Store) BeginIssueWatchReset(ctx context.Context, watchID string) (*watchInvalidation, error) {
	return s.beginWatchInvalidation(ctx, "gitlab_issue_watches", "gitlab_issue_watch_tasks", "issue_watch_id", watchID, false)
}

func (s *Store) BeginReviewWatchDelete(ctx context.Context, watchID string) (*watchInvalidation, error) {
	return s.beginWatchInvalidation(ctx, "gitlab_review_watches", "gitlab_review_mr_tasks", "review_watch_id", watchID, true)
}

func (s *Store) BeginIssueWatchDelete(ctx context.Context, watchID string) (*watchInvalidation, error) {
	return s.beginWatchInvalidation(ctx, "gitlab_issue_watches", "gitlab_issue_watch_tasks", "issue_watch_id", watchID, true)
}

func (s *Store) beginWatchInvalidation(ctx context.Context, watchTable, taskTable, foreignKey, watchID string, deleting bool) (*watchInvalidation, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	set := "generation = generation + 1, updated_at = CURRENT_TIMESTAMP"
	if deleting {
		set += ", deleting = 1, enabled = 0"
	}
	result, err := tx.ExecContext(ctx,
		fmt.Sprintf("UPDATE %s SET %s WHERE id = ? AND deleting = 0", watchTable, set), watchID)
	if err != nil {
		return nil, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		if !deleting {
			return nil, ErrWatchNotFound
		}
		return s.resumeWatchDelete(ctx, tx, watchTable, taskTable, foreignKey, watchID)
	}
	var generation int64
	if err := tx.GetContext(ctx, &generation,
		fmt.Sprintf("SELECT generation FROM %s WHERE id = ?", watchTable), watchID); err != nil {
		return nil, err
	}
	var taskIDs []string
	if err := tx.SelectContext(ctx, &taskIDs,
		fmt.Sprintf("SELECT task_id FROM %s WHERE %s = ? AND generation < ? AND task_id <> '' ORDER BY created_at", taskTable, foreignKey),
		watchID, generation); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &watchInvalidation{Generation: generation, TaskIDs: taskIDs}, nil
}

func (s *Store) resumeWatchDelete(ctx context.Context, tx *sqlx.Tx, watchTable, taskTable, foreignKey, watchID string) (*watchInvalidation, error) {
	var state struct {
		Generation int64 `db:"generation"`
		Deleting   bool  `db:"deleting"`
	}
	err := tx.GetContext(ctx, &state,
		fmt.Sprintf("SELECT generation, deleting FROM %s WHERE id = ?", watchTable), watchID)
	if errors.Is(err, sql.ErrNoRows) {
		return &watchInvalidation{Missing: true}, nil
	}
	if err != nil {
		return nil, err
	}
	if !state.Deleting {
		return nil, ErrWatchOwnershipLost
	}
	var taskIDs []string
	if err := tx.SelectContext(ctx, &taskIDs,
		fmt.Sprintf("SELECT task_id FROM %s WHERE %s = ? AND generation < ? AND task_id <> '' ORDER BY created_at", taskTable, foreignKey),
		watchID, state.Generation); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &watchInvalidation{Generation: state.Generation, TaskIDs: taskIDs}, nil
}

func (s *Store) FinishReviewWatchReset(ctx context.Context, watchID string, generation int64) error {
	return s.finishWatchReset(ctx, "gitlab_review_watches", "gitlab_review_mr_tasks", "review_watch_id", watchID, generation)
}

func (s *Store) FinishIssueWatchReset(ctx context.Context, watchID string, generation int64) error {
	return s.finishWatchReset(ctx, "gitlab_issue_watches", "gitlab_issue_watch_tasks", "issue_watch_id", watchID, generation)
}

func (s *Store) finishWatchReset(ctx context.Context, watchTable, taskTable, foreignKey, watchID string, generation int64) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf("DELETE FROM %s WHERE %s = ? AND generation < ?", taskTable, foreignKey), watchID, generation); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx,
		fmt.Sprintf("UPDATE %s SET last_polled_at = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND generation = ? AND deleting = 0", watchTable),
		watchID, generation)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrWatchOwnershipLost
	}
	return tx.Commit()
}

func (s *Store) ListReviewMRTaskIDsByWatch(ctx context.Context, watchID string) ([]string, error) {
	var ids []string
	if err := s.ro.SelectContext(ctx, &ids,
		`SELECT task_id FROM gitlab_review_mr_tasks WHERE review_watch_id = ? ORDER BY created_at`, watchID); err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *Store) ResetReviewWatchState(ctx context.Context, watchID string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM gitlab_review_mr_tasks WHERE review_watch_id = ?`, watchID); err != nil {
		return fmt.Errorf("clear review watch tasks: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE gitlab_review_watches SET last_polled_at = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, watchID); err != nil {
		return fmt.Errorf("clear review watch poll state: %w", err)
	}
	return tx.Commit()
}

func (s *Store) ListIssueWatchTaskIDsByWatch(ctx context.Context, watchID string) ([]string, error) {
	var ids []string
	if err := s.ro.SelectContext(ctx, &ids,
		`SELECT task_id FROM gitlab_issue_watch_tasks WHERE issue_watch_id = ? ORDER BY created_at`, watchID); err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *Store) ResetIssueWatchState(ctx context.Context, watchID string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM gitlab_issue_watch_tasks WHERE issue_watch_id = ?`, watchID); err != nil {
		return fmt.Errorf("clear issue watch tasks: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE gitlab_issue_watches SET last_polled_at = NULL, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, watchID); err != nil {
		return fmt.Errorf("clear issue watch poll state: %w", err)
	}
	return tx.Commit()
}
