package gitlab

import (
	"context"
	"fmt"

	dbutil "github.com/kandev/kandev/internal/db"
)

var gitlabTaskOwnedTables = []string{
	"gitlab_mr_watches",
	"gitlab_task_mrs",
	"gitlab_task_mr_options",
	"gitlab_task_mr_state",
}

// DeleteTaskMRsByTaskID removes every contribution association owned by a
// hard-deleted task. It is safe to replay after a missed event.
func (s *Store) DeleteTaskMRsByTaskID(ctx context.Context, taskID string) (int64, error) {
	result, err := s.db.ExecContext(ctx, s.db.Rebind(`DELETE FROM gitlab_task_mrs WHERE task_id = ?`), taskID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// DeleteTaskOwnedStateByTaskID removes all GitLab rows owned by a hard-deleted
// task in one transaction. It is safe to replay after a missed event.
func (s *Store) DeleteTaskOwnedStateByTaskID(ctx context.Context, taskID string) (int64, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	var deleted int64
	for _, table := range gitlabTaskOwnedTables {
		result, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM `+table+` WHERE task_id = ?`), taskID)
		if err != nil {
			return 0, fmt.Errorf("delete task-owned rows from %s: %w", table, err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("count deleted rows from %s: %w", table, err)
		}
		deleted += rows
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deleted, nil
}

func (s *Store) healTaskOwnedOrphans() error {
	exists, err := dbutil.TableExists(s.db, "tasks")
	if err != nil {
		return err
	}
	if !exists {
		return nil // tasks table not yet created — skip sweep
	}
	tx, err := s.db.Beginx()
	if err != nil {
		return fmt.Errorf("begin orphan sweep: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, table := range gitlabTaskOwnedTables {
		if _, err := tx.Exec(`DELETE FROM ` + table + ` WHERE NOT EXISTS (SELECT 1 FROM tasks WHERE tasks.id = ` + table + `.task_id)`); err != nil {
			return fmt.Errorf("remove orphaned %s: %w", table, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit orphan sweep: %w", err)
	}
	return nil
}
