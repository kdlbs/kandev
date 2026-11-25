package sqlite

import (
	"context"
	"errors"
	"fmt"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
)

var errDuplicateEnvironmentPreservation = errors.New("duplicate task environment ownership requires explicit preservation")

type duplicateEnvironmentOwner struct {
	ID                string `db:"id"`
	ExecutorType      string `db:"executor_type"`
	ExecutorID        string `db:"executor_id"`
	ExecutorProfileID string `db:"executor_profile_id"`
	WorkspacePath     string `db:"workspace_path"`
	Generation        int64  `db:"ownership_generation"`
	Status            string `db:"status"`
	HasResources      bool   `db:"has_resources"`
}

func lockDuplicateEnvironmentHeal(ctx context.Context, tx *sqlx.Tx) error {
	if !dialect.IsPostgres(tx.DriverName()) {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `SET LOCAL lock_timeout = '30s'`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, taskWorktreeCutoverLockID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `LOCK TABLE task_environments, task_environment_repos, task_sessions, task_session_git_snapshots IN ACCESS EXCLUSIVE MODE`)
	return err
}

// A timestamp picks the canonical row; it is not permission to discard ownership.
func preserveDuplicateEnvironmentChildren(ctx context.Context, tx *sqlx.Tx, taskID, winnerID string) error {
	var owners []duplicateEnvironmentOwner
	err := tx.SelectContext(ctx, &owners, tx.Rebind(`SELECT id,COALESCE(executor_type,'') AS executor_type,
 COALESCE(executor_id,'') AS executor_id,COALESCE(executor_profile_id,'') AS executor_profile_id,
 COALESCE(workspace_path,'') AS workspace_path,ownership_generation,status,
 (COALESCE(container_id,'')!='' OR COALESCE(sandbox_id,'')!='' OR COALESCE(control_port,0)!=0
 OR COALESCE(container_bootstrap_nonce_secret_id,'')!='' OR COALESCE(container_control_auth_token_secret_id,'')!=''
 OR COALESCE(materialization_session_id,'')!='') AS has_resources
 FROM task_environments WHERE task_id=? AND retired_at IS NULL`), taskID)
	if err != nil {
		return err
	}
	var winner duplicateEnvironmentOwner
	for _, owner := range owners {
		if owner.ID == winnerID {
			winner = owner
		}
	}
	if winner.ID == "" {
		return errDuplicateEnvironmentPreservation
	}
	if err := rejectDuplicateEnvironmentRecord(ctx, tx, "task_environment_recovery_claims", "task_environment_id", winnerID); err != nil {
		return err
	}
	for _, owner := range owners {
		if owner.ID == winnerID {
			continue
		}
		if err := preserveDuplicateEnvironmentOwner(ctx, tx, owner, winner); err != nil {
			return err
		}
	}
	return nil
}

func preserveDuplicateEnvironmentOwner(ctx context.Context, tx *sqlx.Tx, owner, winner duplicateEnvironmentOwner) error {
	if owner.HasResources {
		return errDuplicateEnvironmentPreservation
	}
	guards := []struct{ table, column string }{
		{"task_environment_recovery_claims", "task_environment_id"},
		{"task_environment_recovery_artifacts", "task_environment_id"},
		{"task_environment_recovery_operations", "task_environment_id"},
		{"task_environment_checkout_adoptions", "task_environment_id"},
		{"workspace_inventory_recovery_receipts", "task_environment_id"},
		{"task_environment_kubernetes", "environment_id"},
	}
	for _, guard := range guards {
		if err := rejectDuplicateEnvironmentRecord(ctx, tx, guard.table, guard.column, owner.ID); err != nil {
			return err
		}
	}
	if err := transferDuplicateEnvironmentInventory(ctx, tx, owner, winner); err != nil {
		return err
	}
	// Old session-owned snapshot schemas have no environment column and are
	// handled later by the existing snapshot cutover through the saved session.
	has, err := db.ColumnExistsContext(ctx, tx, "task_session_git_snapshots", "task_environment_id")
	if err != nil || !has {
		return err
	}
	_, err = tx.ExecContext(ctx, tx.Rebind(`UPDATE task_session_git_snapshots SET task_environment_id=? WHERE task_environment_id=?`), winner.ID, owner.ID)
	return err
}

func rejectDuplicateEnvironmentRecord(ctx context.Context, tx *sqlx.Tx, table, column, id string) error {
	exists, err := db.TableExistsContext(ctx, tx, table)
	if err != nil || !exists {
		return err
	}
	// Both identifiers are from the fixed ownership-reference list above.
	var n int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s=?", table, column)
	if err := tx.QueryRowContext(ctx, tx.Rebind(query), id).Scan(&n); err != nil {
		return err
	}
	if n != 0 {
		return errDuplicateEnvironmentPreservation
	}
	return nil
}

func transferDuplicateEnvironmentInventory(ctx context.Context, tx *sqlx.Tx, owner, winner duplicateEnvironmentOwner) error {
	var paths []string
	if err := tx.SelectContext(ctx, &paths, tx.Rebind(`SELECT COALESCE(worktree_path,'') FROM task_environment_repos WHERE task_environment_id=?`), owner.ID); err != nil {
		return err
	}
	if len(paths) == 0 {
		return nil
	}
	if !duplicateEnvironmentInventoryCompatible(owner, winner, paths) {
		return errDuplicateEnvironmentPreservation
	}
	// Initial support proves only one incoming checkout. Any existing target
	// inventory, including an earlier loser transfer, needs explicit ownership
	// reconciliation; different logical slots do not prove distinct checkouts.
	var targetRows int
	if err := tx.QueryRowContext(ctx, tx.Rebind(`SELECT COUNT(*) FROM task_environment_repos WHERE task_environment_id=?`), winner.ID).Scan(&targetRows); err != nil {
		return err
	}
	if targetRows != 0 {
		return errDuplicateEnvironmentPreservation
	}
	// Retain every row ID, worktree/branch/source identity and lifecycle field.
	// A conflicting canonical slot aborts the whole transaction; never ignore it.
	_, err := tx.ExecContext(ctx, tx.Rebind(`UPDATE task_environment_repos SET task_environment_id=? WHERE task_environment_id=?`), winner.ID, owner.ID)
	return err
}

func duplicateEnvironmentInventoryCompatible(owner, winner duplicateEnvironmentOwner, paths []string) bool {
	if owner.ExecutorType != string(models.ExecutorTypeWorktree) || winner.ExecutorType != owner.ExecutorType ||
		winner.ExecutorID != owner.ExecutorID || winner.ExecutorProfileID != owner.ExecutorProfileID ||
		winner.Generation != owner.Generation || winner.Status != string(models.TaskEnvironmentStatusReady) || winner.WorkspacePath == "" {
		return false
	}
	if owner.WorkspacePath != "" {
		if owner.WorkspacePath != winner.WorkspacePath {
			return false
		}
	}
	return len(paths) == 1 && paths[0] == winner.WorkspacePath
}
