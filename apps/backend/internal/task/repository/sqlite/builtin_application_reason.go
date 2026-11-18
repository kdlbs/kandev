package sqlite

import (
	"context"
	"time"
)

func (r *Repository) SetBuiltinMCPApplicationPendingReason(ctx context.Context, sessionID string, version int64, reason string) error {
	_, err := r.db.ExecContext(ctx, r.db.Rebind(`UPDATE task_builtin_mcp_applications SET reason=?,row_version=row_version+1,updated_at=? WHERE session_id=? AND row_version=? AND stage='pending' AND reason<>?`), reason, time.Now().UTC(), sessionID, version, reason)
	return err
}
