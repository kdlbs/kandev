package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
)

var ErrTaskHierarchyWorkspaceNotFound = errors.New("task hierarchy workspace not found")

// TaskHierarchyTx is the SQL boundary shared by task and Office parent writers.
type TaskHierarchyTx interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

// LockTaskHierarchy reserves SQLite's writer before any reads. On PostgreSQL,
// immutable workspace locators precede sorted workspace locks; graph reads and
// workflow-step/task locks must follow this call in READ COMMITTED transactions.
// Office participates in serialization while retaining its own parent policy.
func LockTaskHierarchy(ctx context.Context, tx TaskHierarchyTx, driver string, bind func(string) string, workspaceIDs, taskIDs []string) error {
	if driver != pgxDriverName {
		_, err := tx.ExecContext(ctx, `UPDATE tasks SET id = id WHERE 0`)
		return err
	}
	workspaces := make(map[string]bool)
	for _, id := range workspaceIDs {
		workspaces[id] = true
	}
	for _, id := range taskIDs {
		if id == "" {
			continue
		}
		var workspace string
		err := tx.QueryRowContext(ctx, bind(`SELECT workspace_id FROM tasks WHERE id = ?`), id).Scan(&workspace)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return err
		}
		workspaces[workspace] = true
	}
	ids := make([]string, 0, len(workspaces))
	for id := range workspaces {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if id == "" {
			// Config tasks have no workspace row. This key is reserved for their graph.
			if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(1262571076, 24)`); err != nil {
				return err
			}
			continue
		}
		var locked string
		if err := tx.QueryRowContext(ctx, bind(`SELECT id FROM workspaces WHERE id = ? FOR UPDATE`), id).Scan(&locked); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("%w: %s", ErrTaskHierarchyWorkspaceNotFound, id)
			}
			return fmt.Errorf("lock task hierarchy workspace %s: %w", id, err)
		}
	}
	return nil
}
