package sqlite

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// resetUsageSourceGuardsForE2ETx is used only by the gated fixture reset. Other
// workspaces' guard memberships continue to fence shared transcript identities.
func (r *Repository) resetUsageSourceGuardsForE2ETx(ctx context.Context, tx *sqlx.Tx, workspace string) error {
	key := usageSourceGuardHash("usage-workspace", workspace)
	for {
		var sources []string
		if err := tx.SelectContext(ctx, &sources, r.db.Rebind(`SELECT source_hash FROM usage_collection_guard_workspaces
			WHERE workspace_hash = ? ORDER BY source_hash LIMIT ?`), key, usageHistoryBatchLimit); err != nil {
			return err
		}
		if len(sources) == 0 {
			return nil
		}
		if err := r.resetUsageSourceGuardPageTx(ctx, tx, key, sources); err != nil {
			return err
		}
	}
}

func (r *Repository) resetUsageSourceGuardPageTx(ctx context.Context, tx *sqlx.Tx, workspace string, sources []string) error {
	keys := placeholders(len(sources))
	args := stringArgs(sources)
	if r.db.DriverName() == usageDriverPostgres {
		var locked []string
		if err := tx.SelectContext(ctx, &locked, r.db.Rebind(`SELECT source_hash FROM usage_collection_source_guards
			WHERE source_hash IN (`+keys+`) ORDER BY source_hash FOR UPDATE`), args...); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM usage_collection_guard_workspaces WHERE workspace_hash = ?
		AND source_hash IN (`+keys+`)`), append([]any{workspace}, args...)...); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM usage_collection_source_guards WHERE source_hash IN (`+keys+`)
		AND NOT EXISTS (SELECT 1 FROM usage_collection_guard_workspaces m WHERE m.source_hash=usage_collection_source_guards.source_hash)`), args...)
	return err
}
