package sqlite

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/db/dialect"
)

var _ dynamicruntime.RouteSelectionHistoryReader = (*Repository)(nil)

func (r *Repository) ensureRouteSelectionOrdinal(ctx context.Context) error {
	if err := r.migrate.Apply("dynamic_route_attempts.selection_ordinal", `ALTER TABLE dynamic_route_attempts ADD COLUMN selection_ordinal BIGINT`); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_dynamic_route_attempts_profile_ordinal
		ON dynamic_route_attempts(logical_profile_id, selection_ordinal)`)
	return err
}

// The profile lock serializes ordinal allocation, not cross-backend ranking.
// A failed decision transaction consumes no ordinal.
func (r *Repository) appendRouteAttemptTx(ctx context.Context, tx *sqlx.Tx, attempt dynamicruntime.RouteAttempt) error {
	sessionID, routeID, ownerErr := r.routeOwnerColumns(ctx, tx, attempt.SessionID)
	if ownerErr != nil {
		return ownerErr
	}
	if dialect.IsPostgres(r.db.DriverName()) {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`, "dynamic-route-selection:"+attempt.LogicalProfileID); err != nil {
			return err
		}
	}
	var last int64
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT COALESCE(MAX(selection_ordinal), 0)
		FROM dynamic_route_attempts WHERE logical_profile_id = ? AND selection_ordinal > 0`), attempt.LogicalProfileID).Scan(&last); err != nil {
		return err
	}
	if last == int64(^uint64(0)>>1) {
		return fmt.Errorf("route selection ordinal exhausted")
	}
	_, err := tx.ExecContext(ctx, r.db.Rebind(`INSERT INTO dynamic_route_attempts (
		id, session_id, route_id, logical_profile_id, execution_profile_id,
		route_generation, profile_version, reason, created_at, selection_ordinal
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`), uuid.New().String(), sessionID, routeID,
		attempt.LogicalProfileID, attempt.ExecutionProfileID, attempt.Generation,
		attempt.ProfileVersion, attempt.Reason, attempt.CreatedAt, last+1)
	return err
}

// LastRouteSelectionOrdinals ignores legacy NULLs rather than inferring causal
// order from wall clocks, session generations or UUIDs.
func (r *Repository) LastRouteSelectionOrdinals(ctx context.Context, profileID string) (map[string]int64, error) {
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`SELECT execution_profile_id, MAX(selection_ordinal)
		FROM dynamic_route_attempts WHERE logical_profile_id = ? AND selection_ordinal > 0
		GROUP BY execution_profile_id`), profileID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	result := make(map[string]int64)
	for rows.Next() {
		var id string
		var ordinal int64
		if err := rows.Scan(&id, &ordinal); err != nil {
			return nil, err
		}
		result[id] = ordinal
	}
	return result, rows.Err()
}
