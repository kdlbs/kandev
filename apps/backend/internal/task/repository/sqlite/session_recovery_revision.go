package sqlite

import (
	"context"

	"github.com/jmoiron/sqlx"
)

// Recovery block changes share the session snapshot fence used by browser
// notifications. Lock the session before its blocks, as recovery transactions do.
func (r *Repository) advanceRecoverySessionRevisionTx(ctx context.Context, tx *sqlx.Tx, sessionID string) error {
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE task_sessions SET updated_at = updated_at WHERE id = ?`), sessionID); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE task_sessions SET updated_at = ? WHERE id = ?`), r.nowUTC(), sessionID)
	return err
}
