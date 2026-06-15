// Package watchreset orchestrates "reset" for integration watches
// (GitHub issue / GitHub review / Jira / Linear / Sentry). A reset is a
// destructive operation that:
//
//  1. Deletes every Kandev task previously created by the watch (cascade,
//     including archived tasks — "wipe the slate").
//  2. Wipes the per-watch dedup rows so the next poll re-considers every
//     currently-matching external item.
//  3. Clears the watch's last_polled_at so it polls immediately.
//
// Each integration implements the small Resetter interface against its own
// store, and calls Run with a *task.HandoffService for the cascade delete.
package watchreset

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// Resetter is the per-watch surface watchreset needs. Implementations
// close over the watch ID and the integration's store. Methods are called
// in a fixed order by Run: ListTaskIDs first, then Clear after the cascade
// delete loop completes.
type Resetter interface {
	// ListTaskIDs returns every task ID previously created by the watch,
	// including rows whose task_id is empty (those are skipped by Run but
	// the dedup row will still be wiped by Clear).
	ListTaskIDs(ctx context.Context) ([]string, error)
	// Clear wipes all dedup rows for the watch and nulls its
	// last_polled_at column. Implementations should do both in a single
	// transaction so a partial reset cannot leave the watch in a state
	// where it skips items it just decided to forget about.
	Clear(ctx context.Context) error
}

// TaskDeleter is the cascade-delete entry point. Satisfied by
// *task.HandoffService.DeleteTaskTree.
type TaskDeleter interface {
	DeleteTaskTree(ctx context.Context, rootID string, cascade bool) (*taskservice.CascadeOutcome, error)
}

// Result reports what Run did. TasksDeleted counts task trees that were
// successfully cascade-deleted; tasks that failed to delete are logged but
// don't fail the reset (the dedup table is still wiped so the watch
// re-imports them on the next poll).
type Result struct {
	TasksDeleted int
}

// Run executes a watch reset. Deletes are best-effort: a delete failure
// for one task is logged and skipped so the dedup wipe still runs — a
// reset that fails halfway would leave the watch unable to re-import the
// items it just lost references to. Returns the final Result regardless;
// the only fatal error is the dedup/clear step itself.
func Run(ctx context.Context, r Resetter, td TaskDeleter, log *logger.Logger) (Result, error) {
	if r == nil {
		return Result{}, fmt.Errorf("watchreset: nil Resetter")
	}
	if td == nil {
		return Result{}, fmt.Errorf("watchreset: nil TaskDeleter")
	}

	ids, err := r.ListTaskIDs(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("list watch task ids: %w", err)
	}

	deleted := 0
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, err := td.DeleteTaskTree(ctx, id, true); err != nil {
			if log != nil {
				log.Warn("watchreset: delete task failed",
					zap.String("task_id", id), zap.Error(err))
			}
			continue
		}
		deleted++
	}

	if err := r.Clear(ctx); err != nil {
		return Result{TasksDeleted: deleted}, fmt.Errorf("clear watch state: %w", err)
	}

	if log != nil {
		log.Info("watch.reset",
			zap.Int("tasks_deleted", deleted),
			zap.Int("tasks_considered", len(ids)))
	}
	return Result{TasksDeleted: deleted}, nil
}

// Preview returns the number of tasks that would be deleted by Run. It
// counts non-empty task IDs only — dedup rows whose task creation never
// completed don't have a Kandev task to delete.
func Preview(ctx context.Context, r Resetter) (int, error) {
	if r == nil {
		return 0, fmt.Errorf("watchreset: nil Resetter")
	}
	ids, err := r.ListTaskIDs(ctx)
	if err != nil {
		return 0, fmt.Errorf("list watch task ids: %w", err)
	}
	n := 0
	for _, id := range ids {
		if id != "" {
			n++
		}
	}
	return n, nil
}
