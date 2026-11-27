package worktree

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/kandev/kandev/internal/task/recoveryclaim"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/physicalclaim"
)

func (s *SQLiteStore) lockWorktreeSlot(ctx context.Context, tx *sqlx.Tx, id string) (*physicalclaim.Slot, error) {
	if err := physicalclaim.Lock(ctx, tx, id); err != nil {
		return nil, err
	}
	return physicalclaim.Resolve(ctx, tx, id)
}

// execWorktreeSlot restricts an ID-based mutation to its proven inventory row.
func (s *SQLiteStore) execWorktreeSlot(ctx context.Context, id, query string, args ...any) (sql.Result, error) {
	tx, err := s.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	slot, err := s.lockWorktreeSlot(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	rowID := ""
	if slot != nil {
		rowID = slot.ID
	}
	args = append(args, rowID)
	result, err := tx.ExecContext(ctx, tx.Rebind(query+` AND task_environment_repos.id = ?`), args...)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func checkReplacementIdentity(ctx context.Context, tx *sqlx.Tx, expected, replacement *Worktree) error {
	if err := physicalclaim.Lock(ctx, tx, expected.ID, replacement.ID); err != nil {
		return err
	}
	slot := physicalclaim.Slot{EnvironmentID: expected.TaskEnvironmentID, RepositoryID: expected.RepositoryID, BranchSlug: expected.BranchSlug}
	if err := physicalclaim.Check(ctx, tx, expected.ID, slot); err != nil {
		return err
	}
	return physicalclaim.Check(ctx, tx, replacement.ID, slot)
}

func checkCreateIdentity(ctx context.Context, tx *sqlx.Tx, wt *Worktree) error {
	expected := *wt
	err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT COALESCE(worktree_id,'') FROM task_environment_repos
 WHERE task_environment_id = ? AND repository_id = ? AND branch_slug = ?`), wt.TaskEnvironmentID, wt.RepositoryID, wt.BranchSlug).Scan(&expected.ID)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	return checkReplacementIdentity(ctx, tx, &expected, wt)
}

// Recovery authority locks the owner task before the physical claim lock.
func (s *SQLiteStore) lockRecoveryWorktreeSlot(ctx context.Context, tx *sqlx.Tx, id string) (*physicalclaim.Slot, error) {
	observed, err := physicalclaim.Resolve(ctx, tx, id)
	if err != nil || observed == nil {
		return observed, err
	}
	if err := recoveryclaim.EnsureAvailableTx(ctx, s.db, tx, observed.EnvironmentID); err != nil {
		return nil, err
	}
	current, err := s.lockWorktreeSlot(ctx, tx, id)
	if err != nil {
		return nil, err
	}
	if current == nil || *current != *observed {
		return nil, fmt.Errorf("worktree %s ownership changed", id)
	}
	return current, nil
}
