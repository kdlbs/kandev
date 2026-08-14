package github

import (
	"context"
	"fmt"
)

// DeleteTaskPRsByTaskID removes every contribution association owned by a
// hard-deleted task. It is safe to replay after a missed event.
func (s *Store) DeleteTaskPRsByTaskID(ctx context.Context, taskID string) (int64, error) {
	result, err := s.db.ExecContext(ctx, `DELETE FROM github_task_prs WHERE task_id = ?`, taskID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) healTaskContributionOrphans() error {
	if !s.tableExists("tasks") {
		return nil
	}
	for _, table := range []string{"github_pr_watches", "github_task_prs"} {
		if _, err := s.db.Exec(`DELETE FROM ` + table + ` WHERE NOT EXISTS (SELECT 1 FROM tasks WHERE tasks.id = ` + table + `.task_id)`); err != nil {
			return fmt.Errorf("remove orphaned %s: %w", table, err)
		}
	}
	return nil
}
