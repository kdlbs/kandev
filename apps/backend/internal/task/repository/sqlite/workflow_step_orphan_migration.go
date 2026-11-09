package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/db/dialect"

	"go.uber.org/zap"
)

// workflowStepOrphansPrunedMetaKey records completion of one-time orphan cleanup.
const workflowStepOrphansPrunedMetaKey = "workflow_steps_orphans_pruned_at"

// pruneOrphanWorkflowStepsOnce deletes workflow_steps rows whose workflow no
// longer exists, then records workflowStepOrphansPrunedMetaKey so later boots
// skip it.
//
// The cleanup and marker commit in one transaction. An interrupted transaction
// leaves the next boot responsible for retrying the cleanup.
func (r *Repository) pruneOrphanWorkflowStepsOnce() error {
	ctx := r.migrationContext()
	// persistence.Provide creates kandev_meta before any repository opens,
	// but repo-level tests build a bare DB through NewWithDB.
	if _, err := r.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS kandev_meta (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL DEFAULT ''
		)`); err != nil {
		return fmt.Errorf("ensure kandev_meta: %w", err)
	}
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin workflow step orphan prune: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if dialect.IsPostgres(r.db.DriverName()) {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
			"workflow_step_orphan_migration:"+workflowStepOrphansPrunedMetaKey); err != nil {
			return fmt.Errorf("lock workflow step orphan prune: %w", err)
		}
	}
	var value string
	err = tx.QueryRowContext(ctx, r.db.Rebind(`SELECT value FROM kandev_meta WHERE key = ?`),
		workflowStepOrphansPrunedMetaKey).Scan(&value)
	if err == nil {
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("read %s: %w", workflowStepOrphansPrunedMetaKey, err)
	}
	result, err := tx.ExecContext(ctx, `
		DELETE FROM workflow_steps
		WHERE workflow_id NOT IN (SELECT id FROM workflows)
	`)
	if err != nil {
		return fmt.Errorf("prune orphan workflow steps: %w", err)
	}
	pruned, _ := result.RowsAffected()
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO kandev_meta (key, value) VALUES (?, ?)
		ON CONFLICT(key) DO NOTHING
	`), workflowStepOrphansPrunedMetaKey, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
		return fmt.Errorf("write %s: %w", workflowStepOrphansPrunedMetaKey, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit workflow step orphan prune: %w", err)
	}
	if r.log != nil {
		r.log.Info("migration applied",
			zap.String("name", "workflow_steps.prune_orphans"), zap.Int64("rows", pruned))
	}
	return nil
}
