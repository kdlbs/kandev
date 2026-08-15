package gitlab

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// DeleteTaskMRsByTaskID removes every contribution association owned by a
// hard-deleted task. It is safe to replay after a missed event.
func (s *Store) DeleteTaskMRsByTaskID(ctx context.Context, taskID string) (int64, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM gitlab_task_mrs WHERE task_id = ?`, taskID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) healTaskContributionOrphans() error {
	var exists int
	err := s.db.Get(&exists, `SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'tasks'`)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // tasks table not yet created — skip sweep
	}
	if err != nil {
		return err
	}
	for _, table := range []string{"gitlab_mr_watches", "gitlab_task_mrs"} {
		if _, err := s.db.Exec(`DELETE FROM ` + table + ` WHERE NOT EXISTS (SELECT 1 FROM tasks WHERE tasks.id = ` + table + `.task_id)`); err != nil {
			return fmt.Errorf("remove orphaned %s: %w", table, err)
		}
	}
	return nil
}
