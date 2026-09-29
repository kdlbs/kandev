package coordinator

import (
	"context"
	"fmt"
)

// WorkflowDeleted removes the deleted workflow's watch rows and publishes
// coordinator.updated once per coordinator that watched it. It never changes
// a coordinator's scope, policy revision or conversation: the effective watch
// set already omits the workflow, so this only tidies stored rows.
func (s *Service) WorkflowDeleted(ctx context.Context, workflowID string) error {
	type target struct{ coordinatorID, workspaceID string }
	var targets []target
	err := s.store.withWriteTx(ctx, func(tx coordinatorExec) error {
		rows, err := tx.QueryContext(ctx, s.store.db.Rebind(`SELECT DISTINCT coordinator_id, workspace_id FROM coordinator_watches WHERE workflow_id = ? ORDER BY coordinator_id`), workflowID)
		if err != nil {
			return fmt.Errorf("read watchers: %w", err)
		}
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var t target
			if err := rows.Scan(&t.coordinatorID, &t.workspaceID); err != nil {
				return fmt.Errorf("scan watcher: %w", err)
			}
			targets = append(targets, t)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(targets) == 0 {
			return nil
		}
		_, err = tx.ExecContext(ctx, s.store.db.Rebind(`DELETE FROM coordinator_watches WHERE workflow_id = ?`), workflowID)
		return err
	})
	if err != nil {
		return err
	}
	for _, t := range targets {
		s.publishCoordinatorUpdated(ctx, t.workspaceID, t.coordinatorID)
	}
	return nil
}

// withWriteTx runs fn in one writer-pool transaction without the
// per-coordinator lock.
func (s *Store) withWriteTx(ctx context.Context, fn func(tx coordinatorExec) error) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin write tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit write tx: %w", err)
	}
	return nil
}
