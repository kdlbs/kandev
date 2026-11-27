package coordinator

import (
	"context"
	"fmt"
	"time"
)

// recentTurnWindow is how long after finishing an unattended turn its
// coordinator stays in the backstop's visit set, so the per-turn cost
// recompute still runs.
const recentTurnWindow = 10 * time.Minute

func (s *Store) queryIDs(ctx context.Context, what, query string, args ...any) ([]string, error) {
	rows, err := s.ro.QueryContext(ctx, s.ro.Rebind(query), args...)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", what, err)
	}
	defer func() { _ = rows.Close() }()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan %s: %w", what, err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list %s: %w", what, err)
	}
	return ids, nil
}

// AutonomousCoordinatorIDs returns the ids of coordinators with autonomy on.
func (s *Store) AutonomousCoordinatorIDs(ctx context.Context) ([]string, error) {
	return s.queryIDs(ctx, "autonomous coordinators", `SELECT id FROM coordinators WHERE autonomy_enabled = 1 ORDER BY id`)
}

// CoordinatorIDsWithActiveTurns returns the ids of coordinators with an open
// unattended turn or one finished at or after since.
func (s *Store) CoordinatorIDsWithActiveTurns(ctx context.Context, since time.Time) ([]string, error) {
	return s.queryIDs(ctx, "coordinators with turns", `
		SELECT DISTINCT coordinator_id FROM coordinator_unattended_turns
		WHERE outcome IS NULL OR finished_at >= ? ORDER BY coordinator_id`, since)
}

// CoordinatorIDsWithAutomaticClaims returns the ids of coordinators with at
// least one proposal claimed automatically.
func (s *Store) CoordinatorIDsWithAutomaticClaims(ctx context.Context) ([]string, error) {
	return s.queryIDs(ctx, "coordinators with automatic claims", `
		SELECT DISTINCT coordinator_id FROM coordinator_proposals WHERE claimed_automatically = 1 ORDER BY coordinator_id`)
}
