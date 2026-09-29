package coordinator

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/db/dialect"
)

// DeleteWorkspaceState deletes a workspace's coordinators, proposals and
// stall records in one transaction (docs/specs/coordinator/system-design/
// coordinators.md#workspace-deletion). Deleting zero rows is success, so a
// redelivered workspace.deleted event is harmless.
func (s *Store) DeleteWorkspaceState(ctx context.Context, workspaceID string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete workspace coordinator state: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	coordinatorQuery := `SELECT id FROM coordinators WHERE workspace_id = ? ORDER BY id`
	if dialect.IsPostgres(s.db.DriverName()) {
		coordinatorQuery += ` FOR UPDATE`
	}
	var coordinatorIDs []string
	if err := tx.SelectContext(ctx, &coordinatorIDs, tx.Rebind(coordinatorQuery), workspaceID); err != nil {
		return fmt.Errorf("lock workspace coordinators: %w", err)
	}

	if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM coordinator_stalls WHERE workspace_id = ?`), workspaceID); err != nil {
		return fmt.Errorf("delete workspace stalls: %w", err)
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM coordinator_proposals WHERE workspace_id = ?`), workspaceID); err != nil {
		return fmt.Errorf("delete workspace proposals: %w", err)
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM coordinators WHERE workspace_id = ?`), workspaceID); err != nil {
		return fmt.Errorf("delete workspace coordinators: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete workspace coordinator state: %w", err)
	}
	return nil
}
