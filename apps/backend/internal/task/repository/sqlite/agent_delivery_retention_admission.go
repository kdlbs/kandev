package sqlite

import (
	"context"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type taskSessionQueryRowExecutor interface {
	QueryRowxContext(context.Context, string, ...any) *sqlx.Row
}

func (r *Repository) ensureAgentDeliveryIdentityAvailable(
	ctx context.Context,
	exec taskSessionQueryRowExecutor,
	sessionID string,
) error {
	var retained bool
	query, args := agentDeliveryIdentityRetainedQuery(sessionID)
	err := exec.QueryRowxContext(ctx, r.db.Rebind(query), args...).Scan(&retained)
	if err != nil {
		return err
	}
	if retained {
		return repoerrors.ErrAgentDeliveryIdentityRetiring
	}
	return nil
}

// agentDeliveryIdentityRetainedQuery runs inside the session-creation writer
// transaction, so every branch must be an index search on the requested
// session. An effect bound only through its stream is already covered by the
// cursor branch. The effect branch repeats the non-empty session_id predicate
// of idx_agent_delivery_effects_owned_retirement; SQLite cannot use that
// partial index with a bound parameter otherwise.
func agentDeliveryIdentityRetainedQuery(sessionID string) (string, []any) {
	return `
		SELECT EXISTS (
			SELECT 1 FROM agent_delivery_cleanup_jobs WHERE session_id = ?
			UNION ALL SELECT 1 FROM agent_delivery_submissions WHERE session_id = ?
			UNION ALL SELECT 1 FROM agent_delivery_inbox WHERE session_id = ?
			UNION ALL SELECT 1 FROM agent_delivery_cursors WHERE session_id = ?
			UNION ALL SELECT 1 FROM agent_delivery_effects WHERE session_id = ? AND session_id <> ''
		)`, []any{sessionID, sessionID, sessionID, sessionID, sessionID}
}

func (r *Repository) ensureAgentDeliverySessionNotRetired(
	ctx context.Context,
	exec taskSessionQueryRowExecutor,
	sessionID, incarnationID string,
) error {
	var retired, live bool
	if err := exec.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT EXISTS (SELECT 1 FROM agent_delivery_cleanup_jobs WHERE session_id = ?),
		EXISTS (SELECT 1 FROM task_sessions WHERE id = ? AND queue_incarnation_id = ?)`),
		sessionID, sessionID, incarnationID).Scan(&retired, &live); err != nil {
		return err
	}
	if retired {
		return repoerrors.ErrAgentDeliveryIdentityRetiring
	}
	if !live {
		return repoerrors.ErrAgentDeliveryIdentityMismatch
	}
	return nil
}
