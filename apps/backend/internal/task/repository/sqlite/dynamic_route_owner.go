package sqlite

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
)

// Task rows retain their FK. Native rows refer to an already-created domain
// owner; a missing owner cannot allocate an attempt or selection ordinal.
func (r *Repository) routeOwnerColumns(ctx context.Context, tx *sqlx.Tx, routeID string) (any, any, error) {
	if !strings.HasPrefix(routeID, "native:") {
		return routeID, nil, nil
	}
	parts := strings.SplitN(routeID, ":", 3)
	if len(parts) != 3 || parts[2] == "" {
		return nil, nil, fmt.Errorf("invalid native route owner")
	}
	var query string
	switch parts[1] {
	case "utility-call":
		query = `SELECT id FROM utility_agent_calls WHERE id = ?`
	case "review-run":
		query = `SELECT id FROM task_review_runs WHERE id = ?`
	case "office-run-session":
		query = `SELECT id FROM office_run_sessions WHERE id = ?`
	default:
		return nil, nil, fmt.Errorf("unknown native route owner %q", parts[1])
	}
	if dialect.IsPostgres(r.db.DriverName()) {
		query += " FOR KEY SHARE"
	}
	var id string
	if err := tx.QueryRowContext(ctx, r.db.Rebind(query), parts[2]).Scan(&id); err != nil {
		return nil, nil, fmt.Errorf("native route owner unavailable: %w", err)
	}
	return nil, routeID, nil
}
