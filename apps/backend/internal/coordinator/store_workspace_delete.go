package coordinator

import (
	"context"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
)

// DeleteWorkspaceState deletes a workspace's coordinators, proposals, phase 3 rows and
// stall records in one transaction (docs/specs/coordinator/system-design/
// coordinators.md#workspace-deletion). Deleting zero rows is success, so a
// redelivered workspace.deleted event is harmless.
func (s *Store) DeleteWorkspaceState(ctx context.Context, workspaceID string) error {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete workspace coordinator state: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if dialect.IsPostgres(s.db.DriverName()) {
		if err := lockWorkspaceCoordinators(ctx, tx, workspaceID); err != nil {
			return err
		}
	}
	if err := deleteCoordinatorPhase3Rows(ctx, tx, "IN (SELECT id FROM coordinators WHERE workspace_id = ?)", workspaceID); err != nil {
		return err
	}
	for _, table := range []string{"coordinator_watches", "coordinator_activity", "coordinator_standing_orders", "coordinator_goals"} {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM `+table+` WHERE workspace_id = ?`), workspaceID); err != nil {
			return fmt.Errorf("delete workspace %s: %w", table, err)
		}
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

// lockWorkspaceCoordinators takes the row locks of every coordinator in the
// workspace, in id order, so a concurrent locked write finishes before the
// deletion and none starts after it.
func lockWorkspaceCoordinators(ctx context.Context, tx *sqlx.Tx, workspaceID string) error {
	rows, err := tx.QueryxContext(ctx, tx.Rebind(`SELECT id FROM coordinators WHERE workspace_id = ? ORDER BY id FOR UPDATE`), workspaceID)
	if err != nil {
		return fmt.Errorf("lock workspace coordinators: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
	}
	return rows.Err()
}
