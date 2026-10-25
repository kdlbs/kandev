package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
)

func primarySessionNotPromoted(sessionID string, requireNonterminal bool) (bool, error) {
	if requireNonterminal {
		return false, nil
	}
	return false, fmt.Errorf("session not found: %s", sessionID)
}

func (r *Repository) lockNonterminalPrimarySession(ctx context.Context, tx *sqlx.Tx, sessionID string) (bool, error) {
	query := `SELECT id FROM task_sessions WHERE id = ? AND state IN ('CREATED', 'STARTING', 'RUNNING', 'IDLE', 'WAITING_FOR_INPUT')`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += ` FOR UPDATE`
	}
	var lockedSessionID string
	err := tx.QueryRowContext(ctx, r.db.Rebind(query), sessionID).Scan(&lockedSessionID)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return true, err
}
