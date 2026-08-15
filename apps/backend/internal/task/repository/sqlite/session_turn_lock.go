package sqlite

import (
	"context"
	"fmt"
	"sort"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
)

const sessionTurnLockNamespace = "task-session-turn:"

// lockSessionTurnWrites serializes current-turn decisions with successor-turn
// creation on PostgreSQL. The lock must be acquired in its own statement so a
// following READ COMMITTED statement observes commits made by the prior owner.
func lockSessionTurnWrites(
	ctx context.Context,
	tx *sqlx.Tx,
	driverName string,
	sessionIDs ...string,
) error {
	if !dialect.IsPostgres(driverName) {
		return nil
	}
	unique := make(map[string]struct{}, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		if sessionID != "" {
			unique[sessionID] = struct{}{}
		}
	}
	ordered := make([]string, 0, len(unique))
	for sessionID := range unique {
		ordered = append(ordered, sessionID)
	}
	sort.Strings(ordered)
	for _, sessionID := range ordered {
		if _, err := tx.ExecContext(
			ctx,
			`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`,
			sessionTurnLockNamespace+sessionID,
		); err != nil {
			return fmt.Errorf("lock turn writes for session %q: %w", sessionID, err)
		}
	}
	return nil
}
