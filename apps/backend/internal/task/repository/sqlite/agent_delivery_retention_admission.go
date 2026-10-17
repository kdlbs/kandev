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
	err := exec.QueryRowxContext(ctx, r.db.Rebind(`
		SELECT EXISTS (
			SELECT 1 FROM agent_delivery_cleanup_jobs WHERE session_id = ?
			UNION ALL SELECT 1 FROM agent_delivery_submissions WHERE session_id = ?
			UNION ALL SELECT 1 FROM agent_delivery_inbox WHERE session_id = ?
			UNION ALL SELECT 1 FROM agent_delivery_cursors WHERE session_id = ?
			UNION ALL
			SELECT 1 FROM agent_delivery_effects e
			LEFT JOIN agent_delivery_cursors c ON c.stream_id = e.stream_id
			WHERE e.session_id = ? OR c.session_id = ?
		)`), sessionID, sessionID, sessionID, sessionID, sessionID, sessionID).Scan(&retained)
	if err != nil {
		return err
	}
	if retained {
		return repoerrors.ErrAgentDeliveryIdentityRetiring
	}
	return nil
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
